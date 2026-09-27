#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROFILE_DIR="${SCRIPT_DIR}/profile"
WORK_DIR="${SCRIPT_DIR}/work"
OUT_DIR="${SCRIPT_DIR}/out"

# Strip comments and blank lines from a package list and trim whitespace.
# Kept in step with parse() in installer/packages/embed.go, which does the same
# thing to the same files at compile time.
read_package_list() {
  sed '/^[[:blank:]]*#/d; s/#.*//; s/^[[:blank:]]*//; s/[[:blank:]]*$//; /^$/d' "$1"
}

# Phase timings. `phase <name>` closes the phase before it and opens a new one;
# an empty name just closes the last. They are printed when the build finishes
# and left in out/build-timings.tsv, which CI turns into a table — so an
# argument about what to optimise is settled with numbers.
PHASE_TIMINGS=()
_phase_name=""
_phase_start=0

phase() {
  if [[ -n "${_phase_name}" ]]; then
    PHASE_TIMINGS+=("${_phase_name}"$'\t'"$((SECONDS - _phase_start))")
  fi
  _phase_name="$1"
  _phase_start="${SECONDS}"
}

report_timings() {
  phase ""
  PHASE_TIMINGS+=("total"$'\t'"${SECONDS}")
  printf '%s\n' "${PHASE_TIMINGS[@]}" >"${OUT_DIR}/build-timings.tsv"
  echo "==> build phases"
  printf '%s\n' "${PHASE_TIMINGS[@]}" |
    awk -F'\t' '{ printf "    %-14s %3dm%02ds\n", $1, $2 / 60, $2 % 60 }'
}

# Print the names among "$@" that pacman cannot resolve, one per line. Uses the
# profile's own pacman.conf so the repo set matches what the build will use.
# An optional first argument selects an alternate sync database directory.
unresolvable_packages() {
  local dbpath="$1"
  shift

  local pacman_args=(-Si --config "${PROFILE_DIR}/pacman.conf")
  [[ -n "${dbpath}" ]] && pacman_args+=(--dbpath "${dbpath}")

  # pacman exits non-zero when any name is missing; that is the expected case
  # here, so swallow the status and read the names off stderr instead.
  { pacman "${pacman_args[@]}" -- "$@" 2>&1 >/dev/null || true; } |
    sed -n "s/^error: package '\(.*\)' was not found$/\1/p"
}

# Fail before mkarchiso does. Otherwise a package that has left the repos is
# only discovered after options are validated, the airootfs is copied and the
# package databases are synced — and pacstrap reports just the first bad name.
check_package_list() {
  local list_file="$1"
  local packages=()
  mapfile -t packages < <(read_package_list "${list_file}")

  if [[ ${#packages[@]} -eq 0 ]]; then
    echo "error: no packages listed in ${list_file}" >&2
    exit 1
  fi

  local missing
  missing="$(unresolvable_packages "" "${packages[@]}")"
  [[ -n "${missing}" ]] || return 0

  # The host's sync databases may just predate a newly added package, which
  # would be a false alarm. Confirm against a freshly synced throwaway database
  # before failing. The host's own databases are deliberately left untouched:
  # a bare `pacman -Sy` on the build host invites partial upgrades.
  echo "note: ${missing//$'\n'/ } not in the host's package databases; re-checking against a fresh sync..." >&2

  local tmp_db
  tmp_db="$(mktemp -d)"
  # shellcheck disable=SC2064  # expand tmp_db now, not at trap time
  trap "rm -rf '${tmp_db}'" RETURN

  if ! pacman -Sy --config "${PROFILE_DIR}/pacman.conf" --dbpath "${tmp_db}" \
      --logfile /dev/null --noconfirm >/dev/null 2>&1; then
    echo "warning: could not refresh package databases; skipping the check for ${list_file}" >&2
    return 0
  fi

  missing="$(unresolvable_packages "${tmp_db}" "${packages[@]}")"
  [[ -n "${missing}" ]] || return 0

  local missing_list=()
  mapfile -t missing_list <<< "${missing}"
  {
    echo "error: ${list_file} lists packages that are in no configured repository:"
    printf '  - %s\n' "${missing_list[@]}"
    echo "Remove or replace them, then re-run. See the header of profile/packages.x86_64"
    echo "for how that list is kept in sync with archiso's releng profile."
  } >&2
  exit 1
}

# --check-packages resolves the three package lists and stops. It is the cheap
# half of a build, it needs neither root nor go, and CI runs it as a gate: a
# package that has left the repos then fails in a minute rather than forty.
CHECK_ONLY=0
case "${1:-}" in
  --check-packages) CHECK_ONLY=1 ;;
  "") ;;
  *)
    echo "usage: ${0##*/} [--check-packages]" >&2
    exit 1
    ;;
