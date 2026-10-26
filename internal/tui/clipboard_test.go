package tui

import (
	"io"
	"testing"
)

// TestUTF16WithBOM: clip.exe needs the BOM and little-endian UTF-16 to
// keep the snippet's non-ASCII characters.
func TestUTF16WithBOM(t *testing.T) {
	got, _ := io.ReadAll(utf16WithBOM("a—✓"))
	want := []byte{0xFF, 0xFE, 'a', 0, 0x14, 0x20, 0x13, 0x27}
	if string(got) != string(want) {
		t.Errorf("utf16WithBOM = % x, want % x", got, want)
	}
}
