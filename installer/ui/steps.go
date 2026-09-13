package ui

import (
	"fmt"
	"strings"

	"github.com/jaykeraliya0/cairn/installer/config"
	"github.com/jaykeraliya0/cairn/installer/locale"
	"github.com/jaykeraliya0/cairn/installer/sys"
)

// answers holds the raw form state. Sizes stay as strings while they are being
// edited so a half-typed value is never rejected as a number.
type answers struct {
	disk        string
	encrypt     bool
	passphrase  string
	passphrase2 string
	espSize     string
	wantSwap    bool
	swapSize    string
	compression bool

	timezone string
	locale   string
	keymap   string

	hostname  string
	username  string
	password  string
	password2 string
	rootSame  bool
	rootPass  string
	rootPass2 string

	nvidia string
}

// defaultAnswers pre-fills everything that can be guessed, so someone who
// agrees with the defaults reaches Install in a few keystrokes. The user
// account is the only thing genuinely unknowable, and the only thing the hub
// marks as needing attention.
func defaultAnswers(ch Choices) *answers {
	a := &answers{
		espSize:     config.FormatSizeMiB(config.DefaultESPSizeMiB),
		wantSwap:    true,
		swapSize:    "4 GiB",
		compression: true,
		timezone:    ch.DefaultTimezone,
		locale:      ch.DefaultLocale,
		keymap:      ch.DefaultKeymap,
		hostname:    "cairn",
		rootSame:    true,
		nvidia:      string(config.NvidiaOpen),
	}
	if len(ch.Disks) > 0 {
		a.disk = ch.Disks[0].Path
	}
	return a
}

// buildSteps assembles the wizard, in the order the user walks it.
//
// The order follows what depends on what: keyboard first because every later
// screen is typed on it, then the locale pair, then the disk and how it will
// be carved, then the accounts, then review. The graphics step drops out
// entirely on a machine with no Nvidia card.
func buildSteps(ch Choices, a *answers) []step {
	return []step{
		{
			name:  "keyboard",
			icon:  g.Keyboard,
			value: func() string { return a.keymap },
			build: func() screen {
				return newPicker("Keyboard layout",
					"Used for the console, the boot passphrase prompt, and Hyprland.",
					"layouts", layoutItems(ch), a.keymap, func(v string) { a.keymap = v })
			},
		},
		{
			name:  "language",
			icon:  g.Language,
			value: func() string { return a.locale },
			build: func() screen {
				return newPicker("Language", "Sets LANG on the installed system.",
					"locales", localeItems(ch), a.locale, func(v string) { a.locale = v })
			},
		},
		{
			name:  "timezone",
			icon:  g.Timezone,
			value: func() string { return a.timezone },
			build: func() screen {
				return newPicker("Timezone", "", "zones",
					stringItems(ch.Timezones), a.timezone, func(v string) { a.timezone = v })
			},
		},
		{
			name:  "disk",
			icon:  g.Disk,
			value: func() string { return shortDisk(a.disk) },
			build: func() screen {
				return newPicker("Target disk",
					"Everything on it will be erased. The live medium is not listed.",
					"disks", diskItems(ch), a.disk, func(v string) { a.disk = v })
			},
		},
		{
			name: "encryption",
			icon: g.Encryption,
			value: func() string {
				if a.encrypt {
					return "LUKS2"
				}
				return "off"
			},
			build: func() screen { return encryptionForm(a) },
		},
		{
			name:  "layout",
			icon:  g.Layout,
			value: func() string { return shortLayout(a) },
			build: func() screen { return layoutForm(a) },
		},
		{
			name:  "hostname",
			icon:  g.Hostname,
			value: func() string { return a.hostname },
			build: func() screen { return hostnameForm(a) },
		},
		{
			name:  "user",
			icon:  g.User,
			value: func() string { return a.username },
			build: func() screen { return userForm(a) },
		},
		{
			name:  "graphics",
			icon:  g.Graphics,
			skip:  func() bool { return !sys.HasNvidia(ch.GPUs) },
			value: func() string { return nvidiaLabel(a.nvidia) },
			build: func() screen {
				return newPicker("Graphics driver",
					"Detected: "+sys.DescribeGPUs(ch.GPUs)+". Arch ships only the open kernel modules.",
					"options", nvidiaItems(), a.nvidia, func(v string) { a.nvidia = v })
			},
		},
		{
			name:  "review",
			icon:  g.Review,
			build: func() screen { return newReview(ch, a) },
		},
	}
}

// shortDisk trims a device path for the step list, where there are about ten
// columns to play with.
func shortDisk(path string) string {
	return strings.TrimPrefix(path, "/dev/")
}

// shortLayout summarises the partition choices in one short phrase.
func shortLayout(a *answers) string {
	if a.wantSwap {
		return a.espSize + " + " + a.swapSize
	}
	return a.espSize + ", no swap"
}

// ---- screens ---------------------------------------------------------------

