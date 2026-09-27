package ui

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jaykeraliya0/cairn/installer/apply"
	"github.com/jaykeraliya0/cairn/installer/sys"
)

// InstallLogPath is where every line of the install is written. The live
// session's exit notice points users here.
const InstallLogPath = "/var/log/cairn-install.log"

// maxLogLines bounds what the log keeps. pacstrap alone emits tens of
// thousands of progress updates, and holding all of them would grow the
// installer's memory for no benefit — the live ISO runs entirely in RAM.
const maxLogLines = 2000

type (
	// logMsg is one line of output from a running command.
	logMsg string
	// stepMsg announces that a new install step has started.
	stepMsg struct {
		index, total int
		name         string
	}
	// percentMsg is a progress update from within the running step.
	percentMsg int
	// doneMsg reports the final outcome.
	doneMsg struct{ err error }
)

// noPercent is the percent field's value when the running step reports no
// progress of its own, which is every step but the two extractions.
const noPercent = -1

// installModel is the progress screen: the steps as a checklist, the current
// one spinning with its latest line of output beneath it, and the full command
// log a keypress away.
type installModel struct {
	spinner spinner.Model

	steps    []string
	index    int
	lastLine string
	// percent is how far the running step has got, or noPercent. Only the
	// image extraction reports it; see percentOf.
	percent int

	lines []string
	// logOffset is how many rows the log view is scrolled up from the newest
	// line. Zero follows the tail.
	logOffset int
	// logTotal is the row count at the last render, for clamping the scroll.
	logTotal int
	// logPath is shown on the failure screen, when the file opened.
	logPath string

	done   bool
	err    error
	reboot bool

	cancel func()
	// start is closed by Init, which is how the caller knows the program is
	// live and it is safe to begin sending messages into it.
	start    chan struct{}
	width    int
	height   int
	ready    bool
	quitting bool
}

func newInstallModel(steps []string, cancel func(), start chan struct{}) *installModel {
	s := spinner.New()
	// Frames come from the glyph set: braille in a graphical session, ASCII on
	// the console, where braille is not in the font.
	s.Spinner = spinner.Spinner{Frames: g.Spinner, FPS: time.Second / 10}
	s.Style = lipgloss.NewStyle().Foreground(colorAccent)

	return &installModel{
		spinner: s,
		steps:   steps,
		index:   -1,
		percent: noPercent,
		cancel:  cancel,
		start:   start,
	}
}

func (m *installModel) Init() tea.Cmd {
	close(m.start)
	return m.spinner.Tick
}

func (m *installModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height, m.ready = msg.Width, msg.Height, true
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			if m.done {
				return m, tea.Quit
			}
			// Cancelling mid-install kills the running command; apply's
			// cleanup then unmounts the target so a retry can start fresh.
			if !m.quitting {
				m.quitting = true
				m.cancel()
			}
			return m, nil
		case "q":
			if m.done {
				return m, tea.Quit
			}
		case "r":
			if m.done && m.err == nil {
				m.reboot = true
				return m, tea.Quit
			}
		case "up", "k":
			m.scroll(1)
		case "down", "j":
			m.scroll(-1)
		case "pgup":
			m.scroll(10)
		case "pgdown":
			m.scroll(-10)
		case "home", "g":
			m.logOffset = m.logTotal
		case "end", "G":
			m.logOffset = 0
		}
		return m, nil

	case logMsg:
		m.appendLine(string(msg))
		return m, nil

	case percentMsg:
		m.percent = int(msg)
		return m, nil

	case stepMsg:
		m.index = msg.index
		m.lastLine = ""
		m.percent = noPercent
		return m, nil

	case doneMsg:
		m.done, m.err = true, msg.err
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m *installModel) appendLine(line string) {
	m.lines = append(m.lines, line)
	if len(m.lines) > maxLogLines {
		m.lines = m.lines[len(m.lines)-maxLogLines:]
	}
	// The checklist shows only the newest line, and a bare command echo says
	// less about progress than the output that follows it.
	if trimmed := strings.TrimSpace(line); trimmed != "" && !strings.HasPrefix(trimmed, "$ ") {
		m.lastLine = trimmed
	}
}

// scroll moves the log view by delta rows, up being positive.
func (m *installModel) scroll(delta int) {
	m.logOffset += delta
	if m.logOffset < 0 {
		m.logOffset = 0
	}
	if m.logOffset > m.logTotal {
		m.logOffset = m.logTotal
	}
}

