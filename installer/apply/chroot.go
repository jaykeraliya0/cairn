package apply

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/jaykeraliya0/cairn/installer/config"
	"github.com/jaykeraliya0/cairn/installer/sys"
)

// ChrootScriptPath is where the configuration script is written, relative to
// the target root.
const ChrootScriptPath = "root/cairn-chroot.sh"

// chrootScript configures the freshly extracted system.
//
// Not one user-supplied value is substituted into this text. The script is a
// constant; hostname, username, timezone, locale and keymap arrive as
// environment variables that the inner bash expands, and the two passwords
// arrive on stdin. So there is no stage at which a shell parses something the
// user typed — a hostname of `; rm -rf /` is just a strange hostname.
//
// The passwords never reach an argv or an environment block either, which is
// the part the previous shell installer could not manage: exported
// CAIRN_TARGET_*_PASSWORD variables are readable from /proc/<pid>/environ by
// any process running as the same user for as long as the chroot lives.
const chrootScript = `#!/bin/bash
set -euo pipefail

# This system's own identity. The image ships without either on purpose:
# systemd-machine-id-setup writes the machine ID and pacman-key --init generates
# a local signing key, and building one of each into the image would give every
# Cairn install the same machine ID and the same private key.
systemd-machine-id-setup
pacman-key --init
pacman-key --populate archlinux

# Time
ln -sf "/usr/share/zoneinfo/${CAIRN_TIMEZONE}" /etc/localtime
hwclock --systohc

# Locale
printf '%s\n' "${CAIRN_LOCALE_GEN_LINE}" >> /etc/locale.gen
locale-gen
printf 'LANG=%s\n' "${CAIRN_LOCALE}" > /etc/locale.conf

# Console keymap. Written before mkinitcpio runs so the sd-vconsole hook picks
# it up: without it the boot-time passphrase prompt uses a US layout whatever
# the installed system does, which is a fine way to lock yourself out.
printf 'KEYMAP=%s\n' "${CAIRN_KEYMAP}" > /etc/vconsole.conf

# Hostname
printf '%s\n' "${CAIRN_HOSTNAME}" > /etc/hostname
cat >> /etc/hosts <<HOSTS
127.0.0.1   localhost
::1         localhost
127.0.1.1   ${CAIRN_HOSTNAME}.localdomain ${CAIRN_HOSTNAME}
HOSTS

# Accounts. useradd has to come first so chpasswd's second line has a user to
# act on; both passwords are waiting on stdin in chpasswd's own format.
useradd -m -G wheel -s /bin/bash "${CAIRN_USERNAME}"
chpasswd

# Sudo for the wheel group, as a drop-in rather than a patch to /etc/sudoers,
# so a pacman upgrade to that file can never undo it.
printf '%%wheel ALL=(ALL:ALL) ALL\n' > /etc/sudoers.d/10-wheel
chmod 0440 /etc/sudoers.d/10-wheel
visudo -cqf /etc/sudoers.d/10-wheel

# Services
systemctl enable iwd.service NetworkManager.service sddm.service \
                 bluetooth.service power-profiles-daemon.service

# qt5ct and qt6ct store an absolute path to their color scheme, and /etc/skel
# cannot know the user's home directory. Substitute the placeholder now that
# the account, and its home, exist. The username is validated against
# useradd's own charset, so it cannot contain the sed delimiter.
for ct in qt5ct qt6ct; do
  conf="/home/${CAIRN_USERNAME}/.config/${ct}/${ct}.conf"
  if [ -f "$conf" ]; then
    sed -i "s|@HOME@|/home/${CAIRN_USERNAME}|g" "$conf"
  fi
done

# gnome-keyring needs no PAM edit here: Arch's sddm package already lists
# pam_gnome_keyring.so for auth, password and session, each prefixed with "-"
# so it is skipped while the module is absent. Installing gnome-keyring is what
# switches those lines on, and the login password then unlocks the keyring.

# Initramfs, using the drop-in written at /etc/mkinitcpio.conf.d/cairn.conf
mkinitcpio -P

# systemd-boot's binary and EFI entry; the loader config itself is written
# from outside the chroot once the kernel images are known.
bootctl install
`

// chrootEnv returns the non-secret values the script reads.
func chrootEnv(cfg config.Config) []string {
	return []string{
		"CAIRN_TIMEZONE=" + cfg.Timezone,
		"CAIRN_LOCALE=" + cfg.Locale,
		"CAIRN_LOCALE_GEN_LINE=" + cfg.LocaleGenLine,
		"CAIRN_KEYMAP=" + cfg.Keymap,
		"CAIRN_HOSTNAME=" + cfg.Hostname,
		"CAIRN_USERNAME=" + cfg.Username,
	}
}

// chrootStdin renders the two chpasswd lines, root first.
func chrootStdin(cfg config.Config) string {
	return fmt.Sprintf("root:%s\n%s:%s\n", cfg.RootPassword, cfg.Username, cfg.UserPassword)
}

// configureChroot writes the configuration script into the target and runs it
// under arch-chroot.
func configureChroot(ctx context.Context, s *State) error {
	script := s.path(ChrootScriptPath)
	if err := os.MkdirAll(s.path("root"), 0o750); err != nil {
		return fmt.Errorf("creating %s: %w", s.path("root"), err)
	}
	if err := os.WriteFile(script, []byte(chrootScript), 0o700); err != nil {
		return fmt.Errorf("writing %s: %w", script, err)
	}
	// The script holds no secrets, but it has no reason to survive the install
	// either.
	defer func() {
		if err := os.Remove(script); err != nil && !os.IsNotExist(err) {
			s.logf("warning: could not remove %s: %v", script, err)
		}
	}()

	return s.Runner.Run(ctx, sys.Cmd{
		Name:  "arch-chroot",
		Args:  []string{s.Root, "/bin/bash", "/" + ChrootScriptPath},
		Env:   chrootEnv(s.Cfg),
		Stdin: strings.NewReader(chrootStdin(s.Cfg)),
	})
}