esac

if [[ ! -f "${PROFILE_DIR}/profiledef.sh" ]]; then
  echo "error: ${PROFILE_DIR}/profiledef.sh not found." >&2
  exit 1
fi

if [[ ${CHECK_ONLY} -eq 0 ]]; then
  # The installer is a Go program, so the toolchain is a build dependency now.
  if ! command -v go >/dev/null 2>&1; then
    echo "error: go not found, but the installer is written in Go." >&2
    echo "Install it with: pacman -S go" >&2
    exit 1
  fi

  if [[ ${EUID} -ne 0 ]]; then
    echo "error: must run as root (mkarchiso needs loop mounts). Try: sudo ./build.sh" >&2
    exit 1
  fi
fi

phase preflight
check_package_list "${PROFILE_DIR}/packages.x86_64"
# The system image and the Nvidia side repository are built from these next.
# Check them first, so a package that has left the repos fails here rather than
# minutes into pacstrap.
check_package_list "${SCRIPT_DIR}/installer/packages/target-packages.x86_64"
check_package_list "${SCRIPT_DIR}/installer/packages/nvidia-packages.x86_64"

if [[ ${CHECK_ONLY} -eq 1 ]]; then
  echo "all three package lists resolve."
  exit 0
fi

rm -rf "${WORK_DIR}"
mkdir -p "${OUT_DIR}"

# ── the system image ──────────────────────────────────────────────────────────
# The installed system is built here, once, not on the user's machine. The
# target package set is pacstrapped into a directory and packed into a squashfs
# image that ships on the ISO; the installer extracts that image onto the disk
# and then does only the per-machine setup. It is how Manjaro, EndeavourOS's
# offline mode, Fedora and Ubuntu install: no package manager runs during the
# install, so nothing is downloaded, verified or cached on the target, and every
# install of a given ISO is the same system.
#
# Nvidia's drivers are the one thing kept out of the image — 2.4 GiB installed,
# needed by a minority of machines — and ship in a small side repository that
# the installer uses only when an Nvidia driver was chosen.
CAIRN_SHARE="${PROFILE_DIR}/airootfs/usr/share/cairn"
SYSTEM_IMAGE="${CAIRN_SHARE}/cairn-root.sfs"
NVIDIA_REPO="${CAIRN_SHARE}/nvidia"
TARGET_ROOT="${WORK_DIR}/target-root"
# Survives a rebuild, so only what changed upstream is downloaded again.
PACKAGE_CACHE="${SCRIPT_DIR}/.cache/offline-packages"

# The profile's pacman.conf with the persistent cache added. Every pacman and
# pacstrap call below uses it, so they all share one download cache.
BUILD_PACMAN_CONF="$(mktemp)"
trap 'rm -f "${BUILD_PACMAN_CONF}"' EXIT
sed "/^\[options\]/a CacheDir = ${PACKAGE_CACHE}/" "${PROFILE_DIR}/pacman.conf" >"${BUILD_PACMAN_CONF}"

