package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/vaclav-dvorak/twolk/internal/db"
	"github.com/vaclav-dvorak/twolk/internal/score"
)

// The upgrade browser (enter on a gear row): ranked candidates for
// one slot, compare pane below (compare.go renders the pane).

func (m *Model) recomputeUpgrades() {
	m.upList = nil
	m.upCursor = 0
	m.upPaneOff = 0
	rows := m.gearRows()
	if m.scorer == nil || m.upSlot >= len(rows) {
		return
	}
	ws := wornOf(rows)
	m.upList = m.dedupeUps(m.scorer.UpgradesWorn(ws, m.upSlot, 200, m.allowItem))
	// a few nearest-worse items below the upgrades: knowing what sits
	// just under the equipped piece is context, not noise — but the
	// richer the upgrade list, the less it needs: 5 rows when the slot
	// is nearly done, 2 with real choices above, none when upgrades
	// alone overflow the screen
	down := 5
	switch {
	case len(m.upList) > 10:
		down = 0
	case len(m.upList) > 2:
		down = 2
	}
	m.upList = m.dedupeUps(append(m.upList, m.scorer.DowngradesWorn(ws, m.upSlot, down, m.allowItem)...))
}

// dedupeUps collapses candidates that duplicate an earlier row: wowhead
// ships faction variants of one quest reward as distinct item IDs with
// identical name, stats and source ("Lifecrusher Treads" twice, both
// "Quest: Coup de Grace"), and the second row tells the user nothing.
// Anything that renders or scores differently survives.
func (m Model) dedupeUps(ups []score.Upgrade) []score.Upgrade {
	type key struct {
		name, source, sockets string
		ilvl                  int
		stats, bonus          db.Stats
	}
	seen := make(map[key]bool, len(ups))
	out := ups[:0]
	for _, u := range ups {
		k := key{displayName(u.Item), ansi.Strip(m.sourceText(u.Item)),
			fmt.Sprint(u.Item.GemSockets), u.Item.ILvl, u.Item.Stats, u.Item.SocketBonus}
		if !seen[k] {
			seen[k] = true
			out = append(out, u)
		}
	}
	return out
}

func (m Model) updateUpgrades(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q":
		m.mode = modeNormal
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	case "up":
		if m.upCursor > 0 {
			m.upCursor--
			m.upPaneOff = 0 // the compare pane follows the cursor
		}
		return m, nil
	case "down":
		if m.upCursor < len(m.upList)-1 {
			m.upCursor++
			m.upPaneOff = 0
		}
		return m, nil
	case "j", "k", "pgdown", "pgup":
		// scroll the compare pane; the candidate list stays put
		if len(m.upList) == 0 {
			return m, nil
		}
		cursor := min(m.upCursor, len(m.upList)-1)
		pane := m.viewCompare(m.gearRows()[m.upSlot].worn(), m.upList[cursor].Item)
		m.upPaneOff = m.scrollInfoPane(msg.String(), m.upPaneOff, min(len(m.upList), 10), pane)
		return m, nil
	case "f":
		m.mode = modeFilter
		m.fromUps, m.fromBis = true, false
		return m, nil
	}
	return m, nil
}

