package cata

import (
	"github.com/vaclav-dvorak/twolk/internal/db"
	"github.com/vaclav-dvorak/twolk/internal/score"
)

// Druid profiles. Weights are the pinned sim's presets
// (ui/druid/<spec>/presets.ts @ the pin, each in its own units), with
// the fill-to-cap stats MEASURED by direct DPS differences on the sim's
// own test presets (session 43; 50k iterations, seed 101, replicated on
// a disjoint seed, ratios stable within ~0.02):
//
//	Feral cat, P1 bearweave (the sim's default rotation), agility = 1:
//	hit 0.34 and expertise 0.32 below their caps — flat to ±0.01 from
//	200 rating up to the cap, zero past it (8% hit = 961, dodge cap 26
//	expertise = 781). The published 0.36/0.34 sit at the caps and
//	happen to agree; the rest of the preset reproduces within 0.01
//	except haste (0.27 measured vs 0.30). One point of agility is 4.21
//	DPS → EPPerDPS 0.2375, ~75 EP per 1% DPS.
//
//	Balance, T11 preset, spell power = 1: hit 0.96 below the 17% cap
//	(1742), and SPIRIT IDENTICAL TO HIT to the decimal — Balance of
//	Power 2/2 turns spirit from gear into spell hit 1:1 — both exactly
//	zero past the cap (mana never limits the rotation). The published
//	1.27 matches only the T12 preset (1.25 measured), whose extra
//	value is gear-specific: that preset carries Dragonwrath, the
//	legendary staff; T11 gear on the T12 rotation still reads 0.94.
//	One point of spell power is 2.78 DPS → EPPerDPS 0.36, ~106 EP per
//	1% DPS.
const (
	catHitEP     = 0.34
	catExpEP     = 0.32
	balanceHitEP = 0.96
)

var (
	// feral weapons are stat sticks with a DPS that matters (cat damage
	// scales with it); casters add fist weapons to the WotLK set
	druidMelee   = []db.WeaponType{db.WeaponStaff, db.WeaponPolearm, db.WeaponMace, db.WeaponDagger, db.WeaponFist}
	druidCaster  = []db.WeaponType{db.WeaponStaff, db.WeaponMace, db.WeaponDagger, db.WeaponFist}
	relicOnly    = []db.RangedWeaponType{db.RangedRelic}
	twoHandOnly  = []score.WeaponLayout{score.LayoutTwoHand}
	staffOrHeld  = []score.WeaponLayout{score.LayoutTwoHand, score.LayoutWeaponHeld}
	dodgeCap     = 26 * rulesExpertise // 781 rating: dodge removed
	parryCapBear = 56 * rulesExpertise // 14%: a tank also fights parries
	physHitCap   = 8 * rules85.MeleeHitRatingPerPct
	spellHitCap  = 17 * rules85.SpellHitRatingPerPct
)

// Bear-only Feral Combat talents (wowsims ui/core/talents/trees/
// druid.json): Natural Reaction and Pulverize. The cat presets take
// neither, every bear build takes both.
var bearTalents = []int{57878, 57880, 80313}

// balanceOfPower: spirit from gear counts as spell hit, 50% per rank.
var balanceOfPower = score.CapTalent{Name: "Balance of Power", Tree: "Balance",
	Max: 2, SpellIDs: []int{33592, 33596}}

var balanceProfile = &score.Profile{
	Name: "Balance Druid", Class: db.ClassDruid, Tree: "Balance",
	Weights: db.Stats{
		db.Intellect: 1.3, db.SpellPower: 1, db.MeleeHit: balanceHitEP,
		// spirit is worth exactly what it converts to (measured equal
		// to hit), and nothing on its own
		db.Spirit:    balanceHitEP,
		db.MeleeCrit: 0.41, db.MeleeHaste: 0.8, db.Mastery: 0.56,
	},
	HitCap:      &score.HitCapSpec{Stat: db.MeleeHit, BasePct: 17, Spell: true},
	Conversions: []score.StatConversion{{From: db.Spirit, To: db.MeleeHit, Ratio: 1}},
	EPPerDPS:    0.36,
	ArmorType:   db.ArmorLeather, WeaponTypes: druidCaster, RangedTypes: relicOnly,
	Layouts: staffOrHeld,
	Rules:   rules85,
}

