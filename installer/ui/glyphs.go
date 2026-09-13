package ui

import "os"

// glyphSet is every non-alphanumeric mark the interface draws.
//
// There are two of them because there are two places the installer runs. In
// the graphical session (cage + foot + JetBrains Mono Nerd Font) the whole
// Nerd Font private-use range is available. On the bare kernel console it is
// not, and cannot be: the console font format tops out at 512 bitmap glyphs,
// so a Nerd Font simply cannot be loaded there. Rather than let the console
// draw a screenful of blank boxes, that path gets a CP437 set that every kbd
// font carries.
type glyphSet struct {
	// Step-list icons.
	Keyboard   string
	Language   string
	Timezone   string
	Disk       string
	Encryption string
	Layout     string
	Hostname   string
	User       string
	Graphics   string
	Review     string

	// Status marks.
	Done    string
	Current string
	Pending string
	Failed  string
	Warning string

	// Chrome.
	Prompt    string
	Cursor    string
	Search    string
	BarFull   string
	BarEmpty  string
	Toggle    string
	ToggleOff string

	// Spinner frames.
	Spinner []string
}

// nerdGlyphs is the graphical session's set. The private-use codepoints are
// Nerd Font's Font Awesome range, which ttf-jetbrains-mono-nerd provides.
var nerdGlyphs = glyphSet{
	Keyboard:   "", // keyboard
	Language:   "", // globe
	Timezone:   "", // clock
	Disk:       "", // hard drive
	Encryption: "", // lock
	Layout:     "", // database
	Hostname:   "", // desktop
	User:       "", // user
	Graphics:   "", // display
	Review:     "", // list

	Done:    "", // check
	Current: "❯", // heavy angle right
	Pending: "○", // open circle
	Failed:  "", // times
	Warning: "", // triangle warning

	Prompt:    "❯",
	Cursor:    "▏", // left one-eighth block
	Search:    "", // magnifying glass
	BarFull:   "█",
	BarEmpty:  "░",
	Toggle:    "", // toggle on
	ToggleOff: "", // toggle off

	Spinner: []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
}

// asciiGlyphs is the console fallback: CP437 and ASCII only, so it renders on
// a bare VT with whatever font kbd loaded.
var asciiGlyphs = glyphSet{
	// No step icons: there is no glyph in CP437 that says "keyboard" or
	// "timezone", and a column of dashes standing in for them is worse than
	// the blank column that keeps everything else aligned.
	Keyboard:   " ",
	Language:   " ",
	Timezone:   " ",
	Disk:       " ",
	Encryption: " ",
	Layout:     " ",
	Hostname:   " ",
	User:       " ",
	Graphics:   " ",
	Review:     " ",

	Done:    "█", // full block
	Current: ">",
	Pending: "·", // middle dot
	Failed:  "!",
	Warning: "!",

	Prompt:    ">",
	Cursor:    "█",
	Search:    "/",
	BarFull:   "█",
	BarEmpty:  "░",
	Toggle:    "[x]",
	ToggleOff: "[ ]",

	Spinner: []string{"|", "/", "-", "\\"},
}

// g is the glyph set in use, chosen once at startup.
var g = pickGlyphs()

// pickGlyphs decides which set this session can render.
//
// CAIRN_SESSION is set by the live ISO's session launcher, which knows which
// path it took. Without it, standard input being a Linux virtual console is
// the tell: /dev/tty1 is a VT and cannot show Nerd Font glyphs, while
// /dev/pts/N is a terminal emulator and can.
func pickGlyphs() glyphSet {
	switch os.Getenv("CAIRN_SESSION") {
	case "graphical":
		return nerdGlyphs
	case "console":
		return asciiGlyphs
	}
	if onLinuxVT() {
		return asciiGlyphs
	}
	return nerdGlyphs
}

// Graphical reports whether the richer glyph set is in use. The console
// fallback also wants its palette repainted and its font resized, which a
// terminal emulator neither needs nor allows.
func Graphical() bool { return !onLinuxVT() && os.Getenv("CAIRN_SESSION") != "console" }
