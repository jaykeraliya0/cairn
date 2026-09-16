# Cairn

Cairn is an Arch Linux ISO I built for myself, because I got tired of doing the same install by
hand.

The routine was always the same. Boot the official ISO, squint at the wiki, partition, pacstrap,
chroot, install a bootloader, then spend the rest of the evening putting Hyprland back together the
way I like it. Fun the first time. Tedious the fifth. So Cairn is that evening, packaged: boot it,
answer a handful of questions, and you get an Arch box with a Wayland desktop already themed and
already working.

It is still Arch underneath. Nothing is wrapped, patched or hidden. `pacman` is `pacman`, the wiki
still applies, and if you don't like a choice made here you can undo it with one command.

## What you get

A Hyprland session on Wayland, with the pieces that make it a desktop rather than a bare
compositor: waybar across the top, rofi for launching things, mako for notifications, hyprlock and
hypridle for locking and idling, swaybg behind it all. Alacritty is the terminal, Firefox the
browser, Nautilus the file manager, SDDM the greeter you land on at boot. Audio is PipeWire with
WirePlumber. Bluetooth, networking and power profiles are installed *and* enabled, which sounds
obvious until you've used a system where they weren't.

Screenshots are bound to Print (region, screen, or straight to the clipboard). Volume and
brightness keys work. Super+V pulls up clipboard history. Nothing exotic, but nothing missing
either.

## The look

Everything ships in one palette — warm dark browns, bone white text, a soft apricot accent. It
started as a "Last of Us" theme and the name stuck. Wallpaper included.

The part I care about is that the theme actually reaches the applications. GTK 3, GTK 4 and Qt all
get their own treatment, because each of them ignores the others. libadwaita in particular pays no
attention to `gtk-theme-name` at all, so GTK 4 apps get the palette written directly into their
named colors; Qt apps get a hand-written Fusion palette through qt5ct and qt6ct. The result is
that Nautilus and file-roller don't glow white in the middle of a dark session.

This is a one-time bake, not a theme engine. There's no switcher, no daemon watching for changes.
The colours live in `colors.lua` and five other config files copy the same hex values, each in its
own syntax. Changing the palette means editing all of them. I decided that was a fair price for not
running a theming framework.

Hyprland's config is Lua, by the way, not hyprlang. Upstream deprecated its own config language and
now warns about it on every start, so Cairn moved before it became a problem.

## The installer

`cairn-install` is a Go program with a terminal UI, and it starts on its own when the ISO boots.
On a machine with a display it runs inside a tiny Wayland kiosk so you get a real terminal with
truecolor and proper icons; on anything else it falls back to the plain console, swaps the Nerd Font
glyphs for CP437 box-drawing, and repaints the console's sixteen colour slots so it still looks
like Cairn.

It's a wizard with the step list always on screen: keyboard, then locale and timezone, then the
disk and how to carve it, then accounts, then a review. Every answer you've given stays visible, so
the last screen confirms rather than surprises. Almost everything is pre-filled — the user account
is the only thing it genuinely can't guess.

What it does to the disk:

- Wipes one whole disk. GPT, an ESP, and everything else as root.
- Optional LUKS2 encryption on the root partition.
- Btrfs, zstd compression on by default, and the usual subvolume split: `@`, `@home`, `@log`,
  `@cache`, `@snapshots`.
- An optional swapfile.
- systemd-boot.

Then hostname, locale, users, sudo for the wheel group, and the Nvidia driver if you asked for it.

### It never touches the network

This is the bit I'm most pleased with. Cairn doesn't install packages during an install — the
finished system is already on the ISO as a compressed image, and the installer just extracts it and
does the per-machine setup afterwards. It's how Manjaro, EndeavourOS's offline mode, Fedora and
Ubuntu all work, and it means an install takes a couple of minutes on a laptop with no wifi driver
loaded and no cable in reach.

Nvidia's drivers sit in a small side repository on the ISO rather than in the image, because
they're 2.4 GiB installed and most machines don't want them. If you pick Nvidia, they're installed
from the disc. Still no network.

### And it tries not to lie to you

Every command the installer runs, and all of its output, goes to the progress screen and to
`/var/log/cairn-install.log` at the same time. When something fails, the checklist collapses and
the log gets the screen, wrapped rather than truncated, because the half of an error message past
the right margin is usually the half that explains it.

A few rules it holds itself to:

- **No shell anywhere.** Commands are argv slices. The one script it writes into the target is a
  constant, with your answers passed as environment variables — nothing you type is ever pasted
  into text a shell will parse.
- **Secrets go over stdin, never argv or env.** The LUKS passphrase and both account passwords are
  piped in. An argv is world-readable through `/proc` for as long as the process lives, and some of
  these processes live for minutes.
- **The keyboard layout is honoured before you type a passphrase.** If you pick a non-US layout,
  the session restarts under it. Otherwise you'd set a LUKS passphrase under one layout and be
  asked for it at boot under another — an encrypted disk that can never be opened. That one is
  worth the restart.

There are tests for all of this. The install sequence runs end to end against a temp directory,
asserting the exact arguments of every command, with no root, no devices and no chroot involved.

## What it isn't

Cairn is not a distribution. There's no repository, no update channel, no branding on anything but
the installer. After the reboot it is plain Arch that happens to have my dotfiles in `/etc/skel`.
Updates come from the Arch mirrors like they would on any other install.

It's also not configurable in the way a real installer is. One disk, one layout, one desktop. If
you want a different filesystem or a different compositor, the honest answer is to fork it — the
package list is one file and the desktop config is one directory.

And it's built for x86_64 UEFI machines. BIOS boots the live ISO, but the installer writes
systemd-boot.

## Getting it

Grab an ISO from the [releases page](../../releases), write it to a USB stick, and boot it.
