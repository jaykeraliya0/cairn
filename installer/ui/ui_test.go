package ui

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/jaykeraliya0/cairn/installer/config"
	"github.com/jaykeraliya0/cairn/installer/locale"
	"github.com/jaykeraliya0/cairn/installer/sys"
)

func testChoices() Choices {
	return Choices{
		Disks: []sys.Disk{
			{Path: "/dev/nvme0n1", Size: 512110190592, Model: "Samsung SSD 990", Transport: "nvme"},
			{Path: "/dev/sdb", Size: 2000398934016, Transport: "sata"},
		},
		Timezones: []string{"Asia/Kolkata", "Europe/London", "Europe/Paris", "UTC"},
		Locales: []locale.Locale{
			{Name: "en_US.UTF-8", GenLine: "en_US.UTF-8 UTF-8", Label: "English (United States)"},
			{Name: "de_DE.UTF-8", GenLine: "de_DE.UTF-8 UTF-8", Label: "German (Germany)"},
			{Name: "zu_ZA.UTF-8", GenLine: "zu_ZA.UTF-8 UTF-8", Label: "zu_ZA.UTF-8"},
		},
		Layouts: []locale.Layout{
			{Label: "English (US)", Keymap: "us", XKB: "us"},
			{Label: "German", Keymap: "de-latin1", XKB: "de"},
			{Label: "amiga-us", Keymap: "amiga-us", XKB: "us"},
		},
		GPUs:            []sys.GPUVendor{sys.GPUNvidia},
		Microcode:       "amd-ucode",
		DefaultTimezone: "Asia/Kolkata",
		DefaultLocale:   "en_US.UTF-8",
		DefaultKeymap:   "us",
	}
}

// send delivers a message and runs whatever command comes back, feeding its
// message in too.
//
// bubbletea does this in its own loop; a test that drops commands never sees
// an editor close, because closing is a command.
func send(m tea.Model, msg tea.Msg) {
	for depth := 0; depth < 10 && msg != nil; depth++ {
		_, cmd := m.Update(msg)
		if cmd == nil {
			return
		}
		msg = resolve(cmd)
	}
}

// resolve runs a command, giving up on any that does not answer promptly.
//
// Some commands are timers rather than work — the text cursor's blink sleeps
// half a second before reporting — and a test that waits for them spends
// minutes doing nothing. Only the immediate ones matter here, and the one that
// matters most is the editor closing.
func resolve(cmd tea.Cmd) tea.Msg {
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()

	select {
	case msg := <-done:
		return msg
	case <-time.After(10 * time.Millisecond):
		return nil
	}
}

// keys feeds a sequence of keystrokes into a model.
func keys(m tea.Model, sequence ...string) {
	for _, name := range sequence {
		var msg tea.KeyMsg
		switch name {
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		case "up":
			msg = tea.KeyMsg{Type: tea.KeyUp}
		case "down":
			msg = tea.KeyMsg{Type: tea.KeyDown}
		case "esc":
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		case "tab":
			msg = tea.KeyMsg{Type: tea.KeyTab}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(name)}
		}
		send(m, msg)
	}
}

// typeInto sends each character of a string as its own keystroke.
func typeInto(m tea.Model, text string) {
	for _, r := range text {
		keys(m, string(r))
	}
}

// ---- filterList ----------------------------------------------------------

func newTestList() *filterList {
	l := newFilterList([]listItem{
		{Label: "Asia/Kolkata", Value: "Asia/Kolkata"},
		{Label: "Europe/London", Value: "Europe/London"},
		{Label: "Europe/Paris", Value: "Europe/Paris"},
		{Label: "America/New_York", Value: "America/New_York"},
	})
	l.height = 3
	return l
}

func TestFilterListNarrowsOnSubstring(t *testing.T) {
	l := newTestList()
	if l.matches() != 4 {
		t.Fatalf("unfiltered list has %d matches, want 4", l.matches())
	}

	l.setFilter("euro")
	if l.matches() != 2 {
		t.Errorf("'euro' matched %d, want 2", l.matches())
	}

	// Case-insensitive, and matching anywhere in the string rather than only
	// at the start — "kolk" has to find "Asia/Kolkata".
	l.setFilter("KOLK")
	if l.matches() != 1 {
		t.Errorf("'KOLK' matched %d, want 1", l.matches())
	}
	if item, _ := l.selected(); item.Value != "Asia/Kolkata" {
		t.Errorf("selected %q, want Asia/Kolkata", item.Value)
	}

	l.setFilter("zzz")
	if l.matches() != 0 {
		t.Errorf("a query matching nothing gave %d matches", l.matches())
	}
	if _, ok := l.selected(); ok {
		t.Error("an empty list reported a selection")
	}
}

