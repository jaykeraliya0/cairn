package apply

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jaykeraliya0/cairn/installer/config"
	"github.com/jaykeraliya0/cairn/installer/sys"
)

const (
	testLUKSUUID = "11111111-2222-3333-4444-555555555555"
	testRootUUID = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"

	testUserPassword = "correct-horse-battery-staple"
	testRootPassword = "s3cret-root-pw"
	testPassphrase   = "open-sesame-luks"
)

func testConfig() config.Config {
	return config.Config{
		Disk:          "/dev/sda",
		ESPSizeMiB:    1024,
		SwapSizeMiB:   4096,
		Compression:   true,
		Timezone:      "Asia/Kolkata",
		Locale:        "en_IN.UTF-8",
		LocaleGenLine: "en_IN.UTF-8 UTF-8",
		Keymap:        "us",
		XKBLayout:     "us",
		Hostname:      "cairn",
		Username:      "jay",
		UserPassword:  testUserPassword,
		RootPassword:  testRootPassword,
		Microcode:     "amd-ucode",
		Nvidia:        config.NvidiaNone,
	}
}

// newTestState wires a State onto a temp directory and a recording runner, and
// pre-creates the files a real install would have found there by the time the
// later steps run.
func newTestState(t *testing.T, cfg config.Config) (*State, *sys.FakeRunner) {
	t.Helper()

	root := t.TempDir()
	// A stand-in for the system image. The runner is fake, so its contents
	// never matter; extractImage only checks that the file is there.
	image := filepath.Join(t.TempDir(), "cairn-root.sfs")
	if err := os.WriteFile(image, []byte("hsqs"), 0o644); err != nil {
		t.Fatal(err)
	}

	// The image and mkinitcpio would have produced these.
	if err := os.MkdirAll(filepath.Join(root, "boot"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"vmlinuz-linux", "initramfs-linux.img", "initramfs-linux-fallback.img"} {
		if err := os.WriteFile(filepath.Join(root, "boot", name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	runner := &sys.FakeRunner{
		Outputs: map[string]string{
			"genfstab -U " + root:                              "UUID=" + testRootUUID + " / btrfs rw,noatime,compress=zstd,subvol=@ 0 0",
			"blkid -s UUID -o value /dev/sda2":                 testLUKSUUID,
			"blkid -s UUID -o value /dev/mapper/" + MapperName: testRootUUID,
		},
	}
	if !cfg.Encrypt {
		// Unencrypted, partition 2 holds Btrfs itself, so the same blkid call
		// returns the filesystem's UUID.
		runner.Outputs["blkid -s UUID -o value /dev/sda2"] = testRootUUID
	}

	return &State{
		Cfg:           cfg,
		Runner:        runner,
		Root:          root,
		Image:         image,
		BootStage:     filepath.Join(t.TempDir(), "boot-stage"),
		WaitForDevice: func(context.Context, string) error { return nil },
	}, runner
}

// assertOrder checks that the given command fragments appear in the recorded
// command list in this order. Extra commands in between are fine; the point is
// the sequence, since an install that formats before it partitions is a very
// different install.
func assertOrder(t *testing.T, runner *sys.FakeRunner, fragments ...string) {
	t.Helper()

	lines := runner.CommandLines()
	at := 0
	for _, fragment := range fragments {
		found := false
		for ; at < len(lines); at++ {
			if strings.Contains(lines[at], fragment) {
				found = true
				at++
				break
			}
		}
		if !found {
			t.Fatalf("command %q did not appear (in order) in:\n  %s",
				fragment, strings.Join(lines, "\n  "))
		}
	}
}

func TestRunUnencryptedInstall(t *testing.T) {
	st, runner := newTestState(t, testConfig())

	if err := Run(context.Background(), st, nil); err != nil {
		t.Fatalf("Run: %v", err)
	}

	assertOrder(t, runner,
		"sgdisk --zap-all /dev/sda",
		"sgdisk -n1:0:+1024M -t1:ef00 -c1:EFI System /dev/sda",
		"sgdisk -n2:0:0 -t2:8300 -c2:Cairn Root /dev/sda",
		"partprobe /dev/sda",
		"wipefs --all /dev/sda1",
		"wipefs --all /dev/sda2",
		"mkfs.fat -F32 -n EFI /dev/sda1",
		"mkfs.btrfs -f -L cairn_root /dev/sda2",
		"btrfs subvolume create",
		"mount -t btrfs -o noatime,compress=zstd,subvol=@ /dev/sda2 "+st.Root,
		"unsquashfs -f -d "+st.Root,
		"btrfs filesystem mkswapfile --size 4096M",
		"arch-chroot "+st.Root+" /bin/bash /root/cairn-chroot.sh",
		"umount -R "+st.Root,
	)

	if runner.Ran("cryptsetup") {
		t.Errorf("an unencrypted install ran cryptsetup: %q", runner.Find("cryptsetup"))
	}

	// Every subvolume gets mounted, at its own place, with the same options.
	for _, subvol := range Subvolumes {
		want := "mount -t btrfs -o noatime,compress=zstd,subvol=" + subvol.Name + " /dev/sda2 " + filepath.Join(st.Root, subvol.Path)
		if !runner.Ran(want) {
			t.Errorf("missing mount for %s:\nwant %q\ngot  %s",
				subvol.Name, want, strings.Join(runner.CommandLines(), "\n     "))
		}
	}
	if !runner.Ran("mount -t vfat /dev/sda1 " + filepath.Join(st.Root, "boot")) {
		t.Error("the ESP was never mounted at /boot")
	}

	entry := readFile(t, filepath.Join(st.Root, "boot/loader/entries/cairn.conf"))
	if !strings.Contains(entry, "options root=UUID="+testRootUUID+" rootflags=subvol=@ rw") {
		t.Errorf("boot entry has the wrong command line:\n%s", entry)
	}
	if strings.Contains(entry, "rd.luks") {
		t.Errorf("an unencrypted boot entry mentions LUKS:\n%s", entry)
	}
	if !strings.Contains(entry, "linux   /vmlinuz-linux") || !strings.Contains(entry, "initrd  /initramfs-linux.img") {
		t.Errorf("boot entry points at the wrong images:\n%s", entry)
	}

	hooks := readFile(t, filepath.Join(st.Root, "etc/mkinitcpio.conf.d/cairn.conf"))
	if strings.Contains(hooks, "sd-encrypt") {
		t.Errorf("an unencrypted install asked for the sd-encrypt hook:\n%s", hooks)
	}

	fstab := readFile(t, filepath.Join(st.Root, "etc/fstab"))
	if !strings.Contains(fstab, "UUID="+testRootUUID) {
		t.Errorf("fstab is missing genfstab's output:\n%s", fstab)
	}
	// The swap line has to land after genfstab's, or the append order would
	// have interleaved them.
	if got := strings.Index(fstab, "/swapfile"); got < strings.Index(fstab, "UUID=") {
		t.Errorf("the swapfile entry precedes genfstab's output:\n%s", fstab)
	}
}

func TestRunEncryptedInstall(t *testing.T) {
	cfg := testConfig()
	cfg.Encrypt = true
	cfg.LUKSPassphrase = testPassphrase

	st, runner := newTestState(t, cfg)

	if err := Run(context.Background(), st, nil); err != nil {
		t.Fatalf("Run: %v", err)
	}

	assertOrder(t, runner,
		// Partition 2 is typed as Linux LUKS, not a plain filesystem.
		"sgdisk -n2:0:0 -t2:8309 -c2:Cairn Root /dev/sda",
		// The old signatures go before the new container is written.
		"wipefs --all /dev/sda2",
		"cryptsetup luksFormat --type luks2 --batch-mode --key-file - /dev/sda2",
		"cryptsetup open --key-file - /dev/sda2 "+MapperName,
		// Everything downstream addresses the mapper device, not the partition.
		"mkfs.btrfs -f -L cairn_root /dev/mapper/"+MapperName,
		"mount -t btrfs -o noatime,compress=zstd,subvol=@ /dev/mapper/"+MapperName,
		"unsquashfs -f -d "+st.Root,
		"umount -R "+st.Root,
		"cryptsetup close "+MapperName,
	)

	if runner.Ran("mkfs.btrfs", "/dev/sda2") {
		t.Error("Btrfs was written to the raw partition instead of the opened container")
	}

	entry := readFile(t, filepath.Join(st.Root, "boot/loader/entries/cairn.conf"))
	want := "options rd.luks.name=" + testLUKSUUID + "=" + MapperName +
		" root=/dev/mapper/" + MapperName + " rootflags=subvol=@ rw"
	if !strings.Contains(entry, want) {
		t.Errorf("boot entry has the wrong command line:\nwant %q\ngot\n%s", want, entry)
	}

	hooks := readFile(t, filepath.Join(st.Root, "etc/mkinitcpio.conf.d/cairn.conf"))
	if !strings.Contains(hooks, "sd-encrypt") {
		t.Errorf("the encrypted install did not add the sd-encrypt hook:\n%s", hooks)
	}

}

// TestSecretsNeverReachArgvOrEnv is the test that matters most here.
//
// An argv is readable by any local process through /proc/<pid>/cmdline, and an
// environment block through /proc/<pid>/environ, for as long as the process
// lives — and unsquashfs and arch-chroot live for minutes. Passwords and the
// LUKS passphrase must therefore only ever travel over stdin.
func TestSecretsNeverReachArgvOrEnv(t *testing.T) {
	cfg := testConfig()
	cfg.Encrypt = true
	cfg.LUKSPassphrase = testPassphrase

	st, runner := newTestState(t, cfg)
	if err := Run(context.Background(), st, nil); err != nil {
		t.Fatalf("Run: %v", err)
	}

	secrets := map[string]string{
		"user password":   testUserPassword,
		"root password":   testRootPassword,
		"LUKS passphrase": testPassphrase,
	}

	for label, secret := range secrets {
		for _, c := range runner.Calls {
			if strings.Contains(c.String(), secret) {
				t.Errorf("the %s appears in an argv: %s", label, c.Name)
			}
			for _, env := range c.Env {
				if strings.Contains(env, secret) {
					t.Errorf("the %s appears in the environment of %s", label, c.Name)
				}
			}
		}
	}

	// The chroot script itself is written to the target filesystem, so it must
	// be free of them too — and it is removed once the step finishes.
	if _, err := os.Stat(filepath.Join(st.Root, ChrootScriptPath)); !os.IsNotExist(err) {
		t.Errorf("the chroot script was left behind at %s", ChrootScriptPath)
	}

	// Having established they are nowhere else, check they did reach the one
	// place they are supposed to.
	var passphraseSeen, passwordsSeen bool
	for i, c := range runner.Calls {
		switch c.Name {
		case "cryptsetup":
			if runner.Stdins[i] == testPassphrase {
				passphraseSeen = true
			}
		case "arch-chroot":
			want := "root:" + testRootPassword + "\njay:" + testUserPassword + "\n"
			if runner.Stdins[i] == want {
				passwordsSeen = true
			}
		}
	}
	if !passphraseSeen {
		t.Error("the LUKS passphrase never reached cryptsetup's stdin")
	}
	if !passwordsSeen {
		t.Error("the account passwords never reached the chroot's stdin")
	}
}

func TestChrootScriptTakesNoSubstitution(t *testing.T) {
	// The script is a constant. If a user-supplied value is ever formatted
	// into it, a hostname of "; reboot" becomes a command — which is the whole
	// reason the values travel as environment variables instead.
	for _, placeholder := range []string{
		"${CAIRN_HOSTNAME}", "${CAIRN_USERNAME}", "${CAIRN_TIMEZONE}",
		"${CAIRN_LOCALE}", "${CAIRN_LOCALE_GEN_LINE}", "${CAIRN_KEYMAP}",
	} {
		if !strings.Contains(chrootScript, placeholder) {
			t.Errorf("the chroot script does not read %s", placeholder)
		}
	}
	// The script's own printf calls use %s, so the absence of a format verb
	// proves nothing. What matters is that none of the user's answers appear
	// in the text that the inner shell will parse.
	cfg := testConfig()
	cfg.Hostname = "unlikely-hostname-marker"
	cfg.Username = "unlikelyuser"
	cfg.Timezone = "Antarctica/Troll"
	for _, value := range []string{
		cfg.Hostname, cfg.Username, cfg.Timezone,
		cfg.UserPassword, cfg.RootPassword, cfg.LocaleGenLine,
	} {
		if strings.Contains(chrootScript, value) {
			t.Errorf("the chroot script contains the user-supplied value %q", value)
		}
	}

	cfg = testConfig()
	env := chrootEnv(cfg)
	for _, want := range []string{
		"CAIRN_HOSTNAME=cairn", "CAIRN_USERNAME=jay", "CAIRN_TIMEZONE=Asia/Kolkata",
		"CAIRN_LOCALE=en_IN.UTF-8", "CAIRN_LOCALE_GEN_LINE=en_IN.UTF-8 UTF-8", "CAIRN_KEYMAP=us",
	} {
		if !contains(env, want) {
			t.Errorf("chrootEnv is missing %q, got %v", want, env)
		}
	}

	// A hostname full of shell syntax is passed through untouched; it is data,
	// not script.
	hostile := testConfig()
	hostile.Hostname = "$(reboot)"
	if !contains(chrootEnv(hostile), "CAIRN_HOSTNAME=$(reboot)") {
		t.Error("a hostname containing shell syntax was altered rather than passed through as data")
	}
}

func TestStepsMatchTheConfig(t *testing.T) {
	names := func(cfg config.Config) string {
		var out []string
		for _, s := range Steps(cfg) {
			out = append(out, s.Name)
		}
		return strings.Join(out, " | ")
	}

	plain := testConfig()
	plain.SwapSizeMiB = 0
	if got := names(plain); strings.Contains(got, "encryption") || strings.Contains(got, "swapfile") {
		t.Errorf("steps for a plain install include work that will not happen: %s", got)
	}

	full := testConfig()
	full.Encrypt = true
	full.LUKSPassphrase = testPassphrase
	if got := names(full); !strings.Contains(got, "encryption") || !strings.Contains(got, "swapfile") {
		t.Errorf("steps for a full install are missing work that will happen: %s", got)
	}
}

func TestCleanupUnwindsMountsAndEncryption(t *testing.T) {
	cfg := testConfig()
	cfg.Encrypt = true
	cfg.LUKSPassphrase = testPassphrase

	st, runner := newTestState(t, cfg)

	// Stop partway through, the way a failed extraction would.
	ctx := context.Background()
	for _, step := range Steps(cfg)[:5] {
		if err := step.Run(ctx, st); err != nil {
			t.Fatalf("%s: %v", step.Name, err)
		}
	}

	before := len(runner.Calls)
	st.Cleanup(ctx)

	after := runner.CommandLines()[before:]
	joined := strings.Join(after, " | ")
	if !strings.Contains(joined, "umount -R "+st.Root) {
		t.Errorf("cleanup did not unmount the target: %s", joined)
	}
	if !strings.Contains(joined, "cryptsetup close "+MapperName) {
		t.Errorf("cleanup did not close the encrypted volume: %s", joined)
	}

	// Cleanup has to be safe to call twice — the TUI calls it on failure and
	// the caller may call it again on the way out.
	repeat := len(runner.Calls)
	st.Cleanup(ctx)
	if len(runner.Calls) != repeat {
		t.Errorf("a second Cleanup ran %d more commands, want 0", len(runner.Calls)-repeat)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(b)
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

// TestReinstallingOverAnOldInstallWipesItsSignatures is the regression test for
// installing onto a disk that has been installed to before.
//
// sgdisk rewrites only the partition table, and the new partitions sit exactly
// where the old ones were. A LUKS header from an earlier encrypted install
// survived mkfs.btrfs, libblkid probed the partition as crypto_LUKS, and the
// install died at "mount: unknown filesystem type 'crypto_LUKS'".
func TestReinstallingOverAnOldInstallWipesItsSignatures(t *testing.T) {
	st, runner := newTestState(t, testConfig())
	if err := Run(context.Background(), st, nil); err != nil {
		t.Fatalf("Run: %v", err)
	}

	lines := runner.CommandLines()
	index := func(fragment string) int {
		for i, line := range lines {
			if strings.Contains(line, fragment) {
				return i
			}
		}
		return -1
	}

	for _, part := range []string{"/dev/sda1", "/dev/sda2"} {
		wipe := index("wipefs --all " + part)
		if wipe < 0 {
			t.Errorf("%s is never wiped before it is formatted", part)
			continue
		}
		// After the partition exists...
		if wipe < index("partprobe") {
			t.Errorf("%s is wiped before the new partition table is read", part)
		}
		// ...and before anything is written to it.
		if wipe > index("mkfs.fat") || wipe > index("mkfs.btrfs") {
			t.Errorf("%s is wiped after a filesystem was already created", part)
		}
	}

	// And no mount is left to guess the filesystem.
	for _, line := range lines {
		if strings.HasPrefix(line, "mount ") && !strings.HasPrefix(line, "mount -t ") {
			t.Errorf("mount probes for the filesystem type instead of naming it: %q", line)
		}
	}
}
