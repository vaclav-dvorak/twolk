package tui

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"unicode/utf16"
)

// ArmorySnippet is the browser-console export helper (tools/
// armory-export.js), injected by main so the import modal can copy it.
var ArmorySnippet string

// clipboardTool is one platform clipboard CLI and how it wants its input.
type clipboardTool struct {
	args   []string
	encode func(string) io.Reader
}

// clipboardTools lists the CLIs tried in order for this platform.
func clipboardTools() []clipboardTool {
	if runtime.GOOS == "windows" {
		return []clipboardTool{{args: []string{"clip"}, encode: utf16WithBOM}}
	}
	return []clipboardTool{
		{args: []string{"pbcopy"}},                           // macOS
		{args: []string{"wl-copy"}},                          // Wayland
		{args: []string{"xclip", "-selection", "clipboard"}}, // X11
	}
}

// copyToClipboard writes text to the system clipboard via the platform's
// CLI tool.
func copyToClipboard(text string) error {
	var names []string
	for _, t := range clipboardTools() {
		names = append(names, t.args[0])
		if _, err := exec.LookPath(t.args[0]); err != nil {
			continue
		}
		cmd := exec.Command(t.args[0], t.args[1:]...)
		if t.encode != nil {
			cmd.Stdin = t.encode(text)
		} else {
			cmd.Stdin = strings.NewReader(text)
		}
		return cmd.Run()
	}
	return fmt.Errorf("no clipboard tool found (%s)", strings.Join(names, "/"))
}

// utf16WithBOM encodes text the way clip.exe reads it without mangling:
// plain stdin goes through the console's legacy code page (the snippet's
// "—" and "✓" would not survive), a UTF-16LE byte-order mark switches it
// to Unicode.
func utf16WithBOM(text string) io.Reader {
	var b bytes.Buffer
	b.Write([]byte{0xFF, 0xFE})
	for _, u := range utf16.Encode([]rune(text)) {
		_ = binary.Write(&b, binary.LittleEndian, u)
	}
	return &b
}
