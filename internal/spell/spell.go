// Package spell finds misspelled words with the spell checker the system
// already has, so bulb ships no dictionary: NSSpellChecker on macOS (the one
// TextEdit uses, with the user's languages and learned words), and aspell or
// hunspell elsewhere when one is installed. Where none is, Available reports
// false and nothing is ever marked.
//
// Like internal/clipboard, nothing native is loaded until the first check:
// importing the package costs a process nothing.
package spell

import (
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// Span is a misspelled word in the checked text, as byte offsets.
type Span struct{ Start, End int }

// checker is the platform backend: it returns the misspelled words among
// words, each reported once. ok is false when the backend is unusable.
type checker interface {
	misspelled(words []string) (bad map[string]bool, ok bool)
}

var (
	backendOnce sync.Once
	backend     checker

	// cache remembers every verdict, keyed by word, so a draft is never sent
	// to the backend twice for the same word: typing only ever checks the
	// word that changed. It is bounded by maxCached.
	cacheMu sync.Mutex
	cache   = map[string]bool{}
)

const maxCached = 20000

// Available reports whether a spell checker exists on this system. The first
// call looks for one (and on macOS loads AppKit).
func Available() bool {
	backendOnce.Do(func() { backend = newPlatformChecker() })
	return backend != nil
}

// Check returns the misspelled words in text, in order. It skips what is not
// prose (see candidate) and returns nil when no checker is available.
func Check(text string) []Span {
	if !Available() {
		return nil
	}
	words := Words(text)
	if len(words) == 0 {
		return nil
	}
	var unknown []string
	cacheMu.Lock()
	seen := map[string]bool{}
	for _, w := range words {
		word := text[w.Start:w.End]
		if _, ok := cache[word]; !ok && !seen[word] {
			seen[word] = true
			unknown = append(unknown, word)
		}
	}
	cacheMu.Unlock()
	if len(unknown) > 0 {
		bad, ok := backend.misspelled(unknown)
		if !ok {
			return nil
		}
		cacheMu.Lock()
		if len(cache)+len(unknown) > maxCached {
			cache = map[string]bool{}
		}
		for _, w := range unknown {
			cache[w] = bad[w]
		}
		cacheMu.Unlock()
	}
	var out []Span
	cacheMu.Lock()
	for _, w := range words {
		if cache[text[w.Start:w.End]] {
			out = append(out, w)
		}
	}
	cacheMu.Unlock()
	return out
}

// Words returns the spans of the words worth checking in text: runs of
// letters (with inner apostrophes), skipping anything that reads as code or a
// name rather than prose: words with digits, underscores or inner capitals
// (camelCase, HTTPServer), words inside `backticks`, @mentions, /commands,
// paths, URLs and flags, and words of one or two letters.
func Words(text string) []Span {
	var out []Span
	inCode := false
	i := 0
	for i < len(text) {
		r, size := utf8.DecodeRuneInString(text[i:])
		if r == '`' {
			inCode = !inCode
			i += size
			continue
		}
		if unicode.IsSpace(r) {
			i += size
			continue
		}
		// A token is everything up to the next space or backtick.
		start := i
		for i < len(text) {
			r, size := utf8.DecodeRuneInString(text[i:])
			if unicode.IsSpace(r) || r == '`' {
				break
			}
			i += size
		}
		if inCode {
			continue
		}
		out = append(out, tokenWords(text, start, i)...)
	}
	return out
}

// tokenWords picks the checkable word out of one space-separated token.
func tokenWords(text string, start, end int) []Span {
	tok := text[start:end]
	switch {
	case strings.ContainsAny(tok[:1], "@/~$-#.\\"),
		strings.Contains(tok, "://"),
		strings.ContainsAny(tok, "/\\=<>{}[]|*_"):
		return nil
	}
	// Trim surrounding punctuation ("word," "(word)" "'word'"), but not
	// digits: "utf8" is a name, not the word "utf".
	lo, hi := 0, len(tok)
	for lo < hi {
		r, size := utf8.DecodeRuneInString(tok[lo:])
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			break
		}
		lo += size
	}
	for hi > lo {
		r, size := utf8.DecodeLastRuneInString(tok[:hi])
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			break
		}
		hi -= size
	}
	word := tok[lo:hi]
	if !candidate(word) {
		return nil
	}
	return []Span{{start + lo, start + hi}}
}

// candidate reports whether word looks like a prose word: letters and inner
// apostrophes only, at least three letters, and no capital after the first
// letter (an acronym or camelCase identifier).
func candidate(word string) bool {
	letters := 0
	for i, r := range word {
		switch {
		case unicode.IsLetter(r):
			if i > 0 && unicode.IsUpper(r) {
				return false
			}
			letters++
		case r == '\'' || r == '’':
		default:
			return false
		}
	}
	return letters >= 3
}
