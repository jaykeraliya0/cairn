package ui

import (
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// picker is a single-select screen over a searchable list.
//
// The search box is focused the moment the step opens and filters as you type
// — no mode to enter first. On a 553-entry timezone list, a filter hidden
// behind a keystroke the user has to know about is the same as no filter.
type picker struct {
	title  string
	note   string
	unit   string
	search textinput.Model
	list   *filterList
	commit func(value string)
}

func newPicker(title, note, unit string, items []listItem, current string, commit func(string)) *picker {
	search := textinput.New()
	search.Placeholder = "type to search"
	search.Prompt = ""
	search.Focus()

	list := newFilterList(items)
	list.selectValue(current)

	return &picker{title: title, note: note, unit: unit, search: search, list: list, commit: commit}
}

func (p *picker) Title() string { return p.title }

func (p *picker) Footer() string {
	// Not "q quit": every printable key goes to the search box, so the only
	// way out of a picker is the one that cannot be typed.
	return hints("type", "to search", "↑↓", "move", "enter", "continue",
		"esc", "back", "ctrl+c", "quit")
}

func (p *picker) Update(msg tea.Msg) (screen, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return p, nil
	}

	switch keyMsg.String() {
	case "esc":
		return p, back()
	case "enter":
		if item, ok := p.list.selected(); ok {
			p.commit(item.Value)
			return p, advance()
		}
		return p, nil
	case "up", "ctrl+p":
		p.list.move(-1)
		return p, nil
	case "down", "ctrl+n":
		p.list.move(1)
		return p, nil
	case "pgup":
		p.list.move(-p.list.height)
		return p, nil
	case "pgdown":
		p.list.move(p.list.height)
		return p, nil
	case "home":
		p.list.move(-len(p.list.filtered))
		return p, nil
	case "end":
		p.list.move(len(p.list.filtered))
		return p, nil
	}

	// Everything else is typing, which narrows the list.
	before := p.search.Value()
	var cmd tea.Cmd
	p.search, cmd = p.search.Update(msg)
	if p.search.Value() != before {
		p.list.setFilter(p.search.Value())
	}
	return p, cmd
}

func (p *picker) Body(width, height int) string {
	rows := []string{}
	if p.note != "" {
		rows = append(rows, styleLabel.Render(truncate(p.note, width)), "")
	}

	p.list.width = width
	p.list.height = height - len(rows) - 3 // search, rule, status
	if p.list.height < 3 {
		p.list.height = 3
	}
	p.search.Width = width - 10

	rows = append(rows,
		styleLabel.Render(g.Search+"  ")+p.search.View(),
		rule(width),
		p.list.View(),
		styleFaint.Render(p.status()),
	)
	return joinLines(rows...)
}

// status is the line under the list: where the window sits, or how many rows
// survived the filter.
func (p *picker) status() string {
	first, last := p.list.position()
	switch {
	case p.list.matches() == 0:
		return "no matches"
	case p.search.Value() != "":
		return plural(p.list.matches(), "match", "matches")
	case p.list.matches() <= p.list.height:
		return plural(p.list.matches(), singular(p.unit), p.unit)
	default:
		return itoa(first) + "-" + itoa(last) + " of " + itoa(p.list.matches())
	}
}

// singular is a crude de-pluraliser for the count label — good enough for the
// handful of nouns this is ever called with.
func singular(unit string) string {
	if len(unit) > 1 && unit[len(unit)-1] == 's' {
		return unit[:len(unit)-1]
	}
	return unit
}
