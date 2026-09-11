# shellcheck shell=bash
# Target-system installation helpers for the Cairn installer: base install,
# fstab, chroot configuration, and bootloader setup. Runs after the target
# disk has been partitioned, formatted, and mounted at /mnt.
# Source this file; do not execute it directly.

# pacstrap the packages listed in a target package list file into /mnt.
# Usage: cairn::install_base_system /path/to/target-packages.x86_64
cairn::install_base_system() {
  local packages_file="$1"

  if [[ ! -f "${packages_file}" ]]; then
    cairn::die "Target package list not found: ${packages_file}"
  fi

  local target_packages
  mapfile -t target_packages < <(cairn::read_package_list "${packages_file}")

  pacstrap -K /mnt "${target_packages[@]}"
}

# Generate /mnt/etc/fstab from the current mounts under /mnt, keyed by UUID
# rather than device path.
cairn::generate_fstab() {
  genfstab -U /mnt >> /mnt/etc/fstab
}

# Create a Btrfs-native swapfile at /mnt/swapfile and register it in fstab.
# No-op if swap_size is empty (user opted out of swap).
# Usage: cairn::create_swapfile "4G"
cairn::create_swapfile() {
  local swap_size="$1"

  if [[ -z "${swap_size}" ]]; then
    return 0
  fi

  btrfs filesystem mkswapfile --size "${swap_size}" /mnt/swapfile
  echo "/swapfile none swap defaults 0 0" >> /mnt/etc/fstab
}

# Copy a skeleton directory from the live environment into the target system.
# `useradd -m` runs inside the chroot, so it copies the *target's* /etc/skel —
# which pacstrap creates pristine from the `filesystem` package. Without this
# step the Cairn dotfiles shipped in the ISO's /etc/skel never reach the
# installed system and the new user lands on a stock Hyprland desktop.
# A missing source is not fatal: the install still produces a working system,
# just with upstream defaults.
# Usage: cairn::install_skel /etc/skel /mnt/etc/skel
cairn::install_skel() {
  local src="$1"
  local dest="$2"

  if [[ ! -d "${src}" ]]; then
    cairn::log "No skeleton directory at ${src}; the target user will get the default /etc/skel."
    return 0
  fi

  mkdir -p "${dest}"
  # The trailing /. copies the contents (dotfiles included) rather than the
  # directory itself, and -a preserves modes — so the 0755 that
  # profiledef.sh's file_permissions gives the helper scripts in the ISO
  # carries through to the installed system.
  cp -a "${src}/." "${dest}/"
}

# Configure the freshly pacstrapped system inside an arch-chroot: timezone,
# locale, hostname, root/user accounts, wheel sudo access, core services, and
# the systemd-boot binary.
#
# The heredoc below is unquoted, so the password arguments are expanded into
# the generated chroot command in plaintext before being piped to chpasswd —
# same pattern the Arch install guide itself uses. Acceptable for this stage;
# revisit with a non-echoing method if this needs hardening against local
# process-listing/history exposure.
#
# The heredoc delimiter is quoted (<<'CHROOT_EOF') so the outer shell performs
# no substitution on the script body, preventing shell injection via a
# malicious hostname/username; the four values are instead passed through
# exported environment variables that the inner chroot bash expands safely.
# Usage: cairn::configure_chroot hostname root_password username user_password
cairn::configure_chroot() {
  local target_hostname="$1"
  local target_root_password="$2"
  local target_username="$3"
  local target_user_password="$4"

  export CAIRN_TARGET_HOSTNAME="${target_hostname}"
  export CAIRN_TARGET_ROOT_PASSWORD="${target_root_password}"
  export CAIRN_TARGET_USERNAME="${target_username}"
  export CAIRN_TARGET_USER_PASSWORD="${target_user_password}"

  arch-chroot /mnt /bin/bash <<'CHROOT_EOF'
set -euo pipefail

# Timezone (hardcoded per project decision: default, not prompted)
ln -sf /usr/share/zoneinfo/UTC /etc/localtime
hwclock --systohc

# Locale
echo "en_US.UTF-8 UTF-8" >> /etc/locale.gen
locale-gen
echo "LANG=en_US.UTF-8" > /etc/locale.conf

# Hostname
echo "${CAIRN_TARGET_HOSTNAME}" > /etc/hostname
cat >> /etc/hosts <<HOSTS_EOF
127.0.0.1   localhost
::1         localhost
127.0.1.1   ${CAIRN_TARGET_HOSTNAME}.localdomain ${CAIRN_TARGET_HOSTNAME}
HOSTS_EOF

# Root password
echo "root:${CAIRN_TARGET_ROOT_PASSWORD}" | chpasswd

# User account
useradd -m -G wheel -s /bin/bash "${CAIRN_TARGET_USERNAME}"
echo "${CAIRN_TARGET_USERNAME}:${CAIRN_TARGET_USER_PASSWORD}" | chpasswd

# Sudo for wheel group
sed -i 's/^# %wheel ALL=(ALL:ALL) ALL/%wheel ALL=(ALL:ALL) ALL/' /etc/sudoers

# Enable core services
systemctl enable iwd.service
systemctl enable NetworkManager.service
systemctl enable sddm.service

# Bootloader
bootctl install
CHROOT_EOF

  unset CAIRN_TARGET_HOSTNAME CAIRN_TARGET_ROOT_PASSWORD CAIRN_TARGET_USERNAME CAIRN_TARGET_USER_PASSWORD
}

# Write systemd-boot's loader.conf and the Cairn boot entry, pointing root=
# at the given root partition's UUID with the @ subvolume selected. Detects
# the actual installed kernel/initramfs filenames under /mnt/boot rather than
# assuming the `linux` package's names, so it stays correct if the target
# package list ever switches kernels (linux-lts, linux-zen, etc.).
# Usage: cairn::write_bootloader_config /dev/sdX2
cairn::write_bootloader_config() {
  local root_part="$1"
  local root_uuid
  root_uuid="$(blkid -s UUID -o value "${root_part}")"

  local kernel_image initrd_image
  kernel_image="$(find /mnt/boot -maxdepth 1 -name 'vmlinuz-*' -printf '%f\n' | sort | head -n1)"
  initrd_image="$(find /mnt/boot -maxdepth 1 -name 'initramfs-*.img' ! -name '*fallback*' -printf '%f\n' | sort | head -n1)"

  if [[ -z "${kernel_image}" || -z "${initrd_image}" ]]; then
    cairn::die "Could not find a kernel/initramfs image under /mnt/boot"
  fi

  cat > /mnt/boot/loader/loader.conf <<EOF
default cairn.conf
timeout 3
console-mode max
editor no
EOF

  cat > /mnt/boot/loader/entries/cairn.conf <<EOF
title   Cairn Linux
linux   /${kernel_image}
initrd  /${initrd_image}
options root=UUID=${root_uuid} rootflags=subvol=@ rw
EOF
}

# Recursively unmount everything under /mnt. Safe to call even if nothing is
# mounted there (e.g. from an error-cleanup path that may run before any
# mount happened).
cairn::unmount_target() {
  if mountpoint -q /mnt; then
    umount -R /mnt
  fi
}
