#!/usr/bin/env bats

setup() {
  source "${BATS_TEST_DIRNAME}/../scripts/lib/partition.sh"
}

@test "cairn::resolve_partition_names appends digits directly for sata-style disks" {
  cairn::resolve_partition_names /dev/sda efi root
  [ "$efi" = "/dev/sda1" ]
  [ "$root" = "/dev/sda2" ]
}

@test "cairn::resolve_partition_names inserts p before digits for nvme-style disks" {
  cairn::resolve_partition_names /dev/nvme0n1 efi root
  [ "$efi" = "/dev/nvme0n1p1" ]
  [ "$root" = "/dev/nvme0n1p2" ]
}

@test "cairn::resolve_partition_names inserts p before digits for mmcblk-style disks" {
  cairn::resolve_partition_names /dev/mmcblk0 efi root
  [ "$efi" = "/dev/mmcblk0p1" ]
  [ "$root" = "/dev/mmcblk0p2" ]
}
