package apply

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jaykeraliya0/cairn/installer/config"
)

func TestMkinitcpioConfig(t *testing.T) {
	plain := MkinitcpioConfig(testConfig())
	if !strings.Contains(plain, "HOOKS=(base systemd autodetect microcode modconf kms keyboard sd-vconsole block filesystems fsck)") {
		t.Errorf("unexpected hooks for a plain install:\n%s", plain)
	}
	if strings.Contains(plain, "MODULES=") {
		t.Errorf("a plain install should set no MODULES:\n%s", plain)
	}

	encrypted := testConfig()
	encrypted.Encrypt = true
	got := MkinitcpioConfig(encrypted)
	// sd-encrypt has to come before filesystems: the root filesystem cannot be
	// mounted until the container holding it is open.
	if i, j := strings.Index(got, "sd-encrypt"), strings.Index(got, "filesystems"); i < 0 || i > j {
		t.Errorf("sd-encrypt is missing or ordered after filesystems:\n%s", got)
	}

	nvidia := testConfig()
	nvidia.Nvidia = config.NvidiaOpen
	got = MkinitcpioConfig(nvidia)
	if !strings.Contains(got, "MODULES=(nvidia nvidia_modeset nvidia_uvm nvidia_drm)") {
		t.Errorf("Nvidia early-KMS modules are missing:\n%s", got)
	}
	// kms pulls in the in-kernel DRM drivers, which fight the Nvidia module
	// for the console.
	if strings.Contains(got, " kms ") {
		t.Errorf("the kms hook should be dropped on an Nvidia install:\n%s", got)
	}
}

func TestHyprlandHardwareConfig(t *testing.T) {
	cfg := testConfig()
	cfg.XKBLayout = "de"

	got := hyprlandHardwareConfig(cfg)
	if !strings.Contains(got, `kb_layout = "de"`) {
		t.Errorf("the chosen keyboard layout is missing:\n%s", got)
	}
	if strings.Contains(got, "LIBVA_DRIVER_NAME") {
		t.Errorf("a non-Nvidia machine got the Nvidia environment:\n%s", got)
	}

	cfg.Nvidia = config.NvidiaDKMS
	got = hyprlandHardwareConfig(cfg)
	for _, want := range []string{"LIBVA_DRIVER_NAME", "__GLX_VENDOR_LIBRARY_NAME", "NVD_BACKEND"} {
		if !strings.Contains(got, `hl.env("`+want+`"`) {
			t.Errorf("the Nvidia environment is missing %q:\n%s", want, got)
		}
	}

	// It is Lua now, not hyprlang: Hyprland deprecated its own config language
	// in 0.55 and warns on every start when it finds one.
	if strings.Contains(got, "env = ") || strings.Contains(got, "input {") {
		t.Errorf("the fragment still uses hyprlang syntax:\n%s", got)
	}
}

func TestKernelCmdline(t *testing.T) {
	plain := testConfig()
	if got, want := KernelCmdline(plain, "", testRootUUID),
		"root=UUID="+testRootUUID+" rootflags=subvol=@ rw"; got != want {
		t.Errorf("KernelCmdline = %q, want %q", got, want)
	}

	encrypted := testConfig()
	encrypted.Encrypt = true
	want := "rd.luks.name=" + testLUKSUUID + "=" + MapperName + " root=/dev/mapper/" + MapperName + " rootflags=subvol=@ rw"
	if got := KernelCmdline(encrypted, testLUKSUUID, testRootUUID); got != want {
		t.Errorf("KernelCmdline = %q, want %q", got, want)
	}

	nvidia := testConfig()
	nvidia.Nvidia = config.NvidiaOpen
	if got := KernelCmdline(nvidia, "", testRootUUID); !strings.HasSuffix(got, " nvidia_drm.modeset=1") {
		t.Errorf("KernelCmdline = %q, want it to end with the Nvidia modeset parameter", got)
	}
}