func TestFilterListKeepsCatalogueOrder(t *testing.T) {
	// Deliberately not ranked by relevance: these catalogues are already
	// ordered on purpose (English layouts first, common locales first), and a
	// list that reorders as you type is harder to scan.
	l := newTestList()
	l.setFilter("e")

	var got []string
	for _, index := range l.filtered {
		got = append(got, l.items[index].Label)
	}
	want := []string{"Europe/London", "Europe/Paris", "America/New_York"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("filtered order = %v, want %v", got, want)
	}
}

func TestFilterListKeepsTheSelectionAcrossAFilter(t *testing.T) {
	l := newTestList()
	l.selectValue("Europe/Paris")

	// Refining a query must not silently move the highlight to something else.
	l.setFilter("europe")
	if item, _ := l.selected(); item.Value != "Europe/Paris" {
		t.Errorf("selected %q after filtering, want Europe/Paris", item.Value)
	}

	// When the selection is filtered away, fall back to the first row.
	l.setFilter("asia")
	if item, _ := l.selected(); item.Value != "Asia/Kolkata" {
		t.Errorf("selected %q, want the first surviving row", item.Value)
	}
}

func TestFilterListMoveClamps(t *testing.T) {
	l := newTestList()

	// Clamping rather than wrapping: on a 553-entry list, jumping from the top
	// to the bottom is disorienting.
	l.move(-5)
	if l.cursor != 0 {
		t.Errorf("cursor = %d after moving up from the top, want 0", l.cursor)
	}
	l.move(100)
	if l.cursor != 3 {
		t.Errorf("cursor = %d after moving past the end, want 3", l.cursor)
	}

	// The window follows the cursor.
	if first, last := l.position(); first != 2 || last != 4 {
		t.Errorf("position = %d-%d, want 2-4", first, last)
	}
}

// ---- picker --------------------------------------------------------------

func TestPickerTypingFiltersImmediately(t *testing.T) {
	chosen := ""
	p := newPicker("Timezone", "", "zones", stringItems([]string{
		"Asia/Kolkata", "Europe/London", "Europe/Paris",
	}), "Europe/London", func(v string) { chosen = v })
	p.list.height = 5

	// No mode to enter first: characters go straight to the search box.
	typeInto(pickerModel{p}, "kolk")
	if p.list.matches() != 1 {
		t.Fatalf("typing 'kolk' left %d matches, want 1", p.list.matches())
	}

	keys(pickerModel{p}, "enter")
	if chosen != "Asia/Kolkata" {
		t.Errorf("committed %q, want Asia/Kolkata", chosen)
	}
}

func TestPickerEscapeCommitsNothing(t *testing.T) {
	chosen := ""
	p := newPicker("Timezone", "", "zones", stringItems([]string{"A", "B"}), "A",
		func(v string) { chosen = v })

	_, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("esc produced no command")
	}
	if _, ok := cmd().(backMsg); !ok {
		t.Errorf("esc produced %#v, want a step back", cmd())
	}
	if chosen != "" {
		t.Errorf("esc committed %q", chosen)
	}
}

// pickerModel adapts a picker to tea.Model for the keys helper.
type pickerModel struct{ p *picker }

func (m pickerModel) Init() tea.Cmd { return nil }
func (m pickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	_, cmd := m.p.Update(msg)
	return m, cmd
}
func (m pickerModel) View() string { return "" }

// ---- form ----------------------------------------------------------------

func TestFormSkipsHiddenFields(t *testing.T) {
	a := defaultAnswers(testChoices())
	f := encryptionForm(a).(*form)

	// Encryption off: only the toggle is in play.
	if got := len(f.visible()); got != 1 {
		t.Fatalf("%d visible fields with encryption off, want 1", got)
	}

	// Turning it on reveals the passphrase rows immediately, in the same
	// screen — that is what binding the fields by pointer buys.
	keys(formModel{f}, "y")
	if got := len(f.visible()); got != 3 {
		t.Fatalf("%d visible fields with encryption on, want 3", got)
	}
}

