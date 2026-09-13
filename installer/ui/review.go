package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/jaykeraliya0/cairn/installer/apply"
	"github.com/jaykeraliya0/cairn/installer/config"
)

// eraseWord is what the user types to confirm the wipe.
const eraseWord = "ERASE"

// review is the last screen before anything is written: everything that was
// chosen, and a typed confirmation of the one irreversible part.
//
// A typed word rather than a yes/no. Every other screen accepts Enter, so a
// user walking the wizard arrives here with Enter already in their fingers;
// asking for a word that cannot be hit by accident is the point.
type review struct {
	ch    Choices
	a     *answers
	input textinput.Model
	err   string
}

func newReview(ch Choices, a *answers) *review {
	input := textinput.New()
	input.Prompt = ""
	input.Placeholder = "type " + eraseWord + " to confirm"
	input.CharLimit = len(eraseWord)
	input.Focus()

	return &review{ch: ch, a: a, input: input}
}

func (r *review) Title() string { return "Review" }

func (r *review) Footer() string {
	return hints("type "+eraseWord, "to confirm", "enter", "install", "esc", "back", "ctrl+c", "quit")
}

func (r *review) Update(msg tea.Msg) (screen, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return r, nil
	}

	switch keyMsg.String() {
	case "esc":
		return r, back()
	case "enter":
		if !strings.EqualFold(strings.TrimSpace(r.input.Value()), eraseWord) {
			r.err = "Type " + eraseWord + " to confirm that " + r.a.disk + " will be erased."
			return r, nil
		}
		if _, err := r.a.toConfig(r.ch); err != nil {
			r.err = capitalise(err.Error())
			return r, nil
		}
		return r, start()
	}

	var cmd tea.Cmd
	r.input, cmd = r.input.Update(msg)
	r.err = ""
	return r, cmd
}

func (r *review) Body(width, height int) string {
	cfg, err := r.a.toConfig(r.ch)
	if err != nil {
		return styleError.Render(capitalise(err.Error()))
	}

	rows := []string{}
	for _, row := range summaryRows(cfg) {
		rows = append(rows, styleLabel.Render(pad(row[0], 14))+styleText.Render(truncate(row[1], width-14)))
	}

	rows = append(rows,
		"",
		styleError.Render(g.Warning+"  "+truncate(cfg.Disk+" will be erased completely. This cannot be undone.", width-3)),
		"",
		styleLabel.Render(pad("Confirm", 14))+styleAccent.Render(g.Prompt+" ")+r.input.View(),
	)
	if r.err != "" {
		rows = append(rows, strings.Repeat(" ", 14)+styleError.Render(truncate(r.err, width-14)))
	}

	if len(rows) > height {
		rows = rows[:height]
	}
	return joinLines(rows...)
}

// installSource says where the installed system comes from. With a system
// image on the ISO, the answer is always the same, and it needs no network.
func installSource() string {
	if apply.ImageAvailable(apply.DefaultImage) {
		return "the system image on this ISO (no network needed)"
	}
	return "missing — this ISO has no system image"
}

// summaryRows is the review table, also used by the done screen.
func summaryRows(cfg config.Config) [][2]string {
	encryption := "off"
	if cfg.Encrypt {
		encryption = "LUKS2"
	}
	compression := "off"
	if cfg.Compression {
		compression = "zstd"
	}
	drivers := "Intel and AMD included"
	if len(cfg.NvidiaPackages) > 0 {
		drivers += " · " + strings.Join(cfg.NvidiaPackages, " ")
	}
	if cfg.Microcode != "" {
		drivers = cfg.Microcode + " · " + drivers
	}

	return [][2]string{
		{"Disk", cfg.Disk},
		{"Encryption", encryption},
		{"Partitions", config.FormatSizeMiB(cfg.ESPSizeMiB) + " EFI · " +
			config.FormatSizeMiB(cfg.SwapSizeMiB) + " swap · " + compression},
		{"Keyboard", cfg.Keymap + "  (Hyprland: " + cfg.XKBLayout + ")"},
		{"Language", cfg.Locale},
		{"Timezone", cfg.Timezone},
		{"Hostname", cfg.Hostname},
		{"User", cfg.Username + "  (wheel, sudo)"},
		{"Drivers", drivers},
		{"Installs from", installSource()},
	}
}
