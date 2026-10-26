package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/vaclav-dvorak/twolk/internal/score"
)

// maxTips is the target number of tip lines in the pane (the meta
// warning counts toward it; the screen budget may shrink it further).
const maxTips = 5

// itemHint is the best available upgrade for one gear slot.
type itemHint struct {
	rowIdx int
	up     score.Upgrade
}

// tipsData is the precomputed content of the quick-tips pane; rebuilt
// whenever the scorer or filter changes (recomputeTips).
//
// Gem and enchant problems are summarized to a count and a gain: the
// pane points at the advisor screens (g, e) instead of duplicating
// their suggestions — two algorithms rendering the same sockets told
// conflicting stories.
type tipsData struct {
	gemItems  int     // items the optimization plan would re-gem
	gemGain   float64 // the gem section's gain (exact, cap-aware)
	enchItems int     // items it would re-enchant
	enchGain  float64 // the enchant section's combined gain
	refItems  int     // items it would re-reforge (cata only)
	refGain   float64 // the reforge section's combined gain
	sockets   []score.SocketTip
	items     []itemHint
}

// professions returns the character's professions by display name.
func (m Model) professions() map[string]bool {
	out := map[string]bool{}
	if m.char == nil {
		return out
	}
	for _, p := range m.char.Professions {
		out[p.Name] = true
	}
	return out
}

// recomputeTips refreshes the quick-tips pane content.
func (m *Model) recomputeTips() {
	m.tips = tipsData{}
	if m.scorer == nil {
		return
	}
	rows := m.gearRows()
	ws := wornOf(rows)
	if len(ws) == 0 {
		return
	}
	// one joint plan feeds all three advisor pointers; per-domain gains
	// price each section alone against the worn set (indicative — the
	// advisor's total is the exact combined number)
	plan := m.scorer.Optimize(ws)
	m.tips.gemItems, m.tips.gemGain = len(plan.Gems.Tips), plan.Gems.Delta
	m.tips.enchItems, m.tips.enchGain = len(plan.Enchants), m.scorer.EnchantGain(ws, plan.Enchants)
	m.tips.refItems, m.tips.refGain = len(plan.Reforges), m.scorer.ReforgeGain(ws, plan.Reforges)
	m.tips.sockets = m.scorer.SocketTips(ws)
	for i := range ws {
		if ups := m.scorer.UpgradesWorn(ws, i, 1, m.allowItem); len(ups) > 0 {
			m.tips.items = append(m.tips.items, itemHint{rowIdx: i, up: ups[0]})
		}
	}
	// keep the FULL list (one hint per upgradeable slot): the tips pane
	// caps what it shows itself, and the gear list derives its best-in-
	// slot ticks from the slots absent here
	sortHints(m.tips.items)
}

func sortHints(hs []itemHint) {
	for i := 1; i < len(hs); i++ {
		for j := i; j > 0 && hs[j].up.Delta > hs[j-1].up.Delta; j-- {
			hs[j], hs[j-1] = hs[j-1], hs[j]
		}
	}
}

