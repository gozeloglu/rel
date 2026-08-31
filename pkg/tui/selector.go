package tui

import (
	"errors"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ErrAborted is returned when the user quits a TUI screen without confirming.
var ErrAborted = errors.New("aborted by user")

const (
	minVisibleRows     = 3
	defaultVisibleRows = 12
	// Rows taken by the header, filter line, counter and help text.
	chromeRows = 7
)

var (
	titleStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	counterStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	filterLabel    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	filterText     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231"))
	filterHint     = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	cursorStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	checkedStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("42"))
	uncheckedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	activeItem     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("231"))
	normalItem     = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	matchStyle     = lipgloss.NewStyle().Bold(true).Underline(true).Foreground(lipgloss.Color("214"))
	helpStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	warnStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	cursorBlock    = lipgloss.NewStyle().Reverse(true)
)

type selectorModel struct {
	title    string
	all      []string
	visible  []string
	selected map[string]bool

	filter    string
	filtering bool

	cursor      int
	offset      int
	visibleRows int

	aborted   bool
	confirmed bool
}

func newSelectorModel(title string, repos []string) *selectorModel {
	m := &selectorModel{
		title:       title,
		all:         repos,
		selected:    make(map[string]bool, len(repos)),
		visibleRows: defaultVisibleRows,
	}
	m.applyFilter()
	return m
}

func (m *selectorModel) Init() tea.Cmd { return nil }

// applyFilter recomputes the visible rows, keeping the cursor on the same
// repository whenever it survives the new filter.
func (m *selectorModel) applyFilter() {
	current := m.current()

	if m.filter == "" {
		m.visible = m.all
	} else {
		needle := strings.ToLower(m.filter)
		matches := make([]string, 0, len(m.all))
		for _, r := range m.all {
			if strings.Contains(strings.ToLower(r), needle) {
				matches = append(matches, r)
			}
		}
		m.visible = matches
	}

	m.cursor = 0
	for i, r := range m.visible {
		if r == current {
			m.cursor = i
			break
		}
	}
	m.clampView()
}

func (m *selectorModel) current() string {
	if m.cursor < 0 || m.cursor >= len(m.visible) {
		return ""
	}
	return m.visible[m.cursor]
}

func (m *selectorModel) clampView() {
	if len(m.visible) == 0 {
		m.cursor, m.offset = 0, 0
		return
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor > len(m.visible)-1 {
		m.cursor = len(m.visible) - 1
	}
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+m.visibleRows {
		m.offset = m.cursor - m.visibleRows + 1
	}
	if maxOffset := len(m.visible) - m.visibleRows; m.offset > maxOffset {
		m.offset = max(0, maxOffset)
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

func (m *selectorModel) move(delta int) {
	if len(m.visible) == 0 {
		return
	}
	m.cursor += delta
	m.clampView()
}

func (m *selectorModel) toggleCurrent() {
	if repo := m.current(); repo != "" {
		m.selected[repo] = !m.selected[repo]
	}
}

// toggleAllVisible selects every visible repository, or deselects them all when
// they are already selected.
func (m *selectorModel) toggleAllVisible() {
	allSelected := len(m.visible) > 0
	for _, r := range m.visible {
		if !m.selected[r] {
			allSelected = false
			break
		}
	}
	for _, r := range m.visible {
		m.selected[r] = !allSelected
	}
}

func (m *selectorModel) selectedCount() int {
	n := 0
	for _, ok := range m.selected {
		if ok {
			n++
		}
	}
	return n
}

func (m *selectorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.visibleRows = max(minVisibleRows, msg.Height-chromeRows)
		if m.visibleRows > defaultVisibleRows {
			m.visibleRows = defaultVisibleRows
		}
		m.clampView()
		return m, nil

	case tea.KeyMsg:
		if m.filtering {
			return m.updateFiltering(msg)
		}
		return m.updateBrowsing(msg)
	}
	return m, nil
}

func (m *selectorModel) updateFiltering(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		m.aborted = true
		return m, tea.Quit
	case tea.KeyEsc:
		// Stop typing but keep the current filter applied.
		m.filtering = false
		return m, nil
	case tea.KeyEnter:
		m.filtering = false
		return m, nil
	case tea.KeyTab:
		m.toggleCurrent()
		return m, nil
	case tea.KeyBackspace:
		if m.filter != "" {
			runes := []rune(m.filter)
			m.filter = string(runes[:len(runes)-1])
			m.applyFilter()
		}
		return m, nil
	case tea.KeyUp:
		m.move(-1)
		return m, nil
	case tea.KeyDown:
		m.move(1)
		return m, nil
	case tea.KeyCtrlU:
		m.filter = ""
		m.applyFilter()
		return m, nil
	case tea.KeyRunes, tea.KeySpace:
		m.filter += string(msg.Runes)
		if msg.Type == tea.KeySpace {
			m.filter += " "
		}
		m.applyFilter()
		return m, nil
	}
	return m, nil
}

func (m *selectorModel) updateBrowsing(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		m.aborted = true
		return m, tea.Quit
	case tea.KeyEsc:
		if m.filter != "" {
			// Second escape clears the filter and shows the full list again.
			m.filter = ""
			m.applyFilter()
			return m, nil
		}
		m.aborted = true
		return m, tea.Quit
	case tea.KeyEnter:
		m.confirmed = true
		return m, tea.Quit
	case tea.KeyUp:
		m.move(-1)
		return m, nil
	case tea.KeyDown:
		m.move(1)
		return m, nil
	case tea.KeyPgUp:
		m.move(-m.visibleRows)
		return m, nil
	case tea.KeyPgDown:
		m.move(m.visibleRows)
		return m, nil
	case tea.KeyHome:
		m.move(-len(m.visible))
		return m, nil
	case tea.KeyEnd:
		m.move(len(m.visible))
		return m, nil
	case tea.KeyTab, tea.KeySpace:
		m.toggleCurrent()
		return m, nil
	case tea.KeyCtrlA:
		m.toggleAllVisible()
		return m, nil
	case tea.KeyRunes:
		switch string(msg.Runes) {
		case "/":
			m.filtering = true
		case "k":
			m.move(-1)
		case "j":
			m.move(1)
		case "g":
			m.move(-len(m.visible))
		case "G":
			m.move(len(m.visible))
		case "a":
			m.toggleAllVisible()
		case "q":
			m.aborted = true
			return m, tea.Quit
		}
		return m, nil
	}
	return m, nil
}

