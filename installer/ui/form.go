package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// labelWidth is the fixed column the field labels occupy, so every input on a
// screen starts at the same place.
const labelWidth = 12

type fieldKind int

const (
	fieldText fieldKind = iota
	fieldPassword
	fieldToggle
)

// field is one row of a form, bound directly to the answer it edits.
//
// Binding by pointer and writing through on every keystroke is what lets a
// later field's hidden() predicate react live — turning encryption off has to
// make the passphrase rows disappear as you do it, not on the next screen.
type field struct {
	kind        fieldKind
	label       string
	placeholder string
	hint        string
	// text is bound for fieldText and fieldPassword, flag for fieldToggle.
	text *string
	flag *bool
	// validate runs when leaving the field and again before committing.
	validate func(string) error
	// hidden removes the field from the form entirely while it returns true.
	hidden func() bool

	input textinput.Model
	err   string
}

// form edits a small group of related answers on one screen.
type form struct {
	title  string
	note   string
	fields []*field
	cursor int
	// commit runs when the last field is accepted; a returned error keeps the
	// form open with the message shown.
	commit func() error
	err    string
}

func newForm(title, note string, fields []*field, commit func() error) *form {
	for _, f := range fields {
		if f.kind == fieldToggle {
			continue
		}
		input := textinput.New()
		input.Prompt = ""
		input.Placeholder = f.placeholder
		input.SetValue(*f.text)
		if f.kind == fieldPassword {
			input.EchoMode = textinput.EchoPassword
			input.EchoCharacter = '•'
		}
		f.input = input
	}

	fm := &form{title: title, note: note, fields: fields, commit: commit}
	fm.cursor = fm.firstVisible()
	fm.focusCurrent()
	return fm
}

// visible lists the indices of the fields currently in play.
func (f *form) visible() []int {
	var out []int
	for i, field := range f.fields {
		if field.hidden == nil || !field.hidden() {
			out = append(out, i)
		}
	}
	return out
}

func (f *form) firstVisible() int {
	if v := f.visible(); len(v) > 0 {
		return v[0]
	}
	return 0
}

// step moves the cursor by delta places through the visible fields.
func (f *form) step(delta int) {
	visible := f.visible()
	if len(visible) == 0 {
		return
	}

	at := 0
	for i, index := range visible {
		if index == f.cursor {
			at = i
		}
	}
	at += delta
	if at < 0 {
		at = 0
	}
	if at >= len(visible) {
		at = len(visible) - 1
	}

	f.cursor = visible[at]
	f.focusCurrent()
}

// isLast reports whether the cursor sits on the last visible field.
func (f *form) isLast() bool {
	visible := f.visible()
	return len(visible) > 0 && visible[len(visible)-1] == f.cursor
}

func (f *form) focusCurrent() {
	for i, field := range f.fields {
		if field.kind == fieldToggle {
			continue
		}
		if i == f.cursor {
			field.input.Focus()
		} else {
			field.input.Blur()
		}
	}
}

// validateCurrent checks the focused field, recording any message on it.
func (f *form) validateCurrent() bool {
	field := f.fields[f.cursor]
	if field.validate == nil {
		field.err = ""
		return true
	}

	value := ""
	if field.text != nil {
		value = *field.text
	}
	if err := field.validate(value); err != nil {
		field.err = capitalise(err.Error())
		return false
	}
	field.err = ""
	return true
}

func (f *form) Title() string { return f.title }

func (f *form) Footer() string {
	action := "continue"
	if !f.isLast() {
		action = "next field"
	}
	// Not "q quit": a text field needs the letter.
	pairs := []string{"↑↓", "move", "enter", action, "esc", "back", "ctrl+c", "quit"}
	if f.fields[f.cursor].kind == fieldToggle {
		pairs = append([]string{"←→", "change"}, pairs...)
	}
	return hints(pairs...)
}

func (f *form) Update(msg tea.Msg) (screen, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return f, nil
	}

	current := f.fields[f.cursor]

	switch keyMsg.String() {
	case "esc":
		return f, back()

	case "up", "shift+tab":
		f.step(-1)
		return f, nil

	case "tab", "down":
		// Moving on validates, so a bad value is caught where it was typed
		// rather than at the end of the form.
		if !f.validateCurrent() {
			return f, nil
		}
		f.step(1)
		return f, nil

	case "enter":
		if !f.validateCurrent() {
			return f, nil
		}
		if !f.isLast() {
			f.step(1)
			return f, nil
		}
		// Re-check every visible field: one may have been made valid-looking
		// by a later edit, or invalidated by one.
		for _, index := range f.visible() {
			saved := f.cursor
			f.cursor = index
			if !f.validateCurrent() {
				f.focusCurrent()
				return f, nil
			}
			f.cursor = saved
		}
		if f.commit != nil {
			if err := f.commit(); err != nil {
				f.err = capitalise(err.Error())
				return f, nil
			}
		}
		f.err = ""
		return f, advance()
	}

	if current.kind == fieldToggle {
		switch keyMsg.String() {
		case "left", "right", " ", "h", "l":
			*current.flag = !*current.flag
		case "y", "Y":
			*current.flag = true
		case "n", "N":
			*current.flag = false
		}
		return f, nil
	}

	var cmd tea.Cmd
	current.input, cmd = current.input.Update(msg)
	*current.text = current.input.Value()
	// Typing clears the complaint about what you are now fixing.
	current.err = ""
	f.err = ""
	return f, cmd
}

func (f *form) Body(width, height int) string {
	rows := []string{}
	if f.note != "" {
		rows = append(rows, styleLabel.Render(truncate(f.note, width)), "")
	}

	for _, index := range f.visible() {
		field := f.fields[index]
		focused := index == f.cursor

		label := styleLabel.Render(pad(field.label, labelWidth))
		if focused {
			label = styleAccent.Render(pad(field.label, labelWidth))
		}

		var value string
		switch field.kind {
		case fieldToggle:
			value = toggleView(*field.flag, focused)
		default:
			field.input.Width = width - labelWidth - 3
			value = field.input.View()
		}

		rows = append(rows, label+value)

		switch {
		case field.err != "":
			rows = append(rows, strings.Repeat(" ", labelWidth)+styleError.Render(field.err))
		case focused && field.hint != "":
			rows = append(rows, strings.Repeat(" ", labelWidth)+styleFaint.Render(truncate(field.hint, width-labelWidth)))
		default:
			rows = append(rows, "")
		}
	}

	if f.err != "" {
		rows = append(rows, "", styleError.Render(f.err))
	}

	if len(rows) > height {
		rows = rows[:height]
	}
	return joinLines(rows...)
}

// toggleView renders a yes/no field.
//
// The mark comes from the glyph set, so a graphical session gets a real toggle
// and the console fallback gets [x] / [ ] — the shape reads the same either
// way, and neither draws a box the font cannot render.
func toggleView(on, focused bool) string {
	mark, word := g.ToggleOff, "No"
	if on {
		mark, word = g.Toggle, "Yes"
	}

	style := styleText
	if focused {
		style = styleAccent
	}
	if !on {
		style = style.Faint(true)
	}
	return style.Render(mark + "  " + word)
}

func capitalise(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
