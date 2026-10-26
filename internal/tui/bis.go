package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/vaclav-dvorak/twolk/internal/db"
	"github.com/vaclav-dvorak/twolk/internal/score"
)

// The best-in-slot screen (b): score.BestSet solves the whole set at
// once under the active filter — set bonuses, caps, unique-equipped
// rules and every weapon layout the spec can play (2H vs dual-wield vs
// shield/held off-hand) — and the view doubles as a shopping list:
// ✓ pieces already worn, unmarked pieces to acquire with their source and the
// set-score gain of grabbing each one next, plus the total emblem
// budget. Every number here sits on the fresh-drop basis (both sides of
// every delta): mixing it with the worn set's tuned actuals priced a
// strictly better set as a downgrade on every row (session 30). The rows follow the SOLVED layout, so a hunter wearing a
// two-hander sees two one-hand rows when the pair wins.

func (m *Model) openBis() {
	// The BiS screen gets its own scorer, gem-calibrated against the
	// SOLVED set rather than the worn one: the main scorer may still
	// assume hit gems (worn gear under cap) while the BiS gear alone
	// saturates the cap — hit gems everywhere, all capped worthless.
	// Solve → recalibrate → re-solve until the set stops changing.
	rows := m.gearRows()
	ws := wornOf(rows)
	sc := m.newScorer()
	base, set, total := sc.BestSet(ws, m.allowItem)
	for range 3 {
		sc.Calibrate(set)
		// the assumed food follows the SOLVED set, not the worn one: a
		// set that caps expertise on its own wants a different meal
		m.bisFood, m.bisFlask = m.applyConsumes(sc, set)
		nextBase, next, nextTotal := sc.BestSet(ws, m.allowItem)
		same := len(next) == len(set)
		for i := range next {
			same = same && next[i].ID == set[i].ID
		}
		base, set, total = nextBase, next, nextTotal
		if same {
			break
		}
	}
	m.bisWorn, m.bisSet, m.bisScore, m.bisScorer = base, set, total, sc
	_, m.bisRefs = sc.AssumedPlan(set)
	m.solveBisGems()
	m.bisFrom = make([]int, len(rows))
	for i, r := range rows {
		m.bisFrom[i] = r.item.ID
	}
	m.bisCursor = 0
	m.bisPaneOff = 0
	m.mode = modeBiS
}

// solveBisGems runs the SET-level gem optimizer on the solved set — the
// same DP the advisor runs on worn gear, which the per-socket assumption
// the solver prices with cannot match: it lands hit and expertise on
// their caps with one exact gem instead of overshooting, and it spends
// unique gems and the jeweler quota once across the whole set. The
// shopping list shows ITS gems; bisGemGain is what it beats the
// assumption by, so a gap the assumption cannot close stays visible
// instead of being quietly promised.
func (m *Model) solveBisGems() {
	m.bisGems, m.bisGemGain = nil, 0
	ws := make([]score.Worn, len(m.bisSet))
	for i, it := range m.bisSet {
		gems, _ := m.bisScorer.AssumedGems(it)
		ids := make([]int, len(gems))
		for j, g := range gems {
			if g != nil {
				ids[j] = g.ID
			}
		}
		w := score.Worn{Item: it, Gems: ids, HasDetail: true}
		if i < len(m.bisRefs) {
			w.Reforge = m.bisRefs[i]
		}
		if e := m.bisScorer.BestEnchant(it); e != nil {
			w.EnchantID = e.EffectID
		}
		ws[i] = w
	}
	plan := m.bisScorer.OptimizeGems(ws)
	if len(plan.Set) != len(ws) {
		return // nothing socketable in the set
	}
	m.bisGemGain = plan.Delta
	m.bisGems = make([][]*db.Gem, len(plan.Set))
	for i, w := range plan.Set {
		for _, id := range w.Gems {
			m.bisGems[i] = append(m.bisGems[i], m.game.DB.ResolveGem(id))
		}
	}
}

