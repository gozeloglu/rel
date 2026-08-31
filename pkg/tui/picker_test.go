package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func newTestPicker(manage bool) *pickerModel {
	return newPickerModel("Profiles", "Choose the profile to use", []PickerItem{
		{Label: "getir-payments", Description: "Getir/payment-integrations (org) · dev → master"},
		{Label: "personal", Description: "gozeloglu (user) · main → main"},
	}, manage)
}

func feedPicker(t *testing.T, m *pickerModel, msgs ...tea.Msg) *pickerModel {
	t.Helper()
	var model tea.Model = m
	for _, msg := range msgs {
		model, _ = model.Update(msg)
	}
	return model.(*pickerModel)
}

func TestPickerEnterSelectsHighlighted(t *testing.T) {
	m := feedPicker(t, newTestPicker(true),
		tea.KeyMsg{Type: tea.KeyDown},
		tea.KeyMsg{Type: tea.KeyEnter},
	)
	if !m.done || m.action != PickSelected {
		t.Fatalf("expected selection, done=%v action=%v", m.done, m.action)
	}
	if m.items[m.currentIndex()].Label != "personal" {
		t.Fatalf("wrong item selected: %s", m.items[m.currentIndex()].Label)
	}
}

func TestPickerFilterIsVisibleAndMatchesDescription(t *testing.T) {
	m := feedPicker(t, newTestPicker(true),
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")},
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("u")},
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")},
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")},
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")},
	)

	view := m.View()
	if !strings.Contains(view, "Filter: user") {
		t.Fatalf("filter text not visible:\n%s", view)
	}
	// "(user)" only appears in the personal profile's description.
	if len(m.visible) != 1 || m.items[m.visible[0]].Label != "personal" {
		t.Fatalf("description should be searchable, visible=%v", m.visible)
	}
}

func TestPickerEscapeClearsFilterThenAborts(t *testing.T) {
	m := feedPicker(t, newTestPicker(true),
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")},
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")},
		tea.KeyMsg{Type: tea.KeyEsc},
	)
	if m.aborted {
		t.Fatal("first escape should only stop typing")
	}

	m = feedPicker(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.aborted || m.filter != "" {
		t.Fatalf("second escape should clear the filter, aborted=%v filter=%q", m.aborted, m.filter)
	}

	m = feedPicker(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if !m.aborted {
		t.Fatal("third escape should abort")
	}
}

func TestPickerManageShortcuts(t *testing.T) {
	for _, tc := range []struct {
		key  string
		want PickerAction
	}{
		{"n", PickNew},
		{"e", PickEdit},
		{"d", PickDelete},
	} {
		m := feedPicker(t, newTestPicker(true), tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tc.key)})
		if !m.done || m.action != tc.want {
			t.Fatalf("key %q: done=%v action=%v want %v", tc.key, m.done, m.action, tc.want)
		}
	}
}

func TestPickerManageShortcutsDisabled(t *testing.T) {
	m := feedPicker(t, newTestPicker(false), tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	if m.done {
		t.Fatal("management shortcuts must be inert when disabled")
	}
	if strings.Contains(m.View(), "n new") {
		t.Fatal("help should not advertise disabled shortcuts")
	}
}

func TestPickerSpaceIsNotDuplicatedInFilter(t *testing.T) {
	m := feedPicker(t, newTestPicker(true),
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")},
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("dev")},
		tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")},
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("→")},
	)
	if m.filter != "dev →" {
		t.Fatalf("filter = %q, want %q", m.filter, "dev →")
	}
	if len(m.visible) != 1 {
		t.Fatalf("expected the acme profile to match, got %v", m.visible)
	}
}

func TestPickerNewWithoutMatchesReportsNoIndex(t *testing.T) {
	m := feedPicker(t, newTestPicker(true),
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")},
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("zzz")},
		tea.KeyMsg{Type: tea.KeyEsc},
		tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")},
	)
	if !m.done || m.action != PickNew {
		t.Fatalf("expected PickNew, done=%v action=%v", m.done, m.action)
	}
	if m.currentIndex() != -1 {
		t.Fatalf("expected no highlighted item, got %d", m.currentIndex())
	}
}
