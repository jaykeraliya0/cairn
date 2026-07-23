# shellcheck shell=bash
# Disk partitioning helpers for the Cairn installer.
# Source this file; do not execute it directly.

# Single source of truth for the Cairn Btrfs subvolume layout: the root
# subvolume "@" must stay first (its mountpoint, /mnt, must exist before the
# others' mountpoint directories can be created inside it). Both arrays are
# index-aligned — CAIRN_SUBVOLUME_MOUNTPOINTS[i] is where CAIRN_SUBVOLUMES[i]
# gets mounted.
CAIRN_SUBVOLUMES=(@ @home @log @cache @snapshots)
CAIRN_SUBVOLUME_MOUNTPOINTS=(/mnt /mnt/home /mnt/var/log /mnt/var/cache /mnt/.snapshots)

# Wipe the target disk and lay down the Cairn partition scheme:
# a 1GiB EFI System Partition and a Btrfs partition using the rest of the disk.
# Usage: cairn::partition_disk /dev/sdX
cairn::partition_disk() {
  local disk="$1"

  sgdisk --zap-all "${disk}"
  sgdisk -n1:0:+1G -t1:ef00 -c1:"EFI System" "${disk}"
  sgdisk -n2:0:0 -t2:8300 -c2:"Cairn Root" "${disk}"

  partprobe "${disk}"
  udevadm settle --timeout=10 2>/dev/null || true

  # Fallback: wait for the disk's first partition node to actually appear,
  # in case udevadm settle wasn't enough (or udevadm isn't available).
  local waited=0
  while [[ ! -e "${disk}1" && ! -e "${disk}p1" && ${waited} -lt 10 ]]; do
    sleep 0.5
    waited=$((waited + 1))
  done
}

# Derive the EFI/root partition device paths for a disk, accounting for the
# nvmeXnY/mmcblkX "pN" partition-numbering convention.
# Usage: cairn::resolve_partition_names /dev/sdX efi_out_var root_out_var
cairn::resolve_partition_names() {
  local disk="$1"
  local efi_var="$2"
  local root_var="$3"

  if [[ "${disk}" =~ nvme|mmcblk ]]; then
    printf -v "$efi_var" '%s' "${disk}p1"
    printf -v "$root_var" '%s' "${disk}p2"
  else
    printf -v "$efi_var" '%s' "${disk}1"
    printf -v "$root_var" '%s' "${disk}2"
  fi
}

# Format the EFI partition as FAT32 and the root partition as Btrfs.
# Usage: cairn::format_partitions /dev/sdX1 /dev/sdX2
cairn::format_partitions() {
  local efi_part="$1"
  local root_part="$2"

  cairn::log "Formatting EFI partition..."
  mkfs.fat -F32 -n EFI "${efi_part}"

  cairn::log "Formatting root partition as Btrfs..."
  mkfs.btrfs -f -L cairn_root "${root_part}"
}

# Create the Cairn Btrfs subvolume layout on an already-formatted root
# partition (mounts it at /mnt temporarily, then unmounts).
# Usage: cairn::create_subvolumes /dev/sdX2
cairn::create_subvolumes() {
  local root_part="$1"

  mount "${root_part}" /mnt

  local subvol
  for subvol in "${CAIRN_SUBVOLUMES[@]}"; do
    btrfs subvolume create "/mnt/${subvol}"
  done

  umount /mnt
}

# Mount the root subvolume plus @home/@log/@cache/@snapshots and the EFI
# partition under /mnt, using the given Btrfs mount options for every
# subvolume mount.
# Usage: cairn::mount_subvolumes /dev/sdX2 /dev/sdX1 "noatime,compress=zstd"
cairn::mount_subvolumes() {
  local root_part="$1"
  local efi_part="$2"
  local mount_opts="$3"

  local i subvol target
  for i in "${!CAIRN_SUBVOLUMES[@]}"; do
    subvol="${CAIRN_SUBVOLUMES[$i]}"
    target="${CAIRN_SUBVOLUME_MOUNTPOINTS[$i]}"
    mkdir -p "${target}"
    mount -o "${mount_opts},subvol=${subvol}" "${root_part}" "${target}"
  done

  mkdir -p /mnt/boot
  mount "${efi_part}" /mnt/boot
}
