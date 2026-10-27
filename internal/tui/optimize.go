package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/vaclav-dvorak/twolk/internal/db"
	"github.com/vaclav-dvorak/twolk/internal/score"
)

// The optimization advisor (o; g/e/R open it too): gems, enchants and
// reforges solved as ONE plan by score.Optimize — the three domains
// couple through the same capped totals, so separate advisors seesaw
// (a better enchant drops hit, the gem pass compensates, the reforge
// pass undoes it). The reforge section only exists under versions with
// reforging (cata); WotLK sees gems + enchants.

func (m *Model) openOptimize() {
	m.optPlan = m.scorer.Optimize(wornOf(m.gearRows()))
	m.optNote = ""
	m.optOff = 0
	m.mode = modeOptimize
}

func (m Model) updateOptimize(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "o", "g", "e", "R", "q", "enter":
		m.mode = modeNormal
		return m, nil
	case "up", "k", "down", "j", "pgup", "pgdown":
		// the plan has no cursor: the keys scroll the whole body
		body := m.optBody(m.gearRows())
		if body == "" {
			return m, nil
		}
		vis := m.optVisible(lipgloss.Height(m.viewHeader()))
		step := 1
		if k := msg.String(); k == "pgup" || k == "pgdown" {
			step = max(vis-1, 1)
		}
		if k := msg.String(); k == "up" || k == "k" || k == "pgup" {
			step = -step
		}
		m.optOff = clampOff(m.optOff+step, lipgloss.Height(body), vis)
		return m, nil
	case "c":
		text := m.optPlanText()
		if text == "" {
			m.optNote = warnStyle.Render("nothing to copy — no changes suggested")
			return m, nil
		}
		if err := copyToClipboard(text); err != nil {
			m.optNote = errStyle.Render("✗ " + err.Error())
			return m, nil
		}
		m.optNote = okStyle.Render("✓ suggestions copied to clipboard")
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	}
	return m, nil
}

// reforgeText renders one reforge ("67 Mastery → Hit"); nil reads as
// the unreforged state.
func (m Model) reforgeText(r *db.Reforge) string {
	if r == nil {
		return dimStyle.Render("not reforged")
	}
	return fmt.Sprintf("%.0f %s → %s", r.Amount, m.statLabel(r.From), m.statLabel(r.To))
}

// reforgeStats is the stat shift a reforge applies (zero for nil) —
// the compare pane prices both sides' reforges through it.
func reforgeStats(r *db.Reforge) db.Stats {
	var st db.Stats
	if r != nil {
		st[r.From] -= r.Amount
		st[r.To] += r.Amount
	}
	return st
}

// optItemHeader is the "Slot: Item Name  +Δ" line every section uses.
func (m Model) optItemHeader(it *db.Item, delta float64) string {
	return dimStyle.Render(it.Slot.String()+": ") +
		qualityStyles[it.Quality].Render(it.Name) + "  " +
		okStyle.Render(fmt.Sprintf("%+.0f", delta))
}