func TestFindKernelImages(t *testing.T) {
	boot := t.TempDir()
	for _, name := range []string{
		"vmlinuz-linux", "initramfs-linux.img", "initramfs-linux-fallback.img",
		"amd-ucode.img", "loader",
	} {
		if err := os.WriteFile(filepath.Join(boot, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	kernel, initrd, err := findKernelImages(boot)
	if err != nil {
		t.Fatalf("findKernelImages: %v", err)
	}
	if kernel != "vmlinuz-linux" {
		t.Errorf("kernel = %q, want vmlinuz-linux", kernel)
	}
	// The fallback image is deliberately not the default entry's initramfs.
	if initrd != "initramfs-linux.img" {
		t.Errorf("initrd = %q, want initramfs-linux.img", initrd)
	}

	if _, _, err := findKernelImages(t.TempDir()); err == nil {
		t.Error("findKernelImages on an empty /boot = nil error, want an error")
	}
}

// ---- system image ------------------------------------------------------------

func TestExtractImageKeepsUnsquashfsOffTheESP(t *testing.T) {
	st, runner := newTestState(t, testConfig())

	if err := extractImage(context.Background(), st); err != nil {
		t.Fatalf("extractImage: %v", err)
	}

	lines := runner.CommandLines()
	if len(lines) != 3 {
		t.Fatalf("extractImage ran %d commands, want 3:\n  %s", len(lines), strings.Join(lines, "\n  "))
	}

	// Everything except /boot goes straight onto the target...
	if want := "unsquashfs -f -d " + st.Root + " -percentage -excludes " + st.Image + " boot"; lines[0] != want {
		t.Errorf("first extraction:\n got  %q\n want %q", lines[0], want)
	}
	// ...while /boot is unpacked on its own, into the RAM-backed staging dir,
	// never onto the FAT32 ESP where unsquashfs would fail to set ownership.
	if want := "unsquashfs -f -d " + st.BootStage + " -percentage " + st.Image + " boot"; lines[1] != want {
		t.Errorf("boot extraction:\n got  %q\n want %q", lines[1], want)
	}
	// Then copied across without trying to keep what FAT32 cannot store.
	if !strings.HasPrefix(lines[2], "cp -r --no-preserve=mode,ownership,timestamps ") ||
		!strings.HasSuffix(lines[2], filepath.Join(st.Root, "boot")+"/") {
		t.Errorf("boot copy = %q", lines[2])
	}

	if _, err := os.Stat(st.BootStage); !os.IsNotExist(err) {
		t.Error("the staging directory was left behind")
	}
}

func TestExtractImageRefusesToRunWithoutAnImage(t *testing.T) {
	st, runner := newTestState(t, testConfig())
	st.Image = filepath.Join(t.TempDir(), "absent.sfs")

	if err := extractImage(context.Background(), st); err == nil {
		t.Fatal("extractImage succeeded with no image")
	}
	// Nothing may run against an already partitioned disk without one.
	if len(runner.Calls) != 0 {
		t.Errorf("ran %v without an image", runner.CommandLines())
	}
}

func TestInstallNvidiaIsOfflineAndLeavesNoCopies(t *testing.T) {
	cfg := testConfig()
	cfg.Nvidia = config.NvidiaOpen
	cfg.NvidiaPackages = []string{"nvidia-open", "nvidia-utils", "egl-wayland"}
	st, runner := newTestState(t, cfg)

	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, nvidiaRepoConfig), []byte("[options]"), 0o644); err != nil {
		t.Fatal(err)
	}
	st.NvidiaRepo = repo

	if err := installNvidia(context.Background(), st); err != nil {
		t.Fatalf("installNvidia: %v", err)
	}

	want := "pacstrap -C " + filepath.Join(repo, nvidiaRepoConfig) + " -c -G -M " + st.Root +
		" nvidia-open nvidia-utils egl-wayland"
	if got := runner.Find("pacstrap"); got != want {
		t.Errorf("pacstrap:\n got  %q\n want %q", got, want)
	}
}

func TestInstallNvidiaNeedsItsRepository(t *testing.T) {
	cfg := testConfig()
	cfg.NvidiaPackages = []string{"nvidia-open"}
	st, runner := newTestState(t, cfg)
	st.NvidiaRepo = t.TempDir() // no pacman.conf inside

	if err := installNvidia(context.Background(), st); err == nil {
		t.Fatal("installNvidia succeeded with no repository")
	}
	if len(runner.Calls) != 0 {
		t.Errorf("ran %v with no repository", runner.CommandLines())
	}
}

func TestNvidiaStepOnlyWhenADriverWasChosen(t *testing.T) {
	names := func(cfg config.Config) string {
		var out []string
		for _, s := range Steps(cfg) {
			out = append(out, s.Name)
		}
		return strings.Join(out, " | ")
	}

	if got := names(testConfig()); strings.Contains(got, "Nvidia") {
		t.Errorf("a machine with no Nvidia driver gets an Nvidia step: %s", got)
	}

	cfg := testConfig()
	cfg.NvidiaPackages = []string{"nvidia-open"}
	got := names(cfg)
	if !strings.Contains(got, "Nvidia") {
		t.Errorf("an Nvidia install has no Nvidia step: %s", got)
	}
	// The driver's mkinitcpio hook runs during its install, so the drop-in it
	// reads has to be written first.
	if strings.Index(got, "Nvidia") < strings.Index(got, "default configuration") {
		t.Errorf("the Nvidia step runs before the mkinitcpio drop-in is written: %s", got)
	}
}