func (m *installModel) View() string {
	if !m.ready {
		return ""
	}

	l := m.layout()
	rows := []string{
		m.progress(l.CardWidth),
	}
	// On failure, say where the whole log is before anything else: the view
	// holds only recent lines, and it is gone once the user quits to a shell.
	if m.done && m.err != nil && m.logPath != "" {
		rows = append(rows, styleLabel.Render("Full log: ")+styleText.Render(m.logPath))
	} else {
		rows = append(rows, "")
	}
	rows = append(rows, m.checklist(l.CardWidth, m.checklistRows(l))...)
	rows = append(rows, rule(l.CardWidth))
	rows = append(rows, m.logTail(l.CardWidth, m.logRows(l))...)

	// No step list during the install: there are no steps left to choose, and
	// the card wants every column it can get for the command output.
	return renderScreen(l, 0, 0, nil, m.icon(), m.title(), joinLines(rows...), m.footer())
}

// layout gives the install screen the whole terminal.
//
// It carries no step list and no wizard progress, so it takes the width the
// sidebar would have used and the height the header's second and third rows
// would have. That space goes to the command log: this is the screen the user
// stares at for ten minutes, and it is the only place the install says what it
// is actually doing.
func (m *installModel) layout() layout {
	l := layoutFor(m.width, m.height)
	if l.Sidebar {
		l.Sidebar = false
		l.CardWidth += sidebarWidth + gutter
		if l.CardWidth > maxCardWidth+sidebarWidth {
			l.CardWidth = maxCardWidth + sidebarWidth
		}
	}

	// The wordmark and tagline, a blank line, the card's own four rows of
	// chrome, a blank line, the footer, and a row of padding at each end.
	if rows := m.height - logoHeight - 1 - 4 - 1 - 1 - 2; rows > l.CardHeight {
		l.CardHeight = rows
	}
	return l
}

// checklistRows and logRows split the card between the steps and the log. The
// steps take what they need; the log takes everything else.
func (m *installModel) checklistRows(l layout) int {
	// Once something has failed, the steps that went fine are not what anyone
	// is reading. Keep the failed step on screen and give the log the rest.
	if m.done && m.err != nil {
		return 1
	}
	rows := len(m.steps) + 1 // the current step's output line
	if max := l.CardHeight - minLogRows - 3; rows > max {
		rows = max
	}
	if rows < 3 {
		rows = 3
	}
	return rows
}

// minLogRows is the floor on the log. Below about this it stops being a log
// and becomes a flicker.
const minLogRows = 8

func (m *installModel) logRows(l layout) int {
	rows := l.CardHeight - m.checklistRows(l) - 3 // progress, blank, rule
	if rows < 2 {
		rows = 2
	}
	return rows
}

// icon is the card's glyph, which doubles as the outcome at the end.
func (m *installModel) icon() string {
	switch {
	case m.done && m.err != nil:
		return g.Failed
	case m.done:
		return g.Done
	default:
		return g.Disk
	}
}

func (m *installModel) title() string {
	switch {
	case m.done && m.err != nil:
		return "Installation failed"
	case m.done:
		return "Cairn is installed"
	case m.quitting:
		return "Cancelling"
	default:
		return "Installing Cairn"
	}
}

// progress renders the percentage bar at the top of the card.
func (m *installModel) progress(width int) string {
	done := m.index
	if m.done && m.err == nil {
		done = len(m.steps)
	}
	if done < 0 {
		done = 0
	}

	// A step's own progress counts as a fraction of one step, so the bar keeps
	// moving through the image extraction — the one step that takes minutes.
	scaled := done * 100
	if !m.done && m.index >= 0 && m.percent >= 0 {
		scaled += m.percent
	}

	percent := 0
	if len(m.steps) > 0 {
		percent = scaled / len(m.steps)
	}
	if percent > 100 {
		percent = 100
	}

	bar := width - 6
	if bar < 10 {
		bar = 10
	}
	filled := bar * percent / 100

	style := styleAccent
	if m.done && m.err != nil {
		style = styleError
	} else if m.done {
		style = styleSuccess
	}

	return style.Render(strings.Repeat("█", filled)) +
		styleFaint.Render(strings.Repeat("░", bar-filled)) +
		styleLabel.Render(" "+pad(itoa(percent)+"%", 4))
}

