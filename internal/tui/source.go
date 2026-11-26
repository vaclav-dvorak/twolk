package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/vaclav-dvorak/twolk/internal/db"
)

// Item acquisition rendering shared by every list screen: where an
// item comes from, with routes the filter closed shown faint.

// routeOpen reports whether one source's acquisition route passes the
// active filter — the per-source mirror of allowItem's route switch,
// used only for display emphasis (closed routes render faint).
func (m Model) routeOpen(it *db.Item, s db.Source) bool {
	f := m.filter
	switch {
	case s.Drop != nil && m.game.DB.IsRaidDrop(s):
		if db.IsBoE(it.ID) {
			return f.RaidBoE || f.Raids
		}
		return f.Raids
	case s.Drop != nil && s.Drop.OtherName == db.WorldDrop:
		return f.Other
	case s.Drop != nil:
		return f.Dungeon
	case s.SoldBy != nil:
		if base := m.game.DB.UpgradeBase(it.ID); base != nil {
			return m.firestoneOpen() && m.allowItem(base)
		}
		if m.game.DB.RaidGated(it) {
			return f.Raids
		}
		return f.Vendor && !m.zoneLocked(s.SoldBy.ZoneID)
	case s.Rep != nil:
		return f.Vendor
	case s.Crafted != nil:
		return f.Crafted && !m.recipeLocked(s.Crafted.SpellID) && m.craftObtainable(it, s.Crafted)
	case s.Quest != nil:
		if m.game.DB.RaidGated(it) {
			return f.Raids
		}
		return f.Other
	}
	return true
}

// sourceText describes how to obtain the item, resolving names and
// merging drop difficulties ("Marrowgar · ICC (10/25)"). Routes the
// active filter allows come first; filtered-off routes stay listed
// (the item exists there) but render faint.
func (m Model) sourceText(it *db.Item) string { return m.sourceTextSel(it, rowStyler(false)) }

