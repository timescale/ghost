package common

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	tea "charm.land/bubbletea/v2"
	lipgloss "charm.land/lipgloss/v2"
)

// MultiSelectItem describes a single row of a multi-select prompt.
type MultiSelectItem struct {
	// Label is the primary text shown to the right of the checkbox.
	Label string
	// Status is dim explanatory text shown after the label (optional).
	Status string
	// Selected controls the initial checked state of the row.
	Selected bool
	// Dimmed renders the entire row (checkbox, label, status) in a dim
	// style. Used to mark rows the caller has detected as already
	// configured so the user understands why they weren't pre-selected.
	Dimmed bool
}

// MultiSelectReason describes how a multi-select prompt completed.
type MultiSelectReason int

const (
	// MultiSelectConfirmed means the user pressed enter.
	MultiSelectConfirmed MultiSelectReason = iota
	// MultiSelectCanceled means the user pressed esc or q. Callers can
	// treat this as a soft cancel (e.g. return to a parent menu).
	MultiSelectCanceled
	// MultiSelectAborted means the user pressed ctrl+c. Callers should
	// bubble this up and stop the surrounding workflow.
	MultiSelectAborted
)

// MultiSelectResult is the outcome of running a multi-select prompt.
type MultiSelectResult struct {
	Reason  MultiSelectReason
	Indices []int // populated only when Reason == MultiSelectConfirmed
}

// Canned errors to correspond with the MultiSelectReason values
var ErrMultiSelectAborted = errors.New("multi-select aborted")
var ErrMultiSelectCanceled = errors.New("multi-select canceled")

// RunMultiSelect renders an interactive multi-select prompt and returns the
// user's selection. ctrl+c is reported as MultiSelectAborted; esc/q as
// MultiSelectCanceled.
func RunMultiSelect(ctx context.Context, in io.Reader, out io.Writer, title string, items []MultiSelectItem) (*MultiSelectResult, error) {
	if len(items) == 0 {
		return &MultiSelectResult{Reason: MultiSelectConfirmed}, nil
	}

	initial := newMultiSelectModel(title, items)

	program := tea.NewProgram(initial,
		tea.WithInput(in),
		tea.WithOutput(out),
		tea.WithContext(ctx),
		tea.WithoutSignalHandler(),
	)

	finalModel, err := program.Run()
	if err != nil {
		return nil, fmt.Errorf("multi-select failed: %w", err)
	}

	m := finalModel.(multiSelectModel)
	return &MultiSelectResult{Reason: m.reason, Indices: m.selectedIndices()}, nil
}

// multiSelectModel is the BubbleTea model backing RunMultiSelect.
type multiSelectModel struct {
	title  string
	items  []MultiSelectItem
	cursor int
	reason MultiSelectReason
	done   bool
}

func newMultiSelectModel(title string, items []MultiSelectItem) multiSelectModel {
	copied := make([]MultiSelectItem, len(items))
	copy(copied, items)
	return multiSelectModel{
		title:  title,
		items:  copied,
		reason: MultiSelectCanceled,
	}
}

func (m multiSelectModel) Init() tea.Cmd { return nil }

func (m multiSelectModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}

	switch keyMsg.String() {
	case "ctrl+c":
		m.reason = MultiSelectAborted
		m.done = true
		return m, tea.Quit
	case "esc", "q":
		m.reason = MultiSelectCanceled
		m.done = true
		return m, tea.Quit
	case "enter":
		m.reason = MultiSelectConfirmed
		m.done = true
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.items)-1 {
			m.cursor++
		}
	case " ", "space":
		m.items[m.cursor].Selected = !m.items[m.cursor].Selected
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		idx := int(keyMsg.String()[0] - '1')
		if idx < len(m.items) {
			m.cursor = idx
			m.items[idx].Selected = !m.items[idx].Selected
		}
	}
	return m, nil
}

// Style cache. Created lazily on the first View() call.
var (
	multiSelectTitleStyle  = lipgloss.NewStyle().Bold(true)
	multiSelectStatusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	multiSelectDimmedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	multiSelectCursorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)
	multiSelectCheckStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Bold(true)
	multiSelectHelpStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
)

const (
	// Checkbox glyphs. The check is rendered in green; the empty box stays
	// in the terminal's default color so the row reads "to do" not "absent".
	multiSelectCheckedGlyph   = "✓"
	multiSelectUncheckedGlyph = " "
)

func (m multiSelectModel) View() tea.View {
	if m.done {
		// Leave the terminal clean once the program quits — BubbleTea
		// preserves the final view, but we don't want a stale prompt
		// hanging around above the subsequent command output.
		return tea.NewView("")
	}

	var b strings.Builder
	if m.title != "" {
		b.WriteString(multiSelectTitleStyle.Render(m.title))
		b.WriteString("\n\n")
	}

	// Compute the maximum width of the "[✓] N. Label" prefix so the
	// status column lines up across rows. Use a constant checked glyph for
	// width measurement so toggling selection doesn't shift the column.
	labelWidths := make([]int, len(m.items))
	maxLabelWidth := 0
	for i, item := range m.items {
		w := lipgloss.Width(fmt.Sprintf("[%s] %d. %s", multiSelectCheckedGlyph, i+1, item.Label))
		labelWidths[i] = w
		if w > maxLabelWidth {
			maxLabelWidth = w
		}
	}

	for i, item := range m.items {
		cursor := "  "
		if m.cursor == i {
			cursor = multiSelectCursorStyle.Render("> ")
		}
		var checkbox string
		if item.Selected {
			checkbox = "[" + multiSelectCheckStyle.Render(multiSelectCheckedGlyph) + "]"
		} else {
			checkbox = "[" + multiSelectUncheckedGlyph + "]"
		}
		label := fmt.Sprintf(" %d. %s", i+1, item.Label)
		if item.Dimmed {
			label = multiSelectDimmedStyle.Render(label)
		}
		padding := strings.Repeat(" ", maxLabelWidth-labelWidths[i]+2)
		b.WriteString(cursor)
		b.WriteString(checkbox)
		b.WriteString(label)
		if item.Status != "" {
			b.WriteString(padding)
			b.WriteString(multiSelectStatusStyle.Render(item.Status))
		}
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(multiSelectHelpStyle.Render("  ↑/↓ or j/k to navigate, space or 1-9 to toggle, enter to confirm, esc to cancel"))
	b.WriteString("\n")
	return tea.NewView(b.String())
}

// selectedIndices returns the indices of items currently checked.
func (m multiSelectModel) selectedIndices() []int {
	if m.reason != MultiSelectConfirmed {
		return nil
	}
	indices := make([]int, 0, len(m.items))
	for i, item := range m.items {
		if item.Selected {
			indices = append(indices, i)
		}
	}
	return indices
}