func (m *installModel) footer() string {
	scroll := []string{"↑↓ pgup pgdn", "scroll"}
	if m.logOffset > 0 {
		scroll = []string{"↑↓", "scroll", "end", "follow the log"}
	}
	switch {
	case m.done && m.err != nil:
		return hints(append(scroll, "q", "quit to a shell")...)
	case m.done:
		return hints(append(scroll, "r", "reboot now", "q", "quit to a shell")...)
	case m.quitting:
		return styleFaint.Render("unmounting the target…")
	default:
		return hints(append(scroll, "ctrl+c", "abort")...)
	}
}

// checklist renders the steps, windowed to what fits.
func (m *installModel) checklist(width, height int) []string {
	rows := make([]string, 0, len(m.steps)+1)
	focus := 0

	for i, name := range m.steps {
		switch {
		case i < m.index || (m.done && m.err == nil):
			rows = append(rows, styleStepDone.Render(g.Done)+" "+styleLabel.Render(name))
		case i == m.index && m.done && m.err != nil:
			focus = len(rows)
			rows = append(rows, styleError.Render(g.Failed)+" "+styleError.Render(name))
		case i == m.index:
			focus = len(rows)
			row := m.spinner.View() + " " + styleText.Render(name)
			if m.percent >= 0 {
				row += "  " + styleLabel.Render(itoa(m.percent)+"%")
			}
			rows = append(rows, row)
			if m.lastLine != "" {
				rows = append(rows, "    "+styleFaint.Render(truncate(m.lastLine, width-4)))
			}
		default:
			rows = append(rows, styleStepPending.Render(g.Pending)+" "+styleStepPending.Render(name))
		}
	}

	out := window(rows, height, focus)
	for len(out) < height {
		out = append(out, "")
	}
	return out
}

// logTail renders the command log under the checklist.
//
// Lines are wrapped, never cut: the part of an error message past the edge of
// the card is usually the part that says what went wrong. The view follows the
// newest line unless the user has scrolled up.
func (m *installModel) logTail(width, height int) []string {
	var rows []string
	for _, line := range m.lines {
		style := logStyle(line)
		for _, part := range hardWrap(strings.ReplaceAll(line, "\t", "    "), width) {
			rows = append(rows, style.Render(part))
		}
	}

	// Clamped here because only a render knows how many rows the wrapped log
	// has at the current width.
	m.logTotal = len(rows) - height
	if m.logTotal < 0 {
		m.logTotal = 0
	}
	if m.logOffset > m.logTotal {
		m.logOffset = m.logTotal
	}

	end := len(rows) - m.logOffset
	start := end - height
	if start < 0 {
		start = 0
	}

	out := append([]string(nil), rows[start:end]...)
	for len(out) < height {
		out = append(out, "")
	}
	return out
}

// errorLine picks out the lines worth drawing in red. It is a heuristic over
// merged stdout and stderr — the two are read from one pipe precisely so that
// the log keeps their true order, which leaves the words as the only signal.
var errorLine = regexp.MustCompile(`(?i)(^|[^a-z])(error|fatal|failed|failure|cannot|denied|unable to|not found|no such file)([^a-z]|$)`)

// logStyle colours a log line by what it is: a command the installer ran, a
// step heading, a line that reads like an error, or ordinary output.
func logStyle(line string) lipgloss.Style {
	trimmed := strings.TrimSpace(line)
	switch {
	case strings.HasPrefix(trimmed, "$ "):
		return styleAccent
	// Errors before headings: pacstrap and makepkg report failures as
	// "==> ERROR: ...", which would otherwise be dressed as a step heading.
	case errorLine.MatchString(trimmed):
		return styleError
	case strings.HasPrefix(trimmed, "==> "):
		return styleHeading
	default:
		// Output is read closely when something breaks, so it sits at the
		// label tier, not the muted one.
		return styleLabel
	}
}

// hardWrap splits a line into rows of at most width runes, keeping every
// character. Logs are not prose: a path or a package filename has no spaces to
// break at, and a word-wrapper would let it run off the edge.
func hardWrap(s string, width int) []string {
	runes := []rune(s)
	if width < 1 || len(runes) <= width {
		return []string{s}
	}
	var out []string
	for len(runes) > width {
		out = append(out, string(runes[:width]))
		runes = runes[width:]
	}
	return append(out, string(runes))
}

