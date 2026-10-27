package tui

import (
	"fmt"
	"math"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/vaclav-dvorak/twolk/internal/armory"
	"github.com/vaclav-dvorak/twolk/internal/db"
	"github.com/vaclav-dvorak/twolk/internal/score"
)

// The main screen: gear list left, item detail right, quick tips
// below (tips.go renders the pane itself).

func (m Model) updateNormal(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
		return m, nil
	case "down", "j":
		if m.cursor < len(m.gearRows())-1 {
			m.cursor++
		}
		return m, nil
	case "i":
		// adding a character (realm + name) is its own flow; the gear
		// always comes in through the refresh (r) it opens
		m.mode = modeImport
		m.impRefresh = false
		m.pasted, m.typedPaste = nil, false
		m.errMsg = ""
		m.importNote = ""
		m.impStep = 0
		m.impName.Blur()
		m.impName.SetValue("")
		if j, ok := m.homeRow(); ok {
			m.impCursor = j // the home realm is still the likeliest pick
		}
		return m, nil
	case "r":
		if m.char == nil {
			return m, nil
		}
		m.errMsg = ""
		return m.startRefresh()
	case "f":
		m.mode = modeFilter
		m.fromUps, m.fromBis = false, false
		return m, nil
	case "t":
		if m.talentAssumptions() {
			m.mode = modeTalents
			m.tCursor = 0
		}
		return m, nil
	case "c":
		if len(m.cfg.Characters()) > 0 {
			m.mode = modeChars
			m.cCursor = m.cfg.ActiveChar
		}
		return m, nil
	case "w":
		if m.baseProfile() != nil {
			m.mode = modeWeights
			m.wCursor = 0
		}
		return m, nil
	case "o", "g", "e", "R":
		// one advisor for gems + enchants + reforges (g/e/R are the old
		// per-domain keys, kept as aliases)
		if m.scorer != nil {
			m.openOptimize()
		}
		return m, nil
	case "b":
		if m.scorer != nil && len(m.gearRows()) > 0 {
			m.openBis()
		}
		return m, nil
	case "p":
		if m.scorer != nil && len(m.gearRows()) > 0 {
			m.openEmblems()
		}
		return m, nil
	case "enter":
		if m.scorer != nil && len(m.gearRows()) > 0 {
			m.upSlot = min(m.cursor, len(m.gearRows())-1)
			m.recomputeUpgrades()
			m.mode = modeUpgrades
		}
		return m, nil
	}
	return m, nil
}

func (m Model) viewNormal() string {
	title := titleStyle.Render("twolk") + dimStyle.Render("  ·  terminal "+m.game.Name+" gear planner")

	var sections []string
	sections = append(sections, title, "")

	switch {
	case m.char == nil:
		sections = append(sections,
			dimStyle.Render("no character imported yet — press ")+helpKey.Render("i")+dimStyle.Render(" to import"))
	default:
		// the status message lives on the separator row below the
		// character line, so it never adds height (or hides the help row)
		statusLine := ""
		if m.status != "" {
			statusLine = statusStyle.Render(m.status)
		}
		sections = sections[:0]
		sections = append(sections, m.viewHeader(), statusLine)
		if len(m.char.Equipment) == 0 {
			break // a placeholder: the header says what to do
		}
		sections = append(sections, m.viewGear())
		// give the tips pane only the rows left between the gear panes
		// and the bottom help line, so it can't push help off screen
		used := lipgloss.Height(lipgloss.JoinVertical(lipgloss.Left, sections...))
		reserved := 2 // gap + help row
		if m.dbErr != nil {
			reserved += 2
		}
		budget := m.height - used - reserved - 3 // 3: tips border + title
		if m.height <= 0 {
			budget = 32 // size unknown yet: render everything
		}
		if tips := m.viewTips(budget); tips != "" {
			sections = append(sections, tips)
		}
	}

	if m.dbErr != nil {
		sections = append(sections, "", errStyle.Render("item db: "+m.dbErr.Error()))
	}
	if m.status != "" && m.char == nil {
		// no header to host it yet (pre-import screen)
		sections = append(sections, "", statusStyle.Render(m.status))
	}

	talentsHelp := ""
	if m.talentAssumptions() {
		talentsHelp = helpLine("t", "talents") + "  "
	}
	refreshHelp := ""
	if m.char != nil {
		refreshHelp = helpLine("r", "refresh") + "  "
	}
	help := helpLine("↑/↓", "select", "enter", "upgrades", "f", "filter") + "  " +
		talentsHelp +
		helpKey.Render("w") + helpDesc.Render(" weights  ") +
		helpKey.Render("o") + helpDesc.Render(" optimize  ") +
		helpKey.Render("b") + helpDesc.Render(" bis  ") +
		helpKey.Render("p") + helpDesc.Render(" emblems  ") +
		helpKey.Render("i") + helpDesc.Render(" add  ") +
		refreshHelp +
		helpKey.Render("c") + helpDesc.Render(" chars  ") +
		helpKey.Render("q") + helpDesc.Render(" quit")

	return m.pinFooter(lipgloss.JoinVertical(lipgloss.Left, sections...), help)
}

