package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Layout constants.
//
// The whole screen is a fixed-size composition — header, body, footer — so it
// renders identically on a 24-row console and a 48-row one, and nothing moves
// as the user walks through the steps. Only a console too small for the full
// layout shrinks it.
const (
	// sidebarWidth is the step list's total width, borders included.
	sidebarWidth = 30
	// gutter separates the step list from the card.
	gutter = 2
	// maxCardWidth stops the card stretching to unreadable line lengths.
	maxCardWidth = 78
	// sidebarMinWidth is the narrowest terminal that still gets a step list.
	// Below it the card takes the full width and the progress bar in the
	// header carries the orientation on its own.
	sidebarMinWidth = 96

	// bodyRows is the height of the card, borders included.
	bodyRows    = 22
	minBodyRows = 8

	// barWidth is the header's progress bar.
	barWidth = 30
)

// layout is the space a screen has to work with, resolved from the terminal.
type layout struct {
	// Width and Height are the terminal's.
	Width, Height int
	// CardWidth and CardHeight are the card's *inner* area, inside its border
	// and padding — what a step actually draws into.
	CardWidth, CardHeight int
	// Sidebar reports whether there is room for the step list.
	Sidebar bool
}

// layoutFor resolves the layout for a terminal size.
func layoutFor(width, height int) layout {
	l := layout{Width: width, Height: height, Sidebar: width >= sidebarMinWidth}

	available := width - 2*gutter
	if l.Sidebar {
		available -= sidebarWidth + gutter
	}
	if available > maxCardWidth {
		available = maxCardWidth
	}
	if available < 24 {
		available = 24
		l.Sidebar = false
	}
	// Two border columns and a column of padding on each side.
	l.CardWidth = available - 4

	rows := bodyRows
	// Header is the wordmark, the tagline and the bar; then a blank line, the
	// body, a blank line and the footer, inside one row of padding each side.
	if spare := height - logoHeight - 2 - 5; spare < rows {
		rows = spare
	}
	if rows < minBodyRows {
		rows = minBodyRows
	}
	// The card spends four rows on chrome before any content: two borders, the
	// title, and the blank line under it.
	l.CardHeight = rows - 4
	if l.CardHeight < 3 {
		l.CardHeight = 3
	}

	return l
}

// header renders the wordmark, the tagline and the progress bar.
func header(width, step, total int) string {
	rows := []string{
		logo(width),
		styleLabel.Render(centre(tagline, width, len(tagline))),
	}
	// total is zero on the install screen, which has its own progress bar
	// inside the card and no wizard steps left to count.
	if total > 0 {
		rows = append(rows, progressBar(width, step, total))
	}
	return strings.Join(rows, "\n")
}

// progressBar renders a filled bar with the step count beside it.
func progressBar(width, step, total int) string {
	if total < 1 {
		total = 1
	}
	filled := barWidth * step / total
	if filled > barWidth {
		filled = barWidth
	}

	count := " " + itoa(step) + "/" + itoa(total)
	bar := styleAccent.Render(strings.Repeat(g.BarFull, filled)) +
		styleTrack.Render(strings.Repeat(g.BarEmpty, barWidth-filled))

	pad := (width - barWidth - len(count)) / 2
	if pad < 0 {
		pad = 0
	}
	return strings.Repeat(" ", pad) + bar + styleLabel.Render(count)
}

// card draws a titled box around a step's content.
//
// The content is padded to the full inner height so the box is the same size
// on every step and the footer below it never moves.
func card(icon, title, body string, width, height int) string {
	lines := strings.Split(body, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}
	for len(lines) < height {
		lines = append(lines, "")
	}

	inner := strings.Join(lines, "\n")
	if title != "" {
		head := styleHeading.Render(title)
		if strings.TrimSpace(icon) != "" {
			head = styleAccent.Render(icon) + "  " + head
		}
		inner = head + "\n\n" + inner
	}

	// Width counts the padding, so the content area is two columns narrower
	// than whatever is passed here. Adding them back is what keeps a
	// full-width rule or list row from wrapping onto a second line.
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorBorder).
		Padding(0, 1).
		Width(width + 2).
		Render(inner)
}

