# fix for screen readers
if grep -Fqa 'accessibility=' /proc/cmdline &> /dev/null; then
    setopt SINGLE_LINE_ZLE
fi

~/.automated_script.sh

# Start the installer.
#
# cairn-session picks between the graphical kiosk and the plain console and
# always reaches one of them. Guarded to tty1 so an SSH session or a second VT
# gets a plain shell instead — which is also how you get one here: quit the
# installer, then run `cairn-install` to come back to it.
if [[ "$(tty)" == /dev/tty1 ]]; then
    cairn-session
fi
