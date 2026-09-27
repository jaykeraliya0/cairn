package ui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/jaykeraliya0/cairn/installer/apply"
	"github.com/jaykeraliya0/cairn/installer/sys"
)

// TestWireLogDeliversCommandOutput is the regression test for the log that
// showed the installer's own messages and nothing else.
//
// main.go builds the runner as a bare sys.ExecRunner{}, with no Log. Setting
// only State.Log carried the "==> step" headings to the screen, while every
// command, all of its output and every error went to a nil function.
func TestWireLogDeliversCommandOutput(t *testing.T) {
	st := &apply.State{Runner: sys.ExecRunner{}}

	var got []string
	wireLog(st, func(line string) { got = append(got, line) })

	err := st.Runner.Run(context.Background(), sys.Cmd{
		Name: "sh",
		Args: []string{"-c", "echo package-output; echo 'error: target not found' >&2; exit 1"},
	})
	if err == nil {
		t.Fatal("the failing command returned nil")
	}

	joined := strings.Join(got, "\n")
	for _, want := range []string{"$ sh -c", "package-output", "error: target not found"} {
		if !strings.Contains(joined, want) {
			t.Errorf("the sink never received %q; got:\n%s", want, joined)
		}
	}

	// The installer's own messages have to reach the same sink.
	st.Log("==> Installing packages")
	if got[len(got)-1] != "==> Installing packages" {
		t.Errorf("State.Log did not reach the sink: %q", got[len(got)-1])
	}
}

func TestLogTailWrapsInsteadOfTruncating(t *testing.T) {
	m := newInstallModel([]string{"one"}, func() {}, make(chan struct{}))
	long := "error: failed to commit transaction (conflicting files) " +
		strings.Repeat("x", 120) + " THE-END-OF-THE-MESSAGE"
	m.appendLine(long)

	rendered := strings.Join(m.logTail(40, 10), "")
	plain := stripStyles(rendered)
	// Every character survives, including the tail past the card's edge.
	if !strings.Contains(strings.ReplaceAll(plain, " ", ""), strings.ReplaceAll(long, " ", "")) {
		t.Errorf("the wrapped log lost characters:\n%q", plain)
	}
	for _, row := range m.logTail(40, 10) {
		if w := printableWidth(row); w > 40 {
			t.Errorf("a log row is %d columns, wider than the 40 given", w)
		}
	}
}

func TestLogTailScrolls(t *testing.T) {
	m := newInstallModel([]string{"one"}, func() {}, make(chan struct{}))
	for i := 0; i < 100; i++ {
		m.appendLine("line " + itoa(i))
	}

	// Following the tail shows the newest lines.
	tail := stripStyles(strings.Join(m.logTail(60, 10), "\n"))
	if !strings.Contains(tail, "line 99") || strings.Contains(tail, "line 89") {
		t.Errorf("the tail is not the newest ten lines:\n%s", tail)
	}

	// Scrolled up five rows.
	m.scroll(5)
	view := stripStyles(strings.Join(m.logTail(60, 10), "\n"))
	if !strings.Contains(view, "line 94") || strings.Contains(view, "line 95") {
		t.Errorf("scrolling up five rows shows the wrong window:\n%s", view)
	}

	// Scrolling far past the top clamps to the oldest line.
	m.scroll(10000)
	top := stripStyles(strings.Join(m.logTail(60, 10), "\n"))
	if !strings.Contains(top, "line 0") {
		t.Errorf("scrolling to the top does not reach the first line:\n%s", top)
	}

	// And back down follows the tail again.
	m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	if m.logOffset != 0 {
		t.Errorf("end left the log scrolled by %d", m.logOffset)
	}
}

func TestLogStyle(t *testing.T) {
	cases := []struct {
		line string
		want string
	}{
		{"$ pacstrap -K /mnt base", "command"},
		{"==> Installing packages", "heading"},
		// pacstrap's own failure line wears the heading prefix.
		{"==> ERROR: Failed to install packages to new root", "error"},
		{"error: failed to commit transaction", "error"},
		{"sgdisk: cannot open /dev/vda: Permission denied", "error"},
		{"mount: /mnt: special device /dev/vda2 does not exist", "output"},
		{"installing hyprland (0.56.1-1)...", "output"},
		// "terror" must not read as an error: the match is on whole words.
		{"installing terrorbird-utils", "output"},
	}
	styles := map[string]string{
		"command": styleAccent.Render("x"),
		"heading": styleHeading.Render("x"),
		"error":   styleError.Render("x"),
		"output":  styleLabel.Render("x"),
	}
	for _, c := range cases {
		if got := logStyle(c.line).Render("x"); got != styles[c.want] {
			t.Errorf("logStyle(%q) is not the %s style", c.line, c.want)
		}
	}
}

