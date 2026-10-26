package tui

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/vaclav-dvorak/twolk/internal/db"
	"github.com/vaclav-dvorak/twolk/internal/score"
)

// The upgrade screen's compare pane: equipped item on the left, the
// selected candidate on the right, per-stat differences and their score
// impact rendered between them. Every contribution renders exactly once,
// in its own section: the stats table carries the items' OWN stats only,
// while racials, enchants, reforges and gems each get a subsection with
// their full stat effect (real gems/enchant for the enriched equipped
// side, assumed best for the candidate). Procs and weapon DPS get their
// own rows.

// cmpRow is one stat line of the compare table. sts carries the stat
// (or the merged unified-rating pair) so the score impact of the change
// can be weighted — a duplicated rating scores on every side it is
// duplicated into.
type cmpRow struct {
	label string
	sts   []db.Stat
	l, r  float64
}

// compareRows unions the non-zero stats of both sides in enum order,
// collapsing duplicated unified-rating pairs into the single line the
// game shows (like statLines, but across two stat vectors).
func (m Model) compareRows(l, r db.Stats) []cmpRow {
	merged := map[db.Stat]string{} // pair head -> label; "" marks the consumed twin
	pairOf := map[db.Stat]db.Stat{}
	for _, p := range ratingPairs {
		lok := l[p.a] == l[p.b]
		rok := r[p.a] == r[p.b]
		switch {
		case lok && rok && (l[p.a] != 0 || r[p.a] != 0):
			merged[p.a] = p.name
			merged[p.b] = ""
			pairOf[p.a] = p.b
		// unified-rating versions (cata) carry only one side: label it
		// as the unified rating, no twin to fold in
		case !m.game.Rules.HasStat(p.a):
			merged[p.b] = p.name
			pairOf[p.b] = p.b
		case !m.game.Rules.HasStat(p.b):
			merged[p.a] = p.name
			pairOf[p.a] = p.a
		}
	}
	var rows []cmpRow
	for st := db.Stat(0); st < db.NumStats; st++ {
		lv, rv := l[st], r[st]
		if lv == 0 && rv == 0 {
			continue
		}
		label, ok := merged[st]
		if ok && label == "" {
			continue
		}
		sts := []db.Stat{st}
		if !ok {
			label = st.String()
		} else if twin := pairOf[st]; twin != st {
			// only a genuinely folded twin joins the Δscore stat set —
			// a one-sided unified rating (cata) is priced once
			sts = append(sts, twin)
		}
		rows = append(rows, cmpRow{label: label, sts: sts, l: lv, r: rv})
	}
	return rows
}

// cmpCols renders the shared columnar stat tables used by the upgrade
// compare pane and the gem advisor: label | left | Δ | Δscore | right.
// hideSame drops zero-delta rows — useful when both sides are the same
// item and only the changes carry information.
type cmpCols struct {
	labelW, lw, gw, epW, rw int
	hideSame                bool
}

func (c cmpCols) cell(v float64, w int) string {
	if v == 0 {
		return dimStyle.Render(fmt.Sprintf("%*s", w, "—"))
	}
	return fmt.Sprintf("%*.0f", w, v)
}

func (c cmpCols) signed(d float64, w int, format string) string {
	s := okStyle
	if d < 0 {
		s = errStyle
	}
	if d == 0 {
		return strings.Repeat(" ", w)
	}
	return s.Render(fmt.Sprintf("%*s", w, fmt.Sprintf(format, d)))
}

// signedEP renders one signed score figure, green up / red down.
func signedEP(v float64) string {
	st := okStyle
	if v < 0 {
		st = errStyle
	}
	return st.Render(fmt.Sprintf("%+.0f", v))
}

// upgradeFor finds the ranked entry for a candidate, so the compare pane
// can speak about the very number the list showed.
func (m Model) upgradeFor(cand *db.Item) (score.Upgrade, bool) {
	for _, u := range m.upList {
		if u.Item.ID == cand.ID {
			return u, true
		}
	}
	return score.Upgrade{}, false
}