func TestFormValidationBlocksAdvancing(t *testing.T) {
	a := defaultAnswers(testChoices())
	a.username = ""
	f := userForm(a).(*form)

	// An empty username cannot be left behind.
	keys(formModel{f}, "enter")
	if f.cursor != 0 {
		t.Errorf("cursor moved to %d despite an invalid username", f.cursor)
	}
	if f.fields[0].err == "" {
		t.Error("no message was shown for the invalid username")
	}

	// Typing clears the complaint rather than leaving it stale.
	typeInto(formModel{f}, "jay")
	if f.fields[0].err != "" {
		t.Errorf("message %q survived a correction", f.fields[0].err)
	}
	keys(formModel{f}, "enter")
	if f.cursor == 0 {
		t.Error("a valid username did not advance the form")
	}
}

func TestFormMismatchedPasswordsAreCaught(t *testing.T) {
	a := defaultAnswers(testChoices())
	f := userForm(a).(*form)

	typeInto(formModel{f}, "jay")
	keys(formModel{f}, "enter")
	typeInto(formModel{f}, "hunter2")
	keys(formModel{f}, "enter")
	typeInto(formModel{f}, "hunter3")
	keys(formModel{f}, "enter")

	if f.fields[2].err == "" {
		t.Error("mismatched passwords were accepted")
	}
}

func TestFormClearsAnswersItHides(t *testing.T) {
	a := defaultAnswers(testChoices())
	a.encrypt = true
	a.passphrase = "typed-then-abandoned"
	a.passphrase2 = "typed-then-abandoned"

	f := encryptionForm(a).(*form)
	// Turn encryption back off and save.
	keys(formModel{f}, "n", "enter")

	// A passphrase left behind would encrypt a disk the user said not to.
	if a.passphrase != "" || a.passphrase2 != "" {
		t.Errorf("passphrase survived turning encryption off: %q", a.passphrase)
	}
}

type formModel struct{ f *form }

func (m formModel) Init() tea.Cmd { return nil }
func (m formModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	_, cmd := m.f.Update(msg)
	return m, cmd
}
func (m formModel) View() string { return "" }

// ---- wizard ----------------------------------------------------------------

func newTestWizard(t *testing.T) *wizard {
	t.Helper()
	w := newWizard(testChoices())
	w.Update(tea.WindowSizeMsg{Width: 110, Height: 38})
	return w
}

func TestWizardSkipsGraphicsWithoutNvidia(t *testing.T) {
	ch := testChoices()
	ch.GPUs = []sys.GPUVendor{sys.GPUAMD}

	w := newWizard(ch)
	for _, index := range w.live() {
		if w.steps[index].name == "graphics" {
			t.Fatal("the graphics step is offered on a machine with no Nvidia card")
		}
	}

	// The skipped step must not be counted in the progress either.
	_, total := w.position()
	if total != len(w.live()) {
		t.Errorf("progress counts %d steps but %d are live", total, len(w.live()))
	}
}

func TestWizardWalksForwardAndBack(t *testing.T) {
	w := newTestWizard(t)

	if at, _ := w.position(); at != 1 {
		t.Fatalf("started at step %d, want 1", at)
	}
	if w.steps[w.index].name != "keyboard" {
		t.Fatalf("first step is %q, want keyboard", w.steps[w.index].name)
	}

	keys(w, "enter")
	if w.steps[w.index].name != "language" {
		t.Errorf("after one step: %q, want language", w.steps[w.index].name)
	}

	keys(w, "esc")
	if w.steps[w.index].name != "keyboard" {
		t.Errorf("after going back: %q, want keyboard", w.steps[w.index].name)
	}

	// Back from the first step stays put rather than falling off the front.
	keys(w, "esc")
	if w.steps[w.index].name != "keyboard" {
		t.Errorf("back from the first step landed on %q", w.steps[w.index].name)
	}
}

