package tui

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func dropFixture(t *testing.T) (dir string, files map[string]string) {
	t.Helper()
	dir = t.TempDir()
	files = map[string]string{}
	for _, name := range []string{"notes.md", "My Shot.png", "report (final).txt", "plain.txt"} {
		p := filepath.Join(dir, name)
		data := []byte("hello from " + name + "\n")
		if strings.HasSuffix(name, ".png") {
			data = tinyPNG
		}
		if err := os.WriteFile(p, data, 0o644); err != nil {
			t.Fatal(err)
		}
		files[name] = p
	}
	return dir, files
}

// The ways terminals paste a dropped file are all read back to the file.
func TestDroppedPathsQuotingStyles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("backslash escapes are Unix shell quoting")
	}
	dir, f := dropFixture(t)
	esc := func(p string) string {
		return strings.NewReplacer(" ", `\ `, "(", `\(`, ")", `\)`).Replace(p)
	}
	for name, tc := range map[string]struct {
		pasted string
		want   []string
	}{
		"plain":            {f["notes.md"], []string{f["notes.md"]}},
		"backslash spaces": {esc(f["My Shot.png"]), []string{f["My Shot.png"]}},
		"backslash parens": {esc(f["report (final).txt"]), []string{f["report (final).txt"]}},
		"single quoted":    {"'" + f["My Shot.png"] + "'", []string{f["My Shot.png"]}},
		"double quoted":    {`"` + f["My Shot.png"] + `"`, []string{f["My Shot.png"]}},
		"file URL":         {"file://" + strings.ReplaceAll(f["My Shot.png"], " ", "%20"), []string{f["My Shot.png"]}},
		"several, spaces":  {esc(f["notes.md"]) + " " + esc(f["My Shot.png"]), []string{f["notes.md"], f["My Shot.png"]}},
		"several, lines":   {f["notes.md"] + "\n" + f["plain.txt"] + "\n", []string{f["notes.md"], f["plain.txt"]}},
		"relative to cwd":  {"notes.md", []string{f["notes.md"]}},
	} {
		got, ok := droppedPaths(tc.pasted, dir)
		if !ok || !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: droppedPaths(%q) = %q, %v; want %q", name, tc.pasted, got, ok, tc.want)
		}
	}
}

// Anything that is not only existing files is ordinary pasted text.
func TestDroppedPathsRejectsText(t *testing.T) {
	dir, f := dropFixture(t)
	for _, pasted := range []string{
		"",
		"   ",
		"please look at the bug",
		f["notes.md"] + " and explain it",
		filepath.Join(dir, "missing.txt"),
		dir, // a folder is not an attachment
		"'" + f["notes.md"],
		"file://otherhost" + f["notes.md"],
		"fix the thing",
	} {
		if got, ok := droppedPaths(pasted, dir); ok {
			t.Errorf("droppedPaths(%q) = %q, want not a drop", pasted, got)
		}
	}
}

// A drop in the main input adds a chip per file and leaves the draft as it
// was; the same paste anywhere else is text.
func TestDropAddsChipsAndKeepsDraft(t *testing.T) {
	dir, f := dropFixture(t)
	m := footerModel(100, 30)
	m.cwd = dir
	m.ta.SetValue("compare these")
	next, _ := m.Update(tea.PasteMsg{Content: "'" + f["notes.md"] + "' '" + f["plain.txt"] + "'"})
	m = next.(Model)
	if got := m.ta.Value(); got != "compare these" {
		t.Fatalf("a drop changed the draft to %q", got)
	}
	if len(m.attachments) != 2 || m.attachments[0].Path != f["notes.md"] || m.attachments[1].Path != f["plain.txt"] {
		t.Fatalf("attachments = %+v", m.attachments)
	}
	if !strings.Contains(m.banner, "attached 2 files") {
		t.Fatalf("banner = %q", m.banner)
	}
	view := ansi.Strip(m.viewInput(m.paneWidth()))
	for _, want := range []string{"▤ notes.md [1]", "▤ plain.txt [2]", "compare these"} {
		if !strings.Contains(view, want) {
			t.Fatalf("input box lacks %q:\n%s", want, view)
		}
	}
	// Ordinary text is pasted as text.
	next, _ = m.Update(tea.PasteMsg{Content: " please"})
	m = next.(Model)
	if got := m.ta.Value(); got != "compare these please" || len(m.attachments) != 2 {
		t.Fatalf("text paste: draft %q, %d attachments", got, len(m.attachments))
	}
	// In the attach-path prompt a dropped path is the text the prompt wants.
	m.showAttachPrompt = true
	m.ta.Reset()
	next, _ = m.Update(tea.PasteMsg{Content: f["My Shot.png"]})
	m = next.(Model)
	if got := m.ta.Value(); got != f["My Shot.png"] || len(m.attachments) != 2 {
		t.Fatalf("attach prompt: draft %q, %d attachments", got, len(m.attachments))
	}
}