// sidebar draws the step list: every step, its answer so far, and where the
// user is.
//
// This is the orientation a plain wizard lacks. Without it nothing tells you
// how much is left or what you already said, and a progress bar alone cannot
// show that the hostname is already set.
func sidebar(rows []sidebarRow, height int) string {
	lines := make([]string, 0, height)
	lines = append(lines, styleLabel.Render("install"), "")

	// status, icon, then a name column and whatever is left for the answer.
	const nameWidth = 11
	inner := sidebarWidth - 4

	for _, row := range rows {
		var mark, nameStyle = styleFaint.Render(g.Pending), styleFaint
		switch {
		case row.Current:
			mark, nameStyle = styleAccent.Render(g.Current), styleAccent
		case row.Done:
			mark, nameStyle = styleSuccess.Render(g.Done), styleText
		}

		icon := row.Icon
		if icon == "" {
			icon = " "
		}

		line := pad(truncate(row.Name, nameWidth-1), nameWidth)
		if row.Value != "" {
			line += truncate(row.Value, inner-nameWidth-4)
		}
		lines = append(lines,
			mark+" "+styleLabel.Render(icon)+" "+nameStyle.Render(truncate(line, inner-4)))
	}

	for len(lines) < height {
		lines = append(lines, "")
	}
	if len(lines) > height {
		lines = lines[:height]
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorBorder).
		Padding(0, 1).
		Width(sidebarWidth - 2).
		Render(strings.Join(lines, "\n"))
}

// sidebarRow is one entry of the step list.
type sidebarRow struct {
	Name    string
	Icon    string
	Value   string
	Current bool
	Done    bool
}

// renderScreen assembles the finished frame: header, body, footer, centred.
func renderScreen(l layout, step, total int, rows []sidebarRow, icon, title, body, footer string) string {
	content := card(icon, title, body, l.CardWidth, l.CardHeight)
	if l.Sidebar {
		content = lipgloss.JoinHorizontal(lipgloss.Top,
			sidebar(rows, l.CardHeight+1),
			strings.Repeat(" ", gutter),
			content)
	}

	block := strings.Join([]string{
		header(l.Width, step, total),
		"",
		lipgloss.NewStyle().Width(l.Width).Align(lipgloss.Center).Render(content),
		"",
		styleFaint.Render(centreText(footer, l.Width)),
	}, "\n")

	return lipgloss.NewStyle().Padding(1, 0).Render(block)
}

// centreText centres an already-styled line by its printable width.
func centreText(s string, width int) string {
	if pad := (width - lipgloss.Width(s)) / 2; pad > 0 {
		return strings.Repeat(" ", pad) + s
	}
	return s
}

// hints renders a footer from alternating key and description pairs.
func hints(pairs ...string) string {
	var parts []string
	for i := 0; i+1 < len(pairs); i += 2 {
		parts = append(parts, styleKey.Render(pairs[i])+" "+styleFaint.Render(pairs[i+1]))
	}
	return strings.Join(parts, styleFaint.Render("  ·  "))
}

// headerRow renders a title on the left and a muted note on the right.
func headerRow(width int, title, note string) string {
	left := styleHeading.Render(title)
	if note == "" {
		return left
	}
	right := styleFaint.Render(note)
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

// rule draws a horizontal divider.
func rule(width int) string {
	if width < 1 {
		return ""
	}
	return styleTrack.Render(strings.Repeat("─", width))
}

// truncate shortens a string to a column width, with an ellipsis.
func truncate(s string, width int) string {
	runes := []rune(s)
	if width < 4 || len(runes) <= width {
		return s
	}
	return string(runes[:width-1]) + "…"
}

// pad right-pads a string to a column width.
func pad(s string, width int) string {
	if gap := width - lipgloss.Width(s); gap > 0 {
		return s + strings.Repeat(" ", gap)
	}
	return s
}

// joinLines stacks rendered lines into a block.
func joinLines(lines ...string) string { return strings.Join(lines, "\n") }

// window trims a list of rows to a height, keeping the focused row visible.
func window(rows []string, height, focus int) []string {
	if height <= 0 || len(rows) <= height {
		return rows
	}
	start := focus - height/2
	if start < 0 {
		start = 0
	}
	if start+height > len(rows) {
		start = len(rows) - height
	}
	return rows[start : start+height]
}

// itoa avoids pulling strconv into every call site for small counts.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	if neg {
		return "-" + string(digits)
	}
	return string(digits)
}
