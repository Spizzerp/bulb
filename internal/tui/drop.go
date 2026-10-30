package tui

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// Dragging files from Finder (or any file manager) onto the terminal pastes
// their paths. A paste that is nothing but paths to files that exist is taken
// as a drop: each file becomes an attachment chip and the draft is left as it
// was. Anything else, including a single path that does not exist, is pasted
// as text, so pasting a path on purpose still works by typing around it.

// droppedPaths returns the files a pasted text names when it is only paths,
// each resolved against cwd; ok is false when it is anything else.
//
// Terminals quote a drop in their own way, and all of these are accepted:
//
//	/Users/me/My\ Shot.png              backslash-escaped (Terminal.app, iTerm2, Ghostty)
//	'/Users/me/My Shot.png'             single-quoted (kitty, WezTerm, GNOME Terminal)
//	"C:\Users\me\My Shot.png"           double-quoted (Windows Terminal)
//	file:///Users/me/My%20Shot.png      file URL (some Linux file managers)
//
// several files separated by spaces or newlines.
func droppedPaths(pasted, cwd string) (paths []string, ok bool) {
	fields := splitDropFields(strings.TrimSpace(pasted))
	if len(fields) == 0 {
		return nil, false
	}
	for _, f := range fields {
		p := f
		if strings.HasPrefix(p, "file://") {
			u, err := url.Parse(p)
			if err != nil || (u.Host != "" && u.Host != "localhost") {
				return nil, false
			}
			p = u.Path
		}
		if strings.HasPrefix(p, "~/") {
			if home, err := os.UserHomeDir(); err == nil {
				p = filepath.Join(home, p[2:])
			}
		}
		if !filepath.IsAbs(p) {
			p = filepath.Join(cwd, p)
		}
		info, err := os.Stat(p)
		if err != nil || info.IsDir() {
			return nil, false
		}
		paths = append(paths, filepath.Clean(p))
	}
	return paths, true
}

// splitDropFields splits a pasted drop into its paths, undoing the shell
// quoting terminals apply: backslash escapes outside quotes, and single or
// double quotes around a path. An unbalanced quote makes it not a drop.
func splitDropFields(s string) []string {
	var out []string
	var cur strings.Builder
	inField := false
	var quote rune
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
				continue
			}
			cur.WriteRune(r)
		case r == '\'' || r == '"':
			quote = r
			inField = true
		case r == '\\' && i+1 < len(runes) && filepath.Separator != '\\':
			// A backslash escapes the next character (a space, a paren). On
			// Windows a backslash is a path separator, and a path with spaces
			// arrives double-quoted instead.
			i++
			cur.WriteRune(runes[i])
			inField = true
		case unicode.IsSpace(r):
			if inField {
				out = append(out, cur.String())
				cur.Reset()
				inField = false
			}
		default:
			cur.WriteRune(r)
			inField = true
		}
	}
	if quote != 0 {
		return nil
	}
	if inField {
		out = append(out, cur.String())
	}
	return out
}

// attachDropped attaches each dropped file and reports how many were added.
// It runs addAttachment's checks (vision support for images, duplicates), so
// a file the model cannot take is refused the same way ctrl+f refuses it;
// the banner of the last refusal stays up, otherwise one summary replaces
// the per-file ones.
func (m *Model) attachDropped(paths []string) int {
	added := 0
	refused := false
	var last string
	for _, p := range paths {
		if label, ok := m.attachFile(p); ok {
			added++
			last = label
		} else {
			refused = true
		}
	}
	switch {
	case refused:
	case added == 1:
		m.showBanner("attached: "+last+" — hover a chip for its path, click to preview", "success")
	case added > 1:
		m.showBanner(fmt.Sprintf("attached %d files — hover a chip for its path, click to preview", added), "success")
	}
	return added
}
