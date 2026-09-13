package ui

import tea "github.com/charmbracelet/bubbletea"

// screen is one step's content. It draws inside the card; the wizard owns the
// wordmark, the progress bar, the step list and the footer around it.
type screen interface {
	Update(tea.Msg) (screen, tea.Cmd)
	// Title is the card's heading.
	Title() string
	// Body renders the card's inner area at the given size.
	Body(width, height int) string
	// Footer is the key hints for this screen.
	Footer() string
}

// Navigation messages a screen emits to move the wizard.
type (
	// advanceMsg accepts the step and moves to the next one.
	advanceMsg struct{}
	// backMsg returns to the previous step.
	backMsg struct{}
	// installMsg starts the install; only the review screen sends it.
	installMsg struct{}
)

func advance() tea.Cmd { return func() tea.Msg { return advanceMsg{} } }
func back() tea.Cmd    { return func() tea.Msg { return backMsg{} } }
func start() tea.Cmd   { return func() tea.Msg { return installMsg{} } }
