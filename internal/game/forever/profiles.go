package forever

import (
	"github.com/vaclav-dvorak/twolk/internal/db"
	"github.com/vaclav-dvorak/twolk/internal/score"
)

// Hunter weights are MEASURED on the pinned wowsims/forever build
// (sim/hunter, the package's own test setup: prebis_weave gear, full
// buffs and the test consumables, Orc, single target), by direct
// mean-DPS differences on one fixed seed (20k iterations per variant,
// seed 101) — never the sim's EP routine. Ranged attack power is the
// numeraire: every weight is DPS per point divided by DPS per RAP.
//
// Each tree runs the setup its upstream test uses: Beast Mastery the
// turret rotation (no melee weaving) with a pet; Marksmanship and
// Survival the default weave rotation without one. That is why BM
// prices strength, melee AP and main-hand DPS at zero while SV — the
// melee-weaving tree — prices main-hand DPS above ranged.
//
// Both AP slots carry their own measured weight: an in-game "+X Attack
// Power" line grants melee and ranged alike, and the Forever sim does
// NOT derive one from the other, so summing the two slot values is the
// item's true worth (unlike the cata convention, where the published
// presets zeroed the slot a spec doesn't use).
//
// Gear values are the dump's fixed ratings: 10 hit/haste per 1%, 14
// crit per 1% (db.ForeverCritRatingPerPct). Per 1% that is, for BM:
// hit 35, crit 32, haste 13, agility 2.9 per point.
//
// Hit is flat below the cap and near zero above it in every tree; the
// cap is 8% total. Swept at 20k iterations in 1% steps around the
// preset's 5% gear hit: BM and MM fall off between +3% and +4% (8%),
// SV right at +0% — Surefooted's 3% supplies the rest, so the SV cap
// below assumes the talent is taken (3/3).
//
// The weights are level-60 weights, applied at every level: leveling
// characters get "what fits now" ranked by endgame value.
var (
	bmWeights = hunterWeights(2.883, 0, 1.107, 0, 0, 1.000, 3.499, 2.284, 1.280, 0.250)
	mmWeights = hunterWeights(3.695, 0.571, 1.698, 0, 0.519, 1.000, 4.906, 3.162, 3.480, 0.457)
	svWeights = hunterWeights(5.026, 1.055, 2.179, 0, 0.959, 1.000, 6.151, 3.761, 6.204, 0.658)
)

// hunterEPPerDPS converts flat DPS into EP: 1 / (DPS per RAP) on the
// same runs (BM 0.1895, MM 0.1919, SV 0.1415 DPS per RAP).
const (
	bmEPPerDPS = 5.28
	mmEPPerDPS = 5.21
	svEPPerDPS = 7.07
)

func hunterWeights(agi, str, intel, spi, ap, rap, hit, crit, haste, arp float64) db.Stats {
	var w db.Stats
	w[db.Agility] = agi
	w[db.Strength] = str
	w[db.Intellect] = intel
	w[db.Spirit] = spi
	w[db.AttackPower] = ap
	w[db.RangedAttackPower] = rap
	w[db.MeleeHit] = hit // shared hit/crit/haste decode into the melee slots
	w[db.MeleeCrit] = crit
	w[db.MeleeHaste] = haste
	w[db.ArmorPenetration] = arp
	return w
}

// Hunter proposal filters: every weapon type a hunter trains, real
// ranged weapons (thrown fires no Auto Shot). The armor class follows
// the character's level (ArmorAt).
var (
	hunterWeapons = []db.WeaponType{db.WeaponAxe, db.WeaponDagger, db.WeaponFist,
		db.WeaponPolearm, db.WeaponStaff, db.WeaponSword}
	hunterRanged  = []db.RangedWeaponType{db.RangedBow, db.RangedCrossbow, db.RangedGun}
	twoHandOrDual = []score.WeaponLayout{score.LayoutTwoHand, score.LayoutDualWield}
)

func hunterProfile(tree string, weights db.Stats, hitPct, mhDPS, rangedDPS, epPerDPS float64) *score.Profile {
	return &score.Profile{
		Name: tree + " Hunter", Class: db.ClassHunter, Tree: tree,
		Weights:         weights,
		HitCap:          &score.HitCapSpec{Stat: db.MeleeHit, BasePct: hitPct},
		WeaponDPSWeight: mhDPS,
		RangedDPSWeight: rangedDPS,
		EPPerDPS:        epPerDPS,
		ArmorType:       db.ArmorMail,
		WeaponTypes:     hunterWeapons,
		RangedTypes:     hunterRanged,
		Layouts:         twoHandOrDual,
		Rules:           rules60,
	}
}

var profiles = []*score.Profile{
	hunterProfile("Beast Mastery", bmWeights, 8, 0, 8.906, bmEPPerDPS),
	hunterProfile("Marksmanship", mmWeights, 8, 7.148, 8.309, mmEPPerDPS),
	hunterProfile("Survival", svWeights, 5, 14.006, 8.500, svEPPerDPS),
}

func findProfile(class db.Class, tree string, _ map[int]int) *score.Profile {
	if tree == "Feral" { // the armory may shorten the tree's name
		tree = "Feral Combat"
	}
	return score.FindIn(profiles, class, tree)
}

// armorAt is the vanilla armor progression: mail for hunters and
// shamans, plate for warriors and paladins, both taught at 40.
func armorAt(c db.Class, level int) db.ArmorType {
	switch c {
	case db.ClassHunter, db.ClassShaman:
		if level >= 40 {
			return db.ArmorMail
		}
		return db.ArmorLeather
	case db.ClassWarrior, db.ClassPaladin:
		if level >= 40 {
			return db.ArmorPlate
		}
		return db.ArmorMail
	case db.ClassRogue, db.ClassDruid:
		return db.ArmorLeather
	}
	return db.ArmorCloth
}
