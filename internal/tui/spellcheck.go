package tui

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"spettro/internal/spell"
	"spettro/internal/theme"
)

// Spell check marks misspelled words in the input box with a curly red
// underline. The check runs off the Update goroutine, after the draft has
// been still for spellDebounce, through the system's own checker (see
// internal/spell); a terminal without curly underlines shows a straight one.
// spell_check: false in config.json turns it off.

// spellDebounce is how long the draft must stay unchanged before it is
// checked: long enough that typing a word never checks its prefixes, short
// enough that the mark appears as soon as the hands stop.
const spellDebounce = 350 * time.Millisecond

// spellState is the input's latest check: the draft it was made for and the
// misspelled words in it, as rune offsets into that draft.
type spellState struct {
	text  string
	words []runeSpan
	// pending is the draft a debounce timer is waiting on; a timer that
	// fires for another draft was overtaken by typing and does nothing.
	pending string
}

type runeSpan struct{ start, end int }

type spellTickMsg struct{ text string }

type spellResultMsg struct {
	text  string
	words []runeSpan
}

// spellEnabled reports whether the input should be spell checked: on unless
// the user switched it off, and only for prose (not a /command).
func (m Model) spellEnabled() bool {
	return !m.cfg.SpellCheckDisabled
}

// scheduleSpellCheck arms a debounce for the current draft. Called after
// every Update; a draft already checked or already pending costs nothing.
func (m *Model) scheduleSpellCheck() tea.Cmd {
	text := m.ta.Value()
	if !m.spellEnabled() || text == m.spell.text || text == m.spell.pending {
		return nil
	}
	m.spell.pending = text
	if strings.TrimSpace(text) == "" || strings.HasPrefix(text, "/") {
		// Nothing to mark: settle at once without asking the checker.
		m.spell = spellState{text: text}
		return nil
	}
	return tea.Tick(spellDebounce, func(time.Time) tea.Msg { return spellTickMsg{text: text} })
}

// handleSpellTick starts the check when the draft is still the one the timer
// was armed for.
func (m Model) handleSpellTick(msg spellTickMsg) (tea.Model, tea.Cmd) {
	if msg.text != m.ta.Value() {
		return m, nil
	}
	text := msg.text
	return m, func() tea.Msg {
		var spans []runeSpan
		for _, s := range spell.Check(text) {
			spans = append(spans, runeSpan{
				start: len([]rune(text[:s.Start])),
				end:   len([]rune(text[:s.End])),
			})
		}
		return spellResultMsg{text: text, words: spans}
	}
}

func (m Model) handleSpellResult(msg spellResultMsg) (tea.Model, tea.Cmd) {
	if msg.text != m.ta.Value() {
		return m, nil // typing moved on; the next debounce checks the new draft
	}
	m.spell = spellState{text: msg.text, words: msg.words}
	return m, nil
}

// spellMarks returns the misspelled words of the current draft, as rune
// spans of its plain text, or nil when the last check was for another draft
// (marks for an old draft would underline the wrong letters).
func (m Model) spellMarks() []runeSpan {
	if !m.spellEnabled() || m.spell.text != m.ta.Value() {
		return nil
	}
	return m.spell.words
}

// underlineMisspelled underlines the misspelled words in rendered, the
// textarea's view of the draft. The view's visible characters are the
// draft's runes in order, with soft-wrap padding and line ends between them
// and the cursor's own styling around one cell; matching is done on the
// draft's characters only, so wrapping never shifts a mark.
func underlineMisspelled(rendered, draft string, marks []runeSpan) string {
	if len(marks) == 0 || rendered == "" {
		return rendered
	}
	runes := []rune(draft)
	bad := make([]bool, len(runes))
	for _, s := range marks {
		for i := s.start; i < s.end && i < len(bad); i++ {
			bad[i] = true
		}
	}
	style := lipgloss.NewStyle().
		UnderlineStyle(lipgloss.UnderlineCurly).
		UnderlineColor(theme.Current().Error)
	tokens := tokenizeANSI(rendered)
	var out strings.Builder
	var active strings.Builder
	pos := 0 // next draft rune to match
	for _, t := range tokens {
		if t.esc != "" {
			if isSGRReset(t.esc) {
				active.Reset()
			} else if strings.HasSuffix(t.esc, "m") {
				active.WriteString(t.esc)
			}
			out.WriteString(t.esc)
			continue
		}
		// Skip the draft's line breaks: the view shows them as row ends.
		for pos < len(runes) && runes[pos] == '\n' {
			pos++
		}
		if pos < len(runes) && t.r == runes[pos] {
			if bad[pos] && !t.reverse {
				out.WriteString(style.Render(string(t.r)))
				out.WriteString(active.String())
			} else {
				out.WriteRune(t.r)
			}
			pos++
			continue
		}
		// Padding, the cursor cell past the end, or a placeholder.
		out.WriteRune(t.r)
	}
	return out.String()
}
