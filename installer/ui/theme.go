package ui

import "github.com/charmbracelet/lipgloss"

// Cairn's palette, derived from the "Last of Us" theme the installed desktop
// uses (profile/airootfs/etc/skel/.config/hypr/colors.lua).
//
// The foreground ramp has three readable tiers and one decorative one, and the
// distinction matters: colors.lua's `bg_hl` (#352b21) is a *background*
// shade, and using it as a foreground — which an earlier version did for every
// hint, placeholder, status line and the progress bar's track — gives 1.3:1
// against the background. That is invisible, not subtle. Anything carrying
// words lives at 4.5:1 or better; only rules, borders and the bar's empty
// track sit below, and they carry no text. contrast_test.go enforces it.
var (
	// colorBg is the terminal's background, and what every ratio is measured
	// against.
	colorBg = lipgloss.Color("#1a1815")

	// The readable tiers, brightest first.
	colorFg    = lipgloss.Color("#fffdf5") // 17.4:1 — primary text
	colorFgDim = lipgloss.Color("#b9ae9c") //  8.1:1 — labels, prose, key names
	colorMuted = lipgloss.Color("#8a8073") //  4.6:1 — hints, placeholders, secondary values

	// Decorative only: box edges and the progress bar's unfilled track.
	colorBorder = lipgloss.Color("#4d4234") // 1.8:1

	colorAccent = lipgloss.Color("#f7c594") // 11.3:1
	colorGreen  = lipgloss.Color("#9db06a") //  7.5:1
	colorRed    = lipgloss.Color("#f0726a") //  6.2:1
)

// The whole style vocabulary, deliberately small: one accent, three levels of
// text, one decorative rule, and green and red for outcomes. A screen that
// needs a seventh style usually wants less on it instead.
var (
	styleAccent  = lipgloss.NewStyle().Foreground(colorAccent)
	styleHeading = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	styleText    = lipgloss.NewStyle().Foreground(colorFg)
	styleLabel   = lipgloss.NewStyle().Foreground(colorFgDim)
	styleKey     = lipgloss.NewStyle().Foreground(colorFgDim)
	styleFaint   = lipgloss.NewStyle().Foreground(colorMuted)
	styleError   = lipgloss.NewStyle().Foreground(colorRed)
	styleWarn    = lipgloss.NewStyle().Foreground(colorAccent)
	styleSuccess = lipgloss.NewStyle().Foreground(colorGreen).Bold(true)
	styleLogo    = lipgloss.NewStyle().Foreground(colorAccent)

	// styleTrack is for the marks that are structure rather than words: box
	// rules and the progress bar's empty half.
	styleTrack = lipgloss.NewStyle().Foreground(colorBorder)

	// Checklist marks for the install screen.
	styleStepDone    = lipgloss.NewStyle().Foreground(colorGreen)
	styleStepPending = lipgloss.NewStyle().Foreground(colorMuted)
)