// viewTips renders the quick-tips pane (full width, below the gear
// panes), or "" when there is nothing to say or no room. budget is the
// number of tip lines that fit on screen (title and border excluded);
// the pane never grows past it, so the help line stays visible.
//
// Priority: fix what you own before shopping. One-line pointers to the
// gem and enchant advisors come first, then socket fixes, and gear
// upgrade hints fill the remaining lines — with clean gems and enchants
// the pane is all gear hints.
func (m Model) viewTips(budget int) string {
	if m.scorer == nil || budget <= 0 {
		return ""
	}
	rows := m.gearRows()
	slotOf := func(i int) string {
		if i < len(rows) {
			return rows[i].item.Slot.String()
		}
		return "?"
	}
	width := max(m.width-4, 40) // box interior: borders + padding
	// tipLine caps a tip at the pane width, reserving room for the gain
	// suffix so it never pushes past the border
	tipLine := func(body string, delta float64) string {
		g := okStyle.Render(fmt.Sprintf(" +%.0f", delta))
		return truncate(body, width-lipgloss.Width(g)) + g
	}

	var lines []string
	if g, cond := m.scorer.InactiveMeta(wornOf(rows)); g != nil {
		note := ""
		if cond.OneOfEach() {
			note = " (one Nightmare Tear covers all three)"
		}
		lines = append(lines, errStyle.Render("⚠ ")+gemText(g)+errStyle.Render(" likely inactive")+
			dimStyle.Render(" — needs "+cond.String()+note))
	}
	if g := m.scorer.UnknownMeta(wornOf(rows)); g != nil {
		// no rule is invented for an uncurated meta — this line is the
		// prompt to add it to the rules table (db/metagems.go)
		lines = append(lines, warnStyle.Render("⚠ ")+gemText(g)+
			warnStyle.Render(" activation rule unknown")+
			dimStyle.Render(fmt.Sprintf(" — not modeled; curate gem %d in db/metagems.go", g.ID)))
	}

	// advisor pointers first: fix the gear you own before shopping
	countItems := func(n int) string {
		if n == 1 {
			return "1 item"
		}
		return fmt.Sprintf("%d items", n)
	}
	if m.tips.gemItems > 0 {
		lines = append(lines, tipLine(warnStyle.Render("◆ ")+
			dimStyle.Render("gems on "+countItems(m.tips.gemItems)+" not optimal — press ")+
			helpKey.Render("o")+dimStyle.Render(" to optimize"), m.tips.gemGain))
	}
	if m.tips.enchItems > 0 {
		lines = append(lines, tipLine(warnStyle.Render("✎ ")+
			dimStyle.Render("enchants on "+countItems(m.tips.enchItems)+" not optimal — press ")+
			helpKey.Render("o")+dimStyle.Render(" to optimize"), m.tips.enchGain))
	}
	if m.tips.refItems > 0 {
		lines = append(lines, tipLine(warnStyle.Render("⚒ ")+
			dimStyle.Render("reforges on "+countItems(m.tips.refItems)+" not optimal — press ")+
			helpKey.Render("o")+dimStyle.Render(" to optimize"), m.tips.refGain))
	}
	for _, t := range m.tips.sockets {
		name := m.game.Rules.BuckleName
		if t.ItemIdx < len(rows) {
			name = m.game.Rules.ExtraSocketName(rows[t.ItemIdx].item.Slot)
		}
		lines = append(lines, tipLine(warnStyle.Render("▣ ")+
			dimStyle.Render(slotOf(t.ItemIdx)+": apply ")+name+
			dimStyle.Render(", socket ")+gemText(t.Gem), t.Delta))
	}

	// gear upgrade hints fill the remaining lines, best first, each with
	// the same route text the upgrade browser shows (where to farm it,
	// dimmed when the filter has the route closed). The names are padded
	// to the widest one shown so the sources line up as a column.
	limit := min(budget, maxTips)
	var hints []itemHint
	for _, h := range m.tips.items {
		if len(lines)+len(hints) >= limit {
			break
		}
		hints = append(hints, h)
	}
	heads := make([]string, len(hints))
	headW := 0
	for i, h := range hints {
		heads[i] = okStyle.Render("⬆ ") + dimStyle.Render(slotOf(h.rowIdx)+": ") +
			m.itemLabel(h.up.Item, rowStyler(false))
		headW = max(headW, lipgloss.Width(heads[i]))
	}
	headW = min(headW, width/2) // a long name never crowds out the route
	for i, h := range hints {
		head := truncate(heads[i], headW)
		head += strings.Repeat(" ", max(headW-lipgloss.Width(head), 0))
		lines = append(lines, tipLine(head+"  "+m.sourceText(h.up.Item), h.up.Delta))
	}

	if len(lines) == 0 {
		lines = append(lines, okStyle.Render("✦ nothing to improve")+
			dimStyle.Render(" — gems, enchants and gear look optimal within the current filter"))
	}
	// hard clamp: on a tiny terminal even the priority lines give way —
	// the pane must never push the help line off screen
	if len(lines) > budget {
		lines = lines[:budget]
	}
	body := titleStyle.Render("Quick tips") + "\n" + strings.Join(lines, "\n")
	return detailStyle.Width(max(m.width-2, 40)).Render(body)
}
