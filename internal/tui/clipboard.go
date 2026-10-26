package tui

import (
	"fmt"
	"os/exec"
	"strings"
)

// ArmorySnippet is the browser-console export helper (tools/
// armory-export.js), injected by main so the import modal can copy it.
var ArmorySnippet string

// copyToClipboard writes text to the system clipboard via the platform's
// CLI tool.
func copyToClipboard(text string) error {
	var cmd *exec.Cmd
	for _, c := range [][]string{
		{"pbcopy"},                           // macOS
		{"wl-copy"},                          // Wayland
		{"xclip", "-selection", "clipboard"}, // X11
	} {
		if _, err := exec.LookPath(c[0]); err == nil {
			cmd = exec.Command(c[0], c[1:]...)
			break
		}
	}
	if cmd == nil {
		return fmt.Errorf("no clipboard tool found (pbcopy/wl-copy/xclip)")
	}
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}