// viewGear renders the gear list with cursor selection on the left and a
// detail panel for the selected item on the right.
func (m Model) viewGear() string {
	if m.game.DB == nil {
		return helpKey.Render(m.spin.View()) + dimStyle.Render("loading item database…")
	}
	rows := m.gearRows()
	if len(rows) == 0 {
		return dimStyle.Render("none of the equipped items resolve in the item database")
	}
	cursor := min(m.cursor, len(rows)-1)
	avg := ilvlStats(rows)
	lowest := rows[0].item.ILvl
	for _, r := range rows[1:] {
		lowest = min(lowest, r.item.ILvl)
	}

	// Gear list and info pane split the screen 50/50.
	leftW := max(m.width/2, 40)
	nameMax := leftW - 29 // marker + slot + ilvl + gaps + [H] + tier tag

	// red is reserved for the set's weakest link, and only when that
	// RANKS something: if every below-average item sits at the same
	// lowest ilvl (a set spanning one tier, e.g. 346s under 359s), red
	// and yellow would flag the very same rows — the escalation says
	// nothing, so those rows stay yellow and the legend drops the entry
	ranksLowest := false
	for _, r := range rows {
		if float64(r.item.ILvl) < math.Round(avg) && r.item.ILvl > lowest {
			ranksLowest = true
		}
	}

	// slots with no available upgrade under the current filter are best
	// in slot: the tips hints carry exactly the slots that CAN improve
	upgradeable := map[int]bool{}
	for _, h := range m.tips.items {
		upgradeable[h.rowIdx] = true
	}

	var lines []string
	for i, r := range rows {
		it := r.item
		sel := rowStyler(i == cursor)
		// marker column: single-width glyphs only — emoji render at
		// different widths across terminals/fonts and break alignment
		marker := sel(lipgloss.NewStyle()).Render("  ")
		switch {
		case m.scorer != nil && !upgradeable[i]:
			marker = sel(bisStyle).Render("✓ ")
		case float64(it.ILvl) < math.Round(avg):
			// compare against the DISPLAYED (rounded) average — an item
			// matching the shown "avg ilvl 238" must not be flagged as
			// below it just because the true mean is 238.3; the set's
			// lowest ilvl escalates to red — replace it first
			st := warnStyle
			if it.ILvl == lowest && ranksLowest {
				st = errStyle
			}
			marker = sel(st).Render("▲ ")
		}
		row := marker +
			sel(lipgloss.NewStyle()).Render(fmt.Sprintf("%-9s ", it.Slot)) +
			sel(dimStyle).Render(fmt.Sprintf("%4d", it.ILvl)) +
			sel(lipgloss.NewStyle()).Render("  ") +
			sel(qualityStyles[it.Quality]).Render(truncate(it.Name, nameMax))
		if it.Heroic {
			row += sel(okStyle).Render(" [H]")
		}
		if r.rollGuess {
			row += sel(warnStyle).Render("?") // assumed suffix roll
		}
		if t := m.tierLabel(it); t != "" {
			row += sel(tierStyle).Render(" " + t)
		}
		if pad := leftW - lipgloss.Width(row); pad > 0 {
			row += sel(lipgloss.NewStyle()).Render(strings.Repeat(" ", pad))
		}
		lines = append(lines, row)
	}
	ghosts := m.missingSlotLines(rows)
	if l, at := m.offhandLine(rows); l != "" {
		ghosts = append(ghosts, ghost{l, at})
	}
	// display-only rows insert bottom-up so earlier indices stay valid;
	// walking the ascending-sorted list backwards keeps same-position
	// ghosts in their emitted (canonical) order
	sort.SliceStable(ghosts, func(i, j int) bool { return ghosts[i].at < ghosts[j].at })
	for i := len(ghosts) - 1; i >= 0; i-- {
		g := ghosts[i]
		lines = append(lines[:g.at], append([]string{truncate(g.line, leftW)}, lines[g.at:]...)...)
	}
	legend := dimStyle.Render("avg ilvl ") + titleStyle.Render(fmt.Sprintf("%.0f", avg)) +
		dimStyle.Render("  ·  ") + warnStyle.Render("▲ ") + dimStyle.Render("below avg")
	if ranksLowest {
		legend += dimStyle.Render("  ·  ") + errStyle.Render("▲ ") + dimStyle.Render("lowest")
	}
	legend += dimStyle.Render("  ·  ") + bisStyle.Render("✓ ") +
		dimStyle.Render("best in slot (current filter)")
	lines = append(lines, "", legend)
	lines = append(lines, m.setBonusLines(rows, leftW)...)
	left := strings.Join(lines, "\n")

	// Fixed pane size derived from the terminal, not the content, so the
	// frame doesn't jump around while browsing items. Content is wrapped
	// to the pane width and hard-clamped to its height — Height() alone
	// only sets a minimum and would grow with long content.
	detailWidth := max(m.width-leftW-3, 30)
	// Height budget: the screen minus the header block (≈8 lines with
	// the logo) and the legend/status/help below, minus two rows ceded
	// to the quick-tips pane. Enriched imports render more lines (real
	// gems, enchant), so prefer the screen budget over the gear-list
	// height; overflow ends in a "…" line.
	detailHeight := max(min(m.height-24, len(rows)+12), 12)
	inner := lipgloss.NewStyle().Width(detailWidth - 2). // minus padding
								Render(m.viewItemDetail(rows, cursor))
	content := strings.Split(inner, "\n")
	if len(content) > detailHeight {
		content = append(content[:detailHeight-1], dimStyle.Render("…"))
	}
	right := detailStyle.Width(detailWidth).Height(detailHeight).
		Render(strings.Join(content, "\n"))
	return lipgloss.JoinHorizontal(lipgloss.Top, left, " ", right)
}

