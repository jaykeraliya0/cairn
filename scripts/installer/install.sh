#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../lib/common.sh
source "${SCRIPT_DIR}/../lib/common.sh"
# shellcheck source=../lib/partition.sh
source "${SCRIPT_DIR}/../lib/partition.sh"
# shellcheck source=../lib/target.sh
source "${SCRIPT_DIR}/../lib/target.sh"

cairn::require_root

cairn::log "Detecting available disks..."

LIVE_BOOT_DISK=""
if boot_source="$(findmnt -no SOURCE /run/archiso/bootmnt 2>/dev/null)"; then
  boot_parent="$(lsblk -no PKNAME "${boot_source}" 2>/dev/null)"
  if [[ -n "${boot_parent}" ]]; then
    LIVE_BOOT_DISK="/dev/${boot_parent}"
    cairn::log "Excluding live boot device from selection: ${LIVE_BOOT_DISK}"
  fi
fi

mapfile -t disk_lines < <(lsblk -dpno NAME,SIZE,MODEL | grep -v '^/dev/loop')

if [[ -n "${LIVE_BOOT_DISK}" ]]; then
  filtered_disk_lines=()
  for disk_line in "${disk_lines[@]}"; do
    [[ "${disk_line}" == "${LIVE_BOOT_DISK} "* ]] || filtered_disk_lines+=("${disk_line}")
  done
  disk_lines=("${filtered_disk_lines[@]}")
fi

if [[ ${#disk_lines[@]} -eq 0 ]]; then
  cairn::die "No disks detected."
fi

echo
echo "Available disks:"
PS3="Select target disk (number): "
select disk_choice in "${disk_lines[@]}"; do
  if [[ -n "${disk_choice:-}" ]]; then
    TARGET_DISK="$(awk '{print $1}' <<< "$disk_choice")"
    break
  else
    echo "Invalid selection, try again."
  fi
done

cairn::log "Selected disk: ${TARGET_DISK}"

echo
echo "WARNING: This will ERASE ALL DATA on ${TARGET_DISK}."
read -rp "Type 'yes' to continue: " confirm_erase
if [[ "$confirm_erase" != "yes" ]]; then
  cairn::die "Aborted by user."
fi

read -rp "Hostname [cairn]: " TARGET_HOSTNAME
TARGET_HOSTNAME="${TARGET_HOSTNAME:-cairn}"
cairn::log "Hostname set to: ${TARGET_HOSTNAME}"

read -rp "Create swap? (y/n): " want_swap
TARGET_SWAP_SIZE=""
if [[ "$want_swap" =~ ^[Yy]$ ]]; then
  read -rp "Swap size (e.g. 4G): " TARGET_SWAP_SIZE
  cairn::log "Swap requested: ${TARGET_SWAP_SIZE}"
else
  cairn::log "No swap will be created."
fi

read -rp "Username for target system: " TARGET_USERNAME
if [[ -z "$TARGET_USERNAME" ]]; then
  cairn::die "Username cannot be empty."
fi
cairn::log "Username set to: ${TARGET_USERNAME}"

cairn::prompt_password_confirmed "User" TARGET_USER_PASSWORD
cairn::prompt_password_confirmed "Root" TARGET_ROOT_PASSWORD

read -rp "Enable Btrfs compression (zstd)? (y/n): " want_compression
if [[ "$want_compression" =~ ^[Yy]$ ]]; then
  BTRFS_MOUNT_OPTS="noatime,compress=zstd"
  cairn::log "Btrfs compression enabled (zstd)."
else
  BTRFS_MOUNT_OPTS="noatime"
  cairn::log "Btrfs compression disabled."
fi

echo
echo "=== Installation Summary ==="
echo "Target disk:     ${TARGET_DISK}"
echo "Hostname:        ${TARGET_HOSTNAME}"
echo "Swap:            ${TARGET_SWAP_SIZE:-none}"
echo "Username:        ${TARGET_USERNAME}"
echo "Compression:     $([[ "$want_compression" =~ ^[Yy]$ ]] && echo "enabled (zstd)" || echo "disabled")"
echo "============================"
echo

read -rp "Proceed with installation? (yes/no): " final_confirm
if [[ "$final_confirm" != "yes" ]]; then
  cairn::die "Aborted by user before making changes."
fi

cairn::install_cleanup() {
  local exit_code=$?
  if [[ ${exit_code} -ne 0 ]]; then
    cairn::log "Install failed (exit ${exit_code}); attempting to clean up mounts under /mnt..."
    cairn::unmount_target || cairn::log "Warning: cleanup unmount failed; /mnt may still be mounted, check manually before retrying."
  fi
}
trap cairn::install_cleanup EXIT

cairn::log "Partitioning ${TARGET_DISK}..."
cairn::partition_disk "${TARGET_DISK}"

cairn::resolve_partition_names "${TARGET_DISK}" EFI_PART ROOT_PART
cairn::log "EFI partition: ${EFI_PART}"
cairn::log "Root partition: ${ROOT_PART}"

cairn::log "Formatting partitions..."
cairn::format_partitions "${EFI_PART}" "${ROOT_PART}"

cairn::log "Creating Btrfs subvolumes..."
cairn::create_subvolumes "${ROOT_PART}"

cairn::log "Mounting subvolumes..."
cairn::mount_subvolumes "${ROOT_PART}" "${EFI_PART}" "${BTRFS_MOUNT_OPTS}"

cairn::log "Mount layout:"
findmnt /mnt

cairn::log "Installing base system to /mnt (this will take a while)..."
TARGET_PACKAGES_FILE="${SCRIPT_DIR}/target-packages.x86_64"
cairn::install_base_system "${TARGET_PACKAGES_FILE}"
cairn::log "Base system installed."

cairn::log "Generating fstab..."
cairn::generate_fstab

if [[ -n "${TARGET_SWAP_SIZE}" ]]; then
  cairn::log "Creating swapfile (${TARGET_SWAP_SIZE})..."
  cairn::create_swapfile "${TARGET_SWAP_SIZE}"
fi

cairn::log "Installing default user configuration into the target..."
cairn::install_skel /etc/skel /mnt/etc/skel

cairn::log "Configuring target system..."
cairn::configure_chroot "${TARGET_HOSTNAME}" "${TARGET_ROOT_PASSWORD}" "${TARGET_USERNAME}" "${TARGET_USER_PASSWORD}"
cairn::log "Chroot configuration complete."

cairn::log "Writing systemd-boot loader configuration..."
cairn::write_bootloader_config "${ROOT_PART}"
cairn::log "Bootloader configured."

cairn::log "Unmounting..."
cairn::unmount_target

cairn::log "Installation complete. You may now reboot."
