// Package ui is the terminal front end: a guided wizard with a persistent step
// list, and the progress screen shown while apply runs.
package ui

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jaykeraliya0/cairn/installer/config"
	"github.com/jaykeraliya0/cairn/installer/locale"
	"github.com/jaykeraliya0/cairn/installer/sys"
)

// Choices is everything the UI discovered about the machine before asking the
// user anything: what it can offer, and what it should offer by default.
type Choices struct {
	Disks     []sys.Disk
	Timezones []string
	Locales   []locale.Locale
	Layouts   []locale.Layout

	GPUs      []sys.GPUVendor
	Microcode string

	DefaultTimezone string
	DefaultLocale   string
	DefaultKeymap   string
}

// step is one stage of the wizard.
type step struct {
	// name labels the step in the sidebar.
	name string
	// icon is the step's glyph in the sidebar and the card's title.
	icon string
	// value renders the answer so far, for the sidebar.
	value func() string
	// build makes the screen for this step. It is called on arrival, so a
	// step always opens against the current answers.
	build func() screen
	// skip drops the step entirely, e.g. graphics on a machine with no Nvidia.
	skip func() bool
}

// wizard is the root model: the steps, and whichever one is open.
type wizard struct {
	ch    Choices
	a     *answers
	steps []step

	index   int
	current screen
	layout  layout

	cfg       config.Config
	confirmed bool
	// relaunch is set when the keyboard step picked a layout the graphical
	// session cannot switch to from the inside; the session script restarts
	// the kiosk under it.
	relaunch bool
}

func newWizard(ch Choices) *wizard {
	a := defaultAnswers(ch)
	w := &wizard{ch: ch, a: a, steps: buildSteps(ch, a)}
	w.index = w.firstLive(0, 1, 0)
	w.open()
	return w
}

// live lists the indices of the steps that apply to this machine.
func (w *wizard) live() []int {
	var out []int
	for i, s := range w.steps {
		if s.skip == nil || !s.skip() {
			out = append(out, i)
		}
	}
	return out
}

// firstLive walks from an index in a direction until it lands on a live step,
// returning fallback when the walk runs off either end.
//
// The fallback matters: walking back from the first step runs off the front,
// and returning the out-of-range index instead would index the step slice with
// -1 on the next redraw.
func (w *wizard) firstLive(from, direction, fallback int) int {
	for i := from; i >= 0 && i < len(w.steps); i += direction {
		if s := w.steps[i]; s.skip == nil || !s.skip() {
			return i
		}
	}
	return fallback
}

// position is the current step's place among the live ones, 1-based.
func (w *wizard) position() (at, total int) {
	live := w.live()
	for i, index := range live {
		if index == w.index {
			return i + 1, len(live)
		}
	}
	return 1, len(live)
}

func (w *wizard) open() {
	w.current = w.steps[w.index].build()
}

func (w *wizard) Init() tea.Cmd { return nil }

func (w *wizard) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		w.layout = layoutFor(msg.Width, msg.Height)
		return w, nil

	case advanceMsg:
		if w.steps[w.index].name == "keyboard" && w.needsRelaunch() {
			w.relaunch = true
			return w, tea.Quit
		}
		if next := w.firstLive(w.index+1, 1, w.index); next != w.index {
			w.index = next
			w.open()
		}
		return w, nil

	case backMsg:
		if prev := w.firstLive(w.index-1, -1, w.index); prev != w.index {
			w.index = prev
			w.open()
		}
		return w, nil

	case installMsg:
		cfg, err := w.a.toConfig(w.ch)
		if err != nil {
			// Every screen validates as it goes, so this is a rule no single
			// screen can see. Bounce back rather than install something wrong.
			return w, nil
		}
		w.cfg, w.confirmed = cfg, true
		return w, tea.Quit

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return w, tea.Quit
		case "q":
			// Every screen in the wizard takes typed input — a search box or a
			// text field — so q belongs to the screen, not to the wizard.
			// Ctrl+C above is the way out.
		}
	}

	next, cmd := w.current.Update(msg)
	w.current = next
	return w, cmd
}

func (w *wizard) View() string {
	if w.layout.Width == 0 {
		return ""
	}

	at, total := w.position()
	return renderScreen(w.layout, at, total, w.sidebarRows(),
		w.steps[w.index].icon, w.current.Title(),
		w.current.Body(w.layout.CardWidth, w.layout.CardHeight),
		w.current.Footer())
}

// sidebarRows renders the step list: what is answered, what is left, and where
// the user is.
func (w *wizard) sidebarRows() []sidebarRow {
	live := w.live()
	rows := make([]sidebarRow, 0, len(live))

	seen := false
	for _, index := range live {
		s := w.steps[index]
		current := index == w.index
		if current {
			seen = true
		}

		value := ""
		if s.value != nil && (!current || true) {
			value = s.value()
		}
		rows = append(rows, sidebarRow{
			Name:    s.name,
			Icon:    s.icon,
			Value:   value,
			Current: current,
			Done:    !seen && !current,
		})
	}
	return rows
}

// needsRelaunch reports that the chosen keyboard layout is not the one this
// session is running under.
//
// Only the graphical session cares. cage fixes its layout at launch and cannot
// be told to change it, so everything typed afterwards — the disk encryption
// passphrase above all — would be captured under the wrong layout, and the
// boot prompt that asks for it uses the layout the user actually chose. Rather
// than silently store a passphrase that can never unlock the disk, the wizard
// stops here and asks the session to start again.
func (w *wizard) needsRelaunch() bool {
	if !Graphical() {
		return false
	}
	current := os.Getenv("CAIRN_XKB")
	if current == "" {
		current = "us"
	}
	return xkbFor(w.ch.Layouts, w.a.keymap) != current
}

// writeRelaunchRequest hands the session script the layout to restart under.
// A failure here is not fatal: the wizard simply carries on in the layout it
// has, which is what would have happened without this mechanism at all.
func (w *wizard) writeRelaunchRequest() error {
	body := xkbFor(w.ch.Layouts, w.a.keymap) + "\n" + w.a.keymap + "\n"
	return os.WriteFile(relaunchRequestPath, []byte(body), 0o644)
}

// relaunchRequestPath is the contract with /usr/local/bin/cairn-session.
const relaunchRequestPath = "/tmp/cairn-xkb.request"

// Collect runs the wizard and returns the resulting Config.
//
// The second return value is false when the user quit without installing, in
// which case nothing has been touched.
func Collect(ch Choices) (config.Config, bool, error) {
	model := newWizard(ch)

	final, err := tea.NewProgram(model, tea.WithAltScreen()).Run()
	if err != nil {
		return config.Config{}, false, err
	}

	finished, ok := final.(*wizard)
	if !ok {
		return config.Config{}, false, fmt.Errorf("unexpected final model %T", final)
	}
	if finished.relaunch {
		if err := finished.writeRelaunchRequest(); err != nil {
			return config.Config{}, false, fmt.Errorf("requesting a keyboard-layout restart: %w", err)
		}
		return config.Config{}, false, nil
	}
	if !finished.confirmed {
		return config.Config{}, false, nil
	}
	return finished.cfg, true, nil
}