// ghost is one display-only gear-list row (never selectable): an
// unfilled equipment slot or the off-hand note, with its insertion
// index into the rendered rows.
type ghost struct {
	line string
	at   int
}

// missingSlotLines renders display-only rows for the standard slots
// the resolved gear does not fill. A gap whose slot matches a worn
// export entry the item db cannot resolve is gear below the endgame
// pool — say so instead of pretending the slot is naked; the rest
// read "unequipped". Plain pastes carry no slot claims (fetch imports
// do), so their unresolved cosmetics attribute nothing.
func (m Model) missingSlotLines(rows []gearRow) []ghost {
	if m.game.DB == nil || m.char == nil {
		return nil
	}
	count := map[db.Slot]int{}
	for _, r := range rows {
		count[r.item.Slot]++
	}
	// the gear check already attributed every unscorable worn item to
	// a slot, names included (gearcheck.go)
	unscored := map[db.Slot][]gearGap{}
	for _, g := range m.gaps {
		unscored[g.slot] = append(unscored[g.slot], g)
	}
	var out []ghost
	var naked []int // indices of plain empty-slot rows, for shedding
	for _, std := range score.StandardSlots {
		for n := count[std.Slot]; n < std.N; n++ {
			pad := fmt.Sprintf("  %-9s       ", std.Slot)
			if worn := unscored[std.Slot]; len(worn) > 0 {
				unscored[std.Slot] = worn[1:]
				out = append(out, ghost{pad + dimStyle.Render("unscored — "+worn[0].note()),
					ghostAt(rows, std.Slot)})
				continue
			}
			naked = append(naked, len(out))
			out = append(out, ghost{pad + dimStyle.Render("unequipped"), ghostAt(rows, std.Slot)})
		}
	}
	// weapon rows sit outside StandardSlots (their count is the layout
	// solver's business), so an unscorable main hand — the fishing pole
	// left in a hand at logout — would otherwise leave no trace at all
	for _, g := range unscored[db.SlotWeapon] {
		if hasWeaponRow(rows, g) {
			continue // the pool carries it: the item has a real row already
		}
		pad := fmt.Sprintf("  %-9s       ", db.SlotWeapon)
		out = append(out, ghost{pad + dimStyle.Render("unscored — "+g.note()), weaponGhostAt(rows)})
	}

	// short terminals host only so many extra rows: shed plain empty
	// slots bottom-up first — worn-but-unscored rows explain gear the
	// user actually HAS and give way last. Display order stays canonical.
	budget := len(out)
	if m.height > 0 {
		budget = max(m.height-24-len(rows), 0)
	}
	drop := make([]bool, len(out))
	over := len(out) - budget
	for i := len(naked) - 1; i >= 0 && over > 0; i-- {
		drop[naked[i]] = true
		over--
	}
	kept := out[:0]
	for i, g := range out {
		if !drop[i] && len(kept) < budget {
			kept = append(kept, g)
		}
	}
	return kept
}

