package tui

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg" // decode dropped JPEGs for the chip preview
	_ "image/png"  // decode dropped PNGs for the chip preview
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"spettro/internal/termtext"
	"spettro/internal/theme"
)

// Attachment chips sit on the first row inside the input box. Hovering one
// shows the file's full path in the status bar; clicking one opens a preview
// of its content (the text of a document, a picture of an image) over the
// conversation, which esc or a second click closes.

// chipLabel is what a chip reads: ▣ for an image, ▤ for a document, and the
// file name (an image pasted from the clipboard has no file name worth
// showing, so it keeps its "Image #N").
func chipLabel(att attachmentItem, i int) string {
	if att.Kind == "image" {
		name := att.RelPath
		if !strings.HasPrefix(name, "Image #") {
			name = filepath.Base(att.Path)
		}
		return fmt.Sprintf("▣ %s [%d]", name, i+1)
	}
	return fmt.Sprintf("▤ %s [%d]", filepath.Base(att.RelPath), i+1)
}

// chipGap is the blank between two chips; chipPad the padding inside one.
const (
	chipGap = 1
	chipPad = 1
)

// chipSpans returns the columns each chip covers on the chips row, relative
// to the start of the row's content: [start, end) per attachment.
func (m Model) chipSpans() [][2]int {
	spans := make([][2]int, len(m.attachments))
	x := 0
	for i, att := range m.attachments {
		w := ansi.StringWidth(chipLabel(att, i)) + 2*chipPad
		spans[i] = [2]int{x, x + w}
		x += w + chipGap
	}
	return spans
}

// chipsRowY is the screen row the chips are drawn on, and ok whether the
// main input box with chips is on screen at all. The input box is the part
// right above the one-row status bar; its top border is followed by the
// chips row.
func (m Model) chipsRowY() (int, bool) {
	if len(m.attachments) == 0 || m.activeModal() != modalNone ||
		m.pendingAuth != nil || m.pendingQuestion != nil || m.showPlanApproval || m.showSteerChoice ||
		len(m.cmdItems) > 0 {
		return 0, false
	}
	_, input := m.cachedInput(m.paneWidth())
	statusY := m.height - 1
	boxTop := statusY - len(input.rows)
	// A mention palette sits above the box inside the same part.
	if palette := m.viewMentionPalette(m.paneWidth()); palette != "" {
		boxTop += lipgloss.Height(palette)
	}
	return boxTop + 1, true
}

// chipAt returns the attachment under the screen cell (x, y), or -1. The
// box's border and one cell of padding come before the row's content.
func (m Model) chipAt(x, y int) int {
	rowY, ok := m.chipsRowY()
	if !ok || y != rowY || x >= m.paneWidth() {
		return -1
	}
	col := x - 2
	for i, s := range m.chipSpans() {
		if col >= s[0] && col < s[1] {
			return i
		}
	}
	return -1
}

// renderAttachmentChips returns a line of chips for the input area showing
// both file and image attachments. Returns an empty string when there are
// none. The hovered chip is drawn reversed.
func (m Model) renderAttachmentChips(mc color.Color) string {
	if len(m.attachments) == 0 {
		return ""
	}
	pal := theme.Current()
	var chips []string
	for i, att := range m.attachments {
		style := lipgloss.NewStyle().
			Foreground(mc).
			Background(pal.BgCodeInline).
			PaddingLeft(chipPad).PaddingRight(chipPad)
		if i+1 == m.chipHover {
			style = style.Foreground(pal.TextOnAccent).Background(mc)
		}
		chips = append(chips, style.Render(chipLabel(att, i)))
	}
	hint := styleMuted.Render("  click to preview · ctrl+r removes last")
	return strings.Join(chips, strings.Repeat(" ", chipGap)) + hint
}

// hoverChip updates the hovered chip from a mouse position; the status bar
// shows the hovered file's full path (statusBarMessage).
func (m *Model) hoverChip(x, y int) {
	m.chipHover = m.chipAt(x, y) + 1
}

// clickChip opens (or, on the open chip, closes) the preview of the chip
// under the click, and reports whether the click landed on one.
func (m *Model) clickChip(x, y int) bool {
	i := m.chipAt(x, y)
	if i < 0 {
		return false
	}
	m.chipPreview = i + 1
	m.chipPreviewScroll = 0
	m.chipHover = 0
	return true
}

// closeChipPreview closes the preview.
func (m *Model) closeChipPreview() {
	m.chipPreview = 0
	m.chipPreviewScroll = 0
}

// maxPreviewBytes bounds how much of a document the preview reads.
const maxPreviewBytes = 256 << 10