func TestWizardStepSkippedOnTheWayBack(t *testing.T) {
	ch := testChoices()
	ch.GPUs = []sys.GPUVendor{sys.GPUAMD}
	w := newWizard(ch)
	w.Update(tea.WindowSizeMsg{Width: 110, Height: 38})

	// Land on review, then walk back one: graphics is skipped in both
	// directions, so this has to be the user step.
	w.index = len(w.steps) - 1
	w.open()
	keys(w, "esc")
	if got := w.steps[w.index].name; got != "user" {
		t.Errorf("back from review landed on %q, want user", got)
	}
}

func TestSidebarShowsAnswersAndPosition(t *testing.T) {
	w := newTestWizard(t)
	w.a.hostname = "workshop"
	w.index = 6 // hostname
	w.open()

	rows := w.sidebarRows()
	var current, hostname sidebarRow
	for _, row := range rows {
		if row.Current {
			current = row
		}
		if row.Name == "hostname" {
			hostname = row
		}
	}

	if current.Name != "hostname" {
		t.Errorf("the current row is %q, want hostname", current.Name)
	}
	// The value has to reach the sidebar, or it shows position but no answers.
	if hostname.Value != "workshop" {
		t.Errorf("hostname row shows %q, want workshop", hostname.Value)
	}

	// Steps already passed are marked done; later ones are not.
	for _, row := range rows {
		switch row.Name {
		case "keyboard", "language", "timezone", "disk", "encryption", "layout":
			if !row.Done {
				t.Errorf("%q should be marked done", row.Name)
			}
		case "user", "review":
			if row.Done {
				t.Errorf("%q should not be marked done yet", row.Name)
			}
		}
	}
}

func TestReviewRequiresTheTypedWord(t *testing.T) {
	w := newTestWizard(t)
	w.a.username = "jay"
	w.a.password = "hunter2"
	w.index = len(w.steps) - 1
	w.open()

	r, ok := w.current.(*review)
	if !ok {
		t.Fatalf("the last step is %T, want the review screen", w.current)
	}

	// A bare enter must not start an install.
	keys(w, "enter")
	if w.confirmed {
		t.Fatal("enter alone started the install")
	}
	if r.err == "" {
		t.Error("no message explained why nothing happened")
	}

	typeInto(w, eraseWord)
	keys(w, "enter")
	if !w.confirmed {
		t.Fatal("typing the word did not confirm")
	}
	if w.cfg.Username != "jay" || w.cfg.Disk != "/dev/nvme0n1" {
		t.Errorf("confirmed config = %+v", w.cfg)
	}
}

func TestReviewRefusesAnIncompleteConfig(t *testing.T) {
	w := newTestWizard(t)
	// No user was ever set.
	w.index = len(w.steps) - 1
	w.open()

	typeInto(w, eraseWord)
	keys(w, "enter")
	if w.confirmed {
		t.Fatal("an install started without a user account")
	}
}

// ---- settings ------------------------------------------------------------

func TestDefaultAnswersArePreFilled(t *testing.T) {
	ch := testChoices()
	a := defaultAnswers(ch)

	// Everything guessable is set, so the wizard is a walk of confirmations
	// with one screen that genuinely needs an answer.
	if a.disk != "/dev/nvme0n1" {
		t.Errorf("disk = %q, want the first offered disk", a.disk)
	}
	for name, value := range map[string]string{
		"timezone": a.timezone, "locale": a.locale, "keymap": a.keymap, "hostname": a.hostname,
	} {
		if value == "" {
			t.Errorf("%s was left empty", name)
		}
	}
	if a.username != "" || a.password != "" {
		t.Error("the user account was guessed, which it cannot be")
	}
}

func TestToConfig(t *testing.T) {
	ch := testChoices()
	a := defaultAnswers(ch)
	a.username = "jay"
	a.password = "hunter2"
	a.keymap = "de-latin1"
	a.locale = "de_DE.UTF-8"

	cfg, err := a.toConfig(ch)
	if err != nil {
		t.Fatalf("toConfig: %v", err)
	}

	if cfg.ESPSizeMiB != 1024 || cfg.SwapSizeMiB != 4096 {
		t.Errorf("sizes = %d/%d, want 1024/4096", cfg.ESPSizeMiB, cfg.SwapSizeMiB)
	}
	if cfg.LocaleGenLine != "de_DE.UTF-8 UTF-8" {
		t.Errorf("LocaleGenLine = %q", cfg.LocaleGenLine)
	}
	// Console keymap and Hyprland layout are different vocabularies.
	if cfg.Keymap != "de-latin1" || cfg.XKBLayout != "de" {
		t.Errorf("keymap/layout = %q/%q, want de-latin1/de", cfg.Keymap, cfg.XKBLayout)
	}
	// "same password for root" has to carry the user's password over.
	if cfg.RootPassword != "hunter2" {
		t.Errorf("RootPassword = %q, want the user's password", cfg.RootPassword)
	}
}

