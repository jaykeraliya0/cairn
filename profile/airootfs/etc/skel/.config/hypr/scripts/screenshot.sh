#!/usr/bin/env bash
# Screenshot helper: grim -> save to file + copy to clipboard.
# Usage: screenshot.sh region|screen|copy

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
copy)
    # Clipboard only -- nothing written to disk.
    geom=$(slurp) || exit 0
    grim -g "$geom" - | wl-copy
    ;;
*)
    echo "usage: $0 region|screen|copy" >&2
    exit 1
    ;;
esac
