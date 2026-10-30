package spell

import (
	"os"
	"reflect"
	"testing"

	"spettro/internal/testhome"
)

func TestMain(m *testing.M) { os.Exit(testhome.Main(m)) }

func TestHomeIsIsolated(t *testing.T) { testhome.Check(t) }

func words(text string) []string {
	var out []string
	for _, s := range Words(text) {
		out = append(out, text[s.Start:s.End])
	}
	return out
}

// Only prose is checked: code, names, paths, mentions and short words are
// skipped, and punctuation around a word is not part of it.
func TestWordsSkipsWhatIsNotProse(t *testing.T) {
	for text, want := range map[string][]string{
		"Thsi is a smple sentense.":                 {"Thsi", "smple", "sentense"},
		"fix `recieve_data` and recieve it":         {"fix", "and", "recieve"},
		"run gofmt on internal/tui/model.go now":    {"run", "gofmt", "now"},
		"see @docs/spec.md and /help, then ~/notes": {"see", "and", "then"},
		"camelCase HTTPServer v2 utf8 x_y fine":     {"fine"},
		"(quoted) \"words\" don't wouldn’t":         {"quoted", "words", "don't", "wouldn’t"},
		"https://example.com/path --flag ok":        nil,
		"":                                          nil,
		"line one\nline twoo":                       {"line", "one", "line", "twoo"},
	} {
		if got := words(text); !reflect.DeepEqual(got, want) {
			t.Errorf("Words(%q) = %q, want %q", text, got, want)
		}
	}
}

// Spans are byte offsets into the original text, multi-byte runes included.
func TestWordsOffsetsAreBytes(t *testing.T) {
	text := "café naïve façade"
	got := Words(text)
	want := []string{"café", "naïve", "façade"}
	for i, s := range got {
		if text[s.Start:s.End] != want[i] {
			t.Fatalf("span %d = %q, want %q", i, text[s.Start:s.End], want[i])
		}
	}
}

// A fake backend: Check maps its verdicts back onto every occurrence, asks
// the backend about each word once, and never again for a cached word.
type fakeChecker struct {
	bad   map[string]bool
	asked [][]string
}

func (f *fakeChecker) misspelled(words []string) (map[string]bool, bool) {
	f.asked = append(f.asked, append([]string(nil), words...))
	out := map[string]bool{}
	for _, w := range words {
		if f.bad[w] {
			out[w] = true
		}
	}
	return out, true
}

func withFake(t *testing.T, f *fakeChecker) {
	t.Helper()
	backendOnce.Do(func() {})
	saved := backend
	backend = f
	cacheMu.Lock()
	cache = map[string]bool{}
	cacheMu.Unlock()
	t.Cleanup(func() {
		backend = saved
		cacheMu.Lock()
		cache = map[string]bool{}
		cacheMu.Unlock()
	})
}

func TestCheckMarksEveryOccurrenceAndCaches(t *testing.T) {
	f := &fakeChecker{bad: map[string]bool{"teh": true}}
	withFake(t, f)

	text := "teh cat and teh dog"
	got := Check(text)
	want := []Span{{0, 3}, {12, 15}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Check = %v, want %v", got, want)
	}
	if len(f.asked) != 1 || !reflect.DeepEqual(f.asked[0], []string{"teh", "cat", "and", "dog"}) {
		t.Fatalf("backend asked %q, want each word once", f.asked)
	}
	// The same words again: answered from the cache, the backend is not asked.
	Check(text)
	// One new word: only it is asked.
	Check(text + " emu")
	if len(f.asked) != 2 || !reflect.DeepEqual(f.asked[1], []string{"emu"}) {
		t.Fatalf("backend asked %q, want only the new word", f.asked)
	}
}

func TestCheckWithoutBackendMarksNothing(t *testing.T) {
	backendOnce.Do(func() {})
	saved := backend
	backend = nil
	t.Cleanup(func() { backend = saved })
	if got := Check("teh cat"); got != nil {
		t.Fatalf("Check with no checker = %v, want nil", got)
	}
}
