package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// TestScrollbar pins the shared cue: no bar when content fits, a
// visible-tall bar otherwise, thumb at the top for offset 0 and flush
// with the bottom at the maximum offset.
func TestScrollbar(t *testing.T) {
	if scrollbar(10, 10, 0) != "" {
		t.Error("fitting content must render no bar")
	}
	rows := strings.Split(ansi.Strip(scrollbar(40, 10, 0)), "\n")
	if len(rows) != 10 {
		t.Fatalf("bar height %d, want 10", len(rows))
	}
	if rows[0] != "█" || rows[9] != "│" {
		t.Errorf("offset 0 thumb must sit at the top: %q", rows)
	}
	rows = strings.Split(ansi.Strip(scrollbar(40, 10, 30)), "\n")
	if rows[9] != "█" || rows[0] != "│" {
		t.Errorf("max offset thumb must sit at the bottom: %q", rows)
	}
}

// TestScrollRegion pins the clipping: a fitting block passes through
// untouched; an overflowing one clips to the window at the offset and
// carries the bar in the last column.
func TestScrollRegion(t *testing.T) {
	if got := scrollRegion("a\nb", 0, 5, 20); got != "a\nb" {
		t.Errorf("fitting block must pass through, got %q", got)
	}
	block := "l0\nl1\nl2\nl3\nl4\nl5\nl6\nl7"
	got := ansi.Strip(scrollRegion(block, 3, 4, 10))
	lines := strings.Split(got, "\n")
	if len(lines) != 4 || !strings.HasPrefix(lines[0], "l3") {
		t.Fatalf("window at offset 3 wrong:\n%q", got)
	}
	for i, l := range lines {
		if r := []rune(l); len(r) != 10 || (r[9] != '│' && r[9] != '█') {
			t.Errorf("row %d missing the bar column: %q", i, l)
		}
	}
	// offsets past the end clamp to the last window
	if end := ansi.Strip(scrollRegion(block, 99, 4, 10)); !strings.HasPrefix(end, "l4") {
		t.Errorf("overshoot must clamp to the tail:\n%q", end)
	}
}
