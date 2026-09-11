#!/usr/bin/env bash
# Screenshot helper: grim -> save to file + copy to clipboard.
# Usage: screenshot.sh region|screen

set -euo pipefail

OUTDIR="$HOME/Pictures/Screenshots"
mkdir -p "$OUTDIR"
OUTFILE="$OUTDIR/screenshot-$(date +%Y%m%d-%H%M%S).png"

case "${1:-region}" in
region)
    geom=$(slurp) || exit 0
    grim -g "$geom" "$OUTFILE"
    wl-copy < "$OUTFILE"
    ;;
screen)
    grim "$OUTFILE"
    wl-copy < "$OUTFILE"
    ;;
*)
    echo "usage: $0 region|screen" >&2
    exit 1
    ;;
esac
