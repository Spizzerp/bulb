package tui

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// wordmarkRow is a row of the logo distinctive enough to find in a frame.
func wordmarkRow() string { return wordmarkArt[3] }

// The art is the embedded file: eight rows, each exactly wordmarkWidth cells.
func TestWordmarkArtShape(t *testing.T) {
	if len(wordmarkArt) != 8 {
		t.Fatalf("wordmark has %d rows, want 8", len(wordmarkArt))
	}
	for i, row := range wordmarkArt {
		if got := len([]rune(row)); got != wordmarkWidth {
			t.Fatalf("row %d is %d cells, want %d", i, got, wordmarkWidth)
		}
	}
}

// parseWordmark must survive a CRLF checkout: a Windows clone with autocrlf
// hands it carriage returns, which must not end up inside the rows.
func TestParseWordmarkAcceptsCRLF(t *testing.T) {
	for name, data := range map[string]string{
		"lf":   "ab\ncd\n",
		"crlf": "ab\r\ncd\r\n",
	} {
		rows := parseWordmark(data)
		if len(rows) != 2 || rows[0] != "ab" || rows[1] != "cd" {
			t.Fatalf("%s: rows parsed as %q", name, rows)
		}
	}
}

// The logo is static: it is drawn behind the trust prompt and after it, and
// nothing about it keeps the animation tick running.
func TestWordmarkDoesNotAnimate(t *testing.T) {
	m := footerModel(120, 40)
	m.showTrust = true
	next, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = next.(Model)
	if !strings.Contains(ansi.Strip(m.renderMessages()), wordmarkRow()) {
		t.Fatalf("the logo must be drawn behind the trust prompt")
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: '1'})
	m = next.(Model)
	if m.needsAnimation() {
		t.Fatalf("an idle session with only the logo on screen must not animate")
	}
	if !strings.Contains(ansi.Strip(m.renderMessages()), wordmarkRow()) {
		t.Fatalf("the logo must be drawn after the trust prompt")
	}
}

// The logo opens the scrollback instead of sitting above it: it is the first
// thing renderMessages emits, both with and without a transcript, and it is
// not part of the frame's fixed chrome.
func TestWordmarkOpensTheScrollback(t *testing.T) {
	m := footerModel(120, 40)

	empty := m.renderMessages()
	if !strings.HasPrefix(ansi.Strip(empty), strings.Repeat(" ", (m.transcriptWidth()-wordmarkWidth)/2)+wordmarkArt[0]) {
		t.Fatalf("an empty transcript must still open with the logo:\n%q", ansi.Strip(empty))
	}
	if !strings.Contains(empty, "no messages yet") {
		t.Fatalf("the empty-state hint belongs under the logo, not instead of it:\n%s", empty)
	}

	m.messages = append(m.messages, ChatMessage{Role: RoleUser, Content: "hello"})
	body := ansi.Strip(m.renderMessages())
	if idx := strings.Index(body, wordmarkRow()); idx < 0 {
		t.Fatalf("the logo must lead the transcript:\n%s", body)
	} else if idx > strings.Index(body, "hello") {
		t.Fatalf("the logo must come before the first message:\n%s", body)
	}
}

// The banner does not follow the frame counter, keeping repeated renders
// byte-identical.
func TestWordmarkIsStatic(t *testing.T) {
	m := footerModel(120, 40)
	m.messages = append(m.messages, ChatMessage{Role: RoleUser, Content: "hello"})

	m.eyeFrame = 0
	first := m.renderMessages()
	m.eyeFrame = 123
	m.thinking = true
	second := m.renderMessages()

	if first != second {
		t.Fatalf("the inline logo must not animate with the frame counter")
	}
}

// Nothing above the conversation pane draws the logo, and once the transcript
// is long enough the logo scrolls out of the frame entirely.
func TestWordmarkIsNotInTheFixedFrame(t *testing.T) {
	m := footerModel(120, 30)
	m.messages = append(m.messages, ChatMessage{Role: RoleUser, Content: "hello"})
	m = m.recalcLayout()
	m.refreshViewport()

	frame := ansi.Strip(m.View().Content)
	lines := strings.Split(frame, "\n")
	if strings.Contains(lines[0], wordmarkRow()) || strings.Contains(lines[1], wordmarkRow()) {
		t.Fatalf("the logo must not sit between the header and the pane:\n%s", frame)
	}

	for i := range 40 {
		m.messages = append(m.messages, ChatMessage{Role: RoleAssistant, Content: strings.Repeat("filler ", 20) + string(rune('a'+i))})
	}
	m = m.recalcLayout()
	m.refreshViewport()
	if strings.Contains(ansi.Strip(m.View().Content), wordmarkRow()) {
		t.Fatalf("the logo should have scrolled away once the chat filled the pane")
	}
}

// No rows are reserved for the logo: they belong to the conversation.
func TestViewportReservesNoLogoRows(t *testing.T) {
	m := footerModel(120, 40)
	// header, separators, the input box (one text row and its border) and
	// the status bar.
	if got, want := m.vp.Height(), 40-1-2-3-1; got != want {
		t.Fatalf("viewport height = %d, want %d (no logo rows reserved)", got, want)
	}

	m.thinking = true
	m.agentStartAt = time.Now()
	m = m.recalcLayout()
	if got, want := m.vp.Height(), 40-1-2-3-1-1; got != want {
		t.Fatalf("viewport height while working = %d, want %d (one indicator row)", got, want)
	}
}

// The banner lives inside the viewport, which only repaints when its content
// is rebuilt — so a mode switch has to rebuild it or the logo keeps the
// previous mode's accent while the rest of the frame has moved on.
func TestWordmarkFollowsAModeSwitch(t *testing.T) {
	m := footerModel(120, 40)
	m.messages = append(m.messages, ChatMessage{Role: RoleUser, Content: "hello"})
	m.refreshViewport()

	next, _ := m.updateMain(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	m = next.(Model)

	frame := ansi.Strip(m.View().Content)
	if !strings.Contains(frame, wordmarkRow()) {
		t.Fatalf("the logo must remain visible after the mode switch:\n%s", frame)
	}
	if want := renderWordmark(m.mode, m.transcriptWidth()); !strings.HasPrefix(m.renderMessages(), want) {
		t.Fatalf("the logo must be drawn in the new mode's accent")
	}
}

// The art is cropped, never wrapped: a pane too narrow for the full grid still
// gets exactly eight rows, centred on the ink rather than on the blank gutter.
func TestWordmarkFitsNarrowPanes(t *testing.T) {
	for _, width := range []int{10, 20, 30, 36, 46, 80, 120} {
		art := renderWordmark("coding", width)
		if got := lipgloss.Height(art); got != len(wordmarkArt) {
			t.Fatalf("width %d: %d rows, want %d", width, got, len(wordmarkArt))
		}
		if got := lipgloss.Width(art); got > width {
			t.Fatalf("width %d: art is %d columns wide", width, got)
		}
	}
}

// A fresh session is nothing but the logo, so the viewport must not scroll
// past it on a terminal too short to show the whole block.
func TestWordmarkIsPinnedToTopOnAFreshSession(t *testing.T) {
	m := footerModel(120, 18)
	m.refreshViewport()

	if !strings.Contains(ansi.Strip(m.View().Content), wordmarkArt[0]) {
		t.Fatalf("the top of the logo must be visible on a fresh session:\n%s", ansi.Strip(m.View().Content))
	}
}
