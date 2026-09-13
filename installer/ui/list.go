package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// listItem is one row of a picker.
type listItem struct {
	// Label is what the user reads and what the search matches against.
	Label string
	// Note is muted detail shown right-aligned, e.g. a disk's size and model.
	Note string
	// Value is what selecting the row yields.
	Value string
}

// filterList is a scrolling single-select list with a live filter.
//
// Written rather than taken from bubbles/list because the requirement here is
// specific: the filter is always on and always visible, results keep the
// catalogue's own order, and the widget has to fit a fixed number of rows
// between a header and a pinned footer.
type filterList struct {
	items    []listItem
	filtered []int // indices into items, in catalogue order
	cursor   int   // index into filtered
	offset   int   // first visible row, an index into filtered
	height   int   // visible rows
	width    int
}

func newFilterList(items []listItem) *filterList {
	l := &filterList{items: items, height: 8}
	l.setFilter("")
	return l
}

// setFilter narrows the list to the rows containing query, case-insensitively.
//
// Substring, not fuzzy, and the catalogue's order is preserved: an
// alphabetical list that suddenly reorders itself by relevance is harder to
// scan, and these lists are already ordered deliberately — English layouts
// first, common locales first.
func (l *filterList) setFilter(query string) {
	previous, hadSelection := l.selected()

	query = strings.ToLower(strings.TrimSpace(query))
	l.filtered = l.filtered[:0]
	for i, item := range l.items {
		if query == "" ||
			strings.Contains(strings.ToLower(item.Label), query) ||
			strings.Contains(strings.ToLower(item.Value), query) {
			l.filtered = append(l.filtered, i)
		}
	}

	// Keep the highlighted row highlighted if it survived the filter, so
	// refining a query does not silently move the selection somewhere else.
	l.cursor, l.offset = 0, 0
	if hadSelection {
		l.selectValue(previous.Value)
	}
	l.scrollIntoView()
}

// selectValue puts the cursor on a value, if it is currently visible.
func (l *filterList) selectValue(value string) {
	for i, index := range l.filtered {
		if l.items[index].Value == value {
			l.cursor = i
			l.scrollIntoView()
			return
		}
	}
}

// move shifts the cursor, clamping at both ends. Clamping rather than wrapping:
// on a 553-entry list, wrapping from the top to the bottom is disorienting.
func (l *filterList) move(delta int) {
	if len(l.filtered) == 0 {
		return
	}
	l.cursor += delta
	if l.cursor < 0 {
		l.cursor = 0
	}
	if l.cursor >= len(l.filtered) {
		l.cursor = len(l.filtered) - 1
	}
	l.scrollIntoView()
}

func (l *filterList) scrollIntoView() {
	if l.height < 1 {
		return
	}
	if l.cursor < l.offset {
		l.offset = l.cursor
	}
	if l.cursor >= l.offset+l.height {
		l.offset = l.cursor - l.height + 1
	}
	if max := len(l.filtered) - l.height; l.offset > max {
		l.offset = max
	}
	if l.offset < 0 {
		l.offset = 0
	}
}

// selected returns the highlighted item.
func (l *filterList) selected() (listItem, bool) {
	if l.cursor < 0 || l.cursor >= len(l.filtered) {
		return listItem{}, false
	}
	return l.items[l.filtered[l.cursor]], true
}

// matches is how many rows survive the current filter.
func (l *filterList) matches() int { return len(l.filtered) }

// position describes where the visible window sits, for the header. It says
// more than a bare total: on a 554-entry list, "12-27 of 554" tells you both
// how far down you are and that there is more in each direction.
func (l *filterList) position() (first, last int) {
	if len(l.filtered) == 0 {
		return 0, 0
	}
	last = l.offset + l.height
	if last > len(l.filtered) {
		last = len(l.filtered)
	}
	return l.offset + 1, last
}

// View renders the visible rows, plus a scroll hint when there are more.
func (l *filterList) View() string {
	if len(l.filtered) == 0 {
		return styleFaint.Render("  no matches")
	}

	rows := make([]string, 0, l.height)
	end := l.offset + l.height
	if end > len(l.filtered) {
		end = len(l.filtered)
	}

	for i := l.offset; i < end; i++ {
		item := l.items[l.filtered[i]]
		selected := i == l.cursor

		// Unselected rows sit a tier down so the accent on the selected one
		// reads as a selection rather than just a colour change.
		marker, label := "  ", styleLabel
		if selected {
			marker, label = styleAccent.Render(g.Current+" "), styleAccent
		}

		line := item.Label
		if item.Note != "" {
			// Right-align the note against the content column.
			room := l.width - 2 - lipgloss.Width(item.Label) - lipgloss.Width(item.Note) - 2
			if room < 1 {
				line = truncate(item.Label, l.width-2)
			} else {
				line = pad(item.Label, lipgloss.Width(item.Label)+room+2) + styleFaint.Render(item.Note)
			}
		}
		rows = append(rows, marker+label.Render(truncate(line, l.width-2)))
	}

	// Pad to a constant height so the footer never moves as the list shortens.
	for len(rows) < l.height {
		rows = append(rows, "")
	}

	return strings.Join(rows, "\n")
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return itoa(n) + " " + many
}