// An image dropped while the model cannot see images is refused, as ctrl+f
// refuses it, and the reason stays on screen.
func TestDropImageWithoutVisionIsRefused(t *testing.T) {
	dir, f := dropFixture(t)
	m := footerModel(100, 30)
	m.cwd = dir
	m.cfg.ActiveProvider, m.cfg.ActiveModel = "nobody", "no-vision"
	next, _ := m.Update(tea.PasteMsg{Content: "'" + f["My Shot.png"] + "'"})
	m = next.(Model)
	if len(m.attachments) != 0 || !strings.Contains(m.banner, "does not support vision") {
		t.Fatalf("attachments %+v, banner %q", m.attachments, m.banner)
	}
}

// A file outside the project is named by its full path, not by ../../.
func TestAttachmentOutsideProjectKeepsFullPath(t *testing.T) {
	_, f := dropFixture(t)
	m := footerModel(100, 30)
	m.cwd = t.TempDir()
	m.addAttachment(f["notes.md"])
	if len(m.attachments) != 1 || m.attachments[0].RelPath != filepath.ToSlash(f["notes.md"]) {
		t.Fatalf("attachments = %+v", m.attachments)
	}
}

// The chips row is where chipAt looks: hovering a chip names its path in
// the status bar, moving off clears it, and a click opens its preview.
func TestChipHoverAndClick(t *testing.T) {
	dir, f := dropFixture(t)
	m := footerModel(100, 30)
	m.cwd = dir
	next, _ := m.Update(tea.PasteMsg{Content: "'" + f["notes.md"] + "' '" + f["plain.txt"] + "'"})
	m = next.(Model)

	// Find the second chip on screen the way a user would.
	frame := strings.Split(ansi.Strip(m.View().Content), "\n")
	y, x := -1, -1
	for i, row := range frame {
		if c := strings.Index(row, "▤ plain.txt"); c >= 0 {
			y, x = i, len([]rune(row[:c]))
		}
	}
	if y < 0 {
		t.Fatalf("chip not drawn:\n%s", strings.Join(frame, "\n"))
	}
	if got := m.chipAt(x, y); got != 1 {
		t.Fatalf("chipAt(%d,%d) = %d, want 1", x, y, got)
	}
	if got := m.chipAt(x, y-1); got != -1 {
		t.Fatalf("chipAt above the chips row = %d", got)
	}

	statusBar := func(m Model) string {
		rows := strings.Split(ansi.Strip(m.View().Content), "\n")
		return rows[len(rows)-1]
	}
	next, _ = m.Update(tea.MouseMotionMsg{X: x, Y: y})
	m = next.(Model)
	if bar := statusBar(m); !strings.Contains(bar, "plain.txt") || !strings.Contains(bar, "▸") {
		t.Fatalf("hovering the chip: status bar %q, want its path", bar)
	}
	if m.statusBarMessage() == "" || !strings.HasSuffix(ansi.Strip(m.statusBarMessage()), "plain.txt") {
		t.Fatalf("status message %q does not end with the file name", ansi.Strip(m.statusBarMessage()))
	}
	next, _ = m.Update(tea.MouseMotionMsg{X: 1, Y: 2})
	m = next.(Model)
	if bar := statusBar(m); strings.Contains(bar, "▸") {
		t.Fatalf("moving off the chip left the path up: %q", bar)
	}

	next, _ = m.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	m = next.(Model)
	if m.activeModal() != modalChipPreview {
		t.Fatalf("click did not open the preview (modal %v)", m.activeModal())
	}
	view := ansi.Strip(m.View().Content)
	rows := strings.Split(view, "\n")
	pathRow := strings.TrimSpace(rows[1])
	if !strings.Contains(view, "hello from plain.txt") || !strings.HasSuffix(f["plain.txt"], strings.TrimPrefix(pathRow, "…")) {
		t.Fatalf("preview lacks the content, or its path row %q is not the file's path:\n%s", pathRow, view)
	}
	next, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	m = next.(Model)
	if m.activeModal() != modalNone || len(m.attachments) != 2 {
		t.Fatalf("esc: modal %v, %d attachments", m.activeModal(), len(m.attachments))
	}
}

