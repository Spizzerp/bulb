package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Desktop notifications
// ---------------------------------------------------------------------------

// maybeNotify fires a notification (OSC 9 / BEL plus desktop) when the agent
// finishes, but only if the terminal is not currently focused or the run took
// more than 10 s.
func (m *Model) maybeNotify(runErr error) {
	elapsed := time.Since(m.agentStartAt)
	if elapsed < 10*time.Second && m.terminalFocused {
		return
	}
	if runErr != nil {
		m.notifier.Notify("Bulb", "Agent finished with an error")
	} else {
		m.notifier.Notify("Bulb", "Agent finished")
	}
}

// notifyIfUnfocused alerts on mid-run attention requests (approval dialog,
// agent question) only when the user has switched away from the terminal.
func (m *Model) notifyIfUnfocused(body string) {
	if m.terminalFocused {
		return
	}
	m.notifier.Notify("Bulb", body)
}

// ---------------------------------------------------------------------------
// File attachments (ctrl+f)
// ---------------------------------------------------------------------------

// addAttachment resolves path relative to cwd and appends an attachmentItem.
func (m *Model) addAttachment(rawPath string) {
	if label, ok := m.attachFile(rawPath); ok {
		m.showBanner(fmt.Sprintf("attached: %s (%d total)", label, len(m.attachments)), "success")
	}
}

// attachFile adds one attachment and returns its label, or reports the
// reason it was refused in a banner and returns false.
func (m *Model) attachFile(rawPath string) (string, bool) {
	rawPath = strings.TrimSpace(rawPath)
	if rawPath == "" {
		return "", false
	}
	abs := rawPath
	if !filepath.IsAbs(rawPath) {
		abs = filepath.Join(m.cwd, rawPath)
	}
	abs = filepath.Clean(abs)
	if _, err := os.Stat(abs); err != nil {
		m.showBanner("file not found: "+rawPath, "error")
		return "", false
	}
	rel, err := filepath.Rel(m.cwd, abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		// A file from outside the project (a drop from ~/Downloads) is
		// named by its full path: a run of ../../ says nothing useful.
		rel = abs
	}
	// Prevent duplicates
	for _, existing := range m.attachments {
		if existing.Path == abs {
			m.showBanner("already attached: "+rel, "info")
			return "", false
		}
	}
	// Image files go through the provider's vision channel, not inlined into
	// the prompt as (binary) text like ordinary file attachments.
	kind := "file"
	switch strings.ToLower(filepath.Ext(abs)) {
	case ".png", ".jpg", ".jpeg", ".webp", ".gif":
		if !m.providers.SupportsVision(m.cfg.ActiveProvider, m.cfg.ActiveModel) {
			m.showBanner("current model does not support vision", "error")
			return "", false
		}
		kind = "image"
	}
	m.attachments = append(m.attachments, attachmentItem{
		Kind:    kind,
		Path:    abs,
		RelPath: filepath.ToSlash(rel),
	})
	return rel, true
}

// injectAttachments appends each file attachment's content to the prompt.
// Image attachments (Kind="image") are skipped here; they are sent via the
// provider's image channel instead.
func (m Model) injectAttachments(prompt string) string {
	var sb strings.Builder
	sb.WriteString(prompt)
	for _, att := range m.attachments {
		if att.Kind != "file" {
			continue
		}
		content, err := os.ReadFile(att.Path)
		if err != nil {
			continue
		}
		sb.WriteString(fmt.Sprintf("\n\n--- attached: %s ---\n", att.RelPath))
		sb.WriteString(string(content))
	}
	return sb.String()
}

// ensureClipboardTempDir creates a temp directory for pasted images on first
// use and stores its path in m.clipboardTempDir.
func (m *Model) ensureClipboardTempDir() error {
	if m.clipboardTempDir != "" {
		return nil
	}
	dir, err := os.MkdirTemp("", "spettro-clipboard-*")
	if err != nil {
		return err
	}
	m.clipboardTempDir = dir
	return nil
}