// ghostAt places a slot's ghost row next to its slot mates, or at the
// canonical position when the slot is entirely absent — the same rule
// the BiS solver uses for synthesized rows.
func ghostAt(rows []gearRow, s db.Slot) int {
	at := len(rows)
	for i := len(rows) - 1; i >= 0; i-- {
		if rows[i].item.Slot == s {
			return i + 1
		}
		if score.SlotRank(rows[i].item.Slot) > score.SlotRank(s) {
			at = i
		}
	}
	return at
}

// hasWeaponRow reports whether the gap's item is already on the list
// under its own name — true for the handful of tools the item pool
// does carry (the Northrend fishing poles), which need no ghost.
func hasWeaponRow(rows []gearRow, g gearGap) bool {
	for _, r := range rows {
		if r.item.ID == g.id {
			return true
		}
	}
	return false
}

// weaponGhostAt places the weapon row where the character sheet has
// it: after the trinkets, above the ranged slot. SlotRank ties the two
// (weapons are outside StandardSlots), so ghostAt cannot tell them
// apart on its own.
func weaponGhostAt(rows []gearRow) int {
	for i, r := range rows {
		if r.item.Slot == db.SlotRanged {
			return i
		}
	}
	return ghostAt(rows, db.SlotWeapon)
}

// offhandLine renders the character sheet's second weapon slot when
// only one weapon is worn — the slot always exists, so the list always
// shows it: dimmed when the worn weapon blocks it (two-hander, no
// Titan's Grip), as an actionable empty slot when the build could fill
// it. The row is display-only (never selectable) and belongs right
// below the worn weapon; at is that insertion index into the row lines.
func (m Model) offhandLine(rows []gearRow) (line string, at int) {
	if m.scorer == nil {
		return "", 0
	}
	weap := -1
	for i, r := range rows {
		if r.item.Slot != db.SlotWeapon {
			continue
		}
		if weap >= 0 {
			return "", 0 // both hands worn: real rows cover them
		}
		weap = i
	}
	if weap < 0 {
		return "", 0
	}
	mh := rows[weap].item
	pad := fmt.Sprintf("  %-9s       ", "Off hand") // align with the row columns
	if !m.scorer.Profile().OffhandUsable(mh) {
		note := "empty — no off-hand layout for this build"
		if mh.HandType == db.HandTwoHand {
			note = "empty — two-hander occupies both hands"
		}
		return dimStyle.Render(pad + note), weap + 1
	}
	note := "an off-hand item fits here"
	if mh.HandType == db.HandTwoHand {
		note = "Titan's Grip fits a second two-hander"
	}
	return dimStyle.Render(pad + "empty — " + note), weap + 1
}

// setBonusLines summarizes worn tier sets under the gear list: piece
// count, active bonuses (with curated values), and the next threshold.
func (m Model) setBonusLines(rows []gearRow, width int) []string {
	counts := map[string]int{}
	var order []string
	for _, r := range rows {
		if sn := r.item.SetName; sn != "" {
			if counts[sn] == 0 {
				order = append(order, sn)
			}
			counts[sn]++
		}
	}
	var out []string
	for _, sn := range order {
		n := counts[sn]
		if n < 2 {
			continue // a lone piece has no bonus; the row tag covers it
		}
		head := titleStyle.Render("◈ ") + dimStyle.Render("set bonus: ") +
			titleStyle.Render(sn) + dimStyle.Render(fmt.Sprintf(" ×%d", n))
		if len(m.game.Rules.SetBonusesFor(sn)) == 0 {
			// a set we haven't curated bonus values for (crafted sets,
			// off-tier sets): say so instead of a cryptic bare count
			out = append(out, truncate(head+dimStyle.Render("  ·  not valued"), width))
			continue
		}
		out = append(out, truncate(head, width))
		for _, b := range m.game.Rules.SetBonusesForRole(sn, m.profileRole()) {
			var line string
			switch {
			case n >= b.Pieces && b.EP != 0:
				line = okStyle.Render(fmt.Sprintf("  %dpc +%.0f ", b.Pieces, b.EP)) +
					dimStyle.Render(b.Note)
			case n >= b.Pieces:
				line = dimStyle.Render(fmt.Sprintf("  %dpc %s", b.Pieces, b.Note))
			default:
				line = dimStyle.Render(fmt.Sprintf("  %dpc at %d pieces", b.Pieces, b.Pieces))
			}
			out = append(out, truncate(line, width))
		}
	}
	return out
}

