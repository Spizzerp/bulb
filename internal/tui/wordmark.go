package tui

import (
	_ "embed"
	"strings"

	"charm.land/lipgloss/v2"
)

// wordmark.txt is the bulb wordmark (assets/bulbwordmark.png) drawn in
// Braille cells: eight rows of wordmarkWidth cells each.
//
//go:embed wordmark.txt
var wordmarkFile string

const wordmarkWidth = 36

// wordmarkArt holds the wordmark's rows. A Windows checkout with autocrlf
// gives the file CRLF endings, so each row is stripped of its carriage return.
var wordmarkArt = parseWordmark(wordmarkFile)

func parseWordmark(data string) []string {
	rows := strings.Split(strings.TrimRight(data, "\r\n"), "\n")
	for i, row := range rows {
		rows[i] = strings.TrimRight(row, "\r")
	}
	return rows
}

// renderWordmark draws the logo in the mode's accent colour, centred in width
// cells. A pane narrower than the art crops it to its ink, centred on the ink
// rather than on the blank Braille gutter, and never wraps it, so the logo
// always takes exactly len(wordmarkArt) rows.
func renderWordmark(mode string, width int) string {
	style := lipgloss.NewStyle().Foreground(modeColor(mode))
	lead, trail := 0, wordmarkWidth-1
	if width <= wordmarkWidth {
		lead, trail = wordmarkInk(wordmarkArt)
	}
	lines := make([]string, len(wordmarkArt))
	for i, raw := range wordmarkArt {
		runes := []rune(raw)
		hi := min(trail+1, len(runes))
		lo := min(lead, hi)
		cropped := runes[lo:hi]
		if len(cropped) > width {
			cropped = cropped[:width]
		}
		lines[i] = style.Render(string(cropped))
	}
	if pad := (width - (trail - lead + 1)) / 2; pad > 0 {
		prefix := strings.Repeat(" ", pad)
		for i, line := range lines {
			lines[i] = prefix + line
		}
	}
	return strings.Join(lines, "\n")
}

// wordmarkInk returns the column range the art actually paints in, ignoring
// the blank Braille cells around it.
func wordmarkInk(art []string) (lead, trail int) {
	lead, trail = wordmarkWidth, 0
	for _, raw := range art {
		for i, r := range []rune(raw) {
			if r == '\u2800' || r == ' ' {
				continue
			}
			lead = min(lead, i)
			trail = max(trail, i)
		}
	}
	if trail < lead {
		return 0, wordmarkWidth - 1
	}
	return lead, trail
}