// sourceTextSel is sourceText styled for a list row: sel (rowStyler)
// lays the cursor row's highlight under every styled span, so the
// highlighted row keeps its colors — the accent "craftable by
// yourself" included — instead of being flattened to one style.
func (m Model) sourceTextSel(it *db.Item, sel func(lipgloss.Style) lipgloss.Style) string {
	if len(it.Sources) == 0 {
		if m.game.DB.RaidGated(it) {
			// Sanctified upgrades et al. carry no upstream source
			return sel(m.srcStyle(m.filter.Raids)).Render("Vendor: raid tokens")
		}
		if m.game.DB.PvPItem(it) {
			return sel(m.srcStyle(m.filter.PvP)).Render("PvP vendor: honor / marks")
		}
		return sel(dimStyle).Render("source unknown")
	}
	type dropKey struct {
		who  string
		zone int
		cat  string
	}
	type srcPart struct {
		text string
		open bool
		// tag is a suffix in the accent color — the news on the line
		// ("craftable by yourself"); plain when the route is closed
		tag string
		// tail follows the tag in the plain style (the recipe route
		// after "craftable by yourself", so the label never truncates
		// behind a long vendor name)
		tail string
	}
	dropDiffs := map[dropKey][]string{}
	dropEvents := map[dropKey]string{}
	dropWorldBoss := map[dropKey]bool{}
	dropOpen := map[dropKey]bool{}
	var dropOrder []dropKey
	var parts []srcPart

	// the character's side only; an item only the other faction can get
	// (shown when it is worn, say) still names its routes
	sources := m.sideSources(it)
	if len(sources) == 0 {
		sources = it.Sources
	}
	for _, s := range sources {
		switch {
		case s.Drop != nil:
			who := m.game.DB.NPCs[s.Drop.NPCID]
			if who == "" {
				who = s.Drop.OtherName
			}
			k := dropKey{who: who, zone: s.Drop.ZoneID, cat: s.Drop.Category}
			if _, seen := dropDiffs[k]; !seen {
				dropOrder = append(dropOrder, k)
				dropDiffs[k] = nil
			}
			if m.routeOpen(it, s) {
				dropOpen[k] = true
			}
			if ev := m.game.DB.SeasonalEvent(s.Drop.NPCID); ev != "" {
				dropEvents[k] = ev
			}
			if m.game.DB.WorldBoss(s.Drop.NPCID) {
				dropWorldBoss[k] = true
			}
			// T7 raids carry dungeon-style difficulty values; a "HC"
			// label there would be misleading, so only 10/25 labels
			// apply inside raids
			if l := s.Drop.DifficultyLabel(); l != "" &&
				!(s.Drop.Difficulty <= 2 && m.game.DB.IsRaidZone(s.Drop.ZoneID)) {
				dropDiffs[k] = append(dropDiffs[k], l)
			}
		case s.SoldBy != nil:
			parts = append(parts, srcPart{m.vendorText(it, s.SoldBy), m.routeOpen(it, s), "", ""})
		case s.Rep != nil:
			text := "Reputation: "
			if name := m.game.DB.FactionName(s.Rep.FactionID); name != "" {
				text += name
			} else {
				text += fmt.Sprintf("faction %d", s.Rep.FactionID)
			}
			if st := s.Rep.Standing(); st != "" {
				text += " — " + st
			}
			parts = append(parts, srcPart{text, m.routeOpen(it, s), "", ""})
		case s.Quest != nil:
			open := m.routeOpen(it, s)
			switch {
			case s.Quest.Name != "" && m.game.DB.RaidGated(it):
				parts = append(parts, srcPart{"Raid quest: " + s.Quest.Name, open, "", ""})
			case s.Quest.Name != "":
				parts = append(parts, srcPart{"Quest: " + s.Quest.Name, open, "", ""})
			case m.game.DB.RaidGated(it): // nameless raid quest (cata curated)
				parts = append(parts, srcPart{"Raid quest reward", open, "", ""})
			default:
				parts = append(parts, srcPart{"Quest reward", open, "", ""})
			}
		case s.Crafted != nil:
			part := srcPart{text: "Crafted: " + s.Crafted.ProfessionName(), open: m.routeOpen(it, s)}
			recipe := m.recipeText(s.Crafted.SpellID)
			if self, known := m.selfCraftable(s.Crafted); known {
				if self {
					part.text += " · "
					part.tag = "craftable by yourself"
				} else {
					part.text += " · buy on AH"
				}
			}
			if part.tag != "" {
				part.tail = recipe
			} else {
				part.text += recipe
			}
			parts = append(parts, part)
		}
	}

	// the heroic-plus protocol route, once its phase opens: exact
	// fragment price when known, bare currency otherwise. Drop-pool
	// items already name the protocol on their drop line, so their
	// price renders as a plain vendor part.
	if pp, ok := m.protocolFor(it.ID); ok {
		switch {
		case pp.Count > 0 && pp.Drops:
			parts = append(parts, srcPart{
				fmt.Sprintf("Vendor: %d× %s", pp.Count, pp.Emblem), m.filter.Dungeon, "", ""})
		case pp.Count > 0:
			parts = append(parts, srcPart{
				fmt.Sprintf("%s: %d× %s", pp.Label, pp.Count, pp.Emblem), m.filter.Dungeon, "", ""})
		case !pp.Drops:
			parts = append(parts, srcPart{
				fmt.Sprintf("%s: %ss", pp.Label, pp.Emblem), m.filter.Dungeon, "", ""})
		}
	}

	drops := map[bool][]string{}
	for _, k := range dropOrder {
		var seg []string
		if k.who != "" {
			seg = append(seg, k.who)
		}
		if z := m.game.DB.Zones[k.zone]; z != "" {
			seg = append(seg, z)
		}
		line := strings.Join(seg, " · ")
		if diffs := dropDiffs[k]; len(diffs) > 0 {
			line += " (" + strings.Join(diffs, "/") + ")"
		}
		if k.cat != "" {
			line += " [" + k.cat + "]"
		}
		if dropWorldBoss[k] {
			line += " (world boss)"
		}
		if ev := dropEvents[k]; ev != "" {
			line += " " + sel(warnStyle).Render("("+ev+" only)")
		}
		drops[dropOpen[k]] = append(drops[dropOpen[k]], line)
	}
	for _, open := range []bool{true, false} {
		if len(drops[open]) > 0 {
			parts = append([]srcPart{{"Drops: " + strings.Join(drops[open], "; "), open, "", ""}}, parts...)
		}
	}
	// open routes first, each styled by its own reachability
	sort.SliceStable(parts, func(a, b int) bool { return parts[a].open && !parts[b].open })
	// same-named quests from different givers (Argent Dawn Commission
	// x3) render as identical parts: say it once
	seen := map[srcPart]bool{}
	uniq := parts[:0]
	for _, p := range parts {
		if !seen[p] {
			seen[p] = true
			uniq = append(uniq, p)
		}
	}
	parts = uniq
	rendered := make([]string, len(parts))
	for i, p := range parts {
		rendered[i] = m.renderAbbrevs(p.text, sel(m.srcStyle(p.open)), p.open, sel)
		if p.tag != "" {
			st := m.srcStyle(p.open)
			if p.open {
				st = tierStyle // the version's accent color
			}
			rendered[i] += sel(st).Render(p.tag)
		}
		if p.tail != "" {
			rendered[i] += m.renderAbbrevs(p.tail, sel(m.srcStyle(p.open)), p.open, sel)
		}
	}
	text := strings.Join(rendered, sel(dimStyle).Render(" | "))
	if db.IsBoE(it.ID) && len(dropOrder) > 0 {
		// the flag matters where the item DROPS (AH-buyable instead of
		// farmed); crafted/vendor gear being BoE is the default, not news
		text = sel(warnStyle).Render("BoE ") + text
	}
	return text
}

