# Cairn

A custom Arch Linux live ISO built with [`archiso`](https://gitlab.archlinux.org/archlinux/archiso),
shipping a Hyprland desktop and a terminal installer.

## Requirements

Build on an Arch Linux host (or an Arch container) with:

- `archiso` (provides `mkarchiso`)
- `go` — the installer is a Go program, compiled into the ISO by `build.sh`
- `qemu-desktop` or `qemu-full`, and `edk2-ovmf` (only needed to boot-test with `./test.sh`)

## Building

```sh
sudo ./build.sh
```

`mkarchiso` needs root for loop mounts, so the script must run with `sudo`. It wipes `work/` on
every run and writes the resulting ISO to `out/` as `cairn-<YYYY.MM.DD>-x86_64.iso`.

The build also produces the installed system itself, as a compressed image on the ISO, so the
installer needs no network at all. It installs the target package set into a directory, packs it,
and puts Nvidia's drivers in a small side repository next to it. Downloads are cached in
`.cache/`, so a rebuild only fetches what changed upstream. Expect the ISO to be a few GiB, and the
build to need several GiB of free space while it runs.

## Installing

Boot the ISO and the installer starts by itself: a Wayland kiosk (`cage` running `foot`) when the
machine has a display, and the plain console otherwise. Quit it for a shell; run `cairn-install` to
come back.

No network is needed: the whole system, Nvidia's drivers included, is on the ISO.

The installer erases one whole disk and sets up a Btrfs root with optional LUKS2 encryption. It
asks for the target disk, encryption, partition and swap sizes, timezone, locale, keyboard layout,
hostname, accounts, and the graphics driver. Nothing is written until you confirm at the summary.

## Testing

Boot the most recently built ISO in QEMU (UEFI):

```sh
./test.sh
```

This only exercises the live environment — `run_archiso` attaches no writable disk, so the
installer cannot install anything under it. To exercise an install, run QEMU by hand with a target
disk and persistent firmware variables:

```sh
cp /usr/share/edk2/x64/OVMF_VARS.4m.fd OVMF_VARS.cairn.fd
qemu-img create -f qcow2 test-disk.qcow2 32G
qemu-system-x86_64 -enable-kvm -m 4G -smp 4 \
  -drive if=pflash,format=raw,readonly=on,file=/usr/share/edk2/x64/OVMF_CODE.4m.fd \
  -drive if=pflash,format=raw,file=OVMF_VARS.cairn.fd \
  -drive file=test-disk.qcow2,if=virtio \
  -cdrom out/cairn-*-x86_64.iso -boot d
```

Both files are gitignored.

## Development

```sh
cd installer && go test ./...      # unit tests
cd installer && go vet ./...       # static analysis
shellcheck build.sh test.sh        # lint the remaining shell
```

## License

GPL-3.0 — see [LICENSE](LICENSE).
