package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

// The newline keys break a line in the draft; enter still submits it.
func TestNewlineKeysBreakTheDraft(t *testing.T) {
	for _, k := range []tea.KeyPressMsg{
		{Code: tea.KeyEnter, Mod: tea.ModShift},
		{Code: tea.KeyEnter, Mod: tea.ModAlt},
		{Code: 'j', Mod: tea.ModCtrl},
	} {
		t.Run(k.String(), func(t *testing.T) {
			m := footerModel(100, 30)
			m.ta.SetValue("first")
			next, _ := m.updateMain(k)
			m = next.(Model)
			for _, r := range "second" {
				next, _ = m.updateMain(tea.KeyPressMsg{Code: r, Text: string(r)})
				m = next.(Model)
			}
			if got := m.ta.Value(); got != "first\nsecond" {
				t.Fatalf("draft = %q, want %q", got, "first\nsecond")
			}
			if len(m.messages) != 0 {
				t.Fatalf("%s submitted the draft", k)
			}
		})
	}
}

// Plain enter submits the whole multi-line draft as one message.
func TestEnterSubmitsMultiLineDraft(t *testing.T) {
	m := footerModel(100, 30)
	m.ta.SetValue("line one")
	next, _ := m.updateMain(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift})
	m = next.(Model)
	m.ta.InsertString("line two")
	next, _ = m.updateMain(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	if m.ta.Value() != "" {
		t.Fatalf("enter left the draft in the box: %q", m.ta.Value())
	}
	var sent string
	for _, msg := range m.messages {
		if msg.Role == RoleUser {
			sent = msg.Content
		}
	}
	if sent != "line one\nline two" {
		t.Fatalf("sent %q, want both lines in one message", sent)
	}
}

// The textarea's own newline binding no longer includes enter, so a modal
// that forwards keys to it (the question form's free-text field) never gets
// a stray line break from the key that submits it; shift+enter still breaks.
func TestTextareaNewlineBindingIsShiftEnter(t *testing.T) {
	m := footerModel(100, 30)
	m.ta.SetValue("a")
	m.ta, _ = m.ta.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if got := m.ta.Value(); got != "a" {
		t.Fatalf("enter inserted a newline in the textarea: %q", got)
	}
	m.ta, _ = m.ta.Update(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift})
	if got := m.ta.Value(); got != "a\n" {
		t.Fatalf("shift+enter did not insert a newline: %q", got)
	}
}

// The attach-path prompt is one line: a newline key there is ignored.
func TestNewlineIgnoredInAttachPrompt(t *testing.T) {
	m := footerModel(100, 30)
	m.showAttachPrompt = true
	m.ta.SetValue("notes.txt")
	next, _ := m.updateMain(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift})
	m = next.(Model)
	if got := m.ta.Value(); got != "notes.txt" {
		t.Fatalf("attach path = %q", got)
	}
}