// chipPreviewLines renders the preview body for attachment att at width:
// the text of a document (control characters shown, not interpreted), a
// half-block picture of a PNG or JPEG, or a line saying why there is none.
func chipPreviewLines(att attachmentItem, width, height int) []string {
	f, err := os.Open(att.Path)
	if err != nil {
		return []string{styleError.Render("cannot read: " + err.Error())}
	}
	defer f.Close()
	buf := make([]byte, maxPreviewBytes)
	n, _ := f.Read(buf)
	data := buf[:n]
	if att.Kind == "image" {
		if lines := imagePreview(data, width, height); lines != nil {
			return lines
		}
		return []string{styleMuted.Render("(no preview for this image format — it is sent to the model as is)")}
	}
	if bytes.IndexByte(data, 0) >= 0 || !utf8.Valid(data) {
		return []string{styleMuted.Render(fmt.Sprintf("(binary file, %d bytes — sent to the model as text, which may not help)", n))}
	}
	var out []string
	for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		out = append(out, termtext.Fit(termtext.SanitizeLine(line), width))
	}
	if n == maxPreviewBytes {
		out = append(out, styleMuted.Render("… (preview cut at 256 KB; the whole file is sent)"))
	}
	return out
}

// imagePreview draws an image in half-block cells (▀, the top pixel as the
// foreground, the bottom one as the background), scaled to fit width cells
// and height rows; nil when the data is not an image it can decode.
func imagePreview(data []byte, width, height int) []string {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	b := img.Bounds()
	if b.Dx() == 0 || b.Dy() == 0 || width < 1 || height < 1 {
		return nil
	}
	// Each cell is one pixel wide and two tall; terminal cells are about
	// twice as tall as wide, so that keeps the aspect ratio.
	scale := max(float64(b.Dx())/float64(width), float64(b.Dy())/float64(2*height))
	cols := max(int(float64(b.Dx())/scale), 1)
	rows := max(int(float64(b.Dy())/scale/2), 1)
	at := func(cx, py int) color.Color {
		x := b.Min.X + min(int(float64(cx)*scale), b.Dx()-1)
		y := b.Min.Y + min(int(float64(py)*scale), b.Dy()-1)
		return img.At(x, y)
	}
	lines := make([]string, rows)
	for r := range rows {
		var sb strings.Builder
		for c := range cols {
			sb.WriteString(lipgloss.NewStyle().
				Foreground(at(c, 2*r)).
				Background(at(c, 2*r+1)).
				Render("▀"))
		}
		lines[r] = sb.String()
	}
	return lines
}

// viewChipPreview is the full-screen preview of the clicked chip: a title
// with the file's full path, the content, and the keys.
func (m Model) viewChipPreview() string {
	i := m.chipPreview - 1
	if i < 0 || i >= len(m.attachments) {
		return ""
	}
	att := m.attachments[i]
	contentW := max(m.width-2, 10)
	bodyRows := max(m.height-5, 1) // title, path, rule, hint
	doc := chipPreviewLines(att, contentW, bodyRows)
	start := clampOffset(m.chipPreviewScroll, 0, max(len(doc)-bodyRows, 0))
	end := min(start+bodyRows, len(doc))

	mc := m.currentColor()
	title := lipgloss.NewStyle().Bold(true).Foreground(mc).Render(chipLabel(att, i))
	rows := []string{" " + termtext.Fit(title, contentW)}
	rows = append(rows, " "+styleMuted.Render(termtext.FitLeft(att.Path, contentW)))
	rows = append(rows, " "+styleRule.Render(strings.Repeat("─", contentW)))
	for _, line := range doc[start:end] {
		rows = append(rows, " "+line)
	}
	for range bodyRows - (end - start) {
		rows = append(rows, "")
	}
	hint := "esc or click closes"
	if len(doc) > bodyRows {
		hint = fmt.Sprintf("rows %d-%d of %d · ↑↓ pgup/pgdn scroll · %s", start+1, end, len(doc), hint)
	}
	rows = append(rows, " "+styleMuted.Render(termtext.Fit(hint, contentW)))
	return strings.Join(rows, "\n")
}

// updateChipPreview handles keys while the preview is open.
func (m Model) updateChipPreview(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	page := max(m.height-6, 1)
	switch msg.String() {
	case "esc", "q", "enter", "space":
		m.closeChipPreview()
	case "up", "k":
		m.chipPreviewScroll = max(m.chipPreviewScroll-1, 0)
	case "down", "j":
		m.chipPreviewScroll++
	case "pgup":
		m.chipPreviewScroll = max(m.chipPreviewScroll-page, 0)
	case "pgdown":
		m.chipPreviewScroll += page
	case "ctrl+c":
		m.closeChipPreview()
	}
	return m, nil
}