// bisStale reports whether the equipped set changed under the solve
// (re-import while the screen is open).
func (m Model) bisStale(rows []gearRow) bool {
	if len(rows) != len(m.bisFrom) {
		return true
	}
	for i, r := range rows {
		if r.item.ID != m.bisFrom[i] {
			return true
		}
	}
	return false
}

func (m Model) updateBis(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "b":
		m.mode = modeNormal
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	case "up":
		if m.bisCursor > 0 {
			m.bisCursor--
			m.bisPaneOff = 0 // the acquire card follows the cursor
		}
		return m, nil
	case "down":
		if m.bisCursor < len(m.bisSet)-1 {
			m.bisCursor++
			m.bisPaneOff = 0
		}
		return m, nil
	case "j", "k", "pgdown", "pgup":
		// scroll the info panes; the item list never gives up its rows
		rows := m.gearRows()
		if m.bisStale(rows) {
			return m, nil
		}
		panes, _, _ := m.bisPanes(rows)
		m.bisPaneOff = m.scrollInfoPane(msg.String(), m.bisPaneOff, len(m.bisSet), panes)
		return m, nil
	case "f":
		m.mode = modeFilter
		m.fromUps, m.fromBis = false, true
		return m, nil
	}
	return m, nil
}

// bisBaseline is the solver baseline as plain items — the like-for-like
// comparison side: the gear you own, priced under the same fresh-drop
// assumptions the picks get.
func (m Model) bisBaseline() []*db.Item {
	out := make([]*db.Item, len(m.bisWorn))
	for i, w := range m.bisWorn {
		out[i] = w.Item
	}
	return out
}

// bisNeeded reports whether row i's BiS pick differs from the baseline
// (the worn item, or an empty hand the winning layout added).
func (m Model) bisNeeded(i int) bool {
	return i < len(m.bisSet) && i < len(m.bisWorn) &&
		m.bisSet[i].ID != m.bisWorn[i].Item.ID
}

// bisPairDelta prices the weapon rows as one acquisition unit when the
// winning layout displaced a worn weapon (a two-hander fits no
// dual-wield role, a lone pair fits no two-hand role): the displaced
// weapon is in no row's baseline, so per-row SwapDelta would price each
// pick against an empty hand and charge the loss to nobody. The group
// delta — solved weapon rows in, worn weapons out, rest of the gear
// as-is — goes on the first needed weapon row; the others render "pair"
// (the picks only pay together). Not displaced (worn weapons kept their
// rows): per-row deltas are honest, reported false.
func (m Model) bisPairDelta(ws []score.Worn) (first int, delta float64, rows map[int]bool, ok bool) {
	held := map[int]int{} // weapon-row baseline IDs, with multiplicity
	var weap []int
	for i := range m.bisSet {
		if i < len(m.bisWorn) && m.bisWorn[i].Item.Slot == db.SlotWeapon {
			weap = append(weap, i)
			held[m.bisWorn[i].Item.ID]++
		}
	}
	displaced := false
	for _, w := range ws {
		if w.Item.Slot != db.SlotWeapon {
			continue
		}
		if held[w.Item.ID] > 0 {
			held[w.Item.ID]--
		} else {
			displaced = true
		}
	}
	if !displaced {
		return 0, 0, nil, false
	}
	baseline := m.bisBaseline()
	swapped := append([]*db.Item(nil), baseline...)
	rows = map[int]bool{}
	first = -1
	for _, i := range weap {
		if !m.bisNeeded(i) {
			continue
		}
		swapped[i] = m.bisSet[i]
		rows[i] = true
		if first < 0 {
			first = i
		}
	}
	if first < 0 {
		return 0, 0, nil, false
	}
	// the second term is the WORN set (both displaced weapons included),
	// not the baseline rows — that is what the pick has to pay for; both
	// sides on the fresh-drop basis the row deltas use
	wornItems := make([]*db.Item, len(ws))
	for i, w := range ws {
		wornItems[i] = w.Item
	}
	return first, m.bisScorer.AssumedSetScore(swapped) -
		m.bisScorer.AssumedSetScore(wornItems), rows, true
}

