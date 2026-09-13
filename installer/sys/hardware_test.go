package sys

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jaykeraliya0/cairn/installer/config"
	"github.com/jaykeraliya0/cairn/installer/packages"
)

func writeCPUInfo(t *testing.T, vendor string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "cpuinfo")
	body := fmt.Sprintf("processor\t: 0\nvendor_id\t: %s\ncpu family\t: 25\nmodel name\t: Test CPU\n", vendor)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDetectMicrocode(t *testing.T) {
	cases := map[string]string{
		"GenuineIntel":  "intel-ucode",
		"AuthenticAMD":  "amd-ucode",
		"HygonGenuine":  "amd-ucode",
		"SomethingElse": "",
	}
	for vendor, want := range cases {
		if got := DetectMicrocode(writeCPUInfo(t, vendor)); got != want {
			t.Errorf("DetectMicrocode(%s) = %q, want %q", vendor, got, want)
		}
	}

	if got := DetectMicrocode(filepath.Join(t.TempDir(), "absent")); got != "" {
		t.Errorf("DetectMicrocode on a missing file = %q, want %q", got, "")
	}
}

// writePCIDevice fakes one sysfs PCI device directory.
func writePCIDevice(t *testing.T, root, address string, class, vendor uint32) {
	t.Helper()
	dir := filepath.Join(root, address)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]string{
		"class":  fmt.Sprintf("0x%06x\n", class),
		"vendor": fmt.Sprintf("0x%04x\n", vendor),
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(value), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDetectGPUs(t *testing.T) {
	root := t.TempDir()
	// A laptop with Intel graphics and a discrete Nvidia card, plus a network
	// controller that must be ignored.
	writePCIDevice(t, root, "0000:00:02.0", 0x030000, pciVendorIntel)
	writePCIDevice(t, root, "0000:01:00.0", 0x030200, pciVendorNvidia)
	writePCIDevice(t, root, "0000:00:1f.6", 0x020000, pciVendorIntel)

	got := DetectGPUs(root)
	if len(got) != 2 || got[0] != GPUIntel || got[1] != GPUNvidia {
		t.Fatalf("DetectGPUs = %v, want [Intel Nvidia]", got)
	}
	if !HasNvidia(got) {
		t.Error("HasNvidia = false, want true")
	}
	if want := "Intel + Nvidia"; DescribeGPUs(got) != want {
		t.Errorf("DescribeGPUs = %q, want %q", DescribeGPUs(got), want)
	}
}

func TestDetectGPUsVirtualMachine(t *testing.T) {
	root := t.TempDir()
	// QEMU's bochs-drm adapter: a display controller from no vendor we know.
	writePCIDevice(t, root, "0000:00:01.0", 0x030000, 0x1234)

	got := DetectGPUs(root)
	if len(got) != 1 || got[0] != GPUUnknown {
		t.Fatalf("DetectGPUs = %v, want [Unknown]", got)
	}
	if HasNvidia(got) {
		t.Error("HasNvidia = true for a virtual adapter, want false")
	}
}

func TestNvidiaPackages(t *testing.T) {
	if got := NvidiaPackages(config.NvidiaNone); len(got) != 0 {
		t.Errorf("no Nvidia driver chosen, yet %v would be installed", got)
	}

	cases := map[config.NvidiaDriver]string{
		config.NvidiaOpen: "nvidia-open nvidia-utils egl-wayland",
		config.NvidiaDKMS: "nvidia-open-dkms nvidia-utils egl-wayland linux-headers",
	}
	for driver, want := range cases {
		if got := strings.Join(NvidiaPackages(driver), " "); got != want {
			t.Errorf("NvidiaPackages(%s) = %q, want %q", driver, got, want)
		}
	}

	// Every name has to be in the side repository's list. build.sh builds the
	// repository from that list, so a name missing from it is a package the
	// installer asks for on a machine with no network and no way to get it.
	inRepo := map[string]bool{}
	for _, pkg := range packages.Nvidia() {
		inRepo[pkg] = true
	}
	for driver := range cases {
		for _, pkg := range NvidiaPackages(driver) {
			if !inRepo[pkg] {
				t.Errorf("%s needs %q, which nvidia-packages.x86_64 does not list", driver, pkg)
			}
		}
	}
}
