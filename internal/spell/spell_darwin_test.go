//go:build darwin && !ios

package spell

import (
	"reflect"
	"testing"
)

// The real macOS checker: obvious misspellings are reported, common words are
// not, and a batch maps every miss back to its own word.
func TestAppKitChecker(t *testing.T) {
	c := newPlatformChecker()
	if c == nil {
		t.Skip("NSSpellChecker unavailable")
	}
	bad, ok := c.misspelled([]string{"Thsi", "sentence", "recieve", "café", "smple", "the"})
	if !ok {
		t.Fatal("misspelled reported the checker unusable")
	}
	want := map[string]bool{"Thsi": true, "recieve": true, "smple": true}
	if !reflect.DeepEqual(bad, want) {
		t.Fatalf("misspelled = %v, want %v", bad, want)
	}
}