// bisSlotLabel names row i's slot; when the solved layout carries two
// weapon rows they read "Main hand"/"Off hand" instead of "Weapon".
func (m Model) bisSlotLabel(i int) string {
	it := m.bisSet[i]
	if it.Slot != db.SlotWeapon {
		return it.Slot.String()
	}
	var weap []int
	for j, pick := range m.bisSet {
		if pick.Slot == db.SlotWeapon {
			weap = append(weap, j)
		}
	}
	if len(weap) == 2 {
		if i == weap[0] {
			return "Main hand"
		}
		return "Off hand"
	}
	return it.Slot.String()
}

// emblemBudget lists the badge totals of the missing pieces, one line
// per emblem, highest tier first (the order they gate on a fresh
// server). Vendor gear is priced directly; crafted gear contributes
// via its emblem-buyable reagents, noted on the line ("8× Crusader
// Orb") so the number is traceable.
func (m Model) emblemBudget() []string {
	counts := map[db.Emblem]int{}
	viaCount := map[db.Emblem]int{}           // emblems spent on reagents
	viaMats := map[db.Emblem]map[string]int{} // reagent name → quantity
	for i := range m.bisSet {
		if !m.bisNeeded(i) {
			continue
		}
		// a currency the filter phase has not shipped yet buys nothing —
		// don't budget tokens that cannot be earned (see CurrencyPhase)
		if p, ok := m.game.DB.PriceFor(m.bisSet[i].ID, m.filter.ServerPhase); ok {
			if m.currencyOpen(p.Emblem) {
				counts[p.Emblem] += p.Count
			}
			continue // priced as gear: mats are the crafter's, not ours
		}
		if pp, ok := m.protocolFor(m.bisSet[i].ID); ok && pp.Count > 0 {
			counts[pp.Emblem] += pp.Count
			continue // fragment-buyable in heroic-plus protocol runs
		}
		for _, r := range db.CraftCost(m.bisSet[i].ID) {
			p, ok := m.game.DB.PriceFor(r.ID, m.filter.ServerPhase)
			if !ok || !m.currencyOpen(p.Emblem) {
				continue
			}
			counts[p.Emblem] += p.Count * r.Count
			viaCount[p.Emblem] += p.Count * r.Count
			if viaMats[p.Emblem] == nil {
				viaMats[p.Emblem] = map[string]int{}
			}
			viaMats[p.Emblem][r.Name] += r.Count
		}
	}
	var lines []string
	for _, e := range m.game.Currencies {
		n := counts[e]
		if n == 0 {
			continue
		}
		line := fmt.Sprintf("%d× %s", n, e)
		if v := viaCount[e]; v > 0 {
			var mats []string
			for name, q := range viaMats[e] {
				mats = append(mats, fmt.Sprintf("%d× %s", q, name))
			}
			sort.Strings(mats)
			if v < n {
				line += fmt.Sprintf("  (%d via %s)", v, strings.Join(mats, ", "))
			} else {
				line += fmt.Sprintf("  (%s)", strings.Join(mats, ", "))
			}
		}
		lines = append(lines, line)
	}
	return lines
}

