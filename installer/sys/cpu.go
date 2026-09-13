package sys

import (
	"bufio"
	"os"
	"strings"
)

// ProcCPUInfo is where DetectMicrocode reads on a real system.
const ProcCPUInfo = "/proc/cpuinfo"

// DetectMicrocode returns the microcode package for the host CPU, or "" if the
// vendor is not one that ships microcode updates through a package (a VM with
// a masked vendor, say).
//
// Getting this right matters more than it looks: until now Cairn installed no
// microcode at all, so every machine ran without the CPU errata fixes that
// Intel and AMD ship. The package pairs with mkinitcpio's `microcode` hook,
// which folds the blob into the initramfs — so no separate initrd line is
// needed in the boot entry.
func DetectMicrocode(cpuinfoPath string) string {
	f, err := os.Open(cpuinfoPath)
	if err != nil {
		return ""
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), ":")
		if !ok || strings.TrimSpace(key) != "vendor_id" {
			continue
		}
		switch strings.TrimSpace(value) {
		case "GenuineIntel":
			return "intel-ucode"
		case "AuthenticAMD", "HygonGenuine":
			return "amd-ucode"
		default:
			return ""
		}
	}
	return ""
}
