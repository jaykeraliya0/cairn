#!/usr/bin/env bash
# Opens one of the terminal managers in a floating window. Both waybar's
# network and bluetooth modules and the SUPER+SHIFT+{W,N} keybinds come through
# here, so the terminal, the window class and the title are named once.
#
# --class sets the Wayland app_id, which is what hyprland.lua's "float-tui"
# window rule matches. The title is not: a program running inside the terminal
# can rewrite its title, and cannot touch the app_id.
#
# The class is given twice on purpose. alacritty takes "--class
# <instance>[,<general>]" and only the general half reaches the Wayland app_id,
# so a single value would leave the app_id as "Alacritty" and the rule would
# never fire.
set -euo pipefail

case "${1-}" in
wifi)
  exec alacritty --class cairn-tui,cairn-tui --title "Wi-Fi" -e impala
  ;;
bluetooth)
  exec alacritty --class cairn-tui,cairn-tui --title "Bluetooth" -e bluetui
  ;;
*)
  echo "usage: ${0##*/} wifi|bluetooth" >&2
  exit 2
  ;;
esac