func (m Model) viewUpgrades() string {
	rows := m.gearRows()
	curRow := rows[m.upSlot]
	cur := curRow.item

	header := m.viewHeader()
	context := titleStyle.Render("▸ Upgrades — "+cur.Slot.String()) +
		"   " + dimStyle.Render(m.filterSummary())

	help := helpLine("↑/↓", "select", "f", "filter", "esc", "back")

	if len(m.upList) == 0 {
		return m.pinFooter(lipgloss.JoinVertical(lipgloss.Left, header, "", context, "",
			okStyle.Render("✦ best in slot")+dimStyle.Render(" within the current filter")), help)
	}

	cursor := min(m.upCursor, len(m.upList)-1)
	compare := m.viewCompare(curRow.worn(), m.upList[cursor].Item)

	// the list gets everything between the header block and the compare
	// pane; a pane that would squeeze the list below its promised rows
	// clips and scrolls (j/k) instead
	visible, paneVis := m.listPaneSplit(lipgloss.Height(header), min(len(m.upList), 10), lipgloss.Height(compare))
	if lipgloss.Height(compare) > paneVis {
		help = helpLine("j/k", "scroll info") + "  " + help
		compare = scrollRegion(compare, m.upPaneOff, paneVis, m.width)
	}
	// the reforge mark earns its column only when some row carries it
	marked := false
	for _, u := range m.upList {
		marked = marked || u.ReforgeDependent()
	}
	// only context items below: the slot is already best in slot. The
	// note costs one row, so charge it before the window, the
	// scrollbar and the columns are sized off visible
	bis := m.upList[0].Delta <= 0.5
	if bis {
		visible = max(visible-1, 1)
	}
	start := max(0, min(cursor-visible/2, len(m.upList)-visible))
	rowW := m.width
	if len(m.upList) > visible {
		rowW = m.width - 1 // the shared scrollbar takes the last column
	}

	// name column: fixed columns take 22 (delta 8, tier 8, ilvl 4,
	// gap 2), plus 2 for the reforge mark where it shows; leave ~55 for
	// the source text and give the rest to the name, within sane bounds
	fixed := 22
	if marked {
		fixed += 2
	}
	nameW := min(max(rowW-fixed-55, 42), 70)
	var lines []string
	if bis {
		lines = append(lines, okStyle.Render(" ✦ best in slot")+
			dimStyle.Render(" within the current filter — closest alternatives:"))
	}
	for i := start; i < min(start+visible, len(m.upList)); i++ {
		u := m.upList[i]
		sel := rowStyler(i == cursor)
		name := displayName(u.Item)
		// mark the ilvl move vs the EQUIPPED piece: green ↑ above it,
		// red ↓ below (a stat-upgrade can still be an ilvl downgrade)
		ilvl := sel(dimStyle).Render(fmt.Sprintf("%3d ", u.Item.ILvl))
		switch {
		case u.Item.ILvl > cur.ILvl:
			ilvl = sel(okStyle).Render(fmt.Sprintf("%3d↑", u.Item.ILvl))
		case u.Item.ILvl < cur.ILvl:
			ilvl = sel(errStyle).Render(fmt.Sprintf("%3d↓", u.Item.ILvl))
		}
		src := m.sourceText(u.Item)
		if i == cursor {
			// the source text carries its own inner styles; flatten it so
			// the highlight bar runs through it uninterrupted
			src = sel(dimStyle).Render(ansi.Strip(src))
		}
		deltaCell := sel(okStyle).Render(fmt.Sprintf(" ▲%5.0f ", u.Delta))
		if u.Delta <= 0.5 {
			deltaCell = sel(errStyle).Render(fmt.Sprintf(" ▼%5.0f ", u.Delta))
		}
		if marked {
			mark := "  "
			if u.ReforgeDependent() {
				mark = "⟳ "
			}
			deltaCell += sel(dimStyle).Render(mark)
		}
		row := deltaCell +
			heroicCell(sel, qualityStyles[u.Item.Quality], name, nameW) +
			sel(tierStyle).Render(fmt.Sprintf(" %-7s", m.tierLabel(u.Item))) +
			ilvl +
			sel(lipgloss.NewStyle()).Render("  ") +
			src
		if pad := rowW - lipgloss.Width(row); pad > 0 && i == cursor {
			row += sel(lipgloss.NewStyle()).Render(strings.Repeat(" ", pad))
		}
		lines = append(lines, truncate(row, rowW))
	}
	list := strings.Join(lines, "\n")
	if len(m.upList) > visible {
		list = m.listWithBar(list, len(m.upList), min(start+visible, len(m.upList))-start, start)
	}
	scroll := dimStyle.Render(fmt.Sprintf("%d/%d", cursor+1, len(m.upList))) +
		dimStyle.Render("  ·  ") + okStyle.Render("↑") + dimStyle.Render(" ilvl above equipped") +
		dimStyle.Render("  ·  ") + errStyle.Render("↓") + dimStyle.Render(" below") +
		dimStyle.Render("  ·  ") + errStyle.Render("▼") + dimStyle.Render(" worse than equipped (context)")
	if marked {
		scroll += dimStyle.Render("  ·  ⟳ needs a reforge pass")
	}

	return m.pinFooter(lipgloss.JoinVertical(lipgloss.Left,
		header, "", context, "", list, scroll, compare), help)
}
