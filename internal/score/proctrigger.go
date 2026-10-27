package score

import (
	"strings"

	"github.com/vaclav-dvorak/twolk/internal/db"
)

// Proc trigger availability. A proc that only fires from combat events
// a build never generates is dead weight: Schnottz's Medallion of
// Command procs from MELEE attacks only, so a hunter — whose rotation
// is 100% ranged — never sees the buff, and the uptime average must be
// zero, not "as if a melee wore it". Same story for ranged-only procs
// on a rogue, heal-only procs on a DPS, spell-only procs on a warrior.
//
// The trigger comes from the WoWSims sim sources (Effect.ProcMask /
// Callback, both versions). The gate is deliberately binary: either
// the build's rotation exercises the trigger family (full uptime
// model applies) or it cannot fire at all (zero). Anything we cannot
// read — empty mask, templated sim code, unknown token — stays
// ungated; only a recognized, definitely-unreachable trigger zeroes.

// procEvents is the set of proc-trigger event families a build's
// rotation actually generates.
type procEvents struct {
	melee    bool // melee white hits / specials
	ranged   bool // ranged autos / shots
	spellDmg bool // damaging spell hits
	heal     bool // healing casts
}

// procEvents maps the build to its trigger families. Deliberately
// coarse and rotation-practical, not theoretically-possible: a hunter
// CAN raptor-strike and a resto druid CAN wrath, but neither happens
// in play, which is exactly the fiction the gate exists to kill.
// Hybrid melee (DK, enhancement, ret/prot paladin) keeps spellDmg —
// their rotations land real spell damage.
func (p *Profile) procEvents() procEvents {
	switch p.Class {
	case db.ClassWarrior, db.ClassRogue:
		return procEvents{melee: true}
	case db.ClassHunter:
		return procEvents{ranged: true}
	case db.ClassDeathKnight:
		return procEvents{melee: true, spellDmg: true}
	case db.ClassMage, db.ClassWarlock:
		return procEvents{spellDmg: true}
	case db.ClassPaladin:
		if p.Tree == "Holy" {
			return procEvents{heal: true}
		}
		return procEvents{melee: true, spellDmg: true}
	case db.ClassPriest:
		if p.Tree == "Shadow" {
			return procEvents{spellDmg: true}
		}
		return procEvents{heal: true}
	case db.ClassShaman:
		switch p.Tree {
		case "Enhancement":
			return procEvents{melee: true, spellDmg: true}
		case "Restoration":
			return procEvents{heal: true}
		}
		return procEvents{spellDmg: true}
	case db.ClassDruid:
		switch p.Tree {
		case "Feral Combat", "Feral":
			return procEvents{melee: true}
		case "Restoration":
			return procEvents{heal: true}
		}
		return procEvents{spellDmg: true}
	}
	// unknown class: gate nothing
	return procEvents{melee: true, ranged: true, spellDmg: true, heal: true}
}

// CanTrigger reports whether the build's rotation can fire the effect
// at all. Non-procs (on-use, curated averages) and procs without
// readable trigger info always pass.
func (p *Profile) CanTrigger(e *db.Effect) bool {
	if e.Kind != db.EffectProc || e.ProcMask == "" {
		return true
	}
	// "Taken" callbacks fire on INCOMING hits — there the mask describes
	// the attacker's action, not the wearer's rotation.
	if strings.Contains(e.Callback, "Taken") {
		return true
	}
	if p.NoPeriodicDamage && strings.Contains(e.Callback, "OnPeriodicDamageDealt") &&
		!strings.Contains(e.Callback, "OnSpellHitDealt") {
		return false
	}
	need, known := maskEvents(e.ProcMask)
	if !known {
		return true
	}
	have := p.procEvents()
	return need.melee && have.melee || need.ranged && have.ranged ||
		need.spellDmg && have.spellDmg || need.heal && have.heal
}

// CanTriggerEnchant reports whether the profile's rotation fires a
// curated proc enchant's trigger (db.Enchant.Trigger): a hunter's melee
// stat-stick never procs Berserking, a warrior never procs Black Magic.
func (p *Profile) CanTriggerEnchant(e *db.Enchant) bool {
	have := p.procEvents()
	switch e.Trigger {
	case "melee":
		return have.melee
	case "spell":
		return have.spellDmg
	}
	return true
}

// TriggerLabel compresses the effect's proc trigger into one plain
// word for UI copy — "melee", "ranged", "melee/ranged", "spells",
// "heals", "spells/heals", "any hit" — or "" when the trigger is
// absent or unreadable.
func TriggerLabel(e *db.Effect) string {
	if e.Kind != db.EffectProc {
		return ""
	}
	ev, known := maskEvents(e.ProcMask)
	if !known {
		return ""
	}
	switch {
	case ev.melee && ev.ranged && ev.spellDmg:
		return "any hit"
	case ev.melee && ev.ranged:
		return "melee/ranged"
	case ev.melee:
		return "melee"
	case ev.ranged:
		return "ranged"
	case ev.spellDmg && ev.heal:
		return "spells/heals"
	case ev.spellDmg:
		return "spells"
	case ev.heal:
		return "heals"
	}
	return ""
}

// maskEvents folds a full WoWSims ProcMask expression into the event
// families it can fire from, reporting whether ANY token was readable.
// Pure modifier tokens ("Proc") and unrecognized ones don't count, so
// a mask we cannot read never zeroes an effect.
func maskEvents(mask string) (procEvents, bool) {
	var ev procEvents
	known := false
	for _, tok := range strings.Split(mask, "|") {
		t, ok := tokenEvents(strings.TrimSpace(tok))
		if !ok {
			continue
		}
		known = true
		ev.melee = ev.melee || t.melee
		ev.ranged = ev.ranged || t.ranged
		ev.spellDmg = ev.spellDmg || t.spellDmg
		ev.heal = ev.heal || t.heal
	}
	return ev, known
}

// tokenEvents maps one ProcMask token onto event families.
func tokenEvents(tok string) (procEvents, bool) {
	switch tok {
	case "Melee", "MeleeProc", "MeleeOrMeleeProc", "MeleeSpecial", "MeleeWhiteHit":
		return procEvents{melee: true}, true
	case "MeleeOrRanged", "MeleeOrRangedSpecial":
		return procEvents{melee: true, ranged: true}, true
	case "Ranged", "RangedProc", "RangedSpecial", "RangedOrRangedProc":
		return procEvents{ranged: true}, true
	case "SpellDamage", "SpellDamageProc", "SpellProc", "SpellOrProc", "SpellOrSpellProc":
		return procEvents{spellDmg: true}, true
	case "SpellHealing", "SpellHealingProc":
		return procEvents{heal: true}, true
	case "Direct": // any direct hit: melee, ranged or spell
		return procEvents{melee: true, ranged: true, spellDmg: true}, true
	}
	return procEvents{}, false
}
