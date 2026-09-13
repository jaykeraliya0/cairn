package apply

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jaykeraliya0/cairn/installer/config"
)

// loaderConf is systemd-boot's own configuration. `editor no` matters on an
// encrypted install: an editable command line lets anyone at the keyboard boot
// with init=/bin/sh and walk straight past the login prompt.
const loaderConf = `default cairn.conf
timeout 3
console-mode max
editor no
`

// KernelCmdline builds the kernel command line for a config.
//
// On an encrypted root, rd.luks.name names the container by UUID and the
// systemd sd-encrypt hook opens it as /dev/mapper/cryptroot before the root
// filesystem is mounted. The UUID is the container's, captured before it was
// opened — the opened mapper device has a different one.
func KernelCmdline(cfg config.Config, luksUUID, rootUUID string) string {
	var params []string
	if cfg.Encrypt {
		params = append(params,
			fmt.Sprintf("rd.luks.name=%s=%s", luksUUID, MapperName),
			"root=/dev/mapper/"+MapperName)
	} else {
		params = append(params, "root=UUID="+rootUUID)
	}
	params = append(params, "rootflags=subvol=@", "rw")

	if cfg.UsesNvidia() {
		// Required for Wayland compositors on Nvidia; the default is only on
		// for recent driver versions, so set it explicitly.
		params = append(params, "nvidia_drm.modeset=1")
	}
	return strings.Join(params, " ")
}

// BootEntry renders the systemd-boot entry.
func BootEntry(cfg config.Config, kernel, initrd, cmdline string) string {
	return fmt.Sprintf("title   Cairn Linux\nlinux   /%s\ninitrd  /%s\noptions %s\n",
		kernel, initrd, cmdline)
}

// installBootloader writes systemd-boot's loader configuration and the Cairn
// entry. `bootctl install` itself already ran inside the chroot.
func installBootloader(ctx context.Context, s *State) error {
	kernel, initrd, err := findKernelImages(s.path("boot"))
	if err != nil {
		return err
	}

	if !s.Cfg.Encrypt {
		uuid, err := s.Runner.Output(ctx, "blkid", "-s", "UUID", "-o", "value", s.RootDev)
		if err != nil {
			return fmt.Errorf("reading the root filesystem UUID: %w", err)
		}
		if uuid == "" {
			return fmt.Errorf("the root filesystem at %s reported no UUID", s.RootDev)
		}
		s.RootUUID = uuid
	}

	cmdline := KernelCmdline(s.Cfg, s.LUKSUUID, s.RootUUID)

	loaderDir := s.path("boot/loader")
	if err := os.MkdirAll(filepath.Join(loaderDir, "entries"), 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", loaderDir, err)
	}
	if err := os.WriteFile(filepath.Join(loaderDir, "loader.conf"), []byte(loaderConf), 0o644); err != nil {
		return fmt.Errorf("writing loader.conf: %w", err)
	}

	entry := BootEntry(s.Cfg, kernel, initrd, cmdline)
	if err := os.WriteFile(filepath.Join(loaderDir, "entries", "cairn.conf"), []byte(entry), 0o644); err != nil {
		return fmt.Errorf("writing the boot entry: %w", err)
	}

	s.logf("boot entry: %s", strings.TrimSpace(cmdline))
	return nil
}

// findKernelImages picks the kernel and initramfs out of the ESP.
//
// Detected rather than hardcoded to the `linux` package's names, so the target
// package list can switch kernels — linux-lts, linux-zen — without this
// silently writing an entry pointing at a file that is not there.
func findKernelImages(bootDir string) (kernel, initrd string, err error) {
	entries, err := os.ReadDir(bootDir)
	if err != nil {
		return "", "", fmt.Errorf("reading %s: %w", bootDir, err)
	}

	var kernels, initrds []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		switch {
		case strings.HasPrefix(name, "vmlinuz-"):
			kernels = append(kernels, name)
		case strings.HasPrefix(name, "initramfs-") && strings.HasSuffix(name, ".img") &&
			!strings.Contains(name, "fallback"):
			initrds = append(initrds, name)
		}
	}

	sort.Strings(kernels)
	sort.Strings(initrds)
	if len(kernels) == 0 || len(initrds) == 0 {
		return "", "", fmt.Errorf("no kernel and initramfs pair found in %s", bootDir)
	}
	return kernels[0], initrds[0], nil
}
