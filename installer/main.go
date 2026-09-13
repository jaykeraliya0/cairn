// Command cairn-install installs Cairn onto a disk.
//
// It runs from the live ISO, where it is started automatically on the tty1
// autologin and can be relaunched by name. Everything it needs is compiled in;
// there are no companion files to find on disk.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/jaykeraliya0/cairn/installer/apply"
	"github.com/jaykeraliya0/cairn/installer/locale"
	"github.com/jaykeraliya0/cairn/installer/sys"
	"github.com/jaykeraliya0/cairn/installer/ui"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "\ncairn-install: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	if os.Geteuid() != 0 {
		return fmt.Errorf("must run as root")
	}

	// Everything the install writes comes out of the system image, so there is
	// nothing to do without one. Say so before asking a single question.
	if !apply.ImageAvailable(apply.DefaultImage) {
		return fmt.Errorf("this ISO has no system image at %s; rebuild it with build.sh", apply.DefaultImage)
	}

	// SIGINT reaches the TUI as a key event, so it is handled there. This
	// catches SIGTERM — a shutdown while installing — and lets the install's
	// own cleanup unmount the target instead of the mounts being killed.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM)
	defer stop()

	// Repaint the console's palette and size its font before anything is
	// drawn on it, so the first frame the user sees is already themed.
	ui.SetupConsole()

	runner := sys.ExecRunner{}

	if err := waitForKeyring(ctx, runner); err != nil {
		return err
	}

	choices, err := gather(ctx, runner)
	if err != nil {
		return err
	}
	if len(choices.Disks) == 0 {
		return fmt.Errorf("no disks found to install onto (the live boot device is excluded)")
	}

	cfg, confirmed, err := ui.Collect(choices)
	if err != nil {
		return err
	}
	if !confirmed {
		fmt.Println("Nothing was changed.")
		return nil
	}

	state := apply.NewState(cfg, sys.ExecRunner{}, nil)
	reboot, err := ui.RunInstall(ctx, state)
	if err != nil {
		return err
	}

	if reboot {
		return runner.Run(ctx, sys.Cmd{Name: "systemctl", Args: []string{"reboot"}})
	}
	fmt.Println("Installation complete. Reboot when ready.")
	return nil
}

// gather probes the machine for everything the questions need to offer.
func gather(ctx context.Context, runner sys.Runner) (ui.Choices, error) {
	disks, err := sys.ListDisks(ctx, runner)
	if err != nil {
		return ui.Choices{}, err
	}

	timezones, err := locale.Timezones(locale.ZoneinfoDir)
	if err != nil {
		return ui.Choices{}, err
	}
	locales, err := locale.Locales(locale.LocaleGenTpl)
	if err != nil {
		return ui.Choices{}, err
	}
	keymaps, err := locale.Keymaps(locale.KeymapsDir)
	if err != nil {
		return ui.Choices{}, err
	}

	return ui.Choices{
		Disks: disks,
		// Both lists are reordered so the readable, common entries come first
		// and the long tail stays reachable through the picker's filter.
		Timezones:       timezones,
		Locales:         locale.Ordered(locales),
		Layouts:         locale.Layouts(keymaps),
		GPUs:            sys.DetectGPUs(sys.SysfsPCIDevices),
		Microcode:       sys.DetectMicrocode(sys.ProcCPUInfo),
		DefaultTimezone: defaultTimezone(timezones),
		DefaultLocale:   "en_US.UTF-8",
		DefaultKeymap:   defaultKeymap(),
	}, nil
}

// defaultTimezone reads the live system's own zone, so a user who set one at
// the boot prompt does not have to pick it again.
func defaultTimezone(available []string) string {
	target, err := os.Readlink("/etc/localtime")
	if err != nil {
		return firstAvailable(available)
	}
	// The link points at /usr/share/zoneinfo/<zone>, sometimes relatively.
	zone := target
	if i := strings.Index(target, "zoneinfo/"); i >= 0 {
		zone = target[i+len("zoneinfo/"):]
	} else {
		zone = filepath.Base(target)
	}

	for _, candidate := range available {
		if candidate == zone {
			return zone
		}
	}
	return firstAvailable(available)
}

// firstAvailable picks the first of the UTC spellings that the catalogue
// actually offers. Without this the fallback would be available[0], which is
// alphabetically Africa/Abidjan — a confusing default to land a user on.
func firstAvailable(available []string) string {
	preferred := map[string]bool{"UTC": true, "Etc/UTC": true}
	for _, candidate := range available {
		if preferred[candidate] {
			return candidate
		}
	}
	if len(available) > 0 {
		return available[0]
	}
	return "UTC"
}

// defaultKeymap is the layout the picker opens on.
//
// The session launcher puts the previously chosen keymap back in the
// environment when it restarts the kiosk under a new keyboard layout, so the
// user does not have to find their layout a second time.
func defaultKeymap() string {
	if keymap := os.Getenv("CAIRN_KEYMAP"); keymap != "" {
		return keymap
	}
	return "us"
}

// waitForKeyring blocks while archiso is still populating pacman's keyring.
//
// pacman-init.service runs at boot and takes a few seconds. The system image
// needs no keyring, but the Nvidia driver install verifies every package
// against the live system's, and fails outright if it has not finished — with
// an error ("key could not be looked up remotely") that gives no hint that
// waiting would have fixed it.
func waitForKeyring(ctx context.Context, runner sys.Runner) error {
	const timeout = 3 * time.Minute
	deadline := time.Now().Add(timeout)
	announced := false

	for {
		state, err := runner.Output(ctx, "systemctl", "is-active", "pacman-init.service")
		// A non-zero exit means inactive, failed, or no such unit — none of
		// which is worth blocking on. Only "activating" means work in flight.
		if err != nil && state == "" {
			return nil
		}
		if state != "activating" {
			if announced {
				fmt.Println("done.")
			}
			return nil
		}

		if !announced {
			fmt.Print("Waiting for the package keyring to finish initialising... ")
			announced = true
		}
		if time.Now().After(deadline) {
			fmt.Println()
			return fmt.Errorf("pacman-init.service did not finish within %s", timeout)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}