// Hover reports need all-motion mouse tracking, which is on only while chips
// are on screen.
func TestMouseModeFollowsChips(t *testing.T) {
	_, f := dropFixture(t)
	m := footerModel(100, 30)
	if m.View().MouseMode != tea.MouseModeCellMotion {
		t.Fatal("no chips: want cell motion")
	}
	m.addAttachment(f["notes.md"])
	if m.View().MouseMode != tea.MouseModeAllMotion {
		t.Fatal("chips on screen: want all motion")
	}
}

// An image chip previews as a picture; a binary document says so.
func TestChipPreviewContent(t *testing.T) {
	_, f := dropFixture(t)
	img := chipPreviewLines(attachmentItem{Kind: "image", Path: f["My Shot.png"]}, 20, 10)
	if len(img) == 0 || !strings.Contains(img[0], "▀") {
		t.Fatalf("image preview = %q", img)
	}
	bin := filepath.Join(t.TempDir(), "blob.bin")
	if err := os.WriteFile(bin, []byte{0, 1, 2, 3}, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ansi.Strip(strings.Join(chipPreviewLines(attachmentItem{Kind: "file", Path: bin}, 40, 10), "\n")); !strings.Contains(got, "binary file") {
		t.Fatalf("binary preview = %q", got)
	}
}

// The sent message lists what it was sent with, and the chips are cleared.
func TestSentMessageShowsAttachments(t *testing.T) {
	dir, f := dropFixture(t)
	m := footerModel(100, 30)
	m.cwd = dir
	next, _ := m.Update(tea.PasteMsg{Content: "'" + f["notes.md"] + "'"})
	m = next.(Model)
	m.ta.SetValue("summarise this")
	next, _ = m.updateMain(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	if len(m.attachments) != 0 {
		t.Fatalf("chips not cleared after sending: %+v", m.attachments)
	}
	var sent *ChatMessage
	for i := range m.messages {
		if m.messages[i].Role == RoleUser {
			sent = &m.messages[i]
		}
	}
	if sent == nil || len(sent.Attachments) != 1 || sent.Attachments[0].Path != f["notes.md"] {
		t.Fatalf("sent message = %+v", sent)
	}
	body := ansi.Strip(m.renderMessageBlock(*sent, m.currentColor()))
	if !strings.Contains(body, "summarise this") || !strings.Contains(body, "▤ notes.md [1]") {
		t.Fatalf("sent message render lacks its attachment:\n%s", body)
	}
}

// tinyPNG is a 2x2 PNG.
var tinyPNG = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x02, 0x00, 0x00, 0x00, 0x02, 0x08, 0x02, 0x00, 0x00, 0x00, 0xfd, 0xd4, 0x9a,
	0x73, 0x00, 0x00, 0x00, 0x12, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x63, 0xf8, 0xcf, 0xc0, 0xc0,
	0x00, 0xc2, 0x0c, 0xff, 0x81, 0x00, 0x00, 0x1f, 0xee, 0x05, 0xfb, 0x0b, 0xd9, 0x68, 0x8b, 0x00,
	0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
}
