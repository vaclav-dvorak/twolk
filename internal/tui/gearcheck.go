package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/vaclav-dvorak/twolk/internal/db"
	"github.com/vaclav-dvorak/twolk/internal/score"
)

// Gear the planner cannot see.
//
// An armory export carries item IDs; the item database carries the
// endgame pool and nothing else. Everything else a character happens to
// wear — a fishing pole or a tournament lance left in a hand at logout,
// leveling gear, a server's own items — resolves to no item at all and
// is dropped from the gear list. The set then scores as if that slot
// were bare, which is a lie in both directions: the totals sit under
// the truth, and the advisors chase caps the character has already met.
// (Session 39: a hunter who logged out fishing scored 883 hit against
// the 961 cap while the game showed 968, and the advisor offered to
// spend two reforges fixing it.)
//
// So when a slot's contents cannot be scored, twolk does not score at
// all: gearGaps names those slots, the warning modal explains, and the
// scorer stays nil until the character is re-imported wearing its real
// gear.

// gearGap is one equipment slot whose contents the planner cannot
// score.
type gearGap struct {
	slot db.Slot
	id   int    // the worn item's id; 0 when the export never named it
	name string // db.NonGear's name, or the export's own; "" if neither
	junk bool   // a known non-gear item (db.NonGear), not missing data
}

// what describes the offending item as precisely as the data allows:
// db.NonGear names the tools and props by id, plain pastes carry the
// armory's own names, API imports carry neither.
func (g gearGap) what() string {
	switch {
	case g.name != "" && g.id != 0:
		return fmt.Sprintf("%s (item %d)", g.name, g.id)
	case g.name != "":
		return g.name
	case g.id != 0:
		return fmt.Sprintf("item %d", g.id)
	}
	return "an item the export does not name"
}

// note describes the gap for a gear-list row: a named tool or prop
// speaks for itself, anything else is gear below the scored pool.
func (g gearGap) note() string {
	switch {
	case g.junk:
		return g.name
	case g.name != "":
		return g.name + " below the scored item pool"
	}
	return "worn item below the scored item pool"
}

// gearGaps lists the equipment the planner cannot score.
//
// Two kinds. A known non-gear item (db.NonGear — fishing poles,
// jousting lances, profession tools, holiday props) is a gap on sight,
// resolvable or not: the few the item pool does carry would otherwise
// score as the junk they are. The rest is gear the database has never
// heard of.
//
// Items the export files under a real gear slot report themselves. A
// plain paste claims no slots at all, so its unscorable pieces arrive
// unattributed — and cosmetics (shirt, tabard) are unresolvable on
// every character, so their mere presence proves nothing. What is
// still unambiguous there: a character wearing a COMPLETE kit, holding
// something the database does not know, and showing no weapon the
// database could resolve. Weapon rows are the one part of the layout
// StandardSlots does not describe, so nothing else reports them — and
// a full set without a weapon is a logout snapshot, not a build.
func (m Model) gearGaps() []gearGap {
	if m.game.DB == nil || m.char == nil {
		return nil
	}
	var gaps []gearGap
	weapons, unknown := 0, 0
	filled := map[db.Slot]int{}
	for _, eq := range m.char.Equipment {
		it, resolved := m.game.DB.Items[eq.ItemID]
		if name, junk := db.NonGear[eq.ItemID]; junk {
			gaps = append(gaps, gearGap{
				slot: nonGearSlot(eq.Slot, it, resolved),
				id:   eq.ItemID, name: name, junk: true,
			})
			continue
		}
		if resolved {
			filled[it.Slot]++
			if it.Slot == db.SlotWeapon {
				weapons++
			}
			continue
		}
		unknown++
		if eq.Slot != db.SlotUnknown {
			gaps = append(gaps, gearGap{slot: eq.Slot, id: eq.ItemID, name: eq.Name})
		}
	}
	if weapons == 0 && unknown > 0 && fullKit(filled) && !hasGap(gaps, db.SlotWeapon) {
		gaps = append(gaps, gearGap{slot: db.SlotWeapon})
	}
	return gaps
}

// fullKit reports whether every standard slot is filled by a scorable
// item — the state that turns a missing weapon from "not geared yet"
// into "the weapon is one of the items we could not resolve".
func fullKit(filled map[db.Slot]int) bool {
	for _, std := range score.StandardSlots {
		if filled[std.Slot] < std.N {
			return false
		}
	}
	return true
}

// nonGearSlot names the slot a tool or prop was worn in: the export's
// claim first (fetch imports), then the item pool's own slot for the
// few it carries, and failing both the weapon slot — every db.NonGear
// entry is a weapon-class item, and the weapon slot is where they get
// left behind.
func nonGearSlot(claim db.Slot, it *db.Item, resolved bool) db.Slot {
	switch {
	case claim != db.SlotUnknown:
		return claim
	case resolved && it.Slot != db.SlotUnknown:
		return it.Slot
	}
	return db.SlotWeapon
}

func hasGap(gaps []gearGap, s db.Slot) bool {
	for _, g := range gaps {
		if g.slot == s {
			return true
		}
	}
	return false
}

// gapSlots names the affected slots for the one-line header note.
func gapSlots(gaps []gearGap) string {
	var out []string
	for _, g := range gaps {
		out = append(out, g.slot.String())
	}
	return strings.Join(out, ", ")
}

// gapSignature identifies a set of gaps, so the warning modal opens
// once per problem instead of on every scorer rebuild (a weight edit
// rebuilds too).
func gapSignature(gaps []gearGap) string {
	var b strings.Builder
	for _, g := range gaps {
		fmt.Fprintf(&b, "%d:%d;", g.slot, g.id)
	}
	return b.String()
}

// checkGear refreshes the gap list and arms the warning modal whenever
// the problem itself changed.
func (m *Model) checkGear() {
	m.gaps = m.gearGaps()
	if sig := gapSignature(m.gaps); sig != m.gapsSeen {
		m.gapsSeen = sig
		m.gapWarn = len(m.gaps) > 0
	}
}

// viewGearWarning is the full-screen warning: what cannot be scored,
// and why nothing is scored because of it.
func (m Model) viewGearWarning() string {
	name := "this character"
	if m.char != nil {
		name = m.char.Name
	}
	head := errStyle.Bold(true).Render("⚠  " + name + " wears gear twolk cannot score")

	var rows []string
	for _, g := range m.gaps {
		rows = append(rows, "  "+warnStyle.Render(fmt.Sprintf("%-9s", g.slot))+
			"  "+dimStyle.Render(g.what()))
	}

	slots := "Those slots count"
	if len(m.gaps) == 1 {
		slots = "That slot counts"
	}
	body := []string{
		errStyle.Render("Scoring is off.") + dimStyle.Render(
			" "+slots+" as empty, so every total,"),
		dimStyle.Render("cap and suggestion would be wrong."),
		"",
		dimStyle.Render("Equip the gear you want scored, log out, then ") +
			helpKey.Render("r") + dimStyle.Render(" to refresh."),
	}

	modal := modalStyle.Render(lipgloss.JoinVertical(lipgloss.Left,
		head, "",
		strings.Join(rows, "\n"), "",
		strings.Join(body, "\n"), "",
		helpLine("any key", "dismiss")))
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, modal)
}