func encryptionForm(a *answers) screen {
	hidden := func() bool { return !a.encrypt }
	return newForm("Encryption",
		"LUKS2 full-disk encryption. You will be asked for the passphrase at every boot.",
		[]*field{
			{kind: fieldToggle, label: "Encrypt", flag: &a.encrypt},
			{
				kind: fieldPassword, label: "Passphrase", text: &a.passphrase,
				placeholder: "there is no recovery if this is lost",
				hint:        "Typed at boot using the keyboard layout you chose.",
				hidden:      hidden,
				validate:    nonEmpty("passphrase"),
			},
			{
				kind: fieldPassword, label: "Confirm", text: &a.passphrase2,
				placeholder: "must match",
				hidden:      hidden,
				validate:    func(s string) error { return matches(s, a.passphrase, "Passphrases") },
			},
		},
		func() error {
			if !a.encrypt {
				a.passphrase, a.passphrase2 = "", ""
			}
			return nil
		})
}

func hostnameForm(a *answers) screen {
	return newForm("Hostname", "What this machine calls itself on the network.",
		[]*field{{
			kind: fieldText, label: "Hostname", text: &a.hostname,
			placeholder: "cairn",
			hint:        "Letters, digits and dashes.",
			validate:    orDefault(&a.hostname, "cairn", config.ValidateHostname),
		}}, nil)
}

func userForm(a *answers) screen {
	rootHidden := func() bool { return a.rootSame }
	return newForm("User account", "Gets a home directory and sudo through the wheel group.",
		[]*field{
			{
				kind: fieldText, label: "Username", text: &a.username,
				placeholder: "like jay",
				hint:        "Lowercase letters, digits, dashes and underscores.",
				validate:    config.ValidateUsername,
			},
			{
				kind: fieldPassword, label: "Password", text: &a.password,
				placeholder: "used for login and sudo",
				validate:    func(s string) error { return config.ValidatePassword("user", s) },
			},
			{
				kind: fieldPassword, label: "Confirm", text: &a.password2,
				placeholder: "must match",
				validate:    func(s string) error { return matches(s, a.password, "Passwords") },
			},
			{kind: fieldToggle, label: "Same root", flag: &a.rootSame,
				hint: "Use this password for the root account too."},
			{
				kind: fieldPassword, label: "Root pass", text: &a.rootPass,
				placeholder: "for the root account", hidden: rootHidden,
				validate: func(s string) error { return config.ValidatePassword("root", s) },
			},
			{
				kind: fieldPassword, label: "Confirm", text: &a.rootPass2,
				placeholder: "must match", hidden: rootHidden,
				validate: func(s string) error { return matches(s, a.rootPass, "Passwords") },
			},
		},
		func() error {
			if a.rootSame {
				a.rootPass, a.rootPass2 = "", ""
			}
			return nil
		})
}

func layoutForm(a *answers) screen {
	return newForm("Disk layout", "Partition sizes and filesystem options.",
		[]*field{
			{
				kind: fieldText, label: "EFI size", text: &a.espSize,
				placeholder: "1 GiB",
				hint:        "Holds the kernel and initramfs; 1 GiB fits several.",
				validate:    validSize(config.MinESPSizeMiB, config.MaxESPSizeMiB),
			},
			{kind: fieldToggle, label: "Swapfile", flag: &a.wantSwap,
				hint: "A Btrfs-native swapfile at /swapfile."},
			{
				kind: fieldText, label: "Swap size", text: &a.swapSize,
				placeholder: "4 GiB",
				hidden:      func() bool { return !a.wantSwap },
				validate:    validSize(config.MinSwapSizeMiB, 0),
			},
			{kind: fieldToggle, label: "Compress", flag: &a.compression,
				hint: "Btrfs zstd. Saves space and usually speeds up reads."},
		}, nil)
}

// ---- validators ------------------------------------------------------------

func nonEmpty(label string) func(string) error {
	return func(s string) error {
		if strings.TrimSpace(s) == "" {
			return fmt.Errorf("%s cannot be empty", label)
		}
		return nil
	}
}

func matches(got, want, label string) error {
	if got != want {
		return fmt.Errorf("%s do not match", label)
	}
	return nil
}

// orDefault lets an empty answer stand for a default, filling the field in so
// the step list shows what will actually be used.
func orDefault(field *string, fallback string, validate func(string) error) func(string) error {
	return func(s string) error {
		if strings.TrimSpace(s) == "" {
			*field = fallback
			return nil
		}
		return validate(s)
	}
}

// validSize builds a validator for a size input. A max of 0 means unbounded.
func validSize(minMiB, maxMiB int) func(string) error {
	return func(s string) error {
		mib, err := config.ParseSizeMiB(s)
		if err != nil {
			return err
		}
		if mib < minMiB {
			return fmt.Errorf("must be at least %s", config.FormatSizeMiB(minMiB))
		}
		if maxMiB > 0 && mib > maxMiB {
			return fmt.Errorf("must be at most %s", config.FormatSizeMiB(maxMiB))
		}
		return nil
	}
}

// ---- list items and labels -----------------------------------------------