func TestFailureGivesTheLogTheScreen(t *testing.T) {
	m := newInstallModel(make([]string, 12), func() {}, make(chan struct{}))
	m.Update(tea.WindowSizeMsg{Width: 110, Height: 38})
	l := m.layout()

	running := m.logRows(l)
	m.Update(doneMsg{err: context.Canceled})
	failed := m.logRows(l)

	if failed <= running {
		t.Errorf("the log has %d rows after a failure and %d while running; it should grow", failed, running)
	}
}

// stripStyles removes escape sequences so assertions can read rendered text.
func stripStyles(s string) string { return sys.StripANSI(s) }

// TestPercentOfRecognisesExtractionProgress pins what counts as a progress
// update. `unsquashfs -percentage` writes a bare integer per line; anything
// else an install prints is output and belongs in the log.
func TestPercentOfRecognisesExtractionProgress(t *testing.T) {
	for _, tc := range []struct {
		line string
		want int
	}{
		{"0", 0},
		{"47", 47},
		{"100", 100},
		{"  47  ", 47},
		{"101", noPercent},
		{"1234", noPercent},
		{"", noPercent},
		{"   ", noPercent},
		{"4x", noPercent},
		{"-1", noPercent},
		{"47%", noPercent},
		{"$ unsquashfs -f -d /mnt -percentage /usr/share/cairn/cairn-root.sfs", noPercent},
		{"created 200 files", noPercent},
		{"200 inodes (400 blocks) to write", noPercent},
	} {
		if got := percentOf(tc.line); got != tc.want {
			t.Errorf("percentOf(%q) = %d, want %d", tc.line, got, tc.want)
		}
	}
}

// TestProgressStaysOutOfTheLogLines is the regression test for an install log
// that was a column of bare numbers: the extraction's hundred percentage lines
// buried every other line and left the step's output row reading "47".
func TestProgressStaysOutOfTheLogLines(t *testing.T) {
	m := newInstallModel([]string{"Installing the system"}, func() {}, make(chan struct{}))
	m.Update(stepMsg{index: 0, total: 1})
	m.Update(logMsg("200 inodes (400 blocks) to write"))
	for n := 1; n <= 47; n++ {
		m.Update(percentMsg(n))
	}

	if len(m.lines) != 1 {
		t.Errorf("the log kept %d lines, want only the real one: %q", len(m.lines), m.lines)
	}
	if m.lastLine != "200 inodes (400 blocks) to write" {
		t.Errorf("the step's output row reads %q, want the last real line", m.lastLine)
	}
	if m.percent != 47 {
		t.Errorf("percent = %d, want 47", m.percent)
	}

	// The next step starts with no progress of its own.
	m.Update(stepMsg{index: 1, total: 2})
	if m.percent != noPercent {
		t.Errorf("percent = %d after a new step, want noPercent", m.percent)
	}
}

// TestProgressShowsOnTheRunningStepAndTheBar checks that the percentage is not
// merely swallowed: it has to appear somewhere, or a ten-minute step looks
// frozen.
func TestProgressShowsOnTheRunningStepAndTheBar(t *testing.T) {
	m := newInstallModel([]string{"Mounting target", "Installing the system"}, func() {}, make(chan struct{}))
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m.Update(stepMsg{index: 1, total: 2})

	before := stripStyles(m.progress(80))
	m.Update(percentMsg(50))

	if row := stripStyles(strings.Join(m.checklist(80, 4), "\n")); !strings.Contains(row, "50%") {
		t.Errorf("the running step's row does not carry the percentage:\n%s", row)
	}
	// One step of two done, that step half done: 75%.
	if after := stripStyles(m.progress(80)); after == before || !strings.Contains(after, "75%") {
		t.Errorf("the bar reads %q, want 75%% (was %q)", after, before)
	}
}

// TestProgressLogThinsTheFile checks what reaches cairn-install.log. The screen
// shows progress live, so the file wants only enough to say how far an
// extraction got — and both extraction passes count from zero again.
func TestProgressLogThinsTheFile(t *testing.T) {
	p := newProgressLog()

	var kept []int
	for n := 0; n <= 100; n++ {
		if p.worthLogging(n) {
			kept = append(kept, n)
		}
	}
	want := []int{0, 10, 20, 30, 40, 50, 60, 70, 80, 90, 100}
	if len(kept) != len(want) {
		t.Fatalf("a 0-100 sweep kept %v, want %v", kept, want)
	}
	for i, n := range want {
		if kept[i] != n {
			t.Fatalf("a 0-100 sweep kept %v, want %v", kept, want)
		}
	}

	// The second unsquashfs pass starts over rather than being mistaken for
	// progress that never moves again.
	if !p.worthLogging(1) {
		t.Error("a percentage below the last one did not restart the stride")
	}
}
