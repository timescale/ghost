package common

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// keyMsg builds a synthetic KeyPressMsg whose String() matches the given
// keystroke. Letters and digits go through Text; special keys go through
// Code.
func keyMsg(s string) tea.KeyPressMsg {
	switch s {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	case " ":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "ctrl+c":
		return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	}
	if len(s) == 1 {
		return tea.KeyPressMsg{Code: rune(s[0]), Text: s}
	}
	panic("unsupported key: " + s)
}

func runKeys(m multiSelectModel, keys ...string) multiSelectModel {
	for _, k := range keys {
		updated, _ := m.Update(keyMsg(k))
		m = updated.(multiSelectModel)
	}
	return m
}

func sampleItems() []MultiSelectItem {
	return []MultiSelectItem{
		{Label: "First", Selected: true},
		{Label: "Second", Selected: false},
		{Label: "Third", Selected: true, Dimmed: true},
	}
}

func TestMultiSelectModel_CursorMovement(t *testing.T) {
	m := newMultiSelectModel("title", sampleItems())

	m = runKeys(m, "down", "down")
	if m.cursor != 2 {
		t.Fatalf("expected cursor at 2 after two downs, got %d", m.cursor)
	}
	m = runKeys(m, "down")
	if m.cursor != 2 {
		t.Fatalf("cursor should clamp at last index, got %d", m.cursor)
	}
	m = runKeys(m, "up", "up", "up")
	if m.cursor != 0 {
		t.Fatalf("cursor should clamp at first index, got %d", m.cursor)
	}

	m = runKeys(m, "j", "j")
	if m.cursor != 2 {
		t.Fatalf("expected j to move down, got %d", m.cursor)
	}
	m = runKeys(m, "k")
	if m.cursor != 1 {
		t.Fatalf("expected k to move up, got %d", m.cursor)
	}
}

func TestMultiSelectModel_ToggleSpace(t *testing.T) {
	m := newMultiSelectModel("title", sampleItems())
	m = runKeys(m, " ")
	if m.items[0].Selected {
		t.Fatalf("expected item 0 to be unselected after toggle")
	}
	m = runKeys(m, "down", " ")
	if !m.items[1].Selected {
		t.Fatalf("expected item 1 to be selected after toggle")
	}
}

func TestMultiSelectModel_NumericToggle(t *testing.T) {
	m := newMultiSelectModel("title", sampleItems())
	// Press "2" — should jump to index 1 and toggle.
	m = runKeys(m, "2")
	if m.cursor != 1 {
		t.Fatalf("expected cursor at index 1 after pressing 2, got %d", m.cursor)
	}
	if !m.items[1].Selected {
		t.Fatalf("expected item 1 selected after pressing 2")
	}
	// Press "9" — out of range, no-op.
	m = runKeys(m, "9")
	if m.cursor != 1 {
		t.Fatalf("cursor should not move on out-of-range digit, got %d", m.cursor)
	}
}

func TestMultiSelectModel_EnterConfirms(t *testing.T) {
	m := newMultiSelectModel("title", sampleItems())
	updated, cmd := m.Update(keyMsg("enter"))
	m = updated.(multiSelectModel)
	if m.reason != MultiSelectConfirmed {
		t.Fatalf("expected MultiSelectConfirmed, got %v", m.reason)
	}
	if cmd == nil {
		t.Fatal("expected tea.Quit cmd from enter")
	}
	indices := m.selectedIndices()
	if len(indices) != 2 || indices[0] != 0 || indices[1] != 2 {
		t.Fatalf("expected indices [0 2], got %v", indices)
	}
}

func TestMultiSelectModel_EscapeCancels(t *testing.T) {
	m := newMultiSelectModel("title", sampleItems())
	updated, _ := m.Update(keyMsg("esc"))
	m = updated.(multiSelectModel)
	if m.reason != MultiSelectCanceled {
		t.Fatalf("expected MultiSelectCanceled, got %v", m.reason)
	}
	if m.selectedIndices() != nil {
		t.Fatalf("expected nil indices on cancel")
	}
}

func TestMultiSelectModel_QCancels(t *testing.T) {
	m := newMultiSelectModel("title", sampleItems())
	updated, _ := m.Update(keyMsg("q"))
	m = updated.(multiSelectModel)
	if m.reason != MultiSelectCanceled {
		t.Fatalf("expected MultiSelectCanceled on q, got %v", m.reason)
	}
}

func TestMultiSelectModel_CtrlCAborts(t *testing.T) {
	m := newMultiSelectModel("title", sampleItems())
	updated, _ := m.Update(keyMsg("ctrl+c"))
	m = updated.(multiSelectModel)
	if m.reason != MultiSelectAborted {
		t.Fatalf("expected MultiSelectAborted, got %v", m.reason)
	}
}