// viewItemDetail renders the info panel for rows[idx].
func (m Model) viewItemDetail(rows []gearRow, idx int) string {
	it := rows[idx].item
	eq := rows[idx].equipped
	name := m.itemLabel(it, rowStyler(false))
	meta := fmt.Sprintf("%s · ilvl %d", it.Slot, it.ILvl)
	if m.scorer != nil {
		meta += fmt.Sprintf(" · contributes %.0f", m.scorer.ContributionWorn(wornOf(rows), idx))
	}
	if it.SetName != "" {
		meta += "\n" + it.SetName
	}
	sections := []string{name, dimStyle.Render(meta)}

	if dps := score.WeaponDPS(it); dps > 0 {
		line := okStyle.Render(fmt.Sprintf("%.1f DPS", dps)) +
			dimStyle.Render(fmt.Sprintf("  (%.0f – %.0f, %.2f speed)", it.WeaponDmgMin, it.WeaponDmgMax, it.WeaponSpeed))
		if m.char != nil {
			if note := m.game.Rules.RacialWeaponNote(m.char.Race, it); note != "" {
				line += "\n" + okStyle.Render("★ ") + dimStyle.Render(note)
			}
		}
		sections = append(sections, line)
	}

	for _, group := range m.statGroups(it.Stats) {
		sections = append(sections, strings.Join(group, "\n"))
	}

	if sock := m.viewSockets(it, eq); sock != "" {
		sections = append(sections, sock)
	}
	if ench := m.viewEnchant(it, eq); ench != "" {
		sections = append(sections, ench)
	}
	if ref := m.viewWornReforge(it, eq); ref != "" {
		sections = append(sections, ref)
	}

	if effs := m.game.DB.Effects[it.ID]; len(effs) > 0 {
		var lines []string
		for _, e := range effs {
			lines = append(lines, m.effectLines(e)...)
		}
		sections = append(sections, strings.Join(lines, "\n"))
	} else if it.Slot == db.SlotTrinket {
		// no extracted or curated effect data for this trinket
		sections = append(sections, dimStyle.Italic(true).Render("on-use/proc effect unknown (not tracked)"))
	}

	if len(it.Sources) > 0 {
		sections = append(sections, m.sourceText(it))
	}
	return strings.Join(sections, "\n\n")
}

