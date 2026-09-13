// Package apply performs the install described by a config.Config.
//
// Every step is a named function over a shared State, and every external
// command goes through a sys.Runner. That makes the whole sequence — the exact
// sgdisk geometry, the image extraction, the kernel command line — visible
// to tests without a disk to erase.
package apply

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jaykeraliya0/cairn/installer/config"
	"github.com/jaykeraliya0/cairn/installer/sys"
)

// DefaultRoot is where the target filesystem is assembled. Tests point State
// at a temporary directory instead.
const DefaultRoot = "/mnt"

// MapperName is the device-mapper name an encrypted root is opened under. It
// also appears in the kernel command line as the rd.luks.name target.
const MapperName = "cryptroot"

// Subvolume pairs a Btrfs subvolume with its mountpoint relative to the target
// root.
type Subvolume struct {
	Name string
	// Path is relative to the target root; "" is the root itself.
	Path string
}

// Subvolumes is the single source of truth for Cairn's Btrfs layout.
//
// "@" must stay first: its mountpoint is the target root itself, which has to
// be mounted before any of the other mountpoint directories can be created
// inside it.
var Subvolumes = []Subvolume{
	{Name: "@", Path: ""},
	{Name: "@home", Path: "home"},
	{Name: "@log", Path: "var/log"},
	{Name: "@cache", Path: "var/cache"},
	{Name: "@snapshots", Path: ".snapshots"},
}

// State carries everything a step needs, plus what earlier steps discovered.
type State struct {
	// Cfg is the user's answers; steps never modify it.
	Cfg config.Config
	// Runner executes external commands.
	Runner sys.Runner
	// Root is where the target filesystem gets mounted.
	Root string
	// Log receives human-readable progress lines.
	Log func(string)
	// Image is the system image extracted onto the target: the entire
	// installed system, built by build.sh.
	Image string
	// NvidiaRepo is the side repository Nvidia's drivers are installed from,
	// offline, when the user chose one.
	NvidiaRepo string
	// BootStage is a scratch directory, on the live system's RAM-backed /tmp,
	// where the image's /boot is unpacked before it is copied onto the ESP.
	BootStage string
	// WaitForDevice blocks until a device node exists. Leave it nil for the
	// real udev poll; tests override it so they never touch /dev.
	WaitForDevice func(ctx context.Context, path string) error

	// ESPPart is the EFI system partition device.
	ESPPart string
	// RootPart is partition 2 — the LUKS container when encrypting, otherwise
	// the Btrfs filesystem itself.
	RootPart string
	// RootDev is whatever actually holds Btrfs: RootPart, or the opened mapper
	// device when encrypting.
	RootDev string
	// LUKSUUID identifies the LUKS container for rd.luks.name.
	LUKSUUID string
	// RootUUID identifies the Btrfs filesystem for an unencrypted root=.
	RootUUID string

	mounted  bool
	luksOpen bool
}

// NewState builds the State for a real install.
func NewState(cfg config.Config, runner sys.Runner, log func(string)) *State {
	return &State{
		Cfg:        cfg,
		Runner:     runner,
		Root:       DefaultRoot,
		Log:        log,
		Image:      DefaultImage,
		NvidiaRepo: DefaultNvidiaRepo,
		BootStage:  filepath.Join(os.TempDir(), "cairn-boot"),
	}
}

// logf reports progress, tolerating a nil Log.
func (s *State) logf(format string, args ...any) {
	if s.Log != nil {
		s.Log(fmt.Sprintf(format, args...))
	}
}

// path joins a target-relative path onto the target root.
func (s *State) path(parts ...string) string {
	return filepath.Join(append([]string{s.Root}, parts...)...)
}

// run executes a command through the configured runner.
func (s *State) run(ctx context.Context, name string, args ...string) error {
	return s.Runner.Run(ctx, sys.Cmd{Name: name, Args: args})
}

// Step is one named unit of work, shown to the user as it runs.
type Step struct {
	Name string
	Run  func(context.Context, *State) error
}

// Steps returns the ordered install sequence for a config. Steps that do not
// apply — encryption, swap — are omitted rather than turned into no-ops, so
// the progress display only ever lists work that will actually happen.
func Steps(cfg config.Config) []Step {
	steps := []Step{
		{Name: "Partitioning " + cfg.Disk, Run: partitionDisk},
	}
	if cfg.Encrypt {
		steps = append(steps, Step{Name: "Setting up disk encryption", Run: setupEncryption})
	}
	steps = append(steps,
		Step{Name: "Creating filesystems", Run: formatPartitions},
		Step{Name: "Creating Btrfs subvolumes", Run: createSubvolumes},
		Step{Name: "Mounting target", Run: mountTarget},
		Step{Name: "Installing the system", Run: extractImage},
		Step{Name: "Generating fstab", Run: generateFstab},
	)
	if cfg.SwapSizeMiB > 0 {
		steps = append(steps, Step{Name: "Creating swapfile", Run: createSwapfile})
	}
	steps = append(steps, Step{Name: "Installing default configuration", Run: installDefaults})
	if len(cfg.NvidiaPackages) > 0 {
		steps = append(steps, Step{Name: "Installing the Nvidia driver", Run: installNvidia})
	}
	steps = append(steps,
		Step{Name: "Configuring the new system", Run: configureChroot},
		Step{Name: "Installing the bootloader", Run: installBootloader},
		Step{Name: "Unmounting", Run: unmountTarget},
	)
	return steps
}

// Run executes every step in order, reporting each one as it starts.
//
// On failure the caller is expected to call Cleanup: partitioning is already
// destructive by the first step, and leaving /mnt mounted (or a LUKS container
// open) would block a retry.
func Run(ctx context.Context, st *State, onStep func(index, total int, name string)) error {
	steps := Steps(st.Cfg)
	for i, step := range steps {
		if err := ctx.Err(); err != nil {
			return err
		}
		if onStep != nil {
			onStep(i, len(steps), step.Name)
		}
		st.logf("==> %s", step.Name)
		if err := step.Run(ctx, st); err != nil {
			return fmt.Errorf("%s: %w", step.Name, err)
		}
	}
	return nil
}

// Cleanup undoes the mounts an interrupted install left behind, so the user
// can fix whatever went wrong and run the installer again. It is best-effort:
// every failure here is reported and then ignored, because the error that
// brought us here is the one worth showing.
func (s *State) Cleanup(ctx context.Context) {
	if s.mounted {
		if err := s.run(ctx, "umount", "-R", s.Root); err != nil {
			s.logf("warning: could not unmount %s: %v", s.Root, err)
		} else {
			s.mounted = false
		}
	}
	if s.luksOpen {
		if err := s.run(ctx, "cryptsetup", "close", MapperName); err != nil {
			s.logf("warning: could not close the encrypted volume: %v", err)
		} else {
			s.luksOpen = false
		}
	}
}

// waitFor blocks until a device node shows up. udev creates partition nodes
// asynchronously, so the node is routinely missing for a moment after sgdisk
// returns.
func (s *State) waitFor(ctx context.Context, path string) error {
	if s.WaitForDevice != nil {
		return s.WaitForDevice(ctx, path)
	}
	const attempts = 40
	for i := 0; i < attempts; i++ {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
	return fmt.Errorf("device %s did not appear after partitioning", path)
}
