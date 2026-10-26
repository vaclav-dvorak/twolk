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
		return f.Crafted && !m.recipeLocked(s.Crafted.SpellID)
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
func (m Model) sourceText(it *db.Item) string {
	if len(it.Sources) == 0 {
		if m.game.DB.RaidGated(it) {
			// Sanctified upgrades et al. carry no upstream source
			return m.srcStyle(m.filter.Raids).Render("Vendor: raid tokens")
		}
		if m.game.DB.PvPItem(it) {
			return m.srcStyle(m.filter.PvP).Render("PvP vendor: honor / marks")
		}
		return dimStyle.Render("source unknown")
	}
	type dropKey struct {
		who  string
		zone int
		cat  string
	}
	type srcPart struct {
		text string
		open bool
	}
	dropDiffs := map[dropKey][]string{}
	dropEvents := map[dropKey]string{}
	dropWorldBoss := map[dropKey]bool{}
	dropOpen := map[dropKey]bool{}
	var dropOrder []dropKey
	var parts []srcPart

	for _, s := range it.Sources {
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
			parts = append(parts, srcPart{m.vendorText(it, s.SoldBy), m.routeOpen(it, s)})
		case s.Rep != nil:
			text := "Reputation: "
			if name := s.Rep.FactionName(); name != "" {
				text += name
			} else {
				text += fmt.Sprintf("faction %d", s.Rep.FactionID)
			}
			if st := s.Rep.Standing(); st != "" {
				text += " — " + st
			}
			parts = append(parts, srcPart{text, m.routeOpen(it, s)})
		case s.Quest != nil:
			open := m.routeOpen(it, s)
			switch {
			case s.Quest.Name != "" && m.game.DB.RaidGated(it):
				parts = append(parts, srcPart{"Raid quest: " + s.Quest.Name, open})
			case s.Quest.Name != "":
				parts = append(parts, srcPart{"Quest: " + s.Quest.Name, open})
			case m.game.DB.RaidGated(it): // nameless raid quest (cata curated)
				parts = append(parts, srcPart{"Raid quest reward", open})
			default:
				parts = append(parts, srcPart{"Quest reward", open})
			}
		case s.Crafted != nil:
			parts = append(parts, srcPart{"Crafted: " + s.Crafted.ProfessionName() +
				m.recipeText(s.Crafted.SpellID), m.routeOpen(it, s)})
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
				fmt.Sprintf("Vendor: %d× %s", pp.Count, pp.Emblem), m.filter.Dungeon})
		case pp.Count > 0:
			parts = append(parts, srcPart{
				fmt.Sprintf("%s: %d× %s", pp.Label, pp.Count, pp.Emblem), m.filter.Dungeon})
		case !pp.Drops:
			parts = append(parts, srcPart{
				fmt.Sprintf("%s: %ss", pp.Label, pp.Emblem), m.filter.Dungeon})
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
			line += " " + warnStyle.Render("("+ev+" only)")
		}
		drops[dropOpen[k]] = append(drops[dropOpen[k]], line)
	}
	for _, open := range []bool{true, false} {
		if len(drops[open]) > 0 {
			parts = append([]srcPart{{"Drops: " + strings.Join(drops[open], "; "), open}}, parts...)
		}
	}
	// open routes first, each styled by its own reachability
	sort.SliceStable(parts, func(a, b int) bool { return parts[a].open && !parts[b].open })
	rendered := make([]string, len(parts))
	for i, p := range parts {
		rendered[i] = m.srcStyle(p.open).Render(p.text)
	}
	text := strings.Join(rendered, dimStyle.Render(" | "))
	if db.IsBoE(it.ID) && len(dropOrder) > 0 {
		// the flag matters where the item DROPS (AH-buyable instead of
		// farmed); crafted/vendor gear being BoE is the default, not news
		text = warnStyle.Render("BoE ") + text
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
	r, ok := m.game.DB.Recipe(spellID)
	if !ok {
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
	}
	if where == "" {
		return ""
	}
	if r.Rep != "" {
		where += " (" + r.Rep + ")"
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
