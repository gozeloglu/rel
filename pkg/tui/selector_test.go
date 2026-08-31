package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func init() {
	// Keep test output deterministic regardless of the terminal.
	lipgloss.SetColorProfile(termenv.Ascii)
}

func keys(s string) []tea.KeyMsg {
	var msgs []tea.KeyMsg
	for _, r := range s {
		msgs = append(msgs, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return msgs
}

func feed(t *testing.T, m *selectorModel, msgs ...tea.Msg) *selectorModel {
	t.Helper()
	var model tea.Model = m
	for _, msg := range msgs {
		model, _ = model.Update(msg)
	}
	return model.(*selectorModel)
}

func newTestModel() *selectorModel {
	return newSelectorModel("Select Repositories",
		[]string{"payment-alpha", "payment-beta", "billing-gamma"})
}

func TestFilterTextIsVisibleWhileTyping(t *testing.T) {
	m := newTestModel()
	msgs := []tea.Msg{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")}}
	for _, k := range keys("alp") {
		msgs = append(msgs, k)
	}
	m = feed(t, m, msgs...)

	view := m.View()
	if !strings.Contains(view, "Filter: alp") {
		t.Fatalf("filter text not visible while typing, got:\n%s", view)
	}
	if !strings.Contains(view, "payment-alpha") || strings.Contains(view, "billing-gamma") {
		t.Fatalf("filter not applied, got:\n%s", view)
	}
}

func TestEscapeCancelsFilterWithoutAborting(t *testing.T) {
	m := newTestModel()
	msgs := []tea.Msg{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")}}
	for _, k := range keys("alp") {
		msgs = append(msgs, k)
	}
	// First escape stops typing, second one clears the filter.
	msgs = append(msgs, tea.KeyMsg{Type: tea.KeyEsc}, tea.KeyMsg{Type: tea.KeyEsc})
	m = feed(t, m, msgs...)

	if m.aborted {
		t.Fatal("model aborted while cancelling the filter")
	}
	if m.filter != "" {
		t.Fatalf("filter not cleared, got %q", m.filter)
	}

	view := m.View()
	for _, repo := range []string{"payment-alpha", "payment-beta", "billing-gamma"} {
		if !strings.Contains(view, repo) {
			t.Fatalf("expected %q back in the list, got:\n%s", repo, view)
		}
	}
}

func TestBackspaceEditsFilter(t *testing.T) {
	m := newTestModel()
	msgs := []tea.Msg{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")}}
	for _, k := range keys("alpx") {
		msgs = append(msgs, k)
	}
	m = feed(t, m, msgs...)
	if len(m.visible) != 0 {
		t.Fatalf("expected no matches for %q", m.filter)
	}
	if !strings.Contains(m.View(), "No repositories match") {
		t.Fatalf("expected empty-state message, got:\n%s", m.View())
	}

	m = feed(t, m, tea.KeyMsg{Type: tea.KeyBackspace})
	if len(m.visible) != 1 || m.visible[0] != "payment-alpha" {
		t.Fatalf("backspace did not restore matches, got %v", m.visible)
	}
}

func TestSelectionSurvivesFilterChanges(t *testing.T) {
	m := newTestModel()
	// Filter to "beta", toggle with tab while typing, then clear the filter.
	msgs := []tea.Msg{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")}}
	for _, k := range keys("beta") {
		msgs = append(msgs, k)
	}
	msgs = append(msgs,
		tea.KeyMsg{Type: tea.KeyTab},
		tea.KeyMsg{Type: tea.KeyEsc},
		tea.KeyMsg{Type: tea.KeyEsc},
	)
	m = feed(t, m, msgs...)

	got := m.result()
	if len(got) != 1 || got[0] != "payment-beta" {
		t.Fatalf("expected payment-beta selected, got %v", got)
	}
	if !strings.Contains(m.View(), "1 selected") {
		t.Fatalf("expected selection counter, got:\n%s", m.View())
	}
}

func TestEscapeOnFullListAborts(t *testing.T) {
	m := feed(t, newTestModel(), tea.KeyMsg{Type: tea.KeyEsc})
	if !m.aborted {
		t.Fatal("expected abort when escaping without an active filter")
	}
}

func TestEnterConfirmsSelection(t *testing.T) {
	m := feed(t, newTestModel(),
		tea.KeyMsg{Type: tea.KeySpace},
		tea.KeyMsg{Type: tea.KeyDown},
		tea.KeyMsg{Type: tea.KeySpace},
		tea.KeyMsg{Type: tea.KeyEnter},
	)
	if !m.confirmed || m.aborted {
		t.Fatalf("expected confirmation, confirmed=%v aborted=%v", m.confirmed, m.aborted)
	}
	got := m.result()
	if len(got) != 2 || got[0] != "payment-alpha" || got[1] != "payment-beta" {
		t.Fatalf("unexpected selection %v", got)
	}
}

func TestToggleAllAppliesToVisibleOnly(t *testing.T) {
	m := newTestModel()
	msgs := []tea.Msg{tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")}}
	for _, k := range keys("payment") {
		msgs = append(msgs, k)
	}
	msgs = append(msgs, tea.KeyMsg{Type: tea.KeyEsc}, tea.KeyMsg{Type: tea.KeyCtrlA})
	m = feed(t, m, msgs...)

	got := m.result()
	if len(got) != 2 {
		t.Fatalf("expected 2 payment repos selected, got %v", got)
	}
}

func TestScrollingKeepsCursorVisible(t *testing.T) {
	repos := make([]string, 0, 30)
	for i := 0; i < 30; i++ {
		repos = append(repos, "repo-"+string(rune('a'+i%26))+string(rune('0'+i/26)))
	}
	m := newSelectorModel("t", repos)
	m = feed(t, m, tea.KeyMsg{Type: tea.KeyEnd})

	if m.cursor != len(repos)-1 {
		t.Fatalf("cursor should be on last item, got %d", m.cursor)
	}
	if m.cursor < m.offset || m.cursor >= m.offset+m.visibleRows {
		t.Fatalf("cursor %d outside viewport [%d,%d)", m.cursor, m.offset, m.offset+m.visibleRows)
	}
	if !strings.Contains(m.View(), repos[len(repos)-1]) {
		t.Fatal("last repo not rendered after scrolling to the end")
	}
}

// bubbletea delivers space as KeySpace with Runes already set; appending it
// twice would make multi-word filters impossible.
func TestSpaceIsNotDuplicatedInFilter(t *testing.T) {
	m := newSelectorModel("t", []string{"my repo one", "my repo two", "other"})
	m = feed(t, m,
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")},
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("my")},
		tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")},
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("repo")},
	)

	if m.filter != "my repo" {
		t.Fatalf("filter = %q, want %q", m.filter, "my repo")
	}
	if len(m.visible) != 2 {
		t.Fatalf("expected 2 matches, got %v", m.visible)
	}
}