// reforgeSwing explains, in one line, WHY re-planning moves a candidate:
// almost always the hit cap — the equipped piece is holding hit the set
// would fall short without, and the reforge pass sources it elsewhere.
// Empty when the hit line is not what moved.
func (m Model) reforgeSwing(cur score.Worn, cand *db.Item) string {
	prof := m.scorer.Profile()
	if prof.HitCap == nil {
		return ""
	}
	st := prof.HitCap.Stat
	var cap float64
	for _, c := range prof.Caps {
		if c.Stat == st {
			cap = c.Cap
		}
	}
	ws := wornOf(m.gearRows())
	have := m.scorer.Derived(m.scorer.TotalsWorn(ws))[st]
	shift := m.scorer.EffectiveStats(cand)[st] - m.scorer.WornStats(cur)[st]
	if math.Abs(shift) < 1 {
		return ""
	}
	verb := "drops"
	if shift > 0 {
		verb = "adds"
	}
	return fmt.Sprintf("   %s %.0f hit (%.0f → %.0f, cap %.0f); the reforge pass rebalances the set around it",
		verb, math.Abs(shift), have, have+shift, cap)
}

// caption positions the Δscore column header after an arbitrary prefix.
func (c cmpCols) caption(prefix string) string {
	pad := c.labelW + c.lw + c.gw + c.epW - lipgloss.Width(prefix)
	return prefix + dimStyle.Render(fmt.Sprintf("%*s", max(pad, 1), "Δscore"))
}

// table renders the per-stat comparison rows of two stat vectors. val
// prices one row's change for the Δscore column (see Model.statDelta —
// cap-aware, so hit past the cap shows a blank score, not full value).
func (c cmpCols) table(m Model, l, r db.Stats, val func(sts []db.Stat, from, to float64) float64) []string {
	var out []string
	for _, row := range m.compareRows(l, r) {
		d := row.r - row.l
		if c.hideSame && d == 0 {
			continue
		}
		out = append(out, fmt.Sprintf("%-*s", c.labelW, row.label)+
			c.cell(row.l, c.lw)+c.signed(d, c.gw, "%+.0f")+
			c.signed(val(row.sts, row.l, row.r), c.epW, "%+.0f")+c.cell(row.r, c.rw))
	}
	return out
}

// statDelta builds a table's Δscore valuation: the cap-aware marginal
// value of the row's change on top of base — the set totals WITHOUT the
// compared contribution, so points past a profile cap are worth zero.
// Pass zero base when the compared vectors are whole-set totals.
func (m Model) statDelta(base db.Stats) func(sts []db.Stat, from, to float64) float64 {
	return func(sts []db.Stat, from, to float64) float64 {
		if m.scorer == nil {
			return 0
		}
		return m.scorer.MarginalValue(base, sts, from, to)
	}
}

