package tui

import (
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/vaclav-dvorak/twolk/internal/config"
	"github.com/vaclav-dvorak/twolk/internal/db"
	"github.com/vaclav-dvorak/twolk/internal/game"
)

// The filter modal (f) and the item-admission rules it drives:
// phase ceiling and acquisition-route toggles, saved per realm.

// defaultFilter opens every acquisition route except PvP. ServerPhase
// stays 0 (the phase count is the game module's to say) and is set to
// the version's endgame when the module finishes loading.
func defaultFilter() config.UpgradeFilter {
	return config.UpgradeFilter{
		Raids: true, RaidBoE: true, Dungeon: true,
		Vendor: true, Crafted: true, Other: true,
	}
}

// filterKey identifies the active REALM's saved filter. Free-server
// realms progress independently — the same server can run one realm at
// P2 and another at endgame — so the filter is realm-local state, not
// game- or server-local. Without an active character the game slug
// alone keys a provisional filter.
func (m Model) filterKey() string {
	if i := m.cfg.ActiveChar; i >= 0 && i < len(m.cfg.Chars) {
		c := m.cfg.Chars[i]
		return c.Game + "/" + c.Server + "/" + strings.ToLower(c.Realm)
	}
	return m.game.Slug
}

// currencyOpen reports whether a currency exists at the filter phase
// (game.CurrencyPhase).
func (m Model) currencyOpen(e db.Emblem) bool {
	return m.game.CurrencyOpen(e, m.filter.ServerPhase)
}

// protocolFor is the item's heroic-plus protocol route when its phase
// is open (Inferno with Firelands, Twilight with Dragon Soul).
func (m Model) protocolFor(itemID int) (db.ProtocolPrice, bool) {
	pp, ok := m.game.DB.ProtocolFor(itemID)
	if !ok || m.filter.ServerPhase < pp.Phase || !m.currencyOpen(pp.Emblem) {
		return db.ProtocolPrice{}, false
	}
	return pp, true
}

// unlockOpen reports whether the realm has reached an in-phase unlock
// (the filter lists the ones it hasn't).
func (m Model) unlockOpen(slug string) bool {
	return !slices.Contains(m.filter.Locked, slug)
}

// zoneLocked reports whether vendors in the area sit behind an unlock
// the realm hasn't reached yet.
func (m Model) zoneLocked(zone int) bool {
	for _, u := range m.game.Unlocks {
		if zone != 0 && u.Zone == zone && !m.unlockOpen(u.Slug) {
			return true
		}
	}
	return false
}

// recipeLocked reports whether a craft's recipe is sold only behind a
// closed unlock — nobody on the realm can make the item yet.
func (m Model) recipeLocked(spellID int) bool {
	r, ok := m.game.DB.Recipe(spellID)
	return ok && r.Kind == db.RecipeVendor && m.zoneLocked(r.Zone)
}

// assumeEnchants reports whether candidates are scored with their best
// enchant applied. By default only at max level: nobody enchants
// leveling gear they replace in a few levels, so a bare candidate
// against the worn (really enchanted) piece is the honest comparison.
// The upgrade browser's e key overrides it for the session.
func (m Model) assumeEnchants() bool {
	if m.enchAssume != nil {
		return *m.enchAssume
	}
	return !(m.game.Leveling && m.char != nil && m.char.Level < m.game.PlayerLevel)
}

// enchantGate is the scorer's enchant predicate for the active filter:
// item-based enchants (scopes) whose recipe sits behind a closed unlock
// are off the realm, and a leveling character only gets enchants of its
// level; with the enchant assumption off, none pass. nil when nothing
// gates.
func (m Model) enchantGate() func(*db.Enchant) bool {
	if m.game.DB == nil {
		return nil
	}
	if !m.assumeEnchants() {
		return func(*db.Enchant) bool { return false }
	}
	level := 0
	if m.game.Leveling && m.char != nil && m.char.Level < m.game.PlayerLevel {
		level = m.char.Level
	}
	if len(m.filter.Locked) == 0 && level == 0 {
		return nil
	}
	mc := m // the gate must not follow later filter edits
	mc.filter.Locked = slices.Clone(m.filter.Locked)
	maxLevel := m.game.PlayerLevel
	return func(e *db.Enchant) bool {
		if level > 0 {
			// no level upstream: endgame content (Forever's Warden's
			// Signets), off the table until max level
			need := e.ReqLevel
			if need == 0 {
				need = maxLevel
			}
			if need > level {
				return false
			}
		}
		r, ok := mc.game.DB.EnchantRecipe(e)
		return !ok || r.Kind != db.RecipeVendor || !mc.zoneLocked(r.Zone)
	}
}

