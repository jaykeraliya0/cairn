#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROFILE_DIR="${SCRIPT_DIR}/profile"
WORK_DIR="${SCRIPT_DIR}/work"
OUT_DIR="${SCRIPT_DIR}/out"

# Strip comments and blank lines from a package list and trim whitespace.
# Kept in step with cairn::read_package_list in scripts/lib/common.sh; build.sh
# stays free of the installer libs, so the expression lives in both places.
read_package_list() {
  sed '/^[[:blank:]]*#/d; s/#.*//; s/^[[:blank:]]*//; s/[[:blank:]]*$//; /^$/d' "$1"
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

if [[ ! -f "${PROFILE_DIR}/profiledef.sh" ]]; then
  echo "error: ${PROFILE_DIR}/profiledef.sh not found." >&2
  exit 1
fi

if [[ ${EUID} -ne 0 ]]; then
  echo "error: must run as root (mkarchiso needs loop mounts). Try: sudo ./build.sh" >&2
  exit 1
fi

check_package_list "${PROFILE_DIR}/packages.x86_64"
# The target list is pacstrapped later, on the user's machine, by the installer
# on the finished ISO — so a stale name there fails far from here. Check it now.
check_package_list "${SCRIPT_DIR}/scripts/installer/target-packages.x86_64"

rm -rf "${WORK_DIR}"
mkdir -p "${OUT_DIR}"

# scripts/ is the source of truth for the installer; stage a fresh copy into
# the airootfs overlay so the ISO never ships stale installer code. The
# installer/ and lib/ layout is copied as-is so relative sourcing inside
# install.sh resolves the same way here as it does in scripts/.
INSTALLER_DEST="${PROFILE_DIR}/airootfs/root/cairn-installer"
rm -rf "${INSTALLER_DEST}"
mkdir -p "${INSTALLER_DEST}"
cp -r "${SCRIPT_DIR}/scripts/installer" "${SCRIPT_DIR}/scripts/lib" "${INSTALLER_DEST}/"
chmod 755 "${INSTALLER_DEST}/installer/install.sh"

mkarchiso -v -w "${WORK_DIR}" -o "${OUT_DIR}" "${PROFILE_DIR}"

echo "done. ISO written to ${OUT_DIR}/"