// viewCompare renders the equipped-vs-candidate pane for the upgrade
// screen, full terminal width.
func (m Model) viewCompare(cur score.Worn, cand *db.Item) string {
	ws := wornOf(m.gearRows())
	// swap one item, keep the rest: uniques on the kept items are off
	// the table for the candidate, the replaced item's prismatic
	// meta-keepers (Nightmare Tear) carry over onto it
	ctx := m.scorer.SwapContextFor(ws, m.upSlot)
	candEnch := m.scorer.BestEnchant(cand)
	// Δscore base: the rest of the worn set — both sides' stats stack on
	// it, so a capped stat's points only score up to the remaining gap
	base := subStats(m.scorer.TotalsWorn(ws), m.scorer.WornGearStats(cur))

	// header segments anchor the value columns: the equipped value sits
	// right-aligned under the equipped name segment, the delta and its
	// score impact under the arrow gutter, the candidate value under the
	// candidate name
	name := func(it *db.Item, worn bool) string {
		s := m.itemLabel(it, rowStyler(false)) + dimStyle.Render(fmt.Sprintf("  ilvl %d", it.ILvl))
		if worn {
			return dimStyle.Render("equipped: ") + s
		}
		return titleStyle.Render("candidate: ") + s
	}
	leftSeg := name(cur.Item, true)
	gutter := "   " + dimStyle.Render("→") + "   "
	rightSeg := name(cand, false)

	cols := cmpCols{
		labelW:   18,
		lw:       max(lipgloss.Width(leftSeg)-18, 8), // equipped value field
		gw:       lipgloss.Width(gutter),             // stat delta field
		epW:      9,                                  // score impact field
		rw:       12,                                 // candidate value field
		hideSame: true,                               // unchanged rows say nothing
	}
	statTable := func(l, r db.Stats) []string { return cols.table(m, l, r, m.statDelta(base)) }
	// subsections keep zero-delta rows: a contribution shared by both
	// sides (same enchant, same racial) is still the explanation of
	// where those points come from
	fullCols := cols
	fullCols.hideSame = false
	fullTable := func(l, r db.Stats) []string { return fullCols.table(m, l, r, m.statDelta(base)) }

	var lines []string
	lines = append(lines, leftSeg+gutter+rightSeg)
	lines = append(lines, "")

	// --- stats: the items' own stats, nothing else — racials, enchants,
	// reforges and gems each explain themselves in their own section ---
	lines = append(lines, cols.caption(dimStyle.Render("stats:")))
	lines = append(lines, statTable(cur.Item.Stats, cand.Stats)...)
	// visible at the rendered 0.1 precision only — an exact float
	// comparison lets a hair's difference show as "552.2 +0.0 552.2"
	if lDPS, rDPS := score.WeaponDPS(cur.Item), score.WeaponDPS(cand); math.Abs(rDPS-lDPS) >= 0.05 {
		d := rDPS - lDPS
		dpsW := m.scorer.Profile().WeaponDPSWeight
		if cand.Slot == db.SlotRanged {
			dpsW = m.scorer.Profile().RangedDPSWeight
		}
		lines = append(lines, fmt.Sprintf("%-*s", cols.labelW, "weapon DPS")+
			fmt.Sprintf("%*.1f", cols.lw, lDPS)+cols.signed(d, cols.gw, "%+.1f")+
			cols.signed(d*dpsW, cols.epW, "%+.0f")+fmt.Sprintf("%*.1f", cols.rw, rDPS))
	}

	// --- racial (weapon-type virtual stats, e.g. Troll +1% crit with
	// bows): invisible on the in-game tooltip, so it never mixes into the
	// stats table — its own section prices what applying/losing it costs ---
	if m.char != nil {
		lRac := m.game.Rules.RacialWeaponStats(m.char.Race, cur.Item)
		rRac := m.game.Rules.RacialWeaponStats(m.char.Race, cand)
		if lRac != (db.Stats{}) || rRac != (db.Stats{}) {
			side := func(it *db.Item) string {
				if note := m.game.Rules.RacialWeaponNote(m.char.Race, it); note != "" {
					return okStyle.Render("★ ") + dimStyle.Render(note)
				}
				return dimStyle.Render("none")
			}
			lines = append(lines, "", dimStyle.Render("racial: ")+
				side(cur.Item)+dimStyle.Render(" → ")+side(cand))
			lines = append(lines, fullTable(lRac, rRac)...)
		}
	}

	// --- enchants: real (or assumed) vs the candidate's assumption.
	// An identical enchant still shows (the candidate's enchant plan is
	// information); only two bare sides say nothing ---
	curEnch := m.wornEnchant(cur)
	switch {
	case curEnch == nil && candEnch == nil:
		// nothing enchantable either side: stay silent
	default:
		haveName, haveStats := dimStyle.Render("none"), db.Stats{}
		if curEnch != nil {
			haveName, haveStats = enchantText(curEnch.Name), curEnch.Stats
			if !cur.HasDetail {
				haveName += dimStyle.Render(" (assumed)")
			}
		}
		wantName, wantStats := dimStyle.Render("none"), db.Stats{}
		if candEnch != nil {
			wantName, wantStats = enchantText(candEnch.Name)+dimStyle.Render(" (assumed)"), candEnch.Stats
		}
		lines = append(lines, "", dimStyle.Render("enchants: ")+
			haveName+dimStyle.Render(" → ")+wantName)
		lines = append(lines, fullTable(haveStats, wantStats)...)
	}

	// --- reforge (versions with reforging only): the worn instance's
	// actual reforge vs the candidate's assumption — the cap-aware pick
	// the upgrade ranking priced — in full: gained stat AND sacrificed ---
	if m.scorer.Reforging() {
		curRef := cur.Reforge
		candRef := m.scorer.AssumedReforgeWorn(ws, m.upSlot, cand)
		if curRef != nil || candRef != nil {
			want := dimStyle.Render("none")
			if candRef != nil {
				want = m.reforgeText(candRef) + dimStyle.Render(" (assumed)")
			}
			// ⇒ between the sides: → is the reforge's own from→to arrow
			lines = append(lines, "", dimStyle.Render("reforge: ⚒ ")+
				m.reforgeText(curRef)+dimStyle.Render("  ⇒  ")+want)
			lines = append(lines, fullTable(reforgeStats(curRef), reforgeStats(candRef))...)
		}
		// A candidate whose worth turns on re-reforging needs the two
		// numbers side by side, or the row's value looks unreachable:
		// the ranking prices the swap with the WHOLE set free to
		// re-balance, which is what the reforge advisor would then do.
		if u, ok := m.upgradeFor(cand); ok && u.ReforgeDependent() {
			lines = append(lines, "", dimStyle.Render("⟳ ")+
				signedEP(u.Delta)+dimStyle.Render(" after a reforge pass    ")+
				signedEP(u.Frozen)+dimStyle.Render(" as the set sits now"))
			if why := m.reforgeSwing(cur, cand); why != "" {
				lines = append(lines, dimStyle.Render(why))
			}
		}
	}

	// --- gems: socket-by-socket, actuals vs the candidate's assumption.
	// Every socket shows (an identical gem is still information: the
	// candidate's socket colors stay visible); only the per-gem stat
	// rows and an unchanged bonus say nothing ---
	leftGems := m.wornGems(cur)
	rightGems, recMatched := m.scorer.AssumedGemsSwap(cand, ctx)
	if len(leftGems) > 0 || len(rightGems) > 0 {
		// each side's extra (buckle / blacksmith) gem lives just past its
		// own item's sockets; pad the shorter socket list so extras pair
		// with extras instead of leaking into the other side's socket rows
		extraRow := max(len(cur.Item.GemSockets), len(cand.GemSockets))
		leftGems = alignExtras(leftGems, len(cur.Item.GemSockets), extraRow)
		rightGems = alignExtras(rightGems, len(cand.GemSockets), extraRow)
		n := max(len(leftGems), len(rightGems))
		gemAt := func(gs []*db.Gem, i int) *db.Gem {
			if i < len(gs) {
				return gs[i]
			}
			return nil
		}
		sockAt := func(it *db.Item, i int) (db.GemColor, bool) {
			if i < len(it.GemSockets) {
				return it.GemSockets[i], true
			}
			return db.GemPrismatic, false
		}
		gemName := func(g *db.Gem, hasSock bool, assumed bool) string {
			switch {
			case g != nil && assumed:
				return gemText(g) + dimStyle.Render(" (assumed)")
			case g != nil:
				return gemText(g)
			case hasSock:
				return warnStyle.Render("empty")
			}
			return dimStyle.Render("no socket")
		}

		curBonus := m.scorer.BonusActive(cur)
		var lBonus, rBonus db.Stats
		if curBonus {
			lBonus = cur.Item.SocketBonus
		}
		if recMatched {
			rBonus = cand.SocketBonus
		}

		var gl []string
		for i := 0; i < n; i++ {
			lc, lok := sockAt(cur.Item, i)
			rc, rok := sockAt(cand, i)
			l, r := gemAt(leftGems, i), gemAt(rightGems, i)
			if l == nil && r == nil {
				continue // no gem either side (and no socket to show)
			}
			glyphL, glyphR := "", ""
			if lok {
				glyphL = gemColorStyle(lc).Render("◆") + dimStyle.Render(" "+lc.String())
			}
			if rok {
				glyphR = gemColorStyle(rc).Render("◆") + dimStyle.Render(" "+rc.String())
			}
			sock := glyphL
			if lok && rok && lc != rc {
				sock = glyphL + dimStyle.Render(" → ") + glyphR
			} else if !lok {
				sock = glyphR
			}
			leftName := gemName(l, lok, !cur.HasDetail)
			rightName := gemName(r, rok, true)
			if i >= extraRow {
				// the enhancement socket (buckle / blacksmith) on both sides
				sock = gemColorStyle(db.GemPrismatic).Render("◆") + dimStyle.Render(" "+extraSocketLabel(cand.Slot))
				if l == nil {
					leftName = warnStyle.Render("not applied")
				}
			}
			gl = append(gl, sock+dimStyle.Render(": ")+
				dimStyle.Render(leftName)+
				dimStyle.Render(" → ")+rightName)
			var lst, rst db.Stats
			if l != nil {
				lst = l.Stats
			}
			if r != nil {
				rst = r.Stats
			}
			gl = append(gl, fullTable(lst, rst)...)
		}
		// socket bonus: whenever either side counts one (the main table
		// no longer carries it), or its activation changes
		if lBonus != (db.Stats{}) || rBonus != (db.Stats{}) || curBonus != recMatched {
			bonusText := func(st db.Stats, active bool) string {
				if !active {
					return warnStyle.Render("inactive")
				}
				return okStyle.Render("active") + dimStyle.Render(" ("+strings.Join(m.statLines(st, nil), ", ")+")")
			}
			gl = append(gl, dimStyle.Render("◇ socket bonus: ")+
				bonusText(cur.Item.SocketBonus, curBonus)+dimStyle.Render(" → ")+
				bonusText(cand.SocketBonus, recMatched))
			gl = append(gl, fullTable(lBonus, rBonus)...)
		}
		if len(gl) > 0 {
			lines = append(lines, "", dimStyle.Render("gems:"))
			lines = append(lines, gl...)
		}
	}

	// context lines below the sections
	var notes []string
	if cand.SetName != "" {
		label := "set: "
		if t := m.tierLabel(cand); t != "" {
			label = t + " set: "
		}
		notes = append(notes, titleStyle.Render(label)+dimStyle.Render(cand.SetName))
	}
	notes = append(notes, m.setBonusNotes(cur.Item, cand)...)
	for _, e := range m.game.DB.Effects[cand.ID] {
		notes = append(notes, strings.Join(m.effectLines(e), " "))
	}
	if len(m.game.DB.Effects[cur.Item.ID]) > 0 {
		var lost []string
		for _, e := range m.game.DB.Effects[cur.Item.ID] {
			lost = append(lost, m.effectLines(e)[0])
		}
		notes = append(notes, dimStyle.Render("replacing your ")+strings.Join(lost, dimStyle.Render(" · ")))
	}
	if len(notes) > 0 {
		lines = append(lines, "")
		lines = append(lines, notes...)
	}

	// one column short of the screen: the scrollbar takes the last one
	// when the pane outgrows its window
	return detailStyle.Width(max(m.width-3, 40)).
		Render(strings.Join(lines, "\n"))
}

