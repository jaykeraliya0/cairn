package apply

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jaykeraliya0/cairn/installer/config"
)

// DefaultImage is where build.sh places the system image on the live ISO: the
// entire installed system, packed as a squashfs.
const DefaultImage = "/usr/share/cairn/cairn-root.sfs"

// DefaultNvidiaRepo is the side repository Nvidia's drivers ship in, next to
// the image.
const DefaultNvidiaRepo = "/usr/share/cairn/nvidia"

// nvidiaRepoConfig is the pacman.conf build.sh writes into that repository.
const nvidiaRepoConfig = "pacman.conf"

// ImageAvailable reports whether a system image is present at path.
func ImageAvailable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Size() > 0
}

// extractImage writes the system image onto the target.
//
// This is the operating system arriving in one step, already installed: build.sh
// ran pacstrap once, on the build machine, and packed the result. No package
// manager runs here, so nothing is downloaded, verified or cached on the
// target, and every install of an ISO is the same system.
//
// /boot takes a detour. On the target it is the EFI system partition, and
// FAT32 cannot store Unix ownership, permissions or extended attributes.
// unsquashfs restores all three and reports every refusal as an error, so the
// image goes onto the target with /boot excluded; /boot alone is unpacked into
// a staging directory in RAM; and its files are copied onto the ESP without
// trying to keep what FAT32 cannot hold. The kernel and microcode images arrive
// exactly as pacman installed them.
func extractImage(ctx context.Context, s *State) error {
	if !ImageAvailable(s.Image) {
		return fmt.Errorf("no system image at %s; this ISO was built without one", s.Image)
	}

	if err := s.run(ctx, "unsquashfs", "-f", "-d", s.Root, "-percentage",
		"-excludes", s.Image, "boot"); err != nil {
		return err
	}

	if err := os.RemoveAll(s.BootStage); err != nil {
		return fmt.Errorf("clearing %s: %w", s.BootStage, err)
	}
	defer os.RemoveAll(s.BootStage)

	if err := s.run(ctx, "unsquashfs", "-f", "-d", s.BootStage, "-percentage",
		s.Image, "boot"); err != nil {
		return err
	}
	return s.run(ctx, "cp", "-r", "--no-preserve=mode,ownership,timestamps",
		filepath.Join(s.BootStage, "boot")+"/.", s.path("boot")+"/")
}

// installNvidia installs the chosen Nvidia driver from the side repository.
//
// pacstrap into an existing root is how Arch's own install guide adds packages
// to a new system, and it mounts the API filesystems DKMS needs to build the
// module. The flags are what keep it offline and tidy:
//
//	-C  the repository's own config: its only repo, which is also its CacheDir,
//	    so pacman finds every package already in place
//	-c  use that CacheDir instead of the target's /var/cache/pacman/pkg, so
//	    nothing is copied into the new system
//	-G  leave the target's keyring alone; the chroot step generates a fresh one
//	-M  leave the target's mirrorlist alone
//
// Signatures are still checked, against the live system's keyring.
func installNvidia(ctx context.Context, s *State) error {
	if len(s.Cfg.NvidiaPackages) == 0 {
		return nil
	}

	conf := filepath.Join(s.NvidiaRepo, nvidiaRepoConfig)
	if _, err := os.Stat(conf); err != nil {
		return fmt.Errorf("an Nvidia driver was chosen, but this ISO has no Nvidia repository at %s", s.NvidiaRepo)
	}

	args := append([]string{"-C", conf, "-c", "-G", "-M", s.Root}, s.Cfg.NvidiaPackages...)
	return s.run(ctx, "pacstrap", args...)
}

// generateFstab appends UUID-keyed entries for the current target mounts.
//
// genfstab writes to stdout, so its output is captured and appended here
// rather than redirected by a shell.
func generateFstab(ctx context.Context, s *State) error {
	out, err := s.Runner.Output(ctx, "genfstab", "-U", s.Root)
	if err != nil {
		return err
	}
	return appendLines(s.path("etc/fstab"), out)
}

// createSwapfile adds a Btrfs-native swapfile and its fstab entry.
//
// Order matters: this has to run after generateFstab, or genfstab's append
// would land after the swap line and the two would be interleaved oddly.
func createSwapfile(ctx context.Context, s *State) error {
	swapfile := s.path("swapfile")
	if err := s.run(ctx, "btrfs", "filesystem", "mkswapfile",
		"--size", s.Cfg.SwapSizeArg(), swapfile); err != nil {
		return err
	}
	return appendLines(s.path("etc/fstab"), "/swapfile none swap defaults 0 0")
}

// installDefaults writes the two generated config files that depend on the
// user's answers. Cairn's dotfiles themselves are already in place: build.sh
// bakes them into the image's /etc/skel.
func installDefaults(ctx context.Context, s *State) error {
	if err := s.writeHyprlandHardwareConfig(); err != nil {
		return err
	}
	return s.writeMkinitcpioConfig()
}

