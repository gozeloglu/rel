package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
)

func keyMsg(s string) tea.KeyMsg {
	if s == "esc" {
		return tea.KeyMsg{Type: tea.KeyEsc}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

// Cancelling the filter must return to the repository list instead of
// aborting the whole program.
func TestEscapeCancelsFilterWithoutAborting(t *testing.T) {
	var selected []string
	form := newRepoForm([]string{"alpha", "beta"}, &selected)

	var m tea.Model = form
	m, _ = m.Update(form.Init()())
	for _, k := range []string{"/", "a", "l", "esc", "esc"} {
		m, _ = m.Update(keyMsg(k))
	}

	f := m.(*huh.Form)
	if f.State == huh.StateAborted {
		t.Fatal("form aborted while cancelling the filter")
	}

	view := f.View()
	for _, repo := range []string{"alpha", "beta"} {
		if !strings.Contains(view, repo) {
			t.Fatalf("expected %q back in the list after cancelling filter, got:\n%s", repo, view)
		}
	}
}