// setBonusNotes describes the tier bonuses a swap would break or
// complete, with their curated score values.
func (m Model) setBonusNotes(cur, cand *db.Item) []string {
	if cur.SetName == cand.SetName {
		return nil
	}
	counts := map[string]int{}
	for _, r := range m.gearRows() {
		if r.item.SetName != "" {
			counts[r.item.SetName]++
		}
	}
	var notes []string
	if cur.SetName != "" {
		n := counts[cur.SetName]
		for _, b := range m.game.Rules.SetBonusesForRole(cur.SetName, m.profileRole()) {
			if n >= b.Pieces && n-1 < b.Pieces && b.EP != 0 {
				notes = append(notes, warnStyle.Render(fmt.Sprintf("✂ breaks %s %dpc", cur.SetName, b.Pieces))+
					dimStyle.Render(" — "+b.Note+" ")+errStyle.Render(fmt.Sprintf("%+.0f", -b.EP)))
			}
		}
	}
	if cand.SetName != "" {
		n := counts[cand.SetName]
		for _, b := range m.game.Rules.SetBonusesForRole(cand.SetName, m.profileRole()) {
			if n < b.Pieces && n+1 >= b.Pieces && b.EP != 0 {
				notes = append(notes, okStyle.Render(fmt.Sprintf("◈ completes %s %dpc", cand.SetName, b.Pieces))+
					dimStyle.Render(" — "+b.Note+" ")+okStyle.Render(fmt.Sprintf("%+.0f", b.EP)))
			}
		}
	}
	return notes
}

