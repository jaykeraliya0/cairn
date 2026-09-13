package ui

import "strings"

// logoArt is the Cairn wordmark, set in the figlet font "kompaktblk".
//
// Three rows on purpose. A tall wordmark eats the rows the step list and the
// card need; this one brands the screen without crowding it, and it is drawn
// with nothing but U+2588/2580/2584, which every Linux console font carries.
const logoArt = `▄▀▀▀▀ ▄▀▀▀▄ ▀▀█▀▀ █▀▀▀▄ █▄  █
█     █▀▀▀█   █   █▀▀▀▄ █ ▀▄█
 ▀▀▀▀ ▀   ▀ ▀▀▀▀▀ ▀   ▀ ▀   ▀`

// tagline sits under the wordmark. Short enough not to wrap at 80 columns.
const tagline = "an Arch Linux desktop, built to last"

// LogoWidth is the wordmark's width in columns.
const LogoWidth = 29

// logoHeight is how many rows the wordmark occupies.
var logoHeight = strings.Count(logoArt, "\n") + 1

// logo renders the wordmark centred over a column of the given width.
//
// Every row is shifted by the same amount rather than centred individually:
// the rows are different lengths, and centring each one shears the letters.
func logo(width int) string {
	if width < LogoWidth {
		return styleLogo.Render(centre("CAIRN", width, len("CAIRN")))
	}

	var lines []string
	for _, line := range strings.Split(logoArt, "\n") {
		lines = append(lines, styleLogo.Render(centre(line, width, LogoWidth)))
	}
	return strings.Join(lines, "\n")
}

// centre left-pads a line so a block of the given art width sits in the middle.
func centre(line string, width, artWidth int) string {
	if pad := (width - artWidth) / 2; pad > 0 {
		return strings.Repeat(" ", pad) + line
	}
	return line
}