// loadFilter restores the active realm's saved upgrade filter; absent
// config means every route open at the version's endgame phase.
func (m *Model) loadFilter() {
	if f, ok := m.cfg.Filters[m.filterKey()]; ok {
		m.filter = f
		m.filter.ServerPhase = m.game.ClampPhase(f.ServerPhase)
		return
	}
	m.filter = defaultFilter()
	m.filter.ServerPhase = m.game.MaxPhase()
}

// allowItem is the upgrade-candidate predicate for the active filter.
// ServerPhase is a hard content ceiling on EVERY item — later-phase
// raids, dungeons, emblem tiers and craft recipes don't exist yet on a
// progressively-unlocked server. Within open content, an item passes if
// any of its acquisition routes is enabled.
func (m Model) allowItem(it *db.Item) bool {
	f := m.filter
	if it.ILvl < m.game.MinUsefulILvl {
		return false
	}
	// a leveling character only gets suggestions it can wear now
	if m.game.Leveling && m.char != nil && equipLevel(it) > m.char.Level {
		return false
	}
	if m.game.ClampPhase(it.Phase) > f.ServerPhase {
		return false
	}
	// resilience marks PvP-purpose gear. NOT all of it is PvP-acquired
	// (~75 crafted resilience pieces exist — Swiftarrow sets, JC rings);
	// they are hidden with the PvP toggle regardless, as stat-noise for
	// a PvE planner. The currency table + name families catch the
	// inverse case: PvP-acquired items without resilience.
	if (it.Stats[db.Resilience] > 0 || m.game.DB.PvPItem(it)) && !f.PvP {
		return false
	}
	// faction mirror-loot: hide the other faction's copy. MUST precede
	// the raid-gated early return — T9 trophy tier is faction-mirrored,
	// and letting both mirrors through double-counts the 2pc bonus in
	// the BiS solver (2pc Battlegear + 2pc Pursuit on one character).
	if m.char != nil && !it.UsableByFaction(m.char.Faction) {
		return false
	}
	// heroic-plus protocol runs put raid loot on the DUNGEON route once
	// the protocol's phase opens (Cataclysm Classic's Elemental Rune
	// dungeons) — checked before the raid gates so a raids-off filter
	// keeps the catch-up path visible
	if _, ok := m.protocolFor(it.ID); ok && f.Dungeon {
		return true
	}
	// heroic Firelands upgrades: the normal item (its own routes) plus a
	// Crystallized Firestone
	if base := m.game.DB.UpgradeBase(it.ID); base != nil {
		return m.firestoneOpen() && m.allowItem(base)
	}
	// raid content in disguise: tier priced in raid tokens, Sanctified
	// mark upgrades, and raid-quest rewards
	if m.game.DB.RaidGated(it) {
		return f.Raids
	}
	if len(it.Sources) == 0 {
		return f.Other
	}
	// only the character's side of faction-split routes (an item whose
	// every route belongs to the other faction never passes)
	for _, s := range m.sideSources(it) {
		switch {
		case m.game.DB.IsRaidDrop(s):
			// BoE raid drops are AH-buyable: their own toggle
			if db.IsBoE(it.ID) {
				if f.RaidBoE {
					return true
				}
			} else if f.Raids {
				return true
			}
		case s.Drop != nil && s.Drop.OtherName == db.WorldDrop:
			if f.Other { // open-world BoE, not instance loot
				return true
			}
		case s.Drop != nil: // 5-man dungeon (normal/heroic)
			if f.Dungeon {
				return true
			}
		case s.SoldBy != nil:
			if f.Vendor && m.currencyRouteOpen(it) && !m.zoneLocked(s.SoldBy.ZoneID) {
				return true
			}
		case s.Rep != nil:
			// reputation quartermasters ride the vendor route, exactly
			// like the wotlk data attributes them
			if f.Vendor {
				return true
			}
		case s.Crafted != nil:
			if f.Crafted && !m.recipeLocked(s.Crafted.SpellID) && m.craftObtainable(it, s.Crafted) {
				return true
			}
		case s.Quest != nil:
			if f.Other {
				return true
			}
		}
	}
	return false
}

