package config

import "testing"

func TestValidateHostname(t *testing.T) {
	valid := []string{"cairn", "c", "my-laptop", "box123", "a-b-c"}
	for _, name := range valid {
		if err := ValidateHostname(name); err != nil {
			t.Errorf("ValidateHostname(%q) = %v, want nil", name, err)
		}
	}

	invalid := map[string]string{
		"":           "empty",
		"-leading":   "leading hyphen",
		"trailing-":  "trailing hyphen",
		"has space":  "space",
		"has_under":  "underscore",
		"UPPER-ok":   "", // uppercase is legal, checked below
		"a.b":        "dot",
		"héllo":      "non-ASCII",
		"; rm -rf /": "shell metacharacters",
		"x@y":        "at sign",
	}
	for name, why := range invalid {
		if why == "" {
			continue
		}
		if err := ValidateHostname(name); err == nil {
			t.Errorf("ValidateHostname(%q) = nil, want an error (%s)", name, why)
		}
	}

	if err := ValidateHostname("UPPER-ok"); err != nil {
		t.Errorf("ValidateHostname on an uppercase name = %v, want nil", err)
	}

	long := make([]byte, 64)
	for i := range long {
		long[i] = 'a'
	}
	if err := ValidateHostname(string(long)); err == nil {
		t.Error("ValidateHostname on a 64-character name = nil, want an error")
	}
}

func TestValidateUsername(t *testing.T) {
	valid := []string{"jay", "_svc", "a", "user-name", "u0", "machine$"}
	for _, name := range valid {
		if err := ValidateUsername(name); err != nil {
			t.Errorf("ValidateUsername(%q) = %v, want nil", name, err)
		}
	}

	invalid := []string{"", "Jay", "0start", "-start", "has space", "has.dot", "root", "sddm", "with:colon"}
	for _, name := range invalid {
		if err := ValidateUsername(name); err == nil {
			t.Errorf("ValidateUsername(%q) = nil, want an error", name)
		}
	}
}

func TestValidatePassword(t *testing.T) {
	// A colon is fine: chpasswd splits on the first one only, and the first
	// one is the separator we write ourselves.
	if err := ValidatePassword("user", "pa:ss word!"); err != nil {
		t.Errorf("ValidatePassword on a colon-containing password = %v, want nil", err)
	}
	for _, bad := range []string{"", "two\nlines", "carriage\rreturn"} {
		if err := ValidatePassword("user", bad); err == nil {
			t.Errorf("ValidatePassword(%q) = nil, want an error", bad)
		}
	}
}

func TestParseSizeMiB(t *testing.T) {
	cases := map[string]int{
		"8G":      8192,
		"8g":      8192,
		"8GB":     8192,
		"8 GiB":   8192,
		"512M":    512,
		"512 MiB": 512,
		"1T":      1024 * 1024,
		// A bare number is GiB — what someone typing into a swap box means.
		"4":    4096,
		"1.5":  1536,
		"0.5G": 512,
	}
	for input, want := range cases {
		got, err := ParseSizeMiB(input)
		if err != nil {
			t.Errorf("ParseSizeMiB(%q) = error %v", input, err)
			continue
		}
		if got != want {
			t.Errorf("ParseSizeMiB(%q) = %d, want %d", input, got, want)
		}
	}

	for _, bad := range []string{"", "   ", "lots", "8Z", "-4G", "0", "0M", "8B", "G"} {
		if got, err := ParseSizeMiB(bad); err == nil {
			t.Errorf("ParseSizeMiB(%q) = %d, want an error", bad, got)
		}
	}
}

func TestFormatSizeMiB(t *testing.T) {
	cases := map[int]string{0: "none", 512: "512 MiB", 1024: "1 GiB", 8192: "8 GiB", 1048576: "1 TiB"}
	for input, want := range cases {
		if got := FormatSizeMiB(input); got != want {
			t.Errorf("FormatSizeMiB(%d) = %q, want %q", input, got, want)
		}
	}
}

func TestMountOptions(t *testing.T) {
	if got := (Config{Compression: true}).MountOptions(); got != "noatime,compress=zstd" {
		t.Errorf("compressed MountOptions = %q", got)
	}
	if got := (Config{}).MountOptions(); got != "noatime" {
		t.Errorf("uncompressed MountOptions = %q", got)
	}
}

// validConfig is the baseline every Validate case starts from.
func validConfig() Config {
	return Config{
		Disk:          "/dev/sda",
		ESPSizeMiB:    DefaultESPSizeMiB,
		SwapSizeMiB:   4096,
		Timezone:      "Asia/Kolkata",
		Locale:        "en_US.UTF-8",
		LocaleGenLine: "en_US.UTF-8 UTF-8",
		Keymap:        "us",
		XKBLayout:     "us",
		Hostname:      "cairn",
		Username:      "jay",
		UserPassword:  "hunter2",
		RootPassword:  "hunter3",
	}
}

func TestValidate(t *testing.T) {
	if err := validConfig().Validate(); err != nil {
		t.Fatalf("the baseline config should validate, got %v", err)
	}

	cases := map[string]func(*Config){
		"no disk":            func(c *Config) { c.Disk = "" },
		"ESP too small":      func(c *Config) { c.ESPSizeMiB = 16 },
		"ESP too large":      func(c *Config) { c.ESPSizeMiB = MaxESPSizeMiB + 1 },
		"swap too small":     func(c *Config) { c.SwapSizeMiB = 1 },
		"encrypted, no pass": func(c *Config) { c.Encrypt = true },
		"bad hostname":       func(c *Config) { c.Hostname = "-nope" },
		"bad username":       func(c *Config) { c.Username = "Root" },
		"no user password":   func(c *Config) { c.UserPassword = "" },
		"no root password":   func(c *Config) { c.RootPassword = "" },
		"no timezone":        func(c *Config) { c.Timezone = "" },
		"no locale":          func(c *Config) { c.Locale = "" },
		"no locale.gen line": func(c *Config) { c.LocaleGenLine = "" },
		"no keymap":          func(c *Config) { c.Keymap = "" },
	}
	for name, mutate := range cases {
		cfg := validConfig()
		mutate(&cfg)
		if err := cfg.Validate(); err == nil {
			t.Errorf("Validate with %s = nil, want an error", name)
		}
	}

	// Swap is optional, so zero has to stay valid.
	cfg := validConfig()
	cfg.SwapSizeMiB = 0
	if err := cfg.Validate(); err != nil {
		t.Errorf("Validate with no swap = %v, want nil", err)
	}
}
