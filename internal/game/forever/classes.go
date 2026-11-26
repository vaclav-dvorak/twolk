package forever

import (
	"github.com/vaclav-dvorak/twolk/internal/db"
	"github.com/vaclav-dvorak/twolk/internal/score"
)

// Every non-hunter DPS spec, MEASURED the way the hunters were (see
// profiles.go): the pinned wowsims/forever build, each spec on the setup
// of its own upstream test suite (gear set, talents, rotation, race,
// professions, consumables), full buffs and debuffs, one level-63
// target, 20k iterations per variant on seed 101, direct mean-DPS
// differences — never the sim's EP routine. The harnesses live in
// tools/eptool-forever (one per sim package).
//
// Numeraire: melee attack power for physical specs, spell damage for
// casters. Every weight is DPS per point ÷ DPS per numeraire point, so
// EPPerDPS is the reciprocal of the latter.
//
// Hit caps are the RAW knees — 8% physical, 17% spells, 6.5% expertise
// — and each character's detected cap talents (talents.go) lower them;
// every setup's non-gear hit traced to exactly those talents (mage
// Precision 3/3 = 6%, Suppression 5/5, Nature's Reach, Tidal Focus,
// paladin and rogue Precision, Shadow Focus). Specs past the yellow cap keep a residual
// (Fury's white swings, Enhancement's and Retribution's spells): a soft
// cap at the knee, a hard one where the residual ends. Expertise caps at
// 6.5% (dodge only, attacks from behind), 65 rating; Weapon Expertise
// lowers it.
//
// Spell piercing reads high for casters (2–2.5× spell damage) and is
// real: the Forever sim turns piercing beyond the target's resistance
// into bonus damage (sim/core/spell_resistances.go, EV-0142), nearly
// linear over gear amounts.
//
// Measured oddities kept out of the weights: Shadow Priest haste
// measured −0.30 and Enhancement mp5 −0.02 — rotation timing artifacts
// of the test setups, not stats worth avoiding; both read 0 here.
//
// Dual wielders price both hands' weapon DPS at the MAIN-hand weight
// (twolk has one weapon-DPS weight); the off hand measured 0.44×
// (Fury) and 0.25–0.39× (rogues) of it.

func weights(m map[db.Stat]float64) db.Stats {
	var w db.Stats
	for st, v := range m {
		w[st] = v
	}
	return w
}

// hitCaps is a soft cap at soft (rating) worth residual EP per point
// past it, then a hard cap at hard.
func hitCaps(soft, residual, hard float64) []score.StatCap {
	return []score.StatCap{
		{Stat: db.MeleeHit, Cap: soft, SoftWeight: residual},
		{Stat: db.MeleeHit, Cap: hard},
	}
}

var (
	expertiseCap = score.StatCap{Stat: db.Expertise, Cap: 65}

	physRanged = []db.RangedWeaponType{db.RangedBow, db.RangedCrossbow, db.RangedGun, db.RangedThrown}
	wand       = []db.RangedWeaponType{db.RangedWand}

	twoHand     = []score.WeaponLayout{score.LayoutTwoHand}
	dualWield   = []score.WeaponLayout{score.LayoutDualWield}
	casterHands = []score.WeaponLayout{score.LayoutTwoHand, score.LayoutWeaponHeld}
)

// armsExpertise: Overpower feeds on dodges, so each expertise point
// removes a dodge that would have been an Overpower — value falls all
// the way to the cap (sweep in 1% steps from 0 expertise).
var armsExpertise = score.StatCurve{
	{At: 5, Weight: 1.236}, {At: 15, Weight: 0.833}, {At: 25, Weight: 0.660},
	{At: 35, Weight: 0.586}, {At: 45, Weight: 0.469}, {At: 55, Weight: 0.325},
	{At: 62.5, Weight: 0.136}, {At: 65, Weight: 0},
}