// firestoneOpen reports whether a Crystallized Firestone is within
// reach: it drops from heroic Firelands bosses, and Protocol Twilight's
// vendor sells it for Obsidian Fragments (not Inferno's).
func (m Model) firestoneOpen() bool {
	return m.filter.Raids || (m.filter.Dungeon && m.currencyOpen(db.EmblemObsidian))
}

// currencyRouteOpen reports whether an item's vendor route exists at
// the filter phase: gold vendors always do; badge/point-priced items
// need their currency shipped (game.CurrencyPhase) — the item's
// content-phase tag tracks the tier, not the vendor, so a Frost-priced
// piece must not surface through a vendor that isn't on the server yet.
func (m Model) currencyRouteOpen(it *db.Item) bool {
	p, ok := m.game.DB.PriceFor(it.ID, m.filter.ServerPhase)
	return !ok || m.currencyOpen(p.Emblem)
}

// filterRows counts the filter modal's rows: one phase radio per game
// phase, the acquisition toggles, then the game's in-phase unlocks.
func (m Model) filterRows() int {
	return m.game.MaxPhase() + len(filterToggleLabels) + len(m.game.Unlocks)
}

// filterUnlock returns the unlock behind modal row i, nil for the
// phase and acquisition rows.
func (m Model) filterUnlock(i int) *game.Unlock {
	if j := i - m.game.MaxPhase() - len(filterToggleLabels); j >= 0 && j < len(m.game.Unlocks) {
		return &m.game.Unlocks[j]
	}
	return nil
}

// filterToggle returns the toggle behind modal row i (the acquisition
// rows following the phase radios).
func (m Model) filterToggle(f *config.UpgradeFilter, i int) *bool {
	switch i - m.game.MaxPhase() {
	case 0:
		return &f.Raids
	case 1:
		return &f.RaidBoE
	case 2:
		return &f.Dungeon
	case 3:
		return &f.Vendor
	case 4:
		return &f.Crafted
	case 5:
		return &f.Other
	}
	return &f.PvP
}

// emblemVendorLabel is the vendor row's label; versions without emblem
// currencies (Forever) show plain "Vendor gear".
const emblemVendorLabel = "Emblem & vendor gear"

// hasEmblems reports whether the version prices gear in emblem
// currencies; without any, the emblem advisor (p) doesn't exist.
func (m Model) hasEmblems() bool { return len(m.game.Currencies) > 0 }

// filterToggleLabels labels the acquisition rows.
var filterToggleLabels = []string{
	"Raid drops (need a raid group)",
	"BoE raid drops (buyable on AH)",
	"Dungeon drops (5-man)",
	emblemVendorLabel,
	"Crafted items",
	"Quests & other sources",
	pvpLabel,
}

// pvpLabel is the PvP row's label; versions without resilience (Forever)
// say where their PvP gear comes from instead.
const pvpLabel = "PvP gear (resilience items)"

func (m Model) updateFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "f":
		if m.cfg.Filters == nil {
			m.cfg.Filters = map[string]config.UpgradeFilter{}
		}
		m.cfg.Filters[m.filterKey()] = m.filter
		if err := m.cfg.Save(); err != nil {
			m.status = "saving filter failed: " + err.Error()
		}
		if m.scorer != nil {
			// the phase ceiling and unlocks reach enchant picks through
			// the scorer
			m.scorer.SetServerPhase(m.filter.ServerPhase)
			m.scorer.SetEnchantGate(m.enchantGate())
		}
		switch {
		case m.fromUps:
			m.mode = modeUpgrades
			m.recomputeUpgrades()
		case m.fromBis:
			m.openBis() // re-solve under the new filter
		default:
			m.mode = modeNormal
		}
		m.recomputeTips()
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		if m.fCursor > 0 {
			m.fCursor--
		}
		return m, nil
	case "down", "j":
		if m.fCursor < m.filterRows()-1 {
			m.fCursor++
		}
		return m, nil
	case " ", "x", "enter":
		if u := m.filterUnlock(m.fCursor); u != nil {
			// copy, never edit in place: the slice is shared with the
			// saved config value
			if m.unlockOpen(u.Slug) {
				m.filter.Locked = append(slices.Clone(m.filter.Locked), u.Slug)
			} else {
				m.filter.Locked = slices.DeleteFunc(slices.Clone(m.filter.Locked),
					func(s string) bool { return s == u.Slug })
			}
		} else if m.fCursor < m.game.MaxPhase() {
			m.filter.ServerPhase = m.fCursor + 1
		} else {
			flag := m.filterToggle(&m.filter, m.fCursor)
			*flag = !*flag
		}
		return m, nil
	}
	return m, nil
}

