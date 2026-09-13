// Package config holds the answers collected from the user and the rules that
// decide whether an answer is usable.
//
// Nothing in here touches the disk or runs a command: a Config is a plain
// value that the UI fills in and that apply.Run consumes. That split is what
// lets the whole install path be exercised by tests with no terminal and no
// target machine.
package config

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// NvidiaDriver selects which flavour of the Nvidia kernel module to install.
type NvidiaDriver string

// Arch no longer packages the closed kernel module at all — as of driver 610
// only the open-kernel-module builds remain — so the choice here is between
// the prebuilt module and the DKMS one, not between open and proprietary.
const (
	// NvidiaNone installs no Nvidia kernel module; the GPU falls back to
	// nouveau and mesa.
	NvidiaNone NvidiaDriver = "none"
	// NvidiaOpen is nvidia-open, built against the `linux` kernel that
	// target-packages installs. No build step during the install.
	NvidiaOpen NvidiaDriver = "open"
	// NvidiaDKMS is nvidia-open-dkms, rebuilt locally for whatever kernel is
	// running. Needed only if the kernel is going to be swapped later; it
	// costs a compile on install and on every kernel upgrade.
	NvidiaDKMS NvidiaDriver = "dkms"
)

// DefaultESPSizeMiB matches the 1 GiB ESP the original shell installer created.
const DefaultESPSizeMiB = 1024

// MinESPSizeMiB is a little above the point where mkfs.fat -F32 starts to
// struggle, and leaves room for several kernels plus a fallback initramfs.
const MinESPSizeMiB = 512

// MaxESPSizeMiB is an arbitrary sanity bound; nothing needs a larger ESP.
const MaxESPSizeMiB = 8192

// MinSwapSizeMiB keeps a swapfile large enough to be worth creating.
const MinSwapSizeMiB = 128

// Config is the complete set of decisions needed to install Cairn.
type Config struct {
	// Disk is the whole-device path to erase, e.g. "/dev/nvme0n1".
	Disk string
	// ESPSizeMiB sizes partition 1.
	ESPSizeMiB int
	// SwapSizeMiB is the Btrfs swapfile size; 0 means no swap.
	SwapSizeMiB int
	// Compression enables compress=zstd on every Btrfs mount.
	Compression bool

	// Encrypt wraps the root partition in LUKS2.
	Encrypt bool
	// LUKSPassphrase unlocks that container. Never written to disk or env.
	LUKSPassphrase string

	// Timezone is a path under /usr/share/zoneinfo, e.g. "Asia/Kolkata".
	Timezone string
	// Locale is the locale name, e.g. "en_US.UTF-8".
	Locale string
	// LocaleGenLine is the matching /etc/locale.gen line, e.g.
	// "en_US.UTF-8 UTF-8".
	LocaleGenLine string
	// Keymap is the console keymap for /etc/vconsole.conf, e.g. "us".
	Keymap string
	// XKBLayout is the Hyprland kb_layout derived from Keymap.
	XKBLayout string

	Hostname     string
	Username     string
	UserPassword string
	RootPassword string

	// Microcode names the detected CPU's microcode package, for the review
	// screen. Both vendors' microcode is in the system image regardless, and
	// mkinitcpio embeds the one this CPU needs.
	Microcode string
	// NvidiaPackages are installed from the image's Nvidia side repository.
	// Empty on any machine where no Nvidia driver was chosen.
	NvidiaPackages []string
	// Nvidia records which Nvidia driver was picked, if any. It also decides
	// whether early KMS modules and the Hyprland Nvidia environment are set up.
	Nvidia NvidiaDriver
}

// MountOptions returns the Btrfs options used for every subvolume mount.
func (c Config) MountOptions() string {
	if c.Compression {
		return "noatime,compress=zstd"
	}
	return "noatime"
}

// UsesNvidia reports whether an Nvidia kernel module will be installed.
func (c Config) UsesNvidia() bool {
	return c.Nvidia == NvidiaOpen || c.Nvidia == NvidiaDKMS
}

// SwapSizeArg renders SwapSizeMiB the way `btrfs filesystem mkswapfile --size`
// wants it.
func (c Config) SwapSizeArg() string {
	return fmt.Sprintf("%dM", c.SwapSizeMiB)
}

// Validate re-checks every field. The UI validates each answer as it is typed,
// so this is a backstop against a Config assembled in code.
func (c Config) Validate() error {
	if c.Disk == "" {
		return fmt.Errorf("no target disk selected")
	}
	if c.ESPSizeMiB < MinESPSizeMiB || c.ESPSizeMiB > MaxESPSizeMiB {
		return fmt.Errorf("ESP size %d MiB out of range (%d-%d)", c.ESPSizeMiB, MinESPSizeMiB, MaxESPSizeMiB)
	}
	if c.SwapSizeMiB != 0 && c.SwapSizeMiB < MinSwapSizeMiB {
		return fmt.Errorf("swap size %d MiB is below the %d MiB minimum", c.SwapSizeMiB, MinSwapSizeMiB)
	}
	if c.Encrypt && c.LUKSPassphrase == "" {
		return fmt.Errorf("encryption enabled but no passphrase set")
	}
	if err := ValidateHostname(c.Hostname); err != nil {
		return err
	}
	if err := ValidateUsername(c.Username); err != nil {
		return err
	}
	if err := ValidatePassword("user", c.UserPassword); err != nil {
		return err
	}
	if err := ValidatePassword("root", c.RootPassword); err != nil {
		return err
	}
	if c.Timezone == "" {
		return fmt.Errorf("no timezone selected")
	}
	if c.Locale == "" || c.LocaleGenLine == "" {
		return fmt.Errorf("no locale selected")
	}
	if c.Keymap == "" {
		return fmt.Errorf("no console keymap selected")
	}
	return nil
}