// optGemLines renders the plan's gem section (the per-item blocks of
// the former gem optimizer screen).
func (m Model) optGemLines(rows []gearRow, ws []score.Worn, cols cmpCols) []string {
	var lines []string
	for _, t := range m.optPlan.Gems.Tips {
		if t.ItemIdx >= len(rows) {
			continue
		}
		row := rows[t.ItemIdx]
		it := row.item
		lines = append(lines, m.optItemHeader(it, t.Delta))
		// one line per socket change, no per-gem stat rows
		for _, ch := range t.Changes {
			var sock string
			if ch.SocketIdx < len(it.GemSockets) {
				c := it.GemSockets[ch.SocketIdx]
				sock = gemColorStyle(c).Render("◆") + dimStyle.Render(" "+c.String()+": ")
			} else {
				// the enhancement socket (buckle / blacksmith)
				sock = gemColorStyle(db.GemPrismatic).Render("◆") + dimStyle.Render(" "+extraSocketLabel(it.Slot)+": ")
			}
			from := warnStyle.Render("empty")
			if ch.From != nil {
				from = gemText(ch.From)
			} else if ch.SocketIdx >= len(it.GemSockets) {
				from = warnStyle.Render("apply " + m.game.Rules.ExtraSocketName(it.Slot))
			}
			lines = append(lines, sock+from+dimStyle.Render(" → ")+gemText(ch.To))
		}
		// socket bonus: always stated when the item has one; its stats
		// fold into the combined table below
		if bonus := strings.Join(m.statLines(it.SocketBonus, nil), ", "); bonus != "" {
			before := m.scorer.BonusActive(row.worn())
			after := m.scorer.BonusActive(m.optPlan.Gems.Set[t.ItemIdx])
			var state string
			switch {
			case before && after:
				state = okStyle.Render("stays active")
			case !before && !after:
				state = warnStyle.Render("stays inactive") + dimStyle.Render(" (colors not matched)")
			case after:
				state = warnStyle.Render("inactive") + dimStyle.Render(" → ") + okStyle.Render("active")
			default:
				state = okStyle.Render("active") + dimStyle.Render(" → ") + warnStyle.Render("inactive")
			}
			lines = append(lines, dimStyle.Render("◇ socket bonus ("+bonus+"): ")+state)
		}
		// the item's combined gem contribution, all changes + the bonus
		// applied at once, priced against the rest of the set (cap-aware)
		have := m.gemContribution(row.worn())
		lines = append(lines, cols.table(m,
			have, m.gemContribution(m.optPlan.Gems.Set[t.ItemIdx]),
			m.statDelta(subStats(m.scorer.TotalsWorn(ws), have)))...)
		lines = append(lines, "")
	}
	return lines
}

// optEnchantLines renders the plan's enchant section.
func (m Model) optEnchantLines(rows []gearRow) []string {
	var lines []string
	for _, t := range m.optPlan.Enchants {
		if t.ItemIdx >= len(rows) {
			continue
		}
		it := rows[t.ItemIdx].item
		lines = append(lines, m.optItemHeader(it, t.Delta))
		from := warnStyle.Render("no enchant")
		if t.From != nil {
			from = enchantText(t.From.Name)
		}
		to := enchantText(t.To.Name)
		if st := strings.Join(m.statLines(t.To.Stats, nil), ", "); st != "" {
			to += dimStyle.Render(" (" + st + ")")
		}
		lines = append(lines, "  "+from+dimStyle.Render(" → ")+to, "")
	}
	return lines
}

// optReforgeLines renders the plan's reforge section.
func (m Model) optReforgeLines(rows []gearRow) []string {
	var lines []string
	for _, t := range m.optPlan.Reforges {
		if t.ItemIdx >= len(rows) {
			continue
		}
		it := rows[t.ItemIdx].item
		lines = append(lines, m.optItemHeader(it, t.Delta))
		to := okStyle.Render("clear the reforge")
		if t.To != nil {
			to = m.reforgeText(t.To)
		}
		// ⇒ between the sides: → is the reforge's own from→to arrow
		lines = append(lines, "  "+m.reforgeText(t.From)+dimStyle.Render("  ⇒  ")+to, "")
	}
	return lines
}

func (m Model) viewOptimize() string {
	rows := m.gearRows()
	header := m.viewHeader()
	scope := "gems · enchants"
	if m.scorer != nil && m.scorer.Reforging() {
		scope = "reforges · " + scope
	}
	context := titleStyle.Render("▸ Optimization advisor") +
		dimStyle.Render("  ("+scope+" solved as one plan: cap-aware, socket bonuses and meta activation included)")

	help := helpLine("c", "copy suggestions", "esc", "back")

	var body string
	switch {
	case m.scorer == nil || !anyDetail(rows):
		body = dimStyle.Render("the optimization advisor needs an enriched import (browser snippet) —\n" +
			"plain API exports carry no gem/enchant data")
	default:
		body = m.optBody(rows)
		vis := m.optVisible(lipgloss.Height(header))
		switch {
		case body == "":
			body = okStyle.Render("✦ "+scope+" are already optimal") +
				dimStyle.Render(" for the current weights and caps")
		case lipgloss.Height(body) > vis:
			// more plan than screen: scroll it, the bar marks the fold
			help = helpLine("↑/↓", "scroll") + "  " + help
			body = scrollRegion(body, m.optOff, vis, m.width)
		}
	}

	if m.optNote != "" {
		help += "  " + m.optNote
	}
	return m.pinFooter(lipgloss.JoinVertical(lipgloss.Left, header, "", context, "", body), help)
}