// percentOf reports the progress percentage a line of output carries, or
// noPercent when it carries none.
//
// `unsquashfs -percentage` — which apply uses for both extraction passes —
// writes a bare integer per line rather than a progress bar: "1", "2", "3", up
// to "100". Left as log lines they are a hundred rows of noise in the longest
// step of an install, and the step's own output line, which shows the newest
// line, becomes a naked number. Nothing else an install runs prints a bare
// integer on a line of its own.
func percentOf(line string) int {
	s := strings.TrimSpace(line)
	if s == "" || len(s) > 3 {
		return noPercent
	}
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return noPercent
		}
		n = n*10 + int(r-'0')
	}
	if n > 100 {
		return noPercent
	}
	return n
}

// progressLog thins those percentages down to what the log *file* should keep.
//
// The screen shows progress live on the step's row, so the file needs only
// enough to say how far an extraction got before something went wrong. A tenth
// of them is that; a hundred is noise.
type progressLog struct{ logged int }

func newProgressLog() *progressLog { return &progressLog{logged: noPercent} }

// worthLogging reports whether a percentage should reach the log file, and
// records it when it should. A percentage lower than the last one is the second
// unsquashfs pass starting over, not progress going backwards.
func (p *progressLog) worthLogging(n int) bool {
	if n < p.logged {
		p.logged = noPercent
	}
	if p.logged >= 0 && n < 100 && n < p.logged+percentStride {
		return false
	}
	p.logged = n
	return true
}

// percentStride is how far the percentage has to move before the log file
// records it again.
const percentStride = 10

// wireLog routes every line of the install — the installer's own step
// headings, and every command it runs with all of that command's output — into
// one sink.
//
// Both halves have to be set. State.Log carries only the installer's own
// messages; the commands and their output go through the runner's Log, and an
// ExecRunner built without one silently discards all of it. That was the bug:
// the log showed the installer narrating itself and never a single command or
// error.
func wireLog(st *apply.State, sink func(string)) {
	st.Log = sink
	if runner, ok := st.Runner.(sys.ExecRunner); ok {
		runner.Log = sink
		st.Runner = runner
	}
}

// RunInstall drives apply.Run behind the progress screen.
//
// It returns whether the user asked to reboot, and the install's own error.
func RunInstall(ctx context.Context, st *apply.State) (reboot bool, err error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	steps := apply.Steps(st.Cfg)
	names := make([]string, len(steps))
	for i, step := range steps {
		names[i] = step.Name
	}

	start := make(chan struct{})
	m := newInstallModel(names, cancel, start)
	p := tea.NewProgram(m, tea.WithAltScreen())

	logFile, logErr := sys.OpenLogFile(InstallLogPath)
	if logErr == nil {
		m.logPath = InstallLogPath
		defer logFile.Close()
	}
	progress := newProgressLog()
	wireLog(st, func(line string) {
		// A bare percentage is progress, not output: it moves the step's row
		// and the bar, and reaches the file only every percentStride.
		if n := percentOf(line); n >= 0 {
			p.Send(percentMsg(n))
			if progress.worthLogging(n) {
				logFile.Write(itoa(n) + "%")
			}
			return
		}
		logFile.Write(line)
		p.Send(logMsg(line))
	})
	if logErr != nil {
		st.Log(fmt.Sprintf("warning: no log file (%v); this screen is the only record", logErr))
	}

	go func() {
		// Wait until the program is actually running before sending into it.
		<-start

		runErr := apply.Run(ctx, st, func(index, total int, name string) {
			p.Send(stepMsg{index: index, total: total, name: name})
		})
		if runErr != nil {
			// The error goes into the log itself, not only the model: it lands
			// in the file for anyone who quits to a shell, and it sits directly
			// under the command output that explains it.
			st.Log("error: " + runErr.Error())
			// A cancelled context is why we are here, so cleanup gets a live
			// one of its own — otherwise the unmount would be killed too.
			st.Cleanup(context.WithoutCancel(ctx))
		}
		p.Send(doneMsg{err: runErr})
	}()

	final, err := p.Run()
	if err != nil {
		return false, err
	}

	model, ok := final.(*installModel)
	if !ok {
		return false, fmt.Errorf("unexpected final model %T", final)
	}
	return model.reboot, model.err
}

// wrap breaks text to a width, for error messages that outrun the terminal.
func wrap(s string, width int) string {
	if width < 20 {
		width = 20
	}
	var out strings.Builder
	column := 0
	for i, field := range strings.Fields(s) {
		if column > 0 && column+1+len(field) > width {
			out.WriteString("\n")
			column = 0
		} else if i > 0 && column > 0 {
			out.WriteString(" ")
			column++
		}
		out.WriteString(field)
		column += len(field)
	}
	return out.String()
}