func TestToConfigIgnoresNvidiaWithoutACard(t *testing.T) {
	ch := testChoices()
	ch.GPUs = []sys.GPUVendor{sys.GPUAMD}
	a := defaultAnswers(ch)
	a.username, a.password = "jay", "hunter2"

	cfg, err := a.toConfig(ch)
	if err != nil {
		t.Fatalf("toConfig: %v", err)
	}
	// The Graphics row is hidden on this machine, so its default answer must
	// not install an Nvidia driver anyway.
	if cfg.Nvidia != config.NvidiaNone || cfg.UsesNvidia() {
		t.Errorf("Nvidia = %q on an AMD-only machine", cfg.Nvidia)
	}
	for _, pkg := range cfg.NvidiaPackages {
		if strings.HasPrefix(pkg, "nvidia") {
			t.Errorf("an AMD-only machine got %q", pkg)
		}
	}
}

func TestSidebarValuesAreShort(t *testing.T) {
	ch := testChoices()
	a := defaultAnswers(ch)
	a.username = "jay"

	// The step list has about a dozen columns for a value, so these have to be
	// codes and short phrases rather than the long labels the pickers show.
	want := map[string]string{
		"keyboard":   "us",
		"language":   "en_US.UTF-8",
		"timezone":   "Asia/Kolkata",
		"disk":       "nvme0n1",
		"encryption": "off",
		"layout":     "1 GiB + 4 GiB",
		"hostname":   "cairn",
		"user":       "jay",
		"graphics":   "nvidia-open",
	}
	for _, s := range buildSteps(ch, a) {
		if s.value == nil {
			continue
		}
		if expected, ok := want[s.name]; ok && s.value() != expected {
			t.Errorf("%s shows %q, want %q", s.name, s.value(), expected)
		}
	}
}

// ---- layout --------------------------------------------------------------

func TestLayoutFor(t *testing.T) {
	// A wide terminal gets the step list beside a capped card.
	wide := layoutFor(120, 40)
	if !wide.Sidebar {
		t.Error("a 120-column terminal should get the step list")
	}
	if wide.CardWidth > maxCardWidth {
		t.Errorf("card is %d columns, wider than the %d cap", wide.CardWidth, maxCardWidth)
	}

	// An 80-column console cannot fit both, so the card takes the width and
	// the header's progress bar carries the orientation alone.
	narrow := layoutFor(80, 24)
	if narrow.Sidebar {
		t.Error("an 80-column console should drop the step list, not squeeze it")
	}
	if narrow.CardWidth < 40 {
		t.Errorf("card is only %d columns on an 80-column console", narrow.CardWidth)
	}

	// Nothing may compute a negative or absurd size.
	for _, size := range [][2]int{{80, 24}, {80, 48}, {120, 40}, {60, 18}, {40, 12}} {
		l := layoutFor(size[0], size[1])
		if l.CardWidth < 1 || l.CardHeight < 1 {
			t.Errorf("layoutFor(%d,%d) = %+v", size[0], size[1], l)
		}
	}
}

func TestScreenFitsTheTerminal(t *testing.T) {
	// The composition must never render taller or wider than the console it is
	// drawn on, at any size the ISO might boot into.
	for _, size := range [][2]int{{80, 24}, {80, 25}, {80, 48}, {100, 30}, {120, 40}, {132, 43}, {60, 20}} {
		width, height := size[0], size[1]
		l := layoutFor(width, height)
		rows := []sidebarRow{{Name: "keyboard", Value: "us", Current: true}}

		got := lines(renderScreen(l, 3, 10, rows, g.Keyboard, "Timezone", "body", hints("enter", "continue")))
		if len(got) > height {
			t.Errorf("%dx%d rendered %d rows", width, height, len(got))
		}
		for i, line := range got {
			if n := printableWidth(line); n > width {
				t.Errorf("%dx%d row %d is %d columns wide", width, height, i, n)
			}
		}
	}
}