// optBody is the advisor's two-pane block — per-item changes left, the
// whole-set overview in its own pane on the right so a long change
// list keeps its vertical room — unclipped ("" when the plan changes
// nothing); the view scrolls it. Laid out one column short of the
// screen so the scrollbar fits.
func (m Model) optBody(rows []gearRow) string {
	cols := paneCols
	ws := wornOf(rows)

	section := func(icon, name string, body []string) []string {
		if len(body) == 0 {
			return nil
		}
		return append([]string{warnStyle.Render(icon+" ") + titleStyle.Render(name)}, body...)
	}
	// reforges first, matching the solver's round order: the cheap
	// edits lead, the mat-burning ones follow
	var lines []string
	lines = append(lines, section("⚒", "reforges", m.optReforgeLines(rows))...)
	lines = append(lines, section("◆", "gems", m.optGemLines(rows, ws, cols))...)
	lines = append(lines, section("✎", "enchants", m.optEnchantLines(rows))...)
	if len(lines) == 0 {
		return ""
	}

	summary := []string{cols.caption(dimStyle.Render("set totals if applied:"))}
	summary = append(summary, cols.table(m, m.scorer.TotalsWorn(ws), m.scorer.TotalsWorn(m.optPlan.Set), m.statDelta(db.Stats{}))...)
	if line := consumesLine(m.consFood, m.consFlask); line != "" {
		summary = append(summary, line)
	}
	if caps := m.viewCaps(m.optPlan.Set); caps != "" {
		summary = append(summary, "", caps)
	}
	summary = append(summary, m.optPlanMeta(ws)...)
	summary = append(summary, "",
		dimStyle.Render("total set-score gain: ")+
			okStyle.Render(fmt.Sprintf("%+.0f", m.optPlan.Delta)),
		dimStyle.Render("(per-item deltas are each change alone; the total applies all)"))
	// keep the overview pane from starving the change list: long
	// lines (the meta requirement) truncate at a sane pane width
	rightW := min(66, m.width/2)
	for i, l := range summary {
		summary[i] = truncate(l, rightW)
	}
	right := detailStyle.Render(strings.Join(summary, "\n"))

	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return m.twoPane(lines, right)
}

// optVisible is the row window the screen gives the advisor body.
func (m Model) optVisible(headerH int) int {
	return max(m.height-headerH-6, 5)
}