// balanceWithBoP is the balance profile at a lower Balance of Power
// rank: less of the spirit converts, and spirit is worth that share.
func balanceWithBoP(rank int) *score.Profile {
	p := *balanceProfile
	ratio := 0.5 * float64(rank)
	p.Conversions = []score.StatConversion{{From: db.Spirit, To: db.MeleeHit, Ratio: ratio}}
	p.Weights[db.Spirit] = ratio * balanceHitEP
	return &p
}

var catProfile = &score.Profile{
	Name: "Feral Druid (cat)", Class: db.ClassDruid, Tree: "Feral Combat", Role: "cat",
	Weights: db.Stats{
		// AP only: the dump writes "+X Attack Power" into both AP
		// slots; ranged AP does nothing for a druid
		db.Strength: 0.38, db.Agility: 1.0, db.AttackPower: 0.37,
		db.MeleeHit: catHitEP, db.Expertise: catExpEP,
		db.MeleeCrit: 0.32, db.MeleeHaste: 0.30, db.Mastery: 0.33,
	},
	HitCap:          &score.HitCapSpec{Stat: db.MeleeHit, BasePct: 8},
	Caps:            []score.StatCap{{Stat: db.Expertise, Cap: dodgeCap}},
	WeaponDPSWeight: 1.54,
	EPPerDPS:        0.2375,
	ArmorType:       db.ArmorLeather, WeaponTypes: druidMelee, RangedTypes: relicOnly,
	Layouts: twoHandOnly,
	Rules:   rules85,
}

// bearProfile is the sim's SURVIVAL preset (its default), stamina-
// and armor-led. Its caps are the preset's own soft-cap breakpoints:
// hit is worth its physical share (0.185 of 0.22) to 8%, then only the
// spell share (Faerie Fire, Growl) to 17%; expertise full value to the
// dodge cap, half to the parry cap, nothing after. No DPS numeraire —
// a tank's set bonuses stay notes.
var bearProfile = &score.Profile{
	Name: "Feral Druid (bear)", Class: db.ClassDruid, Tree: "Feral Combat", Role: "bear",
	Weights: db.Stats{
		db.Health: 0.05, db.Stamina: 1.08, db.Agility: 1.0, db.Armor: 0.93,
		db.BonusArmor: 0.21, db.Dodge: 0.85, db.Mastery: 0.31,
		db.Strength: 0.08, db.AttackPower: 0.08,
		db.MeleeHit: 0.22, db.Expertise: 0.37, db.MeleeCrit: 0.28, db.MeleeHaste: 0.06,
	},
	Caps: []score.StatCap{
		{Stat: db.MeleeHit, Cap: physHitCap, SoftWeight: 0.035},
		{Stat: db.MeleeHit, Cap: spellHitCap},
		{Stat: db.Expertise, Cap: dodgeCap, SoftWeight: 0.185},
		{Stat: db.Expertise, Cap: parryCapBear},
	},
	ArmorType: db.ArmorLeather, WeaponTypes: druidMelee, RangedTypes: relicOnly,
	Layouts: twoHandOnly,
	Rules:   rules85,
}

// restoProfile: the sim's restoration preset is a placeholder — it has
// no rotation to measure against, prices Intellect BELOW spell power
// (0.38 vs 1, impossible: one intellect IS one spell power in Cata,
// plus mana and crit) and carries no mastery at all. Kept where it is
// plausible (haste 0.77, crit 0.69, spirit 0.34), with two corrections:
// intellect = 1 + the preset's 0.38 (the spell power it grants plus
// its own share), and mastery (Harmony) set at crit's value — a
// placeholder until a healing measurement exists. Approximate by
// construction; see plan.md.
var restoProfile = &score.Profile{
	Name: "Restoration Druid", Class: db.ClassDruid, Tree: "Restoration",
	Weights: db.Stats{
		db.Intellect: 1.38, db.SpellPower: 1, db.Spirit: 0.34,
		db.MeleeCrit: 0.69, db.MeleeHaste: 0.77, db.Mastery: 0.69,
	},
	ArmorType: db.ArmorLeather, WeaponTypes: druidCaster, RangedTypes: relicOnly,
	Layouts: staffOrHeld,
	Rules:   rules85,
}