// wornEnchant is the enchant counted for the equipped side: the real
// one on enriched imports, the assumed best otherwise.
func (m Model) wornEnchant(w score.Worn) *db.Enchant {
	if w.HasDetail {
		return m.game.DB.EnchantByEffect(w.EnchantID, w.Item.Slot)
	}
	return m.scorer.BestEnchant(w.Item)
}

// wornGems lists the equipped side's per-socket gems: the real ones on
// enriched imports (including extra buckle sockets), the assumptions
// otherwise.
func (m Model) wornGems(w score.Worn) []*db.Gem {
	if !w.HasDetail {
		rec, _ := m.scorer.AssumedGems(w.Item)
		return rec
	}
	n := max(len(w.Item.GemSockets), len(w.Gems))
	out := make([]*db.Gem, n)
	for i := 0; i < n; i++ {
		if i < len(w.Gems) && w.Gems[i] != 0 {
			out[i] = m.game.DB.ResolveGem(w.Gems[i])
		}
	}
	return out
}

// extraSocketLabel is the short row label for an item's enhancement
// socket (the long name lives in score.ExtraSocketName).
func extraSocketLabel(slot db.Slot) string {
	if slot == db.SlotWaist {
		return "Buckle"
	}
	return "BS Socket"
}

// alignExtras pads a per-socket gem list so entries beyond the item's
// own sockets (the enhancement socket's gem) start at the shared
// extraRow — the compare loop pairs sockets by raw index, and two items
// may differ in native socket count.
func alignExtras(gs []*db.Gem, native, extraRow int) []*db.Gem {
	if len(gs) <= native || native >= extraRow {
		return gs
	}
	out := make([]*db.Gem, 0, len(gs)+extraRow-native)
	out = append(out, gs[:native]...)
	for i := native; i < extraRow; i++ {
		out = append(out, nil)
	}
	return append(out, gs[native:]...)
}

// addStats is a tiny local sum helper (db.Stats is a value array).
func addStats(a, b db.Stats) db.Stats {
	for i := range a {
		a[i] += b[i]
	}
	return a
}

// subStats is addStats' inverse.
func subStats(a, b db.Stats) db.Stats {
	for i := range a {
		a[i] -= b[i]
	}
	return a
}