func (m Model) viewBis() string {
	rows := m.gearRows()
	header := m.viewHeader()
	context := titleStyle.Render("▸ Best-in-slot set") +
		"   " + dimStyle.Render(m.filterSummary())

	help := helpLine("↑/↓", "select", "f", "filter", "esc", "back")

	if m.bisStale(rows) {
		// gear changed under the solve (re-import): recompute on reopen
		return m.pinFooter(lipgloss.JoinVertical(lipgloss.Left, header, "", context, "",
			dimStyle.Render("press b to re-solve — the equipped set changed")), help)
	}

	ws := wornOf(rows)
	missing := 0
	for i := range m.bisSet {
		if m.bisNeeded(i) {
			missing++
		}
	}

	cursor := min(m.bisCursor, len(m.bisSet)-1)
	pairFirst, pairDelta, pairRows, paired := m.bisPairDelta(ws)
	panes, cur, itemGain := m.bisPanes(rows)

	// shopping summary: progress and ONE honest score pair. The gain
	// shown is the like-for-like ITEM gain (worn items and BiS picks
	// priced under the same fresh-drop assumptions, reforge plan
	// included) — pure gear quality, no tuning noise. The projected score is the current ACTUAL score
	// plus that gain: "what you land on if you farm the list and tune
	// the new pieces the way your gear is tuned now" (≈: the tuning
	// premium is assumed to carry over). The raw assumed-target score
	// is NOT shown against the actual score — the two accounting bases
	// differ (real reforges/gems vs fresh-drop assumptions) and their
	// difference reads as nonsense (a perfectly tuned character showed
	// a NEGATIVE total for a strictly better item set).
	summary := okStyle.Render(fmt.Sprintf("%d/%d pieces owned", len(m.bisSet)-missing, len(m.bisSet))) +
		dimStyle.Render(fmt.Sprintf("  ·  set score %.0f → ~%.0f ", cur, cur+itemGain)) +
		okStyle.Render(fmt.Sprintf("(%+.0f from gear)", itemGain))
	if missing == 0 {
		summary = okStyle.Render("✦ you are wearing the best-in-slot set") +
			dimStyle.Render(" within the current filter")
	}

	// the item list keeps every row; the info panes clip to what is
	// left and scroll (j/k), the bar on the right edge marking the fold
	itemVis, paneVis := m.listPaneSplit(lipgloss.Height(header), len(m.bisSet), lipgloss.Height(panes))
	if lipgloss.Height(panes) > paneVis {
		help = helpLine("j/k", "scroll info") + "  " + help
		panes = scrollRegion(panes, m.bisPaneOff, paneVis, m.width)
	}
	start := max(0, min(cursor-itemVis/2, len(m.bisSet)-itemVis))
	rowW := m.width
	if len(m.bisSet) > itemVis {
		rowW = m.width - 1 // the list scrolls too: its own bar column
	}

	// columns: mark 2, slot 10, delta 8, tier 7, ilvl 4, gaps — leave
	// the rest to name + source, like the upgrade list
	nameW := min(max(m.width-86, 38), 62)
	var lines []string
	for i := start; i < min(start+itemVis, len(m.bisSet)); i++ {
		it := m.bisSet[i]
		sel := rowStyler(i == cursor)
		name := displayName(it)
		mark := sel(okStyle).Render(" ✓ ")
		deltaCell := sel(dimStyle).Render(strings.Repeat(" ", 7))
		tail := sel(dimStyle).Render("equipped")
		if m.bisNeeded(i) {
			mark = sel(lipgloss.NewStyle()).Render("   ")
			switch {
			case paired && pairRows[i] && i != pairFirst:
				deltaCell = sel(dimStyle).Render(fmt.Sprintf("%6s ", "pair"))
			case paired && pairRows[i]:
				deltaCell = sel(okStyle).Render(fmt.Sprintf("%+6.0f ", pairDelta))
			default:
				// a piece that only pays inside the full set (tier
				// threshold, cap interaction) prices negative on its own:
				// dim, not green
				d := m.bisScorer.ItemSwapDelta(m.bisBaseline(), i, it)
				st := okStyle
				if d <= 0 {
					st = dimStyle
				}
				deltaCell = sel(st).Render(fmt.Sprintf("%+6.0f ", d))
			}
			tail = m.sourceText(it)
			if i == cursor {
				tail = sel(dimStyle).Render(ansi.Strip(tail))
			}
		}
		row := mark +
			sel(dimStyle).Render(fmt.Sprintf("%-10s", m.bisSlotLabel(i))) +
			deltaCell +
			heroicCell(sel, qualityStyles[it.Quality], name, nameW) +
			sel(tierStyle).Render(fmt.Sprintf(" %-7s", m.tierLabel(it))) +
			sel(dimStyle).Render(fmt.Sprintf("%3d", it.ILvl)) +
			sel(lipgloss.NewStyle()).Render("  ") +
			tail
		if pad := rowW - lipgloss.Width(row); pad > 0 && i == cursor {
			row += sel(lipgloss.NewStyle()).Render(strings.Repeat(" ", pad))
		}
		lines = append(lines, truncate(row, rowW))
	}
	list := strings.Join(lines, "\n")
	if len(m.bisSet) > itemVis {
		list = m.listWithBar(list, len(m.bisSet), len(lines), start)
	}

	return m.pinFooter(lipgloss.JoinVertical(lipgloss.Left,
		header, "", context, "", list, summary, panes), help)
}

