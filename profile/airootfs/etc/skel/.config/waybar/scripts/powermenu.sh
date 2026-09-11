#!/usr/bin/env bash
options="󰗽  Logout\n󰤄  Suspend\n󰜉  Reboot\n󰐥  Shutdown"
chosen=$(echo -e "$options" | rofi -dmenu -i -p "Power" -theme-str "listview { lines: 4; border: 0px; } mainbox { children: [\"listview\"]; }")
case "$chosen" in
*Logout) hyprctl dispatch exit ;;
*Suspend) systemctl suspend ;;
*Reboot) systemctl reboot ;;
*Shutdown) systemctl poweroff ;;
esac