// filterSummary describes the active filter in one dim line.
func (m Model) filterSummary() string {
	s := "≤ " + m.game.Phases[m.game.ClampPhase(m.filter.ServerPhase)-1].Short
	for _, x := range []struct {
		on    bool
		label string
	}{
		{m.filter.Raids, "raids"},
		{m.filter.RaidBoE, "BoE"},
		{m.filter.Dungeon, "dungeons"},
		{m.filter.Vendor, "vendors"},
		{m.filter.Crafted, "crafted"},
		{m.filter.Other, "quests"},
	} {
		if !x.on {
			s += " · no " + x.label
		}
	}
	if m.filter.PvP {
		s += " · incl. PvP"
	}
	for _, u := range m.game.Unlocks {
		if u.Phase <= m.filter.ServerPhase && !m.unlockOpen(u.Slug) {
			s += " · no " + u.Short
		}
	}
	return s
}

func (m Model) viewFilter() string {
	header := titleStyle.Render("Filter upgrade proposals")
	f := m.filter
	cursor := func(i int) string {
		if i == m.fCursor {
			return helpKey.Render("▸ ")
		}
		return "  "
	}
	tiers := "dungeons, emblem tiers and craft recipes):"
	if !m.hasEmblems() {
		tiers = "dungeons and craft recipes):"
	}
	lines := []string{dimStyle.Render("Server phase — content open up to (hides later raids,"),
		dimStyle.Render(tiers)}
	for i, ph := range m.game.Phases {
		radio := dimStyle.Render("( ) ")
		if f.ServerPhase == i+1 {
			radio = okStyle.Render("(•) ")
		}
		lines = append(lines, cursor(i)+radio+ph.Label)
	}
	lines = append(lines, "", dimStyle.Render("Sources you can access:"))
	for i, label := range filterToggleLabels {
		if label == emblemVendorLabel && !m.hasEmblems() {
			label = "Vendor gear"
		}
		if label == pvpLabel && !m.game.Rules.HasStat(db.Resilience) {
			label = "PvP gear (honor & battlegrounds)" // no resilience before TBC
		}
		box := dimStyle.Render("[ ] ")
		if *m.filterToggle(&f, m.game.MaxPhase()+i) {
			box = okStyle.Render("[x] ")
		}
		lines = append(lines, cursor(m.game.MaxPhase()+i)+box+label)
	}
	if len(m.game.Unlocks) > 0 {
		lines = append(lines, "", dimStyle.Render("Unlocked on your realm:"))
		base := m.game.MaxPhase() + len(filterToggleLabels)
		for i, u := range m.game.Unlocks {
			box := dimStyle.Render("[ ] ")
			if m.unlockOpen(u.Slug) {
				box = okStyle.Render("[x] ")
			}
			lines = append(lines, cursor(base+i)+box+u.Label)
		}
	}
	footer := helpLine("space/enter", "select/toggle", "esc", "done")
	modal := modalStyle.Render(lipgloss.JoinVertical(lipgloss.Left,
		header, "", strings.Join(lines, "\n"), "", footer))
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, modal)
}

// equipLevel is the character level an item needs. Quest rewards (and
// a few new Forever items) carry no required level upstream; their
// item level stands in — quests hand out gear a few levels above their
// pickup level, so the fallback hides a reward a little too long
// rather than suggesting one the character can't get yet.
func equipLevel(it *db.Item) int {
	// Forever's own quest rewards claim level 1 at any item level (an
	// ilvl-55 bandana from a level-55 quest): their quest is the gate,
	// so a level-1 claim on real gear counts as no claim
	if it.ReqLevel > 1 || (it.ReqLevel == 1 && it.ILvl <= 10) {
		return it.ReqLevel
	}
	return it.ILvl
}

// sideSources is the item's routes the character's faction can take:
// the other side's quest givers, vendors and quartermasters drop out.
func (m Model) sideSources(it *db.Item) []db.Source {
	if m.char == nil {
		return it.Sources
	}
	var out []db.Source
	for _, s := range it.Sources {
		if s.SideOpen(m.char.Faction) {
			out = append(out, s)
		}
	}
	return out
}