// ValidatePassword rejects the passwords that would break the chroot step.
//
// Both account passwords are handed to chpasswd as "name:password" lines on a
// single stdin stream, so a newline inside one would be read as the start of
// another account. Everything else, colons included, is fine: chpasswd splits
// on the first colon only.
func ValidatePassword(label, password string) error {
	switch {
	case password == "":
		return fmt.Errorf("%s password cannot be empty", label)
	case strings.ContainsAny(password, "\n\r"):
		return fmt.Errorf("%s password cannot contain a line break", label)
	}
	return nil
}

// hostnameRe is the RFC 1123 rule for a single label: alphanumerics and
// hyphens, never leading or trailing with a hyphen.
var hostnameRe = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?$`)

// ValidateHostname checks a hostname against RFC 1123's single-label rules.
func ValidateHostname(name string) error {
	switch {
	case name == "":
		return fmt.Errorf("hostname cannot be empty")
	case len(name) > 63:
		return fmt.Errorf("hostname cannot be longer than 63 characters")
	case !hostnameRe.MatchString(name):
		return fmt.Errorf("hostname may only contain letters, digits and hyphens, and cannot start or end with a hyphen")
	}
	return nil
}

// usernameRe is shadow-utils' default NAME_REGEX: a lowercase letter or
// underscore, then lowercase letters, digits, underscores and hyphens, with an
// optional trailing '$' for Samba machine accounts.
var usernameRe = regexp.MustCompile(`^[a-z_][a-z0-9_-]*\$?$`)

// reservedUsernames are accounts the `filesystem` package already creates, so
// `useradd` would fail on them partway through the chroot step.
var reservedUsernames = map[string]bool{
	"root": true, "bin": true, "daemon": true, "mail": true, "ftp": true,
	"http": true, "nobody": true, "dbus": true, "systemd-journal-remote": true,
	"systemd-network": true, "systemd-resolve": true, "systemd-timesync": true,
	"systemd-coredump": true, "uuidd": true, "sddm": true, "polkitd": true,
}

// ValidateUsername checks a username against the rules `useradd` enforces.
func ValidateUsername(name string) error {
	switch {
	case name == "":
		return fmt.Errorf("username cannot be empty")
	case len(name) > 32:
		return fmt.Errorf("username cannot be longer than 32 characters")
	case !usernameRe.MatchString(name):
		return fmt.Errorf("username must start with a lowercase letter or underscore and contain only lowercase letters, digits, underscores and hyphens")
	case reservedUsernames[name]:
		return fmt.Errorf("%q is already taken by a system account", name)
	}
	return nil
}

// sizeRe matches a human size: a positive integer or decimal, then an optional
// unit. A bare number is read as GiB, which is what people mean when they type
// "8" into a swap-size box.
var sizeRe = regexp.MustCompile(`^([0-9]+(?:\.[0-9]+)?)\s*([a-zA-Z]*)$`)

// ParseSizeMiB converts a human-written size such as "8G", "512 MiB" or "8"
// into whole mebibytes. Units are binary throughout: "G" means GiB, matching
// how btrfs and sgdisk read the same suffixes.
func ParseSizeMiB(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("size cannot be empty")
	}

	m := sizeRe.FindStringSubmatch(s)
	if m == nil {
		return 0, fmt.Errorf("%q is not a size — try something like 8G or 512M", s)
	}

	value, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, fmt.Errorf("%q is not a size — try something like 8G or 512M", s)
	}

	// Accept every spelling people actually type: G, GB, GiB, and lowercase.
	unit := strings.ToUpper(m[2])
	if unit == "B" {
		return 0, fmt.Errorf("size in bytes is too fine-grained — use M, G or T")
	}
	unit = strings.TrimSuffix(strings.TrimSuffix(unit, "IB"), "B")

	var perMiB float64
	switch unit {
	case "", "G":
		perMiB = 1024
	case "M":
		perMiB = 1
	case "T":
		perMiB = 1024 * 1024
	case "K":
		perMiB = 1.0 / 1024
	default:
		return 0, fmt.Errorf("unknown unit %q — use M, G or T", m[2])
	}

	mib := int(value * perMiB)
	if mib <= 0 {
		return 0, fmt.Errorf("size must be greater than zero")
	}
	return mib, nil
}

// FormatSizeMiB renders a mebibyte count back into something readable.
func FormatSizeMiB(mib int) string {
	switch {
	case mib == 0:
		return "none"
	case mib%(1024*1024) == 0:
		return fmt.Sprintf("%d TiB", mib/(1024*1024))
	case mib%1024 == 0:
		return fmt.Sprintf("%d GiB", mib/1024)
	default:
		return fmt.Sprintf("%d MiB", mib)
	}
}
