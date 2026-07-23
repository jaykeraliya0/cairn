# Cairn

A custom Arch Linux live ISO built with [`archiso`](https://gitlab.archlinux.org/archlinux/archiso).

## Requirements

Build on an Arch Linux host (or an Arch container) with:

- `archiso` (provides `mkarchiso`)
- `qemu-desktop` or `qemu-full`, and `edk2-ovmf` (only needed to boot-test with `./test.sh`)

## Building

```sh
sudo ./build.sh
```

`mkarchiso` needs root for loop mounts, so the script must run with `sudo`. It wipes `work/` on
every run and writes the resulting ISO to `out/` as `cairn-<YYYY.MM.DD>-x86_64.iso`.

## Testing

Boot the most recently built ISO in QEMU (UEFI):

```sh
./test.sh
```

Run the shell unit tests:

```sh
bats tests/
```

Lint the shell scripts:

```sh
shellcheck scripts/**/*.sh build.sh test.sh
```

## License

GPL-3.0 — see [LICENSE](LICENSE).
