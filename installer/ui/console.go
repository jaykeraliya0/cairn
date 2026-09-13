package ui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/term"
)

// vtPalette is Cairn's palette in the Linux console's 16 colour slots, in
// order 0-15 (black, red, green, yellow, blue, magenta, cyan, white, then the
// bright half).
//
// The console has exactly these sixteen colours, so without this the
// installer's truecolor values get squashed to whatever the default VGA
// palette happens to be and it looks nothing like the desktop it installs.
// Redefining the slots themselves is what makes the theme actually arrive.
// Values match profile/airootfs/etc/skel/.config/hypr/colors.lua.
var vtPalette = [16]string{
	"1a1815", // 0  black          background
	"f0726a", // 1  red            error
	"9db06a", // 2  green          success
	"f7c594", // 3  yellow         accent
	"7d8f9c", // 4  blue
	"9d7f55", // 5  magenta        accent_dark
	"b9ae9c", // 6  cyan           labels and prose
	"fffdf5", // 7  white          primary text
	// Slot 8 is what terminals reach for when asked to dim text, so it has to
	// stay readable — this is the console's equivalent of colorMuted, not a
	// background shade.
	"8a8073", // 8  bright black   hints and secondary values
	"f0726a", // 9  bright red
	"9db06a", // 10 bright green
	"f7c594", // 11 bright yellow
	"7d8f9c", // 12 bright blue
	"9d7f55", // 13 bright magenta
	"b9ae9c", // 14 bright cyan
	"fffdf5", // 15 bright white
}

// consoleFonts are the kbd fonts fitConsoleFont chooses between, narrowest
// first. All three ship with `kbd`, which systemd depends on — so it is
// already on the live ISO and on every installed system, and neither this nor
// the keymap catalogue adds a package.
var consoleFonts = []string{"default8x16", "sun12x22", "latarcyrheb-sun32"}

// targetRows is the console height the font search aims for. The installer is
// drawn on whatever mode the firmware left us in; at a native KMS resolution
// the default 8x16 font gives a hundred-plus tiny rows, and the wordmark ends
// up marooned in a corner of the screen.
const targetRows = 48

// SetupConsole makes the Linux virtual console look like Cairn before anything
// is drawn on it. It is a no-op anywhere else — over SSH, in a terminal
// emulator, or in a test — since both halves would be meaningless or rude
// there.
//
// Nothing is restored on exit: the palette *is* the theme, and the installer
// hands the console straight to a reboot.
func SetupConsole() {
	// The graphical session needs neither: foot has 24-bit colour of its own,
	// and its font is chosen by the launcher. Both calls would be meaningless
	// there, and the palette escape would be printed as garbage.
	if !onLinuxVT() || Graphical() {
		return
	}
	fitConsoleFont()
	applyVTPalette()
}

// onLinuxVT reports whether standard input is a Linux virtual console rather
// than a pty. /dev/tty1 is a VT; /dev/pts/3 is not.
func onLinuxVT() bool {
	name, err := os.Readlink("/proc/self/fd/0")
	if err != nil {
		return false
	}
	return strings.HasPrefix(name, "/dev/tty") && name != "/dev/tty"
}

// applyVTPalette redefines the console's sixteen colour slots.
//
// OSC "P" plus a slot nibble and six hex digits is the Linux console's own
// palette escape — it is not an ANSI sequence and does nothing on a pty, which
// is the other reason this is guarded.
func applyVTPalette() {
	fmt.Fprint(os.Stdout, vtPaletteSequence())
}

// vtPaletteSequence builds the escape sequence applyVTPalette writes.
func vtPaletteSequence() string {
	var b strings.Builder
	for slot, rgb := range vtPalette {
		fmt.Fprintf(&b, "\033]P%X%s", slot, rgb)
	}
	// Reset attributes and clear, so nothing painted under the old palette
	// survives into the new one.
	b.WriteString("\033[0m\033[2J\033[H")
	return b.String()
}

// fitConsoleFont picks the console font whose resulting row count lands
// closest to targetRows while still leaving the wordmark room to breathe.
//
// The size is measured after applying each candidate rather than computed from
// the framebuffer: under some virtual GPUs the reported framebuffer size is one
// the text console never actually reaches, and trusting it is how the wordmark
// ends up wrapped.
func fitConsoleFont() {
	best, bestDiff := "", -1

	for _, font := range consoleFonts {
		if err := exec.Command("setfont", font).Run(); err != nil {
			continue
		}
		cols, rows, err := term.GetSize(int(os.Stdout.Fd()))
		if err != nil || cols <= LogoWidth+4 {
			continue
		}

		diff := rows - targetRows
		if diff < 0 {
			diff = -diff
		}
		if bestDiff < 0 || diff < bestDiff {
			best, bestDiff = font, diff
		}
	}

	// Fall back to the narrowest font if nothing cleared the wordmark's width,
	// so the form stays usable on an unexpectedly small console.
	if best == "" {
		best = consoleFonts[0]
	}
	_ = exec.Command("setfont", best).Run()
}