// optPlanText renders the whole plan as plain text for the clipboard:
// per-domain sections, then the combined total. Empty when the plan
// changes nothing.
func (m Model) optPlanText() string {
	rows := m.gearRows()
	plan := m.optPlan
	if len(plan.Gems.Tips) == 0 && len(plan.Enchants) == 0 && len(plan.Reforges) == 0 {
		return ""
	}
	var b strings.Builder
	if len(plan.Reforges) > 0 {
		b.WriteString("reforges:\n")
	}
	for _, t := range plan.Reforges {
		if t.ItemIdx >= len(rows) {
			continue
		}
		it := rows[t.ItemIdx].item
		text := func(r *db.Reforge) string {
			if r == nil {
				return "not reforged"
			}
			return fmt.Sprintf("%.0f %s -> %s", r.Amount, m.statLabel(r.From), m.statLabel(r.To))
		}
		to := "clear the reforge"
		if t.To != nil {
			to = text(t.To)
		}
		fmt.Fprintf(&b, "%s: %s  %+.0f\n  %s => %s\n", it.Slot, it.Name, t.Delta, text(t.From), to)
	}
	if len(plan.Gems.Tips) > 0 {
		b.WriteString("gems:\n")
	}
	for _, t := range plan.Gems.Tips {
		if t.ItemIdx >= len(rows) {
			continue
		}
		row := rows[t.ItemIdx]
		it := row.item
		fmt.Fprintf(&b, "%s: %s  %+.0f\n", it.Slot, it.Name, t.Delta)
		for _, ch := range t.Changes {
			sock := extraSocketLabel(it.Slot)
			if ch.SocketIdx < len(it.GemSockets) {
				sock = it.GemSockets[ch.SocketIdx].String()
			}
			from := "empty"
			if ch.From != nil {
				from = "[ " + ch.From.Name + " ]"
			} else if ch.SocketIdx >= len(it.GemSockets) {
				from = "apply " + m.game.Rules.ExtraSocketName(it.Slot)
			}
			fmt.Fprintf(&b, "  %s: %s -> [ %s ]\n", sock, from, ch.To.Name)
		}
		if bonus := strings.Join(m.statLines(it.SocketBonus, nil), ", "); bonus != "" {
			before := m.scorer.BonusActive(row.worn())
			after := m.scorer.BonusActive(plan.Gems.Set[t.ItemIdx])
			var state string
			switch {
			case before && after:
				state = "stays active"
			case !before && !after:
				state = "stays inactive (colors not matched)"
			case after:
				state = "inactive -> active"
			default:
				state = "active -> inactive"
			}
			fmt.Fprintf(&b, "  socket bonus (%s): %s\n", bonus, state)
		}
	}
	if len(plan.Enchants) > 0 {
		b.WriteString("enchants:\n")
	}
	for _, t := range plan.Enchants {
		if t.ItemIdx >= len(rows) {
			continue
		}
		it := rows[t.ItemIdx].item
		from := "no enchant"
		if t.From != nil {
			from = "[ " + t.From.Name + " ]"
		}
		to := "[ " + t.To.Name + " ]"
		if st := strings.Join(m.statLines(t.To.Stats, nil), ", "); st != "" {
			to += " (" + st + ")"
		}
		fmt.Fprintf(&b, "%s: %s  %+.0f\n  %s -> %s\n", it.Slot, it.Name, t.Delta, from, to)
	}
	ws := wornOf(rows)
	gBefore, _ := m.scorer.InactiveMeta(ws)
	gAfter, condAfter := m.scorer.InactiveMeta(plan.Set)
	switch {
	case gBefore != nil && gAfter == nil:
		fmt.Fprintf(&b, "[ %s ] becomes active\n", gBefore.Name)
	case gAfter != nil && gBefore == nil:
		fmt.Fprintf(&b, "warning: [ %s ] would deactivate — needs %s\n", gAfter.Name, condAfter)
	case gAfter != nil:
		fmt.Fprintf(&b, "warning: [ %s ] stays inactive — needs %s\n", gAfter.Name, condAfter)
	}
	fmt.Fprintf(&b, "total set-score gain: %+.0f\n", plan.Delta)
	return b.String()
}

// gemContribution is what an equipped instance's gem package provides:
// every socketed gem plus the socket bonus when active.
func (m Model) gemContribution(w score.Worn) db.Stats {
	var st db.Stats
	for _, id := range w.Gems {
		if g := m.game.DB.ResolveGem(id); g != nil {
			st = addStats(st, g.Stats)
		}
	}
	if m.scorer.BonusActive(w) {
		st = addStats(st, w.Item.SocketBonus)
	}
	return st
}

// optPlanMeta reports the meta gem's activation before vs after the plan.
func (m Model) optPlanMeta(ws []score.Worn) []string {
	if g := m.optPlan.Gems.UnknownMeta; g != nil {
		return []string{warnStyle.Render("⚠ ") + gemText(g) +
			warnStyle.Render(" activation rule unknown") +
			dimStyle.Render(fmt.Sprintf(" — optimized without it; curate gem %d in db/metagems.go", g.ID))}
	}
	gBefore, _ := m.scorer.InactiveMeta(ws)
	gAfter, condAfter := m.scorer.InactiveMeta(m.optPlan.Set)
	switch {
	case gBefore != nil && gAfter == nil:
		return []string{okStyle.Render("✓ ") + gemText(gBefore) + okStyle.Render(" becomes active")}
	case gAfter != nil && gBefore == nil:
		return []string{errStyle.Render("⚠ ") + gemText(gAfter) + errStyle.Render(" would DEACTIVATE") +
			dimStyle.Render(" — needs "+condAfter.String())}
	case gAfter != nil:
		return []string{errStyle.Render("⚠ ") + gemText(gAfter) + errStyle.Render(" stays inactive") +
			dimStyle.Render(" — needs "+condAfter.String())}
	}
	return nil
}

// anyDetail reports whether any gear row carries actual gem data.
func anyDetail(rows []gearRow) bool {
	for _, r := range rows {
		if r.equipped.HasDetail {
			return true
		}
	}
	return false
}