// srcStyle picks the source-route text style: routes the filter has
// closed render faint instead of dim.
func (m Model) srcStyle(open bool) lipgloss.Style {
	if open {
		return dimStyle
	}
	return faintStyle
}

// recipeText is the crafted line's suffix naming how the recipe is got
// — "recipe sold" (go to the vendor) vs "recipe drop" (farm it or buy
// it on the auction house): " · recipe sold: Braeg Stoutbeard". Empty
// when the route is unknown.
func (m Model) recipeText(spellID int) string {
	faction := ""
	if m.char != nil {
		faction = m.char.Faction
	}
	r, ok := m.game.DB.RecipeFor(spellID, faction)
	if !ok || !m.game.DB.RecipeLearnable(spellID, faction) {
		return ""
	}
	var how, where string
	switch r.Kind {
	case db.RecipeTrainer:
		return " (trainer)"
	case db.RecipeVendor:
		how, where = "recipe sold", r.Where
	case db.RecipeDrop:
		how, where = "recipe drop", r.Where
	case db.RecipeWorldDrop:
		how, where = "recipe drop", "world"
	case db.RecipeQuest:
		how, where = "recipe quest", r.Where
	case db.RecipeUnknown:
		return " · recipe unknown"
	}
	if where == "" {
		return ""
	}
	if r.Rep != "" {
		where += " (" + r.Rep + ")"
	}
	// a legend abbreviation names the vendor kind itself ("ACA · 30
	// Favor"): no "recipe sold" before it
	for _, a := range m.game.DB.Rules.Legend {
		if strings.HasPrefix(where, a.Short+" ") {
			return " · " + where
		}
	}
	return " · " + how + ": " + where
}

// vendorText renders a sold-by source. Emblem gear gets its exact badge
// price from the generated 3.3.5 table; named vendors their name. No
// guessing: an item not in the emblem table is not badge-buyable.
func (m Model) vendorText(it *db.Item, sb *db.SoldBySource) string {
	if base := m.game.DB.UpgradeBase(it.ID); base != nil {
		return "Upgrade: " + base.Name + " + Crystallized Firestone"
	}
	if m.game.DB.RaidGated(it) {
		return "Vendor: raid tokens"
	}
	if p, ok := m.game.DB.PriceFor(it.ID, m.filter.ServerPhase); ok {
		return fmt.Sprintf("Vendor: %d× %s", p.Count, p.Emblem)
	}
	line := "Vendor"
	if sb.NPCName != "" {
		line = "Sold by: " + sb.NPCName
	}
	if z := m.game.DB.Zones[sb.ZoneID]; z != "" {
		line += " · " + z
	}
	return line
}

// selfCraftable reports whether the character can craft the item
// itself: it has the profession at the recipe's learn level. known is
// false for versions without craft-skill data (and for the few crafts
// it lacks), where no claim is made either way.
func (m Model) selfCraftable(c *db.CraftedSource) (self, known bool) {
	if m.game.DB.CraftSkills == nil || m.char == nil {
		return false, false
	}
	if !m.game.DB.RecipeLearnable(c.SpellID, m.char.Faction) {
		return false, true // the other side's reputation recipe
	}
	need, ok := m.game.DB.CraftSkills[c.SpellID]
	if !ok {
		return false, true // no learn level: assume a crafter's job
	}
	for _, p := range m.char.Professions {
		if p.Name == c.ProfessionName() && p.Skill >= need.Learn {
			return true, true
		}
	}
	return false, true
}

// craftObtainable gates a crafted route: a craft binding on pickup
// never reaches the auction house, so only its own crafter gets it.
func (m Model) craftObtainable(it *db.Item, c *db.CraftedSource) bool {
	if !m.game.DB.BoPCrafts[it.ID] {
		return true
	}
	self, known := m.selfCraftable(c)
	return self || !known
}

// renderAbbrevs renders text in base, with the version's legend
// marked abbreviations ("ACA") in gold on open routes, so the camp stands out
// on a long source line; closed routes stay uniformly faint.
func (m Model) renderAbbrevs(text string, base lipgloss.Style, open bool, sel func(lipgloss.Style) lipgloss.Style) string {
	if !open || m.game.DB == nil || len(m.game.DB.Rules.Legend) == 0 {
		return base.Render(text)
	}
	var b strings.Builder
	for text != "" {
		at, short := -1, ""
		for _, a := range m.game.DB.Rules.Legend {
			if !a.Marked {
				continue
			}
			if loc := legendRe(a.Short).FindStringIndex(text); loc != nil && (at < 0 || loc[0] < at) {
				at, short = loc[0], a.Short
			}
		}
		if at < 0 {
			b.WriteString(base.Render(text))
			break
		}
		if at > 0 {
			b.WriteString(base.Render(text[:at]))
		}
		b.WriteString(sel(goldStyle).Render(short))
		text = text[at+len(short):]
	}
	return b.String()
}