build_system_image() {
  local packages=()
  mapfile -t packages < <(read_package_list "${SCRIPT_DIR}/installer/packages/target-packages.x86_64")

  echo "==> installing ${#packages[@]} packages into the system image"
  mkdir -p "${TARGET_ROOT}" "${PACKAGE_CACHE}"
  # -c  use the config's CacheDir, so package archives land in the build cache
  #     and not in the image's /var/cache/pacman/pkg
  # -G  copy no keyring in: every installed system must generate its own
  # -M  keep this machine's mirrorlist out: the image gets its own, below
  pacstrap -C "${BUILD_PACMAN_CONF}" -c -G -M "${TARGET_ROOT}" "${packages[@]}"

  echo "==> preparing the image"

  # Cairn's desktop defaults. Baked in here, so the installed system's
  # /etc/skel is already Cairn's and `useradd -m` copies it straight into the
  # new user's home.
  cp -a "${PROFILE_DIR}/airootfs/etc/skel/." "${TARGET_ROOT}/etc/skel/"
  chown -R 0:0 "${TARGET_ROOT}/etc/skel"
  # git does not reliably carry the exec bit, and these are run by keybinds.
  find "${TARGET_ROOT}/etc/skel" -type f -path '*/scripts/*.sh' -exec chmod 0755 {} +

  # NetworkManager's Wi-Fi backend. The chroot step enables iwd.service next to
  # NetworkManager.service, and NetworkManager defaults to wpa_supplicant --
  # which leaves two supplicants contending for the same radio. This makes iwd
  # NetworkManager's backend instead, so there is one manager of the device and
  # impala, which talks to iwd directly, agrees with it. iwd's own
  # EnableNetworkConfiguration already defaults to off, which is what
  # NetworkManager needs, so iwd itself has nothing to configure here.
  mkdir -p "${TARGET_ROOT}/etc/NetworkManager/conf.d"
  cat >"${TARGET_ROOT}/etc/NetworkManager/conf.d/wifi_backend.conf" <<'EOF'
[device]
wifi.backend=iwd
EOF

  # A mirrorlist that works the first time the user runs `pacman -Syu`. The
  # stock one from pacman-mirrorlist has every server commented out.
  cat >"${TARGET_ROOT}/etc/pacman.d/mirrorlist" <<'EOF'
# Arch Linux's GeoDNS mirror, which routes to a nearby mirror by itself.
# Replace this with a hand-picked list, or generate one with reflector.
Server = https://geo.mirror.pkgbuild.com/$repo/os/$arch
EOF

  # Anything unique to one machine must not be in an image every machine gets.
  # The installer creates each of these fresh, on the target, per install.
  : >"${TARGET_ROOT}/etc/machine-id"                  # systemd-machine-id-setup
  rm -rf "${TARGET_ROOT}/etc/pacman.d/gnupg"          # pacman-key --init
  rm -f "${TARGET_ROOT}/var/lib/systemd/random-seed"

  # Built on this machine, for this machine's hardware; the installer builds
  # the real one with the target's mkinitcpio configuration.
  rm -f "${TARGET_ROOT}"/boot/initramfs-*.img
  rm -f "${TARGET_ROOT}/var/log/pacman.log"
}

