package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Shared scrolling vocabulary: any region holding more lines than the
// screen gives it clips to a window and pins a slim scrollbar to the
// right screen edge — the one cue, identical on every screen, that
// there is more above or below the fold. Regions that may scroll are
// laid out one column short of the terminal so the bar never wraps.

var (
	scrollTrack = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	// the thumb carries the game version's accent color (applyTheme),
	// like the rest of the chrome
	scrollThumb = lipgloss.NewStyle().Foreground(lipgloss.Color("81"))
)

// clampOff clamps a scroll offset so a visible-row window stays inside
// total content lines.
func clampOff(off, total, visible int) int {
	return max(0, min(off, total-visible))
}

// scrollbar renders the bar column for a visible-row window at off
// into total lines; "" when everything fits.
func scrollbar(total, visible, off int) string {
	if total <= visible || visible < 1 {
		return ""
	}
	thumb := max(1, visible*visible/total)
	maxOff := total - visible
	pos := (clampOff(off, total, visible)*(visible-thumb) + maxOff/2) / maxOff
	rows := make([]string, visible)
	for i := range rows {
		if i >= pos && i < pos+thumb {
			rows[i] = scrollThumb.Render("█")
		} else {
			rows[i] = scrollTrack.Render("│")
		}
	}
	return strings.Join(rows, "\n")
}

// paneScrollStep maps a key to an info-pane scroll step: j/k line by
// line, page keys by a window minus one row of overlap. The cursor
// keys stay with the list — every screen splits them the same way.
func paneScrollStep(key string, window int) (step int, ok bool) {
	switch key {
	case "j":
		return 1, true
	case "k":
		return -1, true
	case "pgdown":
		return max(window-1, 1), true
	case "pgup":
		return -max(window-1, 1), true
	}
	return 0, false
}

// listPaneSplit divides the rows under the header between a cursor
// list and its info pane: the list is promised listWant rows — the
// promise holds even on short terminals (the pane shrinks to a 3-row
// scrolling window before the list gives up a single wanted row) —
// the pane takes the rest up to its full height and scrolls when
// clipped, and leftover space goes back to the list. Both full-screen
// list views share the same fixed chrome around the two regions:
// header gap, context row, gap, one summary/legend row and the help
// footer.
func (m Model) listPaneSplit(headerH, listWant, paneFull int) (listVis, paneVis int) {
	avail := max(m.height-headerH-7, 9)
	listVis = max(min(listWant, avail-3), 3)
	paneVis = min(paneFull, avail-listVis)
	listVis = max(avail-paneVis, listVis)
	return listVis, paneVis
}

// scrollRegion clips a rendered block to a visible-row window starting
// at off and pins the scrollbar to column width; a block that already
// fits comes back unchanged. Content lines must stay under width — the
// bar takes the last column.
func scrollRegion(block string, off, visible, width int) string {
	lines := strings.Split(block, "\n")
	if len(lines) <= visible {
		return block
	}
	off = clampOff(off, len(lines), visible)
	view := strings.Join(lines[off:off+visible], "\n")
	return lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.NewStyle().Width(width-1).Render(view),
		scrollbar(len(lines), visible, off))
}