func diskItems(ch Choices) []listItem {
	items := make([]listItem, 0, len(ch.Disks))
	for _, d := range ch.Disks {
		note := sys.HumanBytes(d.Size)
		if d.Transport != "" {
			note += "  " + d.Transport
		}
		label := d.Path
		if d.Model != "" {
			label += "  " + d.Model
		}
		items = append(items, listItem{Label: label, Note: note, Value: d.Path})
	}
	return items
}

func diskLabel(ch Choices, path string) string {
	for _, d := range ch.Disks {
		if d.Path == path {
			return fmt.Sprintf("%s · %s", d.Path, sys.HumanBytes(d.Size))
		}
	}
	return path
}

func layoutItems(ch Choices) []listItem {
	items := make([]listItem, 0, len(ch.Layouts))
	for _, l := range ch.Layouts {
		items = append(items, listItem{Label: l.Label, Note: l.Keymap, Value: l.Keymap})
	}
	return items
}

func layoutLabel(ch Choices, keymap string) string {
	for _, l := range ch.Layouts {
		if l.Keymap == keymap {
			return l.Label
		}
	}
	return keymap
}

func localeItems(ch Choices) []listItem {
	items := make([]listItem, 0, len(ch.Locales))
	for _, l := range ch.Locales {
		note := ""
		if l.Label != l.Name {
			note = l.Name
		}
		items = append(items, listItem{Label: l.Label, Note: note, Value: l.Name})
	}
	return items
}

func localeLabel(ch Choices, name string) string {
	for _, l := range ch.Locales {
		if l.Name == name {
			return l.Label
		}
	}
	return name
}

func nvidiaItems() []listItem {
	return []listItem{
		{Label: "nvidia-open", Note: "prebuilt, nothing to compile", Value: string(config.NvidiaOpen)},
		{Label: "nvidia-open-dkms", Note: "survives a kernel change", Value: string(config.NvidiaDKMS)},
		{Label: "None", Note: "nouveau and mesa", Value: string(config.NvidiaNone)},
	}
}

func nvidiaLabel(value string) string {
	for _, item := range nvidiaItems() {
		if item.Value == value {
			return item.Label
		}
	}
	return value
}

func stringItems(values []string) []listItem {
	items := make([]listItem, 0, len(values))
	for _, v := range values {
		items = append(items, listItem{Label: v, Value: v})
	}
	return items
}

// advancedSummary spells the partition choices out for the review screen.
func advancedSummary(a *answers) string {
	swap := "no swap"
	if a.wantSwap {
		swap = a.swapSize + " swap"
	}
	compression := "no compression"
	if a.compression {
		compression = "zstd"
	}
	return fmt.Sprintf("%s ESP · %s · %s", a.espSize, swap, compression)
}

// toConfig converts the collected answers into a validated Config.
func (a *answers) toConfig(ch Choices) (config.Config, error) {
	espMiB, err := config.ParseSizeMiB(a.espSize)
	if err != nil {
		return config.Config{}, fmt.Errorf("EFI partition size: %w", err)
	}

	swapMiB := 0
	if a.wantSwap {
		if swapMiB, err = config.ParseSizeMiB(a.swapSize); err != nil {
			return config.Config{}, fmt.Errorf("swap size: %w", err)
		}
	}

	rootPassword := a.rootPass
	if a.rootSame {
		rootPassword = a.password
	}

	nvidia := config.NvidiaNone
	if sys.HasNvidia(ch.GPUs) {
		nvidia = config.NvidiaDriver(a.nvidia)
	}

	cfg := config.Config{
		Disk:           a.disk,
		ESPSizeMiB:     espMiB,
		SwapSizeMiB:    swapMiB,
		Compression:    a.compression,
		Encrypt:        a.encrypt,
		LUKSPassphrase: a.passphrase,
		Timezone:       a.timezone,
		Locale:         a.locale,
		LocaleGenLine:  genLineFor(ch.Locales, a.locale),
		Keymap:         a.keymap,
		XKBLayout:      xkbFor(ch.Layouts, a.keymap),
		Hostname:       a.hostname,
		Username:       a.username,
		UserPassword:   a.password,
		RootPassword:   rootPassword,
		Microcode:      ch.Microcode,
		Nvidia:         nvidia,
		NvidiaPackages: sys.NvidiaPackages(nvidia),
	}
	if !a.encrypt {
		cfg.LUKSPassphrase = ""
	}

	if err := cfg.Validate(); err != nil {
		return config.Config{}, err
	}
	return cfg, nil
}

// genLineFor finds the locale.gen line matching a chosen locale name.
func genLineFor(locales []locale.Locale, name string) string {
	for _, l := range locales {
		if l.Name == name {
			return l.GenLine
		}
	}
	// Every UTF-8 locale in the catalogue has this shape, so this is a
	// reasonable reconstruction for a name that came from somewhere else.
	return name + " UTF-8"
}

// xkbFor finds the Hyprland layout paired with a chosen console keymap.
func xkbFor(layouts []locale.Layout, keymap string) string {
	for _, l := range layouts {
		if l.Keymap == keymap {
			return l.XKB
		}
	}
	return locale.XKBLayout(keymap)
}
