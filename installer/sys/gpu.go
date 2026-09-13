package sys

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jaykeraliya0/cairn/installer/config"
)

// SysfsPCIDevices is where DetectGPUs reads on a real system. Probing sysfs
// rather than shelling out to lspci keeps the detection working even if
// pciutils is ever dropped from the live ISO.
const SysfsPCIDevices = "/sys/bus/pci/devices"

// GPUVendor identifies the make of a detected graphics device.
type GPUVendor string

const (
	GPUIntel   GPUVendor = "Intel"
	GPUAMD     GPUVendor = "AMD"
	GPUNvidia  GPUVendor = "Nvidia"
	GPUUnknown GPUVendor = "Unknown"
)

// PCI vendor IDs for the three makers whose drivers differ.
const (
	pciVendorIntel  = 0x8086
	pciVendorAMD    = 0x1002
	pciVendorATI    = 0x1022
	pciVendorNvidia = 0x10de
)

// pciClassDisplay is the PCI base class for display controllers; the full
// class word is 0x03xxxx.
const pciClassDisplay = 0x03

// DetectGPUs returns the distinct graphics vendors present, in a stable order.
// A machine with both an integrated and a discrete GPU returns both.
func DetectGPUs(pciRoot string) []GPUVendor {
	entries, err := os.ReadDir(pciRoot)
	if err != nil {
		return nil
	}

	seen := map[GPUVendor]bool{}
	var vendors []GPUVendor
	for _, entry := range entries {
		class, err := readHexFile(filepath.Join(pciRoot, entry.Name(), "class"))
		if err != nil || class>>16 != pciClassDisplay {
			continue
		}
		vendorID, err := readHexFile(filepath.Join(pciRoot, entry.Name(), "vendor"))
		if err != nil {
			continue
		}

		var vendor GPUVendor
		switch vendorID {
		case pciVendorIntel:
			vendor = GPUIntel
		case pciVendorAMD, pciVendorATI:
			vendor = GPUAMD
		case pciVendorNvidia:
			vendor = GPUNvidia
		default:
			// Virtual display adapters (QEMU's bochs, VMware, virtio) all land
			// here and are served perfectly well by mesa.
			vendor = GPUUnknown
		}
		if !seen[vendor] {
			seen[vendor] = true
			vendors = append(vendors, vendor)
		}
	}
	return vendors
}

// NvidiaPackages returns the packages to install from the image's Nvidia side
// repository for the chosen driver, or nil when no Nvidia driver was chosen.
//
// Intel and AMD need nothing here: mesa and both vendors' Vulkan drivers are
// part of the system image, where they cost nothing at install time.
//
// Keep in step with installer/packages/nvidia-packages.x86_64 and with
// build_nvidia_repo in build.sh, which resolves exactly these two flavours.
func NvidiaPackages(driver config.NvidiaDriver) []string {
	switch driver {
	case config.NvidiaOpen:
		return []string{"nvidia-open", "nvidia-utils", "egl-wayland"}
	case config.NvidiaDKMS:
		// linux-headers is what lets DKMS rebuild the module on a kernel
		// upgrade; without it the first `pacman -Syu` leaves an unbootable
		// desktop.
		return []string{"nvidia-open-dkms", "nvidia-utils", "egl-wayland", "linux-headers"}
	}
	return nil
}

// HasNvidia reports whether an Nvidia GPU is among the detected vendors.
func HasNvidia(vendors []GPUVendor) bool {
	for _, v := range vendors {
		if v == GPUNvidia {
			return true
		}
	}
	return false
}

// DescribeGPUs renders the detected vendors for the hardware summary.
func DescribeGPUs(vendors []GPUVendor) string {
	if len(vendors) == 0 {
		return "none detected"
	}
	names := make([]string, len(vendors))
	for i, v := range vendors {
		names[i] = string(v)
	}
	return strings.Join(names, " + ")
}

// readHexFile reads a sysfs attribute holding a single "0x..." value.
func readHexFile(path string) (uint64, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.ParseUint(strings.TrimPrefix(strings.TrimSpace(string(b)), "0x"), 16, 64)
}