// highlightMatch underlines the filtered substring so it is obvious why a row
// is being shown.
func highlightMatch(repo, filter string, base lipgloss.Style) string {
	if filter == "" {
		return base.Render(repo)
	}
	idx := strings.Index(strings.ToLower(repo), strings.ToLower(filter))
	if idx < 0 {
		return base.Render(repo)
	}
	end := idx + len(filter)
	return base.Render(repo[:idx]) + matchStyle.Render(repo[idx:end]) + base.Render(repo[end:])
}

func (m *selectorModel) filterLine() string {
	switch {
	case m.filtering:
		return filterLabel.Render("Filter: ") + filterText.Render(m.filter) + cursorBlock.Render(" ") +
			filterHint.Render("   (enter/esc: done · ctrl+u: clear · tab: toggle)")
	case m.filter != "":
		return filterLabel.Render("Filter: ") + filterText.Render(m.filter) +
			filterHint.Render("   (/: edit · esc: clear)")
	default:
		return filterHint.Render("Filter: (press '/' to search)")
	}
}

func (m *selectorModel) View() string {
	if m.confirmed || m.aborted {
		return ""
	}

	var sb strings.Builder

	sb.WriteString(titleStyle.Render(m.title))
	sb.WriteString("\n")
	sb.WriteString(counterStyle.Render(fmt.Sprintf(
		"%d selected · showing %d of %d repositories",
		m.selectedCount(), len(m.visible), len(m.all))))
	sb.WriteString("\n\n")
	sb.WriteString(m.filterLine())
	sb.WriteString("\n\n")

	if len(m.visible) == 0 {
		sb.WriteString(warnStyle.Render(fmt.Sprintf("  No repositories match %q", m.filter)))
		sb.WriteString("\n")
	}

	end := min(m.offset+m.visibleRows, len(m.visible))
	for i := m.offset; i < end; i++ {
		repo := m.visible[i]

		cursor := "  "
		base := normalItem
		if i == m.cursor {
			cursor = cursorStyle.Render("❯ ")
			base = activeItem
		}

		check := uncheckedStyle.Render("[ ]")
		if m.selected[repo] {
			check = checkedStyle.Render("[✓]")
		}

		sb.WriteString(cursor + check + " " + highlightMatch(repo, m.filter, base) + "\n")
	}

	if len(m.visible) > m.visibleRows {
		sb.WriteString(counterStyle.Render(fmt.Sprintf("  … %d more (↑/↓ to scroll)\n",
			len(m.visible)-m.visibleRows)))
	}

	sb.WriteString("\n")
	if m.filtering {
		sb.WriteString(helpStyle.Render("↑/↓ move · tab toggle · enter/esc apply filter · ctrl+c quit"))
	} else {
		sb.WriteString(helpStyle.Render("↑/↓ move · space toggle · ctrl+a all · / filter · enter confirm · ctrl+c quit"))
	}

	return sb.String()
}

// result returns the selected repositories in their original order.
func (m *selectorModel) result() []string {
	out := make([]string, 0, len(m.selected))
	for _, r := range m.all {
		if m.selected[r] {
			out = append(out, r)
		}
	}
	return out
}

// SelectRepos shows a multi-select screen to choose repositories.
func SelectRepos(repos []string) ([]string, error) {
	m := newSelectorModel("Select Repositories", repos)

	final, err := tea.NewProgram(m).Run()
	if err != nil {
		return nil, err
	}

	res := final.(*selectorModel)
	if res.aborted {
		return nil, ErrAborted
	}
	return res.result(), nil
}
