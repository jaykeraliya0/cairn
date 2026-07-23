#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROFILE_DIR="${SCRIPT_DIR}/profile"
WORK_DIR="${SCRIPT_DIR}/work"
OUT_DIR="${SCRIPT_DIR}/out"

if [[ ! -f "${PROFILE_DIR}/profiledef.sh" ]]; then
  echo "error: ${PROFILE_DIR}/profiledef.sh not found." >&2
  exit 1
fi

if [[ ${EUID} -ne 0 ]]; then
  echo "error: must run as root (mkarchiso needs loop mounts). Try: sudo ./build.sh" >&2
  exit 1
fi

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