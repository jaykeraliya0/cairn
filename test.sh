#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
OUT_DIR="${SCRIPT_DIR}/out"

ISO="$(find "${OUT_DIR}" -maxdepth 1 -name '*.iso' -print 2>/dev/null | sort | tail -n1)"

if [[ -z "${ISO}" ]]; then
  echo "error: no ISO in ${OUT_DIR}. Run ./build.sh first." >&2
  exit 1
fi

echo "booting (UEFI): ${ISO}"
run_archiso -u -i "${ISO}"