// bisPanes builds the bottom info block — acquisition card for the
// selected piece left, target-set overview right (comparing against
// current gear lives in the upgrade browser; here the questions are
// "what do I farm" and "where does the set land") — plus the honest
// score pair the summary line reuses. Deltas, panes and scores all use
// the BiS-calibrated scorer (assumed gems match the solved set's cap
// situation, not the worn set's). The block is laid out one column
// short of the screen so the scrollbar fits when it outgrows its
// window.
func (m Model) bisPanes(rows []gearRow) (panes string, cur, itemGain float64) {
	ws := wornOf(rows)
	mc := m
	mc.scorer = m.bisScorer
	cur = m.bisScorer.SetScoreWorn(ws)
	itemGain = m.bisScore - m.bisScorer.AssumedSetScore(m.bisBaseline())
	right := mc.bisTargetPane(ws, cur, itemGain)
	left := mc.bisAcquireCard(min(m.bisCursor, len(m.bisSet)-1))
	return m.twoPane(left, right), cur, itemGain
}

// bisAcquireCard describes how to get the selected BiS piece: source,
// exact vendor/emblem price (in the source text) and, for crafted
// gear, the full reagent bill.
func (m Model) bisAcquireCard(cursor int) []string {
	it := m.bisSet[cursor]
	if !m.bisNeeded(cursor) {
		return []string{"", okStyle.Render(" ✓ "+it.Name) +
			dimStyle.Render(" — equipped, nothing to farm")}
	}
	lines := []string{"",
		dimStyle.Render("to acquire: ") + m.itemLabel(it, rowStyler(false)) +
			dimStyle.Render(fmt.Sprintf("  ilvl %d", it.ILvl)),
		"  " + m.sourceText(it),
	}
	// the pick was priced with these gems/enchant/reforge assumed — say
	// so, or the score promises stats the bare drop doesn't have. The
	// gems are the set-level optimizer's (solveBisGems), which the
	// per-socket assumption only approximates.
	gems, _ := m.bisScorer.AssumedGems(it)
	if cursor < len(m.bisGems) && m.bisGems[cursor] != nil {
		gems = m.bisGems[cursor]
	}
	for i, g := range gems {
		if g == nil {
			continue
		}
		glyph := gemColorStyle(db.GemPrismatic).Render("◆ " + extraSocketLabel(it.Slot))
		if i < len(it.GemSockets) {
			glyph = gemColorStyle(it.GemSockets[i]).Render("◆ " + it.GemSockets[i].String())
		}
		lines = append(lines, "  "+glyph+" "+gemText(g))
	}
	if e := m.bisScorer.BestEnchant(it); e != nil {
		lines = append(lines, "  "+warnStyle.Render("✎ ")+enchantText(e.Name))
	}
	if r := m.bisReforge(cursor); r != nil {
		lines = append(lines, "  "+warnStyle.Render("⚒ ")+m.reforgeText(r))
	}
	for i, r := range db.CraftCost(it.ID) {
		prefix := dimStyle.Render("        ")
		if i == 0 {
			prefix = dimStyle.Render("  mats: ")
		}
		line := prefix + dimStyle.Render(fmt.Sprintf("%3d× ", r.Count)) + r.Name
		if p, ok := m.game.DB.PriceFor(r.ID, m.filter.ServerPhase); ok {
			line += dimStyle.Render(fmt.Sprintf("  = %d× %s", p.Count*r.Count, p.Emblem))
		}
		lines = append(lines, line)
	}
	return lines
}

