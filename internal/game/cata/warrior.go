package cata

import (
	"github.com/vaclav-dvorak/twolk/internal/db"
	"github.com/vaclav-dvorak/twolk/internal/score"
)

// Warrior profiles. Weights are the pinned sim's P3 presets
// (ui/warrior/<spec>/presets.ts @ the pin, attack power = 1 — Arms
// ships no P1 table, and the reference realm is in Firelands), adopted
// as published: unlike the hunter and druid profiles, the fill-to-cap
// stats are NOT yet measured against the headless sim, and neither is
// EPPerDPS, so damage procs and tier bonuses stay unpriced (notes only)
// until they are. See plan.md.
//
// Fury carries the sim's own hit breakpoints (ui/warrior/fury/sim.ts):
// the spec passive Precision (sim/warrior/fury/fury.go) adds 3% hit, so
// gear needs only 5% to take yellow attacks to the 8% cap, and past it
// dual-wield white swings keep gaining up to 27% total — 24% from gear —
// at the sim's post-cap 1.23 per rating.
var (
	furyYellowCap = 5 * rules85.MeleeHitRatingPerPct  // 601 rating: 8% with Precision
	furyWhiteCap  = 24 * rules85.MeleeHitRatingPerPct // 2883 rating: 27% with Precision
	parryCap      = 56 * rulesExpertise               // 14%: a tank also fights parries
)

const furyWhiteHitEP = 1.23

// Titan's Grip and Single-Minded Fury (wowsims ui/core/talents/trees/
// warrior.json) decide which fury build the character plays.
const (
	titansGrip       = 46917
	singleMindedFury = 81099
)

var (
	// Arms and TG swing two-handers; SMF and prot one-handers. Staves
	// and polearms fit no fury hand (Titan's Grip takes axes, maces and
	// swords only).
	armsWeapons     = []db.WeaponType{db.WeaponAxe, db.WeaponMace, db.WeaponSword, db.WeaponPolearm}
	furyWeapons     = []db.WeaponType{db.WeaponAxe, db.WeaponMace, db.WeaponSword, db.WeaponDagger, db.WeaponFist}
	warriorRanged   = []db.RangedWeaponType{db.RangedBow, db.RangedCrossbow, db.RangedGun, db.RangedThrown}
	titanGripOrDW   = []score.WeaponLayout{score.LayoutTitanGrip, score.LayoutDualWield}
	dualWieldOnly   = []score.WeaponLayout{score.LayoutDualWield}
	weaponAndShield = []score.WeaponLayout{score.LayoutWeaponShield}
)

// warriorWeights fills the shared shape of the DPS presets. AP only:
// the dump writes "+X Attack Power" into both AP slots, and ranged AP
// does nothing for a warrior.
func warriorWeights(str, agi, exp, hit, crit, haste, mastery float64) db.Stats {
	return db.Stats{
		db.Strength: str, db.Agility: agi, db.AttackPower: 1,
		db.Expertise: exp, db.MeleeHit: hit,
		db.MeleeCrit: crit, db.MeleeHaste: haste, db.Mastery: mastery,
	}
}

var armsProfile = &score.Profile{
	Name: "Arms Warrior", Class: db.ClassWarrior, Tree: "Arms",
	Weights:         warriorWeights(2.21, 1.12, 1.75, 2.77, 1.45, 0.68, 0.89),
	HitCap:          &score.HitCapSpec{Stat: db.MeleeHit, BasePct: 8},
	Caps:            []score.StatCap{{Stat: db.Expertise, Cap: dodgeCap}},
	WeaponDPSWeight: 9.22,
	ArmorType:       db.ArmorPlate, WeaponTypes: armsWeapons, RangedTypes: warriorRanged,
	Layouts: twoHandOnly,
	Rules:   rules85,
}

// furyCaps: full value to the yellow cap, white-only value to the
// dual-wield cap, nothing past it. No HitCap — the breakpoints already
// carry Precision, and the flat 8% default must not be added on top.
var furyCaps = []score.StatCap{
	{Stat: db.MeleeHit, Cap: furyYellowCap, SoftWeight: furyWhiteHitEP},
	{Stat: db.MeleeHit, Cap: furyWhiteCap},
	{Stat: db.Expertise, Cap: dodgeCap},
}

// Weapon DPS: the sim prices main and off hand separately; the engine
// takes one blended weight (see plan.md caveats), the mean of the two.
var furyTGProfile = &score.Profile{
	Name: "Fury Warrior (TG)", Class: db.ClassWarrior, Tree: "Fury", Role: "tg",
	Weights:         warriorWeights(2.21, 1.14, 1.71, 2.66, 1.48, 1.19, 1.17),
	Caps:            furyCaps,
	WeaponDPSWeight: (3.77 + 1.6) / 2,
	ArmorType:       db.ArmorPlate, WeaponTypes: furyWeapons, RangedTypes: warriorRanged,
	Layouts: titanGripOrDW,
	Rules:   rules85,
}

var furySMFProfile = &score.Profile{
	Name: "Fury Warrior (SMF)", Class: db.ClassWarrior, Tree: "Fury", Role: "smf",
	Weights:         warriorWeights(2.2, 1.07, 1.57, 2.46, 1.39, 1.19, 1.02),
	Caps:            furyCaps,
	WeaponDPSWeight: (3.49 + 1.71) / 2,
	ArmorType:       db.ArmorPlate, WeaponTypes: furyWeapons, RangedTypes: warriorRanged,
	Layouts: dualWieldOnly,
	Rules:   rules85,
}

// protProfile is the sim's preset, stamina-, mastery- and avoidance-
// led. The preset defines no breakpoints, so the caps follow the bear
// tank's shape: hit is worth nothing past 8% (every threat tool is
// physical), expertise full value to the dodge cap and half to the
// parry cap. No DPS numeraire — a tank's set bonuses stay notes.
var protProfile = &score.Profile{
	Name: "Protection Warrior", Class: db.ClassWarrior, Tree: "Protection",
	Weights: db.Stats{
		db.Armor: 2.155, db.BonusArmor: 2.155, db.Stamina: 12.442,
		db.Strength: 1.4, db.Agility: 0.26, db.AttackPower: 0.196,
		db.Expertise: 0.863, db.MeleeHit: 0.736, db.MeleeCrit: 0.336,
		db.MeleeHaste: 0.048, db.Dodge: 4.801, db.Parry: 4.801, db.Mastery: 7.415,
	},
	HitCap: &score.HitCapSpec{Stat: db.MeleeHit, BasePct: 8},
	Caps: []score.StatCap{
		{Stat: db.Expertise, Cap: dodgeCap, SoftWeight: 0.863 / 2},
		{Stat: db.Expertise, Cap: parryCap},
	},
	WeaponDPSWeight: 6.081,
	ArmorType:       db.ArmorPlate, WeaponTypes: furyWeapons, RangedTypes: warriorRanged,
	Layouts: weaponAndShield,
	Rules:   rules85,
}

// furyProfileFor picks the fury build by its capstone talent: Single-
// Minded Fury plays one-handers, anything else (Titan's Grip, or no
// talent detail) the two-hander build the sim defaults to.
func furyProfileFor(ranks map[int]int) *score.Profile {
	if ranks[singleMindedFury] > 0 && ranks[titansGrip] == 0 {
		return furySMFProfile
	}
	return furyTGProfile
}