func init() {
	profiles = append(profiles,
		// --- Warrior (sim/warrior/dps, Orc). Arms: p1_arms, arms APL.
		// Fury: p1_fury dual wield, fury APL; its 2H variant (fury_2h,
		// Arcanite Reaper) measured MH DPS 13.7 and hit 4.6 — close
		// enough that one Fury profile serves both layouts.
		&score.Profile{
			Name: "Arms Warrior", Class: db.ClassWarrior, Tree: "Arms",
			Weights: weights(map[db.Stat]float64{
				db.Strength: 2.2000, db.Agility: 1.8313, db.AttackPower: 1,
				db.MeleeHit: 4.3808, db.MeleeCrit: 2.3715, db.MeleeHaste: 1.8787,
				db.Expertise: 1.0346, db.ArmorPenetration: 0.3921,
			}),
			Curves:          map[db.Stat]score.StatCurve{db.Expertise: armsExpertise},
			HitCap:          &score.HitCapSpec{Stat: db.MeleeHit, BasePct: 8},
			WeaponDPSWeight: 16.607, EPPerDPS: 4.818,
			ArmorType:   db.ArmorPlate,
			WeaponTypes: []db.WeaponType{db.WeaponAxe, db.WeaponMace, db.WeaponSword, db.WeaponPolearm},
			RangedTypes: physRanged, Layouts: twoHand, Rules: rules60,
		},
		&score.Profile{
			Name: "Fury Warrior", Class: db.ClassWarrior, Tree: "Fury",
			Weights: weights(map[db.Stat]float64{
				db.Strength: 2.2000, db.Agility: 2.0251, db.AttackPower: 1,
				db.MeleeHit: 3.3566, db.MeleeCrit: 2.6398, db.MeleeHaste: 1.4474,
				db.Expertise: 1.8678, db.ArmorPenetration: 0.3453,
			}),
			// yellow attacks cap at 8%; white swings keep ~half value
			// at least to 13% (the end of the sweep)
			Caps:            []score.StatCap{expertiseCap, {Stat: db.MeleeHit, Cap: 80, SoftWeight: 1.645}},
			WeaponDPSWeight: 8.880, EPPerDPS: 4.219,
			ArmorType: db.ArmorPlate,
			WeaponTypes: []db.WeaponType{db.WeaponAxe, db.WeaponFist, db.WeaponMace,
				db.WeaponSword, db.WeaponPolearm},
			RangedTypes: physRanged,
			Layouts:     []score.WeaponLayout{score.LayoutDualWield, score.LayoutTwoHand},
			Rules:       rules60,
		},

		// --- Druid. Cat (sim/druid/feralcat, Night Elf, the suite runs
		// with no consumables): sheet 6.7% hit of which 2% talents.
		// Balance (p1, Night Elf): 4% non-gear spell hit.
		&score.Profile{
			Name: "Feral Druid", Class: db.ClassDruid, Tree: "Feral Combat",
			Weights: weights(map[db.Stat]float64{
				db.Strength: 2.4322, db.Agility: 2.6389, db.Intellect: 0.1709, db.Spirit: 0.0819,
				db.AttackPower: 1, db.FeralAttackPower: 0.9951,
				db.MeleeHit: 1.9038, db.MeleeCrit: 1.9620, db.MeleeHaste: 0.7351,
				db.Expertise: 1.8058, db.ArmorPenetration: 0.2959, db.MP5: 0.3389,
			}),
			Caps:            []score.StatCap{expertiseCap},
			HitCap:          &score.HitCapSpec{Stat: db.MeleeHit, BasePct: 8},
			WeaponDPSWeight: 8.796, EPPerDPS: 3.876,
			ArmorType:   db.ArmorLeather,
			WeaponTypes: []db.WeaponType{db.WeaponDagger, db.WeaponFist, db.WeaponMace, db.WeaponStaff, db.WeaponOffHand},
			RangedTypes: []db.RangedWeaponType{db.RangedIdol},
			Layouts:     casterHands, Rules: rules60,
		},
		casterProfile("Balance Druid", db.ClassDruid, "Balance", 17, 1.392, map[db.Stat]float64{
			db.Intellect: 0.2113, db.ArcaneDamage: 0.6690, db.NatureDamage: 0.3310,
			db.MeleeHit: 1.2546, db.MeleeCrit: 0.8302, db.MeleeHaste: 1.1637,
			db.MP5: 0.0025, db.SpellPenetration: 2.5085,
		}, db.ArmorLeather, []db.WeaponType{db.WeaponDagger, db.WeaponMace, db.WeaponStaff, db.WeaponOffHand},
			[]db.RangedWeaponType{db.RangedIdol}),

		// --- Mage (sim/mage, High Order Skyborne, the suite's generic
		// "preraid" set for all three): 6% non-gear spell hit.
		casterProfile("Arcane Mage", db.ClassMage, "Arcane", 17, 1.800, map[db.Stat]float64{
			db.Intellect: 0.8880, db.Spirit: 0.2746, db.ArcaneDamage: 0.8973, db.FireDamage: 0.1027,
			db.MeleeHit: 1.5578, db.MeleeCrit: 0.6327, db.MeleeHaste: 2.0041,
			db.MP5: 0.2510, db.SpellPenetration: 2.5627,
		}, db.ArmorCloth, mageWeapons, wand),
		casterProfile("Fire Mage", db.ClassMage, "Fire", 17, 1.392, map[db.Stat]float64{
			db.Intellect: 0.4110, db.Spirit: 0.1062, db.FireDamage: 0.8776,
			db.MeleeHit: 1.6202, db.MeleeCrit: 0.7474, db.MeleeHaste: 0.2792,
			db.MP5: 0.2503, db.SpellPenetration: 2.2292,
		}, db.ArmorCloth, mageWeapons, wand),
		casterProfile("Frost Mage", db.ClassMage, "Frost", 17, 1.487, map[db.Stat]float64{
			db.Intellect: 0.5315, db.Spirit: 0.3034, db.FrostDamage: 1.0000,
			db.MeleeHit: 1.4365, db.MeleeCrit: 0.5668, db.MeleeHaste: 0.9467,
			db.MP5: 0.8374, db.SpellPenetration: 2.1261,
		}, db.ArmorCloth, mageWeapons, wand),

		// --- Warlock (sim/warlock build presets, Orc): Affliction
		// sm_ruin and Destruction carry 5% non-gear spell hit,
		// Demonology (Fire Brand) 1%.
		casterProfile("Affliction Warlock", db.ClassWarlock, "Affliction", 17, 1.336, map[db.Stat]float64{
			db.Intellect: 0.3306, db.Spirit: 0.1168, db.ShadowDamage: 0.8965, db.FireDamage: 0.0882,
			db.MeleeHit: 0.9662, db.MeleeCrit: 0.7683, db.MeleeHaste: 0.7771,
			db.MP5: 0.3014, db.SpellPenetration: 1.2501,
		}, db.ArmorCloth, warlockWeapons, wand),
		casterProfile("Demonology Warlock", db.ClassWarlock, "Demonology", 17, 1.274, map[db.Stat]float64{
			db.Intellect: 0.2869, db.Spirit: 0.0534, db.ShadowDamage: 0.2230, db.FireDamage: 0.8011,
			db.MeleeHit: 1.3045, db.MeleeCrit: 0.5404, db.MeleeHaste: 0.5779,
			db.MP5: 0.2470, db.SpellPenetration: 1.1860,
		}, db.ArmorCloth, warlockWeapons, wand),
		casterProfile("Destruction Warlock", db.ClassWarlock, "Destruction", 17, 1.240, map[db.Stat]float64{
			db.Intellect: 0.2966, db.Spirit: 0.1075, db.ShadowDamage: 0.3233, db.FireDamage: 0.6704,
			db.MeleeHit: 1.0214, db.MeleeCrit: 0.6039, db.MeleeHaste: 0.0667,
			db.MP5: 0.2817, db.SpellPenetration: 1.2903,
		}, db.ArmorCloth, warlockWeapons, wand),

		// --- Paladin: Retribution (sim/paladin/retribution "default",
		// Unsealed Wrath talents, Undead). 1% non-gear hit; spells keep
		// ~1 EP per hit point past the melee knee up to 17%.
		&score.Profile{
			Name: "Retribution Paladin", Class: db.ClassPaladin, Tree: "Retribution",
			Weights: weights(map[db.Stat]float64{
				db.Strength: 2.4333, db.Agility: 1.9557, db.Intellect: 1.4940, db.Spirit: 0.0029,
				db.AttackPower: 1, db.SpellPower: 1.9636, db.HolyDamage: 1.5982,
				db.MeleeHit: 7.7360, db.MeleeCrit: 2.7836, db.MeleeHaste: 4.0335,
				db.Expertise: 5.4817, db.ArmorPenetration: 0.2342, db.MP5: 0.0056,
			}),
			Caps:            append([]score.StatCap{expertiseCap}, hitCaps(80, 0.965, 170)...),
			WeaponDPSWeight: 14.137, EPPerDPS: 5.077,
			ArmorType:   db.ArmorPlate,
			WeaponTypes: []db.WeaponType{db.WeaponAxe, db.WeaponMace, db.WeaponSword, db.WeaponPolearm},
			RangedTypes: []db.RangedWeaponType{db.RangedLibram},
			Layouts:     twoHand, Rules: rules60,
		},

		// --- Rogue (sim/rogue TestRogue: Human, the "muti" set, default
		// APL; 12.2% hit already, so hit keeps its value through the
		// sweep — poisons to 17%, white swings past it — and no cap).
		rogueProfile("Assassination", 2.0570, 2.2398, 1.9300, 1.5299, 2.6899, 0.4091, 7.767, 3.863, 65),
		rogueProfile("Combat", 1.7865, 1.7391, 1.3519, 1.3842, 2.1798, 0.3542, 9.064, 3.590, 65),
		rogueProfile("Subtlety", 1.8741, 1.6458, 1.5078, 1.3308, 2.3545, 0.2292, 8.423, 3.452, 65),

		// --- Shaman. Enhancement (Windfury, Orc, 2H): 2% non-gear hit,
		// spells keep ~2.2 EP per hit point to 17%. Elemental (Windshaper
		// Skyborne): 5% non-gear spell hit.
		&score.Profile{
			Name: "Enhancement Shaman", Class: db.ClassShaman, Tree: "Enhancement",
			Weights: weights(map[db.Stat]float64{
				db.Strength: 2.2000, db.Agility: 1.4592, db.Intellect: 1.9192,
				db.AttackPower: 1, db.SpellPower: 1.3586, db.NatureDamage: 0.6634, db.FireDamage: 0.6712,
				db.MeleeHit: 7.3383, db.MeleeCrit: 2.7538, db.MeleeHaste: 2.3317,
				db.Expertise: 5.1618, db.ArmorPenetration: 0.3904,
			}),
			Caps:            append([]score.StatCap{expertiseCap}, hitCaps(80, 2.19, 170)...),
			WeaponDPSWeight: 14.160, EPPerDPS: 4.874,
			ArmorType:   db.ArmorMail,
			WeaponTypes: []db.WeaponType{db.WeaponAxe, db.WeaponMace},
			RangedTypes: []db.RangedWeaponType{db.RangedTotem},
			Layouts:     twoHand, Rules: rules60,
		},
		casterProfile("Elemental Shaman", db.ClassShaman, "Elemental", 17, 1.557, map[db.Stat]float64{
			db.Intellect: 0.2594, db.Spirit: 0.0540, db.NatureDamage: 0.8859, db.FireDamage: 0.1141,
			db.MeleeHit: 1.6433, db.MeleeCrit: 0.6693, db.MeleeHaste: 0.9576,
			db.MP5: 0.1587, db.SpellPenetration: 2.2786,
		}, db.ArmorMail, []db.WeaponType{db.WeaponAxe, db.WeaponDagger, db.WeaponFist,
			db.WeaponMace, db.WeaponShield, db.WeaponStaff},
			[]db.RangedWeaponType{db.RangedTotem}),

		// --- Priest: Shadow (sim/priest "testing" build, Troll): 5%
		// shadow hit from talents.
		casterProfile("Shadow Priest", db.ClassPriest, "Shadow", 17, 2.075, map[db.Stat]float64{
			db.Intellect: 0.2482, db.Spirit: 0.0074, db.ShadowDamage: 1.0000,
			db.MeleeHit: 1.2136, db.MeleeCrit: 0.9101,
			db.MP5: 0.0238, db.SpellPenetration: 2.4931,
		}, db.ArmorCloth, []db.WeaponType{db.WeaponDagger, db.WeaponMace, db.WeaponStaff, db.WeaponOffHand}, wand),
	)
}