build_nvidia_repo() {
  local db="${TARGET_ROOT}/var/lib/pacman"
  # The two flavours conflict, so each is resolved on its own. Keep in step
  # with NvidiaPackages in installer/sys/gpu.go.
  local flavours=("nvidia-open nvidia-utils egl-wayland"
    "nvidia-open-dkms nvidia-utils egl-wayland linux-headers")
  local wanted=() found=() names=() flavour file

  rm -rf "${NVIDIA_REPO}"
  mkdir -p "${NVIDIA_REPO}"

  for flavour in "${flavours[@]}"; do
    read -r -a names <<<"${flavour}"
    # Resolved against the image's own package database, so only what the
    # image lacks is downloaded and listed.
    pacman -Sw --config "${BUILD_PACMAN_CONF}" --dbpath "${db}" --root "${TARGET_ROOT}" \
      --logfile /dev/null --noconfirm -- "${names[@]}" >/dev/null
    mapfile -t found < <(
      pacman -Sp --config "${BUILD_PACMAN_CONF}" --dbpath "${db}" --root "${TARGET_ROOT}" \
        --print-format '%f' -- "${names[@]}"
    )
    wanted+=("${found[@]}")
  done
  mapfile -t wanted < <(printf '%s\n' "${wanted[@]}" | sort -u)

  for file in "${wanted[@]}"; do
    # The installer verifies every package against its Arch signature, and a
    # package without one fails there — on a user's machine, with no network
    # to fetch it. Fail the build instead.
    if [[ ! -f "${PACKAGE_CACHE}/${file}.sig" ]]; then
      echo "error: ${file} has no signature in ${PACKAGE_CACHE}; the offline install could not verify it." >&2
      exit 1
    fi
    for part in "${file}" "${file}.sig"; do
      # Hardlink where the cache and the profile share a filesystem, so the
      # packages are not stored twice; fall back to a copy when they do not.
      ln "${PACKAGE_CACHE}/${part}" "${NVIDIA_REPO}/${part}" 2>/dev/null ||
        cp "${PACKAGE_CACHE}/${part}" "${NVIDIA_REPO}/${part}"
    done
  done

  # Pass the resolved package files, not a glob: *.pkg.tar.* also matches the
  # .sig beside every package, which repo-add rejects as "not a package file".
  local entries=("${wanted[@]/#/${NVIDIA_REPO}/}")
  if ! repo-add --quiet "${NVIDIA_REPO}/cairn-nvidia.db.tar.zst" "${entries[@]}" >/dev/null; then
    echo "error: repo-add failed to build the Nvidia repository." >&2
    exit 1
  fi

  cat >"${NVIDIA_REPO}/pacman.conf" <<'EOF'
# Used by the Cairn installer to install Nvidia's drivers with no network.
[options]
Architecture      = auto
SigLevel          = Required DatabaseOptional
LocalFileSigLevel = Optional
# The repository doubles as the package cache: pacman finds every package
# already in place and copies nothing into the new system.
CacheDir          = /usr/share/cairn/nvidia/

[cairn-nvidia]
# Packages keep their Arch signatures; only this locally built index is not
# signed.
SigLevel = PackageRequired DatabaseNever
Server   = file:///usr/share/cairn/nvidia
EOF

  echo "==> nvidia repository: ${#wanted[@]} packages, $(du -sh "${NVIDIA_REPO}" | cut -f1)"
}

pack_system_image() {
  # The sync databases were needed only to resolve the Nvidia repository.
  # Shipped, they would leave every install with a months-old package index
  # that invites partial upgrades; without them, the first thing pacman asks
  # for is a proper -Syu.
  rm -rf "${TARGET_ROOT}/var/lib/pacman/sync"
  rm -f "${TARGET_ROOT}/var/lib/pacman/db.lck"

  echo "==> packing the system image"
  mkdir -p "${CAIRN_SHARE}"
  rm -f "${SYSTEM_IMAGE}"
  mksquashfs "${TARGET_ROOT}" "${SYSTEM_IMAGE}" -noappend \
    -comp zstd -Xcompression-level 15 -b 1M -no-progress
  rm -rf "${TARGET_ROOT}"

  echo "==> system image: $(du -h "${SYSTEM_IMAGE}" | cut -f1)"
}

# Earlier builds shipped the whole package closure as a repository here.
rm -rf "${CAIRN_SHARE}/repo"
phase "system image"
build_system_image
phase "nvidia repo"
build_nvidia_repo
phase "pack image"
pack_system_image

# installer/ is the source of truth for the installer; compile it fresh into
# the airootfs overlay so the ISO never ships a stale binary.
phase installer
INSTALLER_BIN="${PROFILE_DIR}/airootfs/usr/local/bin/cairn-install"
mkdir -p "$(dirname "${INSTALLER_BIN}")"
rm -f "${INSTALLER_BIN}"

go_build=(go build -C "${SCRIPT_DIR}/installer" -trimpath -ldflags '-s -w' -o "${INSTALLER_BIN}" .)
if [[ -n "${SUDO_USER:-}" ]]; then
  # Build as the invoking user so the Go module cache lands in their home
  # rather than becoming root-owned under /root.
  sudo -u "${SUDO_USER}" -H "${go_build[@]}"
else
  "${go_build[@]}"
fi
# mkarchiso applies profiledef.sh's file_permissions to this path as well, but
# a correct mode here keeps the overlay directly usable.
chmod 755 "${INSTALLER_BIN}"

phase mkarchiso
mkarchiso -v -w "${WORK_DIR}" -o "${OUT_DIR}" "${PROFILE_DIR}"

report_timings
echo "done. ISO written to ${OUT_DIR}/"