// hyprlandHardwareConfig renders the Hyprland fragment holding the settings
// that depend on this particular machine and this particular install.
//
// Lua, not hyprlang: Hyprland deprecated its own config language in 0.55 and
// warns on every start, so Cairn's whole desktop config is Lua now.
// hyprland.lua requires this file from its very last line, so anything set
// here wins over the shipped defaults.
func hyprlandHardwareConfig(cfg config.Config) string {
	var b strings.Builder
	b.WriteString("-- Machine-specific Hyprland settings, written by the Cairn installer.\n")
	b.WriteString("-- hyprland.lua requires this last, so anything here overrides the\n")
	b.WriteString("-- defaults above it. Edit freely — the installer only writes it once.\n\n")

	// The layout comes from the curated keyboard table, whose values are plain
	// XKB names, so there is nothing here to quote defensively.
	fmt.Fprintf(&b, "hl.config({\n    input = {\n        kb_layout = %q,\n    },\n})\n", cfg.XKBLayout)

	if cfg.UsesNvidia() {
		b.WriteString("\n-- Nvidia: hardware video acceleration and GLX vendor selection under\n")
		b.WriteString("-- Wayland. See https://wiki.hypr.land/Nvidia/\n")
		b.WriteString("hl.env(\"LIBVA_DRIVER_NAME\", \"nvidia\")\n")
		b.WriteString("hl.env(\"__GLX_VENDOR_LIBRARY_NAME\", \"nvidia\")\n")
		b.WriteString("hl.env(\"NVD_BACKEND\", \"direct\")\n")
	}
	return b.String()
}

func (s *State) writeHyprlandHardwareConfig() error {
	path := s.path("etc/skel/.config/hypr/hardware.lua")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(path), err)
	}
	return os.WriteFile(path, []byte(hyprlandHardwareConfig(s.Cfg)), 0o644)
}

// MkinitcpioConfig renders the mkinitcpio drop-in for a config.
//
// A drop-in under mkinitcpio.conf.d beats editing mkinitcpio.conf in place:
// nothing has to be parsed or pattern-matched, and a pacman upgrade to the
// stock mkinitcpio.conf can never collide with it.
func MkinitcpioConfig(cfg config.Config) string {
	// systemd-flavoured hooks throughout, to match a systemd-boot target:
	// sd-encrypt and sd-vconsole are the systemd equivalents of the busybox
	// encrypt and keymap hooks, and `microcode` is what folds the CPU's
	// microcode into the initramfs.
	hooks := []string{
		"base", "systemd", "autodetect", "microcode", "modconf", "kms",
		"keyboard", "sd-vconsole", "block", "filesystems", "fsck",
	}
	if cfg.Encrypt {
		hooks = insertBefore(hooks, "filesystems", "sd-encrypt")
	}

	var modules []string
	if cfg.UsesNvidia() {
		// Early-load the Nvidia modules and drop `kms`: the kms hook pulls in
		// the DRM drivers for early modesetting, which on Nvidia means
		// simpledrm fighting the real driver for the console.
		modules = []string{"nvidia", "nvidia_modeset", "nvidia_uvm", "nvidia_drm"}
		hooks = remove(hooks, "kms")
	}

	var b strings.Builder
	b.WriteString("# Written by the Cairn installer. Overrides /etc/mkinitcpio.conf.\n")
	if len(modules) > 0 {
		fmt.Fprintf(&b, "MODULES=(%s)\n", strings.Join(modules, " "))
	}
	fmt.Fprintf(&b, "HOOKS=(%s)\n", strings.Join(hooks, " "))
	return b.String()
}

func (s *State) writeMkinitcpioConfig() error {
	path := s.path("etc/mkinitcpio.conf.d/cairn.conf")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(path), err)
	}
	return os.WriteFile(path, []byte(MkinitcpioConfig(s.Cfg)), 0o644)
}

// appendLines appends text to a file, creating it if needed and making sure
// the result ends with exactly one newline.
func appendLines(path, text string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(path), err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("opening %s: %w", path, err)
	}
	defer f.Close()

	if _, err := fmt.Fprintf(f, "%s\n", strings.TrimRight(text, "\n")); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

func insertBefore(items []string, before, item string) []string {
	for i, existing := range items {
		if existing == before {
			out := make([]string, 0, len(items)+1)
			out = append(out, items[:i]...)
			out = append(out, item)
			return append(out, items[i:]...)
		}
	}
	return append(items, item)
}

func remove(items []string, item string) []string {
	out := make([]string, 0, len(items))
	for _, existing := range items {
		if existing != item {
			out = append(out, existing)
		}
	}
	return out
}