var (
	mageWeapons    = []db.WeaponType{db.WeaponDagger, db.WeaponSword, db.WeaponStaff, db.WeaponOffHand}
	warlockWeapons = []db.WeaponType{db.WeaponDagger, db.WeaponSword, db.WeaponStaff, db.WeaponOffHand}
)

// casterProfile builds a spell-damage-numeraire profile: spell damage
// is 1 EP, the spell hit cap hitPct before talents.
func casterProfile(name string, class db.Class, tree string, hitPct, epPerDPS float64,
	w map[db.Stat]float64, armor db.ArmorType, weapons []db.WeaponType, ranged []db.RangedWeaponType) *score.Profile {
	w[db.SpellPower] = 1
	layouts := casterHands
	for _, t := range weapons {
		if t == db.WeaponShield {
			layouts = []score.WeaponLayout{score.LayoutTwoHand, score.LayoutWeaponShield, score.LayoutWeaponHeld}
		}
	}
	return &score.Profile{
		Name: name, Class: class, Tree: tree,
		Weights:     weights(w),
		HitCap:      &score.HitCapSpec{Stat: db.MeleeHit, BasePct: hitPct},
		EPPerDPS:    epPerDPS,
		ArmorType:   armor,
		WeaponTypes: weapons,
		RangedTypes: ranged,
		Layouts:     layouts,
		Rules:       rules60,
	}
}

func rogueProfile(tree string, agi, hit, crit, haste, exp, arp, mhDPS, epPerDPS, expCap float64) *score.Profile {
	return &score.Profile{
		Name: tree + " Rogue", Class: db.ClassRogue, Tree: tree,
		Weights: weights(map[db.Stat]float64{
			db.Strength: 1.1, db.Agility: agi, db.AttackPower: 1,
			db.MeleeHit: hit, db.MeleeCrit: crit, db.MeleeHaste: haste,
			db.Expertise: exp, db.ArmorPenetration: arp,
		}),
		Caps:            []score.StatCap{{Stat: db.Expertise, Cap: expCap}},
		WeaponDPSWeight: mhDPS, EPPerDPS: epPerDPS,
		ArmorType: db.ArmorLeather,
		WeaponTypes: []db.WeaponType{db.WeaponAxe, db.WeaponDagger, db.WeaponFist,
			db.WeaponMace, db.WeaponSword},
		RangedTypes: physRanged, Layouts: dualWield, Rules: rules60,
	}
}
