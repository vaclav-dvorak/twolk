package cata

import (
	"time"

	"github.com/vaclav-dvorak/twolk/internal/db"
	"github.com/vaclav-dvorak/twolk/internal/score"
)

// Mage and warlock profiles. EVERY weight is MEASURED, not adopted: the
// pinned sim's published tables (ui/<class>/<spec>/presets.ts) reproduce
// only on P1 gear and only for some specs, and the realm is in
// Firelands. Direct DPS differences on each spec's own P3 preset
// (tools/eptool-cata, 50k iterations, seed 101, replicated on the
// disjoint seed range 1e6 — every ratio within 0.02), spell power = 1:
//
//   - The harness passes the validation gate session 37 failed for the
//     wotlk casters: on P1 gear, affliction reproduces its published
//     table to the decimal (crit 0.50/0.52, haste 0.62/0.58, mastery
//     0.38/0.38), and intellect matches every spec's table at both
//     phases (1.28-1.37 vs 1.25-1.33).
//   - Dragonwrath, Tarecgosa's Rest is swapped for the Runestaff of
//     Nordrassil — every P3 preset wields the legendary, which few
//     characters own and which inflates hit (~5% for the mages, as it
//     did for the balance druid in session 43).
//   - Hit is flat below the 17% cap (1742 rating; 830 and 1230 read the
//     same) and exactly zero past it, and enters as its slope against
//     spell power AT the cap — the trade the engine actually makes.
//   - Caster haste is a staircase (DoT ticks, GCD-bound casts): its
//     local 300-rating slope swings 1.5-4.2 DPS per point with where
//     the breakpoints fall. Haste, crit and mastery are each averaged
//     over the 1200 rating around the preset (-600..+600); that window
//     reproduces the published arcane and fire haste (0.70, 0.82).
//   - Spirit is worth nothing to either class (±0.02 DPS per point:
//     no spirit-to-hit talent, mana never limits the rotation).
//
// The published tables are off in both directions at P3: fire crit
// 0.62 against 0.89 (Ignite and Hot Streak grow with the crit already
// worn), demonology haste 2.75 against 0.72 (reproduces at no phase),
// warlock and fire hit 30-50% under-stated.
//
// EPPerDPS is 1 / the DPS of one spell power on the same gear; the
// ProcInterval is the preset's landed spell hits AND periodic ticks per
// second — cata spell-damage procs roll on both.
const (
	arcaneHitEP = 1.60
	fireHitEP   = 1.41
	affHitEP    = 0.90
	demoHitEP   = 1.37
	destroHitEP = 1.23
)

var (
	// mages and warlocks wield staves, or a one-hander with a held
	// off-hand; a wand in the ranged slot
	casterWeapons = []db.WeaponType{db.WeaponStaff, db.WeaponDagger, db.WeaponSword}
	wandOnly      = []db.RangedWeaponType{db.RangedWand}
)

// casterProfile fills the shared shape of the mage and warlock specs:
// intellect, spell power and the three secondaries, hit to the spell
// cap.
func casterProfile(class db.Class, tree, className string, int_, hit, crit, haste, mastery, epPerDPS float64, interval time.Duration) *score.Profile {
	return &score.Profile{
		Name: tree + " " + className, Class: class, Tree: tree,
		Weights: db.Stats{
			db.Intellect: int_, db.SpellPower: 1, db.MeleeHit: hit,
			db.MeleeCrit: crit, db.MeleeHaste: haste, db.Mastery: mastery,
		},
		HitCap:       &score.HitCapSpec{Stat: db.MeleeHit, BasePct: 17, Spell: true},
		EPPerDPS:     epPerDPS,
		ProcInterval: interval,
		ArmorType:    db.ArmorCloth, WeaponTypes: casterWeapons, RangedTypes: wandOnly,
		Layouts: staffOrHeld,
		Rules:   rules85,
	}
}

// arcane deals no periodic damage: Necromantic Focus never stacks for it
// in the sim (its passive intellect alone accounts for the whole
// measured value), so periodic-only procs are gated off.
func arcane(p *score.Profile) *score.Profile {
	p.NoPeriodicDamage = true
	return p
}

// Frost has no profile: the pinned sim carries no cata frost rotation
// (its test is commented out and its preset is a wotlk leftover), so
// there is nothing to measure against.
var (
	arcaneProfile = arcane(casterProfile(db.ClassMage, "Arcane", "Mage", 1.67, arcaneHitEP, 0.61, 0.70, 0.67, 0.390, 800*time.Millisecond))
	fireProfile   = casterProfile(db.ClassMage, "Fire", "Mage", 1.37, fireHitEP, 0.89, 0.82, 0.58, 0.316, 520*time.Millisecond)

	afflictionProfile  = casterProfile(db.ClassWarlock, "Affliction", "Warlock", 1.28, affHitEP, 0.61, 0.69, 0.45, 0.378, 525*time.Millisecond)
	demonologyProfile  = casterProfile(db.ClassWarlock, "Demonology", "Warlock", 1.32, demoHitEP, 0.67, 0.72, 0.89, 0.377, 450*time.Millisecond)
	destructionProfile = casterProfile(db.ClassWarlock, "Destruction", "Warlock", 1.29, destroHitEP, 0.59, 0.79, 0.54, 0.366, 325*time.Millisecond)
)
