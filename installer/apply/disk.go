package apply

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/jaykeraliya0/cairn/installer/sys"
)

// GPT partition type codes, as sgdisk spells them.
const (
	typeEFISystem = "ef00"
	typeLinuxFS   = "8300"
	typeLinuxLUKS = "8309"
)

// partitionDisk wipes the target disk and lays down Cairn's two partitions: an
// EFI system partition, then one partition taking the whole remainder.
func partitionDisk(ctx context.Context, s *State) error {
	disk := s.Cfg.Disk

	rootType := typeLinuxFS
	if s.Cfg.Encrypt {
		rootType = typeLinuxLUKS
	}

	if err := s.run(ctx, "sgdisk", "--zap-all", disk); err != nil {
		return err
	}
	if err := s.run(ctx, "sgdisk",
		fmt.Sprintf("-n1:0:+%dM", s.Cfg.ESPSizeMiB),
		"-t1:"+typeEFISystem,
		"-c1:EFI System",
		disk); err != nil {
		return err
	}
	if err := s.run(ctx, "sgdisk",
		"-n2:0:0",
		"-t2:"+rootType,
		"-c2:Cairn Root",
		disk); err != nil {
		return err
	}

	if err := s.run(ctx, "partprobe", disk); err != nil {
		return err
	}
	// udevadm may be absent or time out; the poll below is the real guarantee.
	if err := s.run(ctx, "udevadm", "settle", "--timeout=10"); err != nil {
		s.logf("note: udevadm settle failed (%v); waiting for the partition nodes directly", err)
	}

	s.ESPPart = sys.PartitionName(disk, 1)
	s.RootPart = sys.PartitionName(disk, 2)
	s.RootDev = s.RootPart

	for _, part := range []string{s.ESPPart, s.RootPart} {
		if err := s.waitFor(ctx, part); err != nil {
			return err
		}
	}

	// A disk that has been installed to before still carries the old
	// filesystems' signatures. sgdisk rewrites only the partition table, and
	// the new partitions start exactly where the old ones did, so whatever was
	// in them is still there. mkfs cannot be relied on to erase all of it: a
	// LUKS header left by an earlier encrypted install can survive mkfs.btrfs,
	// and libblkid then probes the partition as crypto_LUKS — mount refuses it,
	// and blkid can hand genfstab and the boot entry the wrong UUID. Erase every
	// signature libblkid knows about before anything new is created.
	for _, part := range []string{s.ESPPart, s.RootPart} {
		if err := s.run(ctx, "wipefs", "--all", part); err != nil {
			return err
		}
	}
	return nil
}

// setupEncryption turns partition 2 into a LUKS2 container and opens it.
//
// The passphrase goes in over stdin with --key-file -, never as an argument:
// an argv is world-readable through /proc for as long as the process lives.
func setupEncryption(ctx context.Context, s *State) error {
	format := sys.Cmd{
		Name: "cryptsetup",
		Args: []string{
			"luksFormat",
			"--type", "luks2",
			"--batch-mode",
			"--key-file", "-",
			s.RootPart,
		},
		Stdin: strings.NewReader(s.Cfg.LUKSPassphrase),
	}
	if err := s.Runner.Run(ctx, format); err != nil {
		return fmt.Errorf("creating the encrypted container: %w", err)
	}

	open := sys.Cmd{
		Name:  "cryptsetup",
		Args:  []string{"open", "--key-file", "-", s.RootPart, MapperName},
		Stdin: strings.NewReader(s.Cfg.LUKSPassphrase),
	}
	if err := s.Runner.Run(ctx, open); err != nil {
		return fmt.Errorf("unlocking the encrypted container: %w", err)
	}
	s.luksOpen = true
	s.RootDev = "/dev/mapper/" + MapperName

	// Captured now rather than at boot-entry time: this is the UUID of the
	// container, which stops existing under that name once it is open.
	uuid, err := s.Runner.Output(ctx, "blkid", "-s", "UUID", "-o", "value", s.RootPart)
	if err != nil {
		return fmt.Errorf("reading the encrypted volume's UUID: %w", err)
	}
	if uuid == "" {
		return fmt.Errorf("the encrypted volume at %s reported no UUID", s.RootPart)
	}
	s.LUKSUUID = uuid
	return nil
}

// formatPartitions puts FAT32 on the ESP and Btrfs on the root device.
func formatPartitions(ctx context.Context, s *State) error {
	if err := s.run(ctx, "mkfs.fat", "-F32", "-n", "EFI", s.ESPPart); err != nil {
		return err
	}
	return s.run(ctx, "mkfs.btrfs", "-f", "-L", "cairn_root", s.RootDev)
}

// createSubvolumes mounts the bare Btrfs filesystem, creates the subvolume
// layout on it, and unmounts again. Nothing is mounted at the layout's final
// positions yet — that is mountTarget's job.
func createSubvolumes(ctx context.Context, s *State) error {
	if err := os.MkdirAll(s.Root, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", s.Root, err)
	}
	// Every mount names its filesystem type rather than letting mount probe
	// for one. After the wipe above the probe should agree, but if anything on
	// the partition still looks like another filesystem, this fails loudly
	// instead of mounting the wrong thing.
	if err := s.run(ctx, "mount", "-t", "btrfs", s.RootDev, s.Root); err != nil {
		return err
	}
	s.mounted = true

	for _, subvol := range Subvolumes {
		if err := s.run(ctx, "btrfs", "subvolume", "create", s.path(subvol.Name)); err != nil {
			return err
		}
	}

	if err := s.run(ctx, "umount", s.Root); err != nil {
		return err
	}
	s.mounted = false
	return nil
}

// mountTarget mounts every subvolume at its place under the target root, then
// the ESP at /boot.
func mountTarget(ctx context.Context, s *State) error {
	opts := s.Cfg.MountOptions()

	for _, subvol := range Subvolumes {
		target := s.path(subvol.Path)
		if err := os.MkdirAll(target, 0o755); err != nil {
			return fmt.Errorf("creating %s: %w", target, err)
		}
		if err := s.run(ctx, "mount", "-t", "btrfs", "-o", opts+",subvol="+subvol.Name, s.RootDev, target); err != nil {
			return err
		}
		s.mounted = true
	}

	boot := s.path("boot")
	if err := os.MkdirAll(boot, 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", boot, err)
	}
	return s.run(ctx, "mount", "-t", "vfat", s.ESPPart, boot)
}

// unmountTarget tears the target down cleanly at the end of a successful
// install, so the user can reboot straight into it.
func unmountTarget(ctx context.Context, s *State) error {
	if err := s.run(ctx, "umount", "-R", s.Root); err != nil {
		return err
	}
	s.mounted = false

	if s.luksOpen {
		if err := s.run(ctx, "cryptsetup", "close", MapperName); err != nil {
			return err
		}
		s.luksOpen = false
	}
	return nil
}