// viewSockets renders the socket section: the actual gems when the
// import carried them (enriched export), the scorer's assumptions
// otherwise.
func (m Model) viewSockets(it *db.Item, eq armory.EquippedItem) string {
	if !eq.HasDetail {
		var rec []*db.Gem
		matched := true
		if m.scorer != nil {
			rec, matched = m.scorer.AssumedGems(it)
		}
		if len(it.GemSockets) == 0 && len(rec) == 0 {
			return ""
		}
		var socks []string
		for i, c := range it.GemSockets {
			// the scorer decides the package (it may skip weak socket
			// bonuses); the static family name is the fallback before
			// the scorer is ready
			assumed := c.EpicFamily()
			if i < len(rec) && rec[i] != nil {
				assumed = rec[i].Name
			}
			socks = append(socks, gemColorStyle(c).Render("◆ "+c.String())+
				dimStyle.Render(" — assume "+assumed))
		}
		// the enhancement socket (buckle / blacksmith; the assumed gem
		// package runs one gem past the item's own sockets)
		for i := len(it.GemSockets); i < len(rec); i++ {
			assumed := "any gem"
			if rec[i] != nil {
				assumed = rec[i].Name
			}
			socks = append(socks, gemColorStyle(db.GemPrismatic).Render("◆ "+extraSocketLabel(it.Slot))+
				dimStyle.Render(" — assume "+m.game.Rules.ExtraSocketName(it.Slot)+" with "+assumed))
		}
		if bonus := m.statLines(it.SocketBonus, nil); len(bonus) > 0 {
			line := "bonus: " + strings.Join(bonus, ", ")
			if matched {
				socks = append(socks, dimStyle.Render(line))
			} else {
				socks = append(socks, warnStyle.Render(line+" — deliberately skipped"))
			}
		}
		return strings.Join(socks, "\n")
	}

	// actual gems from the enriched export
	assumesExtra := m.scorer != nil && m.scorer.AssumesExtraSocket(it)
	n := max(len(it.GemSockets), len(eq.Gems))
	if n == 0 && !assumesExtra {
		return ""
	}
	var socks []string
	matched := true
	for i := 0; i < n; i++ {
		var g *db.Gem
		if i < len(eq.Gems) && eq.Gems[i] != 0 {
			g = m.game.DB.ResolveGem(eq.Gems[i])
		}
		inSocket := i < len(it.GemSockets)
		if !inSocket && g == nil {
			continue // trailing zero for a socket the item doesn't have
		}
		label, style := "Extra", dimStyle
		switch {
		case inSocket:
			label, style = it.GemSockets[i].String(), gemColorStyle(it.GemSockets[i])
		case m.game.Rules.ExtraSocketName(it.Slot) != "":
			label, style = extraSocketLabel(it.Slot), gemColorStyle(db.GemPrismatic)
		}
		switch {
		case g == nil:
			matched = false
			socks = append(socks, style.Render("◆ "+label)+errStyle.Render(" — empty socket!"))
		case inSocket && !db.ColorMatches(it.GemSockets[i], g.Color):
			matched = false
			socks = append(socks, style.Render("◆ "+label)+" "+gemText(g)+
				warnStyle.Render(" (bonus mismatch)"))
		default:
			socks = append(socks, style.Render("◆ "+label)+" "+gemText(g))
		}
	}
	if assumesExtra && !score.HasExtraGem(score.Worn{Item: it, Gems: eq.Gems}) {
		socks = append(socks, gemColorStyle(db.GemPrismatic).Render("◆ "+extraSocketLabel(it.Slot))+
			errStyle.Render(" — no "+m.game.Rules.ExtraSocketName(it.Slot)+"!"))
	}
	if bonus := m.statLines(it.SocketBonus, nil); len(bonus) > 0 && len(it.GemSockets) > 0 {
		line := "bonus: " + strings.Join(bonus, ", ")
		if matched {
			socks = append(socks, dimStyle.Render(line+" ✓"))
		} else {
			socks = append(socks, warnStyle.Render(line+" — inactive"))
		}
	}
	return strings.Join(socks, "\n")
}

// enchantableSlots warrant a "no enchant" warning; others (neck, rings
// without enchanting, trinkets) don't.
var enchantableSlots = map[db.Slot]bool{
	db.SlotHead: true, db.SlotShoulder: true, db.SlotBack: true,
	db.SlotChest: true, db.SlotWrist: true, db.SlotHands: true,
	db.SlotLegs: true, db.SlotFeet: true, db.SlotWeapon: true,
	db.SlotRanged: true,
}

// viewEnchant renders the actual enchant line for enriched imports.
func (m Model) viewEnchant(it *db.Item, eq armory.EquippedItem) string {
	if !eq.HasDetail {
		return ""
	}
	if e := m.game.DB.EnchantByEffect(eq.EnchantID, it.Slot); e != nil {
		text := enchantText(e.Name)
		if stats := m.statLines(e.Stats, nil); len(stats) > 0 {
			text += dimStyle.Render(" " + strings.Join(stats, ", "))
		}
		return okStyle.Render("Enchant: ") + text
	}
	if eq.EnchantID != 0 {
		return okStyle.Render("Enchant: ") + dimStyle.Render(fmt.Sprintf("#%d (not in db)", eq.EnchantID))
	}
	if enchantableSlots[it.Slot] {
		return errStyle.Render("no enchant!")
	}
	return ""
}

// viewWornReforge renders the worn item's ACTIVE reforge line — only
// under versions with reforging (cata), only for enriched imports.
// An unreforged item with reforgeable stats says so plainly; the
// advisor is the place that judges whether that's a loss.
func (m Model) viewWornReforge(it *db.Item, eq armory.EquippedItem) string {
	if m.scorer == nil || !m.scorer.Reforging() || !eq.HasDetail {
		return ""
	}
	if r := eq.Reforge; r != nil {
		return okStyle.Render("⚒ Reforged: ") +
			fmt.Sprintf("%.0f %s", r.Amount, m.statLabel(r.From)) +
			dimStyle.Render(" → ") + m.statLabel(r.To)
	}
	if score.CanReforge(it) {
		return dimStyle.Render("⚒ not reforged")
	}
	return ""
}
