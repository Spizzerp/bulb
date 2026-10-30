//go:build !darwin || ios

package spell

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"time"
)

// listChecker runs a command-line checker in list mode (aspell list,
// hunspell -l): words in on stdin, the misspelled ones out, one per line.
type listChecker struct {
	path string
	args []string
}

func newPlatformChecker() checker {
	for _, c := range []listChecker{
		{path: "aspell", args: []string{"list"}},
		{path: "hunspell", args: []string{"-l"}},
	} {
		if p, err := exec.LookPath(c.path); err == nil {
			c.path = p
			return c
		}
	}
	return nil
}

func (c listChecker) misspelled(words []string) (map[string]bool, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.path, c.args...)
	cmd.Stdin = strings.NewReader(strings.Join(words, "\n"))
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, false
	}
	bad := map[string]bool{}
	for _, w := range strings.Fields(out.String()) {
		bad[w] = true
	}
	return bad, true
}
