package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// curly underline SGR (4:3) as lipgloss writes it.
const curlyUnderline = "4:3"

// underlined returns the plain characters drawn with a curly underline.
func underlined(rendered string) string {
	var out strings.Builder
	on := false
	for _, t := range tokenizeANSI(rendered) {
		if t.esc != "" {
			if strings.HasSuffix(t.esc, "m") {
				p := strings.TrimSuffix(strings.TrimPrefix(t.esc, "\x1b["), "m")
				switch {
				case strings.Contains(p, curlyUnderline):
					on = true
				case p == "" || p == "0":
					on = false
				}
			}
			continue
		}
		if on {
			out.WriteRune(t.r)
		}
	}
	return out.String()
}

// The marked words, and only they, are underlined, and the text reads the
// same with the marks stripped.
func TestUnderlineMisspelledMarksOnlyTheWords(t *testing.T) {
	m := footerModel(80, 24)
	m.ta.SetValue("teh cat sat on teh mat")
	marks := []runeSpan{{0, 3}, {15, 18}}
	view := m.ta.View()
	got := underlineMisspelled(view, m.ta.Value(), marks)
	if u := underlined(got); u != "tehteh" {
		t.Fatalf("underlined %q, want %q", u, "tehteh")
	}
	if ansi.Strip(got) != ansi.Strip(view) {
		t.Fatalf("marking changed the text:\n%q\n%q", ansi.Strip(got), ansi.Strip(view))
	}
}

// Soft wrapping and hard line breaks do not shift a mark onto another word.
func TestUnderlineMisspelledSurvivesWrapping(t *testing.T) {
	m := footerModel(40, 24)
	draft := strings.Repeat("word ", 12) + "speling\nsecond lne"
	m.ta.SetValue(draft)
	m = m.recalcLayout()
	runes := []rune(draft)
	idx := func(w string) runeSpan {
		i := strings.Index(draft, w)
		s := len([]rune(draft[:i]))
		return runeSpan{s, s + len([]rune(w))}
	}
	_ = runes
	got := underlineMisspelled(m.ta.View(), draft, []runeSpan{idx("speling"), idx("lne")})
	if u := underlined(got); u != "spelinglne" {
		t.Fatalf("underlined %q, want %q", u, "spelinglne")
	}
}

// Marks belong to the draft they were made for: once the text changes they
// are dropped until the next check, never drawn on the wrong letters.
func TestSpellMarksDroppedWhenDraftChanges(t *testing.T) {
	m := footerModel(80, 24)
	m.ta.SetValue("teh cat")
	next, _ := m.Update(spellResultMsg{text: "teh cat", words: []runeSpan{{0, 3}}})
	m = next.(Model)
	if len(m.spellMarks()) != 1 {
		t.Fatalf("marks for the current draft = %v", m.spellMarks())
	}
	m.ta.SetValue("the cat")
	if m.spellMarks() != nil {
		t.Fatalf("stale marks kept after the draft changed: %v", m.spellMarks())
	}
	// A late result for an old draft is ignored.
	next, _ = m.Update(spellResultMsg{text: "teh cat", words: []runeSpan{{0, 3}}})
	m = next.(Model)
	if m.spellMarks() != nil {
		t.Fatalf("a stale result was applied: %v", m.spellMarks())
	}
}

// Typing arms one debounce for the latest draft; an idle box arms nothing,
// and a /command or the off switch is never checked.
func TestSpellCheckDebounce(t *testing.T) {
	m := footerModel(80, 24)
	m.ta.SetValue("helo")
	if cmd := m.scheduleSpellCheck(); cmd == nil {
		t.Fatal("a new draft did not arm the check")
	}
	if cmd := m.scheduleSpellCheck(); cmd != nil {
		t.Fatal("the same draft armed a second check")
	}
	// A timer for a draft typing has moved past does nothing.
	m.ta.SetValue("hello")
	next, cmd := m.handleSpellTick(spellTickMsg{text: "helo"})
	if cmd != nil || next.(Model).spell.text == "helo" {
		t.Fatal("a stale debounce ran a check")
	}

	m.ta.SetValue("/help")
	if cmd := m.scheduleSpellCheck(); cmd != nil {
		t.Fatal("a /command was spell checked")
	}

	m.cfg.SpellCheckDisabled = true
	m.ta.SetValue("teh")
	if cmd := m.scheduleSpellCheck(); cmd != nil {
		t.Fatal("spell_check_disabled still checked")
	}
	m.spell = spellState{text: "teh", words: []runeSpan{{0, 3}}}
	if m.spellMarks() != nil {
		t.Fatal("spell_check_disabled still drew marks")
	}
}

// An empty, settled input box does nothing on an idle update.
func TestSpellCheckKeepsIdleAsleep(t *testing.T) {
	m := footerModel(80, 24)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = next.(Model)
	if cmd := m.scheduleSpellCheck(); cmd != nil {
		t.Fatal("an empty input armed a spell check")
	}
}