func TestProgressBar(t *testing.T) {
	for _, at := range []int{0, 1, 5, 10} {
		bar := progressBar(80, at, 10)
		if !strings.Contains(bar, itoa(at)+"/10") {
			t.Errorf("bar at %d does not show the count: %q", at, bar)
		}
		if n := printableWidth(bar); n > 80 {
			t.Errorf("bar at %d is %d columns", at, n)
		}
	}
}

func lines(view string) []string {
	return strings.Split(strings.TrimRight(view, "\n"), "\n")
}

// printableWidth measures a rendered line, ignoring escape sequences.
func printableWidth(s string) int {
	var n, i int
	for i < len(s) {
		if s[i] == 0x1b {
			for i < len(s) && s[i] != 'm' {
				i++
			}
			i++
			continue
		}
		_, size := utf8.DecodeRuneInString(s[i:])
		i += size
		n++
	}
	return n
}

func TestTruncateAndPad(t *testing.T) {
	if got := truncate("short", 20); got != "short" {
		t.Errorf("truncate = %q, want it untouched", got)
	}
	if got := truncate("a much longer string", 10); len([]rune(got)) != 10 {
		t.Errorf("truncate = %q, want 10 runes", got)
	}
	if got := pad("ab", 5); got != "ab   " {
		t.Errorf("pad = %q", got)
	}
}

func TestWindowKeepsTheFocusedRowVisible(t *testing.T) {
	rows := make([]string, 12)
	for i := range rows {
		rows[i] = itoa(i)
	}

	if got := window(rows, 20, 3); len(got) != 12 {
		t.Errorf("window with room returned %d rows, want all 12", len(got))
	}
	if got := window(rows, 5, 11); got[4] != "11" {
		t.Errorf("window at the end = %v, want the last five", got)
	}
	if got := window(rows, 5, 0); got[0] != "0" {
		t.Errorf("window at the start = %v, want the first five", got)
	}
}

func TestVTPaletteMatchesTheStyles(t *testing.T) {
	seq := vtPaletteSequence()
	for slot, rgb := range vtPalette {
		if len(rgb) != 6 {
			t.Errorf("slot %d colour %q is not six hex digits", slot, rgb)
		}
		if !strings.Contains(seq, rgb) {
			t.Errorf("slot %d (%s) missing from the sequence", slot, rgb)
		}
	}
	// The console palette and the lipgloss colours describe the same theme; if
	// they drift, the installer looks different on a VT than in a terminal.
	if want := strings.TrimPrefix(string(colorAccent), "#"); vtPalette[3] != want {
		t.Errorf("slot 3 = %q, want the accent %q", vtPalette[3], want)
	}
	if want := strings.TrimPrefix(string(colorFg), "#"); vtPalette[7] != want {
		t.Errorf("slot 7 = %q, want the foreground %q", vtPalette[7], want)
	}
}

func TestLogo(t *testing.T) {
	if !strings.Contains(logo(68), "█") {
		t.Error("a wide column should get the wordmark")
	}
	if strings.Contains(logo(20), "█") {
		t.Error("a column narrower than the wordmark still got it")
	}

	// LogoWidth gates the console-font search, so a wordmark that outgrew the
	// constant would silently make fitConsoleFont reject every font.
	for i, line := range strings.Split(logoArt, "\n") {
		if width := len([]rune(line)); width > LogoWidth {
			t.Errorf("logoArt line %d is %d columns, wider than LogoWidth (%d)", i, width, LogoWidth)
		}
	}

	// Full, upper and lower block plus spaces. All three are CP437 and are in
	// every console font kbd ships; anything else is a bet on a font the user
	// did not choose.
	for _, r := range logoArt {
		switch r {
		case '█', '▀', '▄', ' ', '\n':
		default:
			t.Errorf("logoArt contains %q, which is not guaranteed on a console font", r)
		}
	}

	// Every row is shifted by the same amount, or the wordmark shears. The art
	// has its own uneven leading spaces, so compare against the source rows
	// rather than measuring indentation.
	art := strings.Split(logoArt, "\n")
	rendered := strings.Split(logo(68), "\n")
	if len(rendered) != len(art) {
		t.Fatalf("logo rendered %d rows, want %d", len(rendered), len(art))
	}
	shift := strings.Index(rendered[1], "█")
	if shift <= 0 {
		t.Fatalf("the wordmark was not centred: %q", rendered[1])
	}
	for i := range art {
		if want := strings.Repeat(" ", shift) + art[i]; rendered[i] != want {
			t.Errorf("row %d = %q, want it shifted by %d", i, rendered[i], shift)
		}
	}
}

