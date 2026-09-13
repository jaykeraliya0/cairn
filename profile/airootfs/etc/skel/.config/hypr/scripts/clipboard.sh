#!/usr/bin/env bash
# Clipboard helper: cliphist history behind a rofi picker.
# `store` is what the wl-paste watchers in hyprland.lua feed; `menu` is SUPER+V.
# Usage: clipboard.sh menu|store|wipe

set -euo pipefail

case "${1:-menu}" in
store)
    # Password managers tag their offer with this mime type. Skip those so
    # secrets never land in ~/.cache/cliphist/db.
    if wl-paste --list-types 2>/dev/null | grep -q 'x-kde-passwordManagerHint'; then
        exit 0
    fi
    cliphist store
    ;;
menu)
    # Loops so deleting an entry drops straight back into the list.
    while :; do
        rc=0
        sel=$(cliphist list | rofi -dmenu -i -p "clipboard" \
            -kb-custom-1 "Alt+Delete" \
            -mesg "Alt+Delete removes the highlighted entry") || rc=$?

        [[ -z "$sel" ]] && exit 0

        case "$rc" in
        0)
            cliphist decode <<<"$sel" | wl-copy
            exit 0
            ;;
        10)
            cliphist delete <<<"$sel"
            ;;
        *)
            exit 0
            ;;
        esac
    done
    ;;
wipe)
    cliphist wipe
    ;;
*)
    echo "usage: $0 menu|store|wipe" >&2
    exit 1
    ;;
esac