// bisReforge is the reforge the solved set plans for row i — the
// cap-aware set-level pick the score was built on, not the per-item
// assumption (which cannot dump surplus hit or land the cap).
func (m Model) bisReforge(i int) *db.Reforge {
	if i < 0 || i >= len(m.bisRefs) {
		return nil
	}
	return m.bisRefs[i]
}

// bisTargetPane is the bordered overview of the solved set: stat
// totals vs the current gear, the caps landing spot, the score
// derivation (how the summary's projected number is built), active
// set bonuses and the emblem budget.
func (m Model) bisTargetPane(ws []score.Worn, cur, itemGain float64) string {
	cols := paneCols
	bisTot := m.bisScorer.AssumedTotals(m.bisSet)
	lines := []string{cols.caption(dimStyle.Render("target set totals:"))}
	lines = append(lines, cols.table(m, m.scorer.TotalsWorn(ws), bisTot,
		m.statDelta(db.Stats{}))...)
	if caps := m.viewCapsTotals(bisTot); caps != "" {
		lines = append(lines, "", caps)
	}
	// the target's caps are read with the meal the SOLVED set wants,
	// which need not be the one the worn set wants
	if line := consumesLine(m.bisFood, m.bisFlask); line != "" {
		lines = append(lines, line)
	}
	// score derivation: the summary's numbers, spelled out. The item
	// gain compares both sides under the SAME fresh-drop assumptions;
	// the projection adds it to the actual tuned score — the target's
	// raw assumed score is a different accounting basis and never
	// meets the actual one in the UI.
	lines = append(lines, "",
		dimStyle.Render(fmt.Sprintf("%-22s %8.0f", "score now (tuned)", cur)),
		dimStyle.Render(fmt.Sprintf("%-22s ", "better items alone"))+
			okStyle.Render(fmt.Sprintf("%+8.0f", itemGain)),
		dimStyle.Render(fmt.Sprintf("%-22s ", "projected, re-tuned"))+
			okStyle.Render(fmt.Sprintf("%8.0f", cur+itemGain)))
	// the solve prices sockets one at a time; the set-level gem plan
	// the shopping list actually hands out can do better (one gem lands
	// a cap exactly). Say by how much rather than hide the gap.
	if m.bisGemGain > 1 {
		lines = append(lines, dimStyle.Render(fmt.Sprintf("%-22s ", "gem plan adds"))+
			okStyle.Render(fmt.Sprintf("%+8.0f", m.bisGemGain)))
	}
	var sets []string
	counts := map[string]int{}
	for _, it := range m.bisSet {
		if it.SetName != "" {
			counts[it.SetName]++
		}
	}
	for name, n := range counts {
		if n >= 2 && m.game.Rules.SetBonusesFor(name) != nil {
			sets = append(sets, fmt.Sprintf("◈ %s (%d pc)", name, n))
		}
	}
	sort.Strings(sets)
	for _, s := range sets {
		lines = append(lines, titleStyle.Render(s))
	}
	if budget := m.emblemBudget(); len(budget) > 0 {
		lines = append(lines, dimStyle.Render("emblem budget:"))
		for _, b := range budget {
			lines = append(lines, dimStyle.Render("  "+b))
		}
	}
	w := min(66, m.width/2)
	for i, l := range lines {
		lines[i] = truncate(l, w)
	}
	return detailStyle.Render(strings.Join(lines, "\n"))
}