// ---- glyphs ----------------------------------------------------------------

func TestGlyphSetsAreComplete(t *testing.T) {
	// A missing glyph renders as an empty string and silently collapses a
	// column, which is worse than an ugly one.
	for name, set := range map[string]glyphSet{"nerd": nerdGlyphs, "ascii": asciiGlyphs} {
		v := reflect.ValueOf(set)
		for i := 0; i < v.NumField(); i++ {
			field := v.Type().Field(i)
			switch value := v.Field(i); value.Kind() {
			case reflect.String:
				if value.String() == "" {
					t.Errorf("%s glyph set has no %s", name, field.Name)
				}
			case reflect.Slice:
				if value.Len() == 0 {
					t.Errorf("%s glyph set has no %s frames", name, field.Name)
				}
			}
		}
	}
}

func TestAsciiGlyphsStayInsideTheConsoleFont(t *testing.T) {
	// The console fallback runs where the font holds about 256 glyphs. Every
	// mark it draws has to be ASCII or CP437, or the VT shows blanks.
	cp437 := map[rune]bool{'█': true, '▀': true, '▄': true, '░': true, '▒': true,
		'▓': true, '·': true, '■': true, '▌': true, '▐': true}

	v := reflect.ValueOf(asciiGlyphs)
	for i := 0; i < v.NumField(); i++ {
		name := v.Type().Field(i).Name
		var runes []rune
		switch value := v.Field(i); value.Kind() {
		case reflect.String:
			runes = []rune(value.String())
		case reflect.Slice:
			for j := 0; j < value.Len(); j++ {
				runes = append(runes, []rune(value.Index(j).String())...)
			}
		}
		for _, r := range runes {
			if r > 126 && !cp437[r] {
				t.Errorf("ascii %s uses %q (U+%04X), which a console font may not have", name, r, r)
			}
		}
	}
}

func TestNerdGlyphsAreSingleWidth(t *testing.T) {
	// Every column of the layout assumes one cell per glyph. A double-width
	// icon would shear every box on the screen.
	v := reflect.ValueOf(nerdGlyphs)
	for i := 0; i < v.NumField(); i++ {
		value := v.Field(i)
		if value.Kind() != reflect.String {
			continue
		}
		if w := lipgloss.Width(value.String()); w != 1 {
			t.Errorf("nerd %s is %d cells wide, want 1", v.Type().Field(i).Name, w)
		}
	}
}

// ---- keyboard-layout relaunch ----------------------------------------------

func TestNeedsRelaunchOnlyWhenTheLayoutActuallyChanges(t *testing.T) {
	w := newTestWizard(t)

	t.Setenv("CAIRN_SESSION", "graphical")
	t.Setenv("CAIRN_XKB", "us")

	// The default layout is already the session's; restarting would be noise.
	w.a.keymap = "us"
	if w.needsRelaunch() {
		t.Error("asked for a restart when the layout had not changed")
	}

	// A German keymap maps to the `de` XKB layout, which cage is not running.
	w.a.keymap = "de-latin1"
	if !w.needsRelaunch() {
		t.Error("did not ask for a restart after the layout changed")
	}

	// On the console there is no cage to restart, and loadkeys applies live.
	t.Setenv("CAIRN_SESSION", "console")
	if w.needsRelaunch() {
		t.Error("asked for a restart on the plain console")
	}
}

func TestRelaunchRequestNamesBothLayouts(t *testing.T) {
	w := newTestWizard(t)
	w.a.keymap = "de-latin1"

	path := filepath.Join(t.TempDir(), "request")
	body := xkbFor(w.ch.Layouts, w.a.keymap) + "\n" + w.a.keymap + "\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	// The session script reads the XKB layout first, then the console keymap,
	// and passes them back as CAIRN_XKB and CAIRN_KEYMAP.
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := "de\nde-latin1\n"; string(got) != want {
		t.Errorf("request = %q, want %q", got, want)
	}
}
