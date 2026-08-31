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

type selectorModel struct {
	title    string
	all      []string
	visible  []string
	selected map[string]bool
	notes    map[string]string

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
		// bubbletea reports space as KeySpace but still fills Runes.
		m.filter += string(msg.Runes)
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

		sb.WriteString(cursor + check + " " + highlightMatch(repo, m.filter, base))
		if note := m.notes[repo]; note != "" {
			sb.WriteString(noteStyle.Render("  " + note))
		}
		sb.WriteString("\n")
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
	return runSelector(newSelectorModel("Select Repositories", repos))
}

// SelectReposWithPreset shows the same screen with the given repositories
// already ticked, so a repeated pass keeps the previous choice. Preset entries
// that are no longer in the list are ignored.
func SelectReposWithPreset(repos []string, preset []string) ([]string, error) {
	return runSelector(newPresetSelectorModel("Select Repositories", repos, preset))
}

// newPresetSelectorModel builds a selector with preset ticked, ignoring preset
// entries that are no longer in the list so the counter cannot drift.
func newPresetSelectorModel(title string, repos []string, preset []string) *selectorModel {
	m := newSelectorModel(title, repos)

	known := make(map[string]bool, len(repos))
	for _, r := range repos {
		known[r] = true
	}
	for _, r := range preset {
		if known[r] {
			m.selected[r] = true
		}
	}

	return m
}

// RepoNote pairs a repository with a short annotation rendered next to its
// name, such as the release that triggered it.
type RepoNote struct {
	Repo string
	Note string
}

// ConfirmRepos shows the same multi-select screen with every entry already
// checked, so the user only has to uncheck what they want to leave out.
func ConfirmRepos(title string, items []RepoNote) ([]string, error) {
	repos := make([]string, 0, len(items))
	notes := make(map[string]string, len(items))
	for _, it := range items {
		repos = append(repos, it.Repo)
		notes[it.Repo] = it.Note
	}

	m := newSelectorModel(title, repos)
	m.notes = notes
	for _, r := range repos {
		m.selected[r] = true
	}

	return runSelector(m)
}

func runSelector(m *selectorModel) ([]string, error) {
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
