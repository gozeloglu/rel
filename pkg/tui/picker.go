package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// PickerItem is one selectable row with an optional second line.
type PickerItem struct {
	Label       string
	Description string
}

// PickerAction is what the user asked for on the picker screen.
type PickerAction int

const (
	PickSelected PickerAction = iota
	PickNew
	PickDelete
	PickEdit
)

// PickerResult reports the chosen action and item.
type PickerResult struct {
	Action PickerAction
	Index  int
	Label  string
}

type pickerModel struct {
	title   string
	hint    string
	items   []PickerItem
	visible []int

	filter    string
	filtering bool

	cursor      int
	offset      int
	visibleRows int

	allowManage bool
	aborted     bool
	done        bool
	action      PickerAction
}

func newPickerModel(title, hint string, items []PickerItem, allowManage bool) *pickerModel {
	m := &pickerModel{
		title:       title,
		hint:        hint,
		items:       items,
		visibleRows: defaultVisibleRows,
		allowManage: allowManage,
	}
	m.applyFilter()
	return m
}

func (m *pickerModel) Init() tea.Cmd { return nil }

func (m *pickerModel) applyFilter() {
	current := m.currentIndex()

	m.visible = m.visible[:0]
	needle := strings.ToLower(m.filter)
	for i, item := range m.items {
		haystack := strings.ToLower(item.Label + " " + item.Description)
		if needle == "" || strings.Contains(haystack, needle) {
			m.visible = append(m.visible, i)
		}
	}

	m.cursor = 0
	for pos, idx := range m.visible {
		if idx == current {
			m.cursor = pos
			break
		}
	}
	m.clampView()
}

func (m *pickerModel) currentIndex() int {
	if m.cursor < 0 || m.cursor >= len(m.visible) {
		return -1
	}
	return m.visible[m.cursor]
}

func (m *pickerModel) clampView() {
	if len(m.visible) == 0 {
		m.cursor, m.offset = 0, 0
		return
	}
	m.cursor = min(max(m.cursor, 0), len(m.visible)-1)
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+m.visibleRows {
		m.offset = m.cursor - m.visibleRows + 1
	}
	m.offset = max(0, min(m.offset, max(0, len(m.visible)-m.visibleRows)))
}

func (m *pickerModel) move(delta int) {
	if len(m.visible) > 0 {
		m.cursor += delta
		m.clampView()
	}
}

func (m *pickerModel) finish(action PickerAction) (tea.Model, tea.Cmd) {
	m.action = action
	m.done = true
	return m, tea.Quit
}

func (m *pickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}

	if m.filtering {
		switch key.Type {
		case tea.KeyCtrlC:
			m.aborted = true
			return m, tea.Quit
		case tea.KeyEsc, tea.KeyEnter:
			m.filtering = false
		case tea.KeyBackspace:
			if m.filter != "" {
				runes := []rune(m.filter)
				m.filter = string(runes[:len(runes)-1])
				m.applyFilter()
			}
		case tea.KeyCtrlU:
			m.filter = ""
			m.applyFilter()
		case tea.KeyUp:
			m.move(-1)
		case tea.KeyDown:
			m.move(1)
		case tea.KeyRunes, tea.KeySpace:
			// bubbletea reports space as KeySpace but still fills Runes.
			m.filter += string(key.Runes)
			m.applyFilter()
		}
		return m, nil
	}

	switch key.Type {
	case tea.KeyCtrlC:
		m.aborted = true
		return m, tea.Quit
	case tea.KeyEsc:
		if m.filter != "" {
			m.filter = ""
			m.applyFilter()
			return m, nil
		}
		m.aborted = true
		return m, tea.Quit
	case tea.KeyEnter:
		if m.currentIndex() >= 0 {
			return m.finish(PickSelected)
		}
	case tea.KeyUp:
		m.move(-1)
	case tea.KeyDown:
		m.move(1)
	case tea.KeyRunes:
		switch string(key.Runes) {
		case "/":
			m.filtering = true
		case "k":
			m.move(-1)
		case "j":
			m.move(1)
		case "q":
			m.aborted = true
			return m, tea.Quit
		case "n":
			if m.allowManage {
				return m.finish(PickNew)
			}
		case "d":
			if m.allowManage && m.currentIndex() >= 0 {
				return m.finish(PickDelete)
			}
		case "e":
			if m.allowManage && m.currentIndex() >= 0 {
				return m.finish(PickEdit)
			}
		}
	}
	return m, nil
}

func (m *pickerModel) View() string {
	if m.done || m.aborted {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(titleStyle.Render(m.title))
	sb.WriteString("\n")
	if m.hint != "" {
		sb.WriteString(counterStyle.Render(m.hint))
		sb.WriteString("\n")
	}
	sb.WriteString("\n")

	if len(m.items) > m.visibleRows || m.filter != "" || m.filtering {
		sb.WriteString(m.filterLine())
		sb.WriteString("\n\n")
	}

	if len(m.visible) == 0 {
		sb.WriteString(warnStyle.Render(fmt.Sprintf("  Nothing matches %q\n", m.filter)))
	}

	end := min(m.offset+m.visibleRows, len(m.visible))
	for pos := m.offset; pos < end; pos++ {
		item := m.items[m.visible[pos]]

		cursor, base := "  ", normalItem
		if pos == m.cursor {
			cursor, base = cursorStyle.Render("❯ "), activeItem
		}

		sb.WriteString(cursor + highlightMatch(item.Label, m.filter, base) + "\n")
		if item.Description != "" {
			sb.WriteString("    " + counterStyle.Render(item.Description) + "\n")
		}
	}

	sb.WriteString("\n")
	switch {
	case m.filtering:
		sb.WriteString(helpStyle.Render("↑/↓ move · enter/esc apply filter · ctrl+u clear · ctrl+c quit"))
	case m.allowManage:
		sb.WriteString(helpStyle.Render("↑/↓ move · enter use · n new · e edit · d delete · / filter · ctrl+c quit"))
	default:
		sb.WriteString(helpStyle.Render("↑/↓ move · enter select · / filter · ctrl+c quit"))
	}

	return sb.String()
}

func (m *pickerModel) filterLine() string {
	switch {
	case m.filtering:
		return filterLabel.Render("Filter: ") + filterText.Render(m.filter) + cursorBlock.Render(" ") +
			filterHint.Render("   (enter/esc: done · ctrl+u: clear)")
	case m.filter != "":
		return filterLabel.Render("Filter: ") + filterText.Render(m.filter) +
			filterHint.Render("   (/: edit · esc: clear)")
	default:
		return filterHint.Render("Filter: (press '/' to search)")
	}
}

// Pick shows a single-select screen. When allowManage is true the new/edit/
// delete shortcuts are offered as well.
func Pick(title, hint string, items []PickerItem, allowManage bool) (PickerResult, error) {
	m := newPickerModel(title, hint, items, allowManage)

	final, err := tea.NewProgram(m).Run()
	if err != nil {
		return PickerResult{}, err
	}

	res := final.(*pickerModel)
	if res.aborted || !res.done {
		return PickerResult{}, ErrAborted
	}

	idx := res.currentIndex()
	out := PickerResult{Action: res.action, Index: idx}
	if idx >= 0 {
		out.Label = res.items[idx].Label
	}
	return out, nil
}
