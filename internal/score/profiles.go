package score

import (
	"strings"

	"github.com/vaclav-dvorak/twolk/internal/db"
)

// profiles is the registry of built-in build profiles, one per talent
// tree of every class (tree names exactly as the armory exports them).
//
// Weight provenance: every spec adopts the WoWSims default stat weights
// VERBATIM (simweights_gen.go, from ui/<spec>/sim.ts at the pinned
// commit) — DPS-derived for damage specs, HPS-derived for healers,
// survival/threat-derived for tanks, each in the sim's own
// normalization (weights are relative; re-anchoring everything to
// 1.0 AP would be meaningless for healers and tanks). Trees sharing a
// sim share its weights (BM/MM/SV all use the hunter sim's defaults).
//
// The exception is the FILL-TO-CAP stats — hit, and for melee DPS
// expertise — which take a pre-cap value instead (simW's second
// argument): a sim measured on gear already AT a cap reports what the
// points past it are worth, while the engine values these below the cap
// and applies the cap itself. Where those pre-cap slopes have been
// measured they are the sim's own numbers (see the block above
// meleeDPSCaps); the specs still carrying a guessed floor are listed
// there too. Either way the engine ends up cap-SEEKING — reach the cap,
// then stop — which is how these specs are played; see gemopt's
// expTarget for the matching gem-search dimension that prevents
// overshoot.
//
// Twin-stat note: single-role specs weight one side of a duplicated
// rating; hybrid sims (DK, ret, enhancement) deliberately weight BOTH
// sides — items carry the rating on both, and its full value is the
// sum of what each side contributes.
//
// Ambiguous trees pick the common PvE reading: Feral Combat = cat DPS,
// DK Blood/Frost = DPS (not tank), Subtlety/PvP-leaning trees get
// PvE-flavored weights anyway.
var profiles = []*Profile{
	// ---- Warrior ----
	{
		Name: "Arms Warrior", Class: db.ClassWarrior, Tree: "Arms",
		Weights:         simWWith("warrior", db.Stats{db.MeleeHit: armsHitEP, db.Expertise: armsExpertiseEP}),
		Caps:            meleeDPSCaps,
		Curves:          map[db.Stat]StatCurve{db.ArmorPenetration: armsArPCurve},
		HitCap:          &HitCapSpec{Stat: db.MeleeHit, BasePct: 8},
		WeaponDPSWeight: simMeleeDPS("warrior"), RangedDPSWeight: simRangedDPS("warrior"),
		ArmorType: db.ArmorPlate, WeaponTypes: plateWeapons, RangedTypes: physRanged,
		Layouts: twoHandOnly,
	},
	{
		Name: "Fury Warrior", Class: db.ClassWarrior, Tree: "Fury",
		Weights:         simWWith("warrior", db.Stats{db.MeleeHit: furyHitEP, db.Expertise: furyExpertiseEP}),
		Caps:            meleeDPSCaps,
		Curves:          map[db.Stat]StatCurve{db.ArmorPenetration: furyArPCurve},
		HitCap:          &HitCapSpec{Stat: db.MeleeHit, BasePct: 8},
		WeaponDPSWeight: simMeleeDPS("warrior"), RangedDPSWeight: simRangedDPS("warrior"),
		ArmorType: db.ArmorPlate, WeaponTypes: plateWeapons, RangedTypes: physRanged,
		Layouts: furyLayouts,
	},
	{
		Name: "Protection Warrior", Class: db.ClassWarrior, Tree: "Protection",
		Weights:         simW("protection_warrior", db.Stats{db.MeleeHit: 0.3}),
		Caps:            tankCaps,
		HitCap:          &HitCapSpec{Stat: db.MeleeHit, BasePct: 8},
		WeaponDPSWeight: simMeleeDPS("protection_warrior"), RangedDPSWeight: simRangedDPS("protection_warrior"),
		ArmorType: db.ArmorPlate, WeaponTypes: oneHanders, RangedTypes: physRanged,
		Layouts: shieldOnly,
	},

	// ---- Paladin ----
	{
		Name: "Holy Paladin", Class: db.ClassPaladin, Tree: "Holy",
		Weights:   simW("holy_paladin", db.Stats{}),
		ArmorType: db.ArmorPlate, WeaponTypes: plateWeapons, RangedTypes: relicOnly,
		Layouts: shieldOnly,
	},
	{
		Name: "Protection Paladin", Class: db.ClassPaladin, Tree: "Protection",
		Weights:         simW("protection_paladin", db.Stats{db.MeleeHit: 0.3}),
		Caps:            tankCaps,
		HitCap:          &HitCapSpec{Stat: db.MeleeHit, BasePct: 8},
		WeaponDPSWeight: simMeleeDPS("protection_paladin"), RangedDPSWeight: simRangedDPS("protection_paladin"),
		ArmorType: db.ArmorPlate, WeaponTypes: oneHanders, RangedTypes: relicOnly,
		Layouts: shieldOnly,
	},
	{
		Name: "Retribution Paladin", Class: db.ClassPaladin, Tree: "Retribution",
		Weights:         simWWith("retribution_paladin", db.Stats{db.MeleeHit: retHitEP, db.Expertise: retExpertiseEP}),
		Caps:            meleeDPSCaps,
		Curves:          map[db.Stat]StatCurve{db.ArmorPenetration: retArPCurve},
		HitCap:          &HitCapSpec{Stat: db.MeleeHit, BasePct: 8},
		WeaponDPSWeight: simMeleeDPS("retribution_paladin"), RangedDPSWeight: simRangedDPS("retribution_paladin"),
		ArmorType: db.ArmorPlate, WeaponTypes: plateWeapons, RangedTypes: relicOnly,
		Layouts: twoHandOnly,
	},

	// ---- Hunter ----
	{
		Name: "Beast Mastery Hunter", Class: db.ClassHunter, Tree: "Beast Mastery",
		Weights:         simWWith("hunter", db.Stats{db.MeleeHit: bmHitEP}),
		Caps:            rangedCaps,
		Curves:          map[db.Stat]StatCurve{db.ArmorPenetration: bmArPCurve},
		HitCap:          &HitCapSpec{Stat: db.MeleeHit, BasePct: 8},
		WeaponDPSWeight: simMeleeDPS("hunter"), RangedDPSWeight: simRangedDPS("hunter"),
		ArmorType: db.ArmorMail, WeaponTypes: hunterWeapons, RangedTypes: hunterRanged,
		Layouts: twoHandOrDual,
	},
	{
		Name: "Marksmanship Hunter", Class: db.ClassHunter, Tree: "Marksmanship",
		Weights:         simWWith("hunter", db.Stats{db.MeleeHit: mmHitEP}),
		Caps:            rangedCaps,
		Curves:          map[db.Stat]StatCurve{db.ArmorPenetration: mmArPCurve},
		HitCap:          &HitCapSpec{Stat: db.MeleeHit, BasePct: 8},
		WeaponDPSWeight: simMeleeDPS("hunter"), RangedDPSWeight: simRangedDPS("hunter"),
		ArmorType: db.ArmorMail, WeaponTypes: hunterWeapons, RangedTypes: hunterRanged,
		Layouts: twoHandOrDual,
	},
	{
		Name: "Survival Hunter", Class: db.ClassHunter, Tree: "Survival",
		Weights:         simWWith("hunter", db.Stats{db.MeleeHit: svHitEP}),
		Caps:            rangedCaps,
		Curves:          map[db.Stat]StatCurve{db.ArmorPenetration: svArPCurve},
		HitCap:          &HitCapSpec{Stat: db.MeleeHit, BasePct: 8},
		WeaponDPSWeight: simMeleeDPS("hunter"), RangedDPSWeight: simRangedDPS("hunter"),
		ArmorType: db.ArmorMail, WeaponTypes: hunterWeapons, RangedTypes: hunterRanged,
		Layouts: twoHandOrDual,
	},

	// ---- Rogue ----
	{
		Name: "Assassination Rogue", Class: db.ClassRogue, Tree: "Assassination",
		Weights:         simWWith("rogue", db.Stats{db.MeleeHit: rogueHitEP, db.Expertise: rogueExpertiseEP}),
		Caps:            meleeDPSCaps,
		Curves:          map[db.Stat]StatCurve{db.ArmorPenetration: rogueArPCurve},
		HitCap:          &HitCapSpec{Stat: db.MeleeHit, BasePct: 8},
		WeaponDPSWeight: simMeleeDPS("rogue"), RangedDPSWeight: simRangedDPS("rogue"),
		ArmorType: db.ArmorLeather, WeaponTypes: rogueWeapons, RangedTypes: physRanged,
		Layouts: dualWieldOnly,
	},
	{
		Name: "Combat Rogue", Class: db.ClassRogue, Tree: "Combat",
		Weights:         simWWith("rogue", db.Stats{db.MeleeHit: rogueHitEP, db.Expertise: rogueExpertiseEP}),
		Caps:            meleeDPSCaps,
		Curves:          map[db.Stat]StatCurve{db.ArmorPenetration: rogueArPCurve},
		HitCap:          &HitCapSpec{Stat: db.MeleeHit, BasePct: 8},
		WeaponDPSWeight: simMeleeDPS("rogue"), RangedDPSWeight: simRangedDPS("rogue"),
		ArmorType: db.ArmorLeather, WeaponTypes: rogueWeapons, RangedTypes: physRanged,
		Layouts: dualWieldOnly,
	},
	{
		Name: "Subtlety Rogue", Class: db.ClassRogue, Tree: "Subtlety",
		Weights:         simWWith("rogue", db.Stats{db.MeleeHit: rogueHitEP, db.Expertise: rogueExpertiseEP}),
		Caps:            meleeDPSCaps,
		Curves:          map[db.Stat]StatCurve{db.ArmorPenetration: rogueArPCurve},
		HitCap:          &HitCapSpec{Stat: db.MeleeHit, BasePct: 8},
		WeaponDPSWeight: simMeleeDPS("rogue"), RangedDPSWeight: simRangedDPS("rogue"),
		ArmorType: db.ArmorLeather, WeaponTypes: rogueWeapons, RangedTypes: physRanged,
		Layouts: dualWieldOnly,
	},

	// ---- Priest ----
	{
		Name: "Discipline Priest", Class: db.ClassPriest, Tree: "Discipline",
		Weights:   simW("healing_priest", db.Stats{}),
		ArmorType: db.ArmorCloth, WeaponTypes: casterWeapons, RangedTypes: wandOnly,
		Layouts: staffOrHeld,
	},
	{
		Name: "Holy Priest", Class: db.ClassPriest, Tree: "Holy",
		Weights:   simW("healing_priest", db.Stats{}),
		ArmorType: db.ArmorCloth, WeaponTypes: casterWeapons, RangedTypes: wandOnly,
		Layouts: staffOrHeld,
	},
	{
		Name: "Shadow Priest", Class: db.ClassPriest, Tree: "Shadow",
		Weights:   simWWith("shadow_priest", db.Stats{db.SpellHit: spriestSpellHitEP}),
		HitCap:    &HitCapSpec{Stat: db.SpellHit, BasePct: 17},
		ArmorType: db.ArmorCloth, WeaponTypes: casterWeapons, RangedTypes: wandOnly,
		Layouts: staffOrHeld,
	},

	// ---- Death Knight ----
	{
		Name: "Blood Death Knight", Class: db.ClassDeathKnight, Tree: "Blood",
		Weights: simWWith("deathknight", db.Stats{db.MeleeHit: dkHitEP, db.Expertise: dkExpertiseEP,
			db.Strength: dkStrengthEP, db.Agility: dkAgilityEP, db.MeleeCrit: dkCritEP,
			db.MeleeHaste: dkHasteEP, db.SpellHit: dkSpellHitEP}),
		Caps:            meleeDPSCaps,
		Curves:          map[db.Stat]StatCurve{db.ArmorPenetration: dkArPCurve},
		HitCap:          &HitCapSpec{Stat: db.MeleeHit, BasePct: 8},
		WeaponDPSWeight: simMeleeDPS("deathknight"), RangedDPSWeight: simRangedDPS("deathknight"),
		ArmorType: db.ArmorPlate, WeaponTypes: plateWeapons, RangedTypes: relicOnly,
		Layouts: twoHandOnly,
	},
	{
		Name: "Frost Death Knight", Class: db.ClassDeathKnight, Tree: "Frost",
		Weights: simWWith("deathknight", db.Stats{db.MeleeHit: dkHitEP, db.Expertise: dkExpertiseEP,
			db.Strength: dkStrengthEP, db.Agility: dkAgilityEP, db.MeleeCrit: dkCritEP,
			db.MeleeHaste: dkHasteEP, db.SpellHit: dkSpellHitEP}),
		Caps:            meleeDPSCaps,
		Curves:          map[db.Stat]StatCurve{db.ArmorPenetration: dkArPCurve},
		HitCap:          &HitCapSpec{Stat: db.MeleeHit, BasePct: 8},
		WeaponDPSWeight: simMeleeDPS("deathknight"), RangedDPSWeight: simRangedDPS("deathknight"),
		ArmorType: db.ArmorPlate, WeaponTypes: plateWeapons, RangedTypes: relicOnly,
		Layouts: twoHandOrDual, // dual-wield frost vs 2H solved side by side
	},
	{
		Name: "Unholy Death Knight", Class: db.ClassDeathKnight, Tree: "Unholy",
		Weights: simWWith("deathknight", db.Stats{db.MeleeHit: dkHitEP, db.Expertise: dkExpertiseEP,
			db.Strength: dkStrengthEP, db.Agility: dkAgilityEP, db.MeleeCrit: dkCritEP,
			db.MeleeHaste: dkHasteEP, db.SpellHit: dkSpellHitEP}),
		Caps:            meleeDPSCaps,
		Curves:          map[db.Stat]StatCurve{db.ArmorPenetration: dkArPCurve},
		HitCap:          &HitCapSpec{Stat: db.MeleeHit, BasePct: 8},
		WeaponDPSWeight: simMeleeDPS("deathknight"), RangedDPSWeight: simRangedDPS("deathknight"),
		ArmorType: db.ArmorPlate, WeaponTypes: plateWeapons, RangedTypes: relicOnly,
		Layouts: twoHandOnly,
	},

	// ---- Shaman ----
	{
		Name: "Elemental Shaman", Class: db.ClassShaman, Tree: "Elemental",
		Weights:   simWWith("elemental_shaman", db.Stats{db.SpellHit: eleSpellHitEP}),
		HitCap:    &HitCapSpec{Stat: db.SpellHit, BasePct: 17},
		ArmorType: db.ArmorMail, WeaponTypes: shamanWeapons, RangedTypes: relicOnly,
		Layouts: staffOrShield,
	},
	{
		Name: "Enhancement Shaman", Class: db.ClassShaman, Tree: "Enhancement",
		Weights:         simWWith("enhancement_shaman", db.Stats{db.MeleeHit: enhHitEP, db.Expertise: enhExpertiseEP}),
		Caps:            meleeDPSCaps,
		Curves:          map[db.Stat]StatCurve{db.ArmorPenetration: enhArPCurve},
		HitCap:          &HitCapSpec{Stat: db.MeleeHit, BasePct: 8},
		WeaponDPSWeight: simMeleeDPS("enhancement_shaman"), RangedDPSWeight: simRangedDPS("enhancement_shaman"),
		ArmorType: db.ArmorMail, WeaponTypes: shamanWeapons, RangedTypes: relicOnly,
		Layouts: dualWieldOnly,
	},
	{
		Name: "Restoration Shaman", Class: db.ClassShaman, Tree: "Restoration",
		Weights:   simW("restoration_shaman", db.Stats{}),
		ArmorType: db.ArmorMail, WeaponTypes: shamanWeapons, RangedTypes: relicOnly,
		Layouts: staffOrShield,
	},

	// ---- Mage ----
	{
		Name: "Arcane Mage", Class: db.ClassMage, Tree: "Arcane",
		Weights:   simWWith("mage", db.Stats{db.SpellHit: mageSpellHitEP}),
		HitCap:    &HitCapSpec{Stat: db.SpellHit, BasePct: 17},
		ArmorType: db.ArmorCloth, WeaponTypes: mageWeapons, RangedTypes: wandOnly,
		Layouts: staffOrHeld,
	},
	{
		Name: "Fire Mage", Class: db.ClassMage, Tree: "Fire",
		Weights:   simWWith("mage", db.Stats{db.SpellHit: mageSpellHitEP}),
		HitCap:    &HitCapSpec{Stat: db.SpellHit, BasePct: 17},
		ArmorType: db.ArmorCloth, WeaponTypes: mageWeapons, RangedTypes: wandOnly,
		Layouts: staffOrHeld,
	},
	{
		Name: "Frost Mage", Class: db.ClassMage, Tree: "Frost",
		Weights:   simWWith("mage", db.Stats{db.SpellHit: mageSpellHitEP}),
		HitCap:    &HitCapSpec{Stat: db.SpellHit, BasePct: 17},
		ArmorType: db.ArmorCloth, WeaponTypes: mageWeapons, RangedTypes: wandOnly,
		Layouts: staffOrHeld,
	},

	// ---- Warlock ----
	{
		Name: "Affliction Warlock", Class: db.ClassWarlock, Tree: "Affliction",
		Weights:   simWWith("warlock", db.Stats{db.SpellHit: warlockSpellHitEP}),
		HitCap:    &HitCapSpec{Stat: db.SpellHit, BasePct: 17},
		ArmorType: db.ArmorCloth, WeaponTypes: mageWeapons, RangedTypes: wandOnly,
		Layouts: staffOrHeld,
	},
	{
		Name: "Demonology Warlock", Class: db.ClassWarlock, Tree: "Demonology",
		Weights:   simWWith("warlock", db.Stats{db.SpellHit: warlockSpellHitEP}),
		HitCap:    &HitCapSpec{Stat: db.SpellHit, BasePct: 17},
		ArmorType: db.ArmorCloth, WeaponTypes: mageWeapons, RangedTypes: wandOnly,
		Layouts: staffOrHeld,
	},
	{
		Name: "Destruction Warlock", Class: db.ClassWarlock, Tree: "Destruction",
		Weights:   simWWith("warlock", db.Stats{db.SpellHit: warlockSpellHitEP}),
		HitCap:    &HitCapSpec{Stat: db.SpellHit, BasePct: 17},
		ArmorType: db.ArmorCloth, WeaponTypes: mageWeapons, RangedTypes: wandOnly,
		Layouts: staffOrHeld,
	},

	// ---- Druid ----
	{
		Name: "Balance Druid", Class: db.ClassDruid, Tree: "Balance",
		Weights:   simWWith("balance_druid", db.Stats{db.SpellHit: balanceSpellHitEP}),
		HitCap:    &HitCapSpec{Stat: db.SpellHit, BasePct: 17},
		ArmorType: db.ArmorLeather, WeaponTypes: druidWeapons, RangedTypes: relicOnly,
		Layouts: staffOrHeld,
	},
	{
		Name: "Feral Druid", Class: db.ClassDruid, Tree: "Feral Combat",
		Weights:         simWWith("feral_druid", db.Stats{db.MeleeHit: feralHitEP, db.Expertise: feralExpertiseEP}),
		Caps:            meleeDPSCaps,
		Curves:          map[db.Stat]StatCurve{db.ArmorPenetration: feralArPCurve},
		HitCap:          &HitCapSpec{Stat: db.MeleeHit, BasePct: 8},
		WeaponDPSWeight: simMeleeDPS("feral_druid"), RangedDPSWeight: simRangedDPS("feral_druid"),
		ArmorType: db.ArmorLeather, WeaponTypes: druidWeapons, RangedTypes: relicOnly,
		Layouts: twoHandOnly,
	},
	{
		Name: "Restoration Druid", Class: db.ClassDruid, Tree: "Restoration",
		Weights:   simW("restoration_druid", db.Stats{}),
		ArmorType: db.ArmorLeather, WeaponTypes: druidWeapons, RangedTypes: relicOnly,
		Layouts: staffOrHeld,
	},
}

// ArP CURVES, measured per spec with tools/eptool (sessions 32/36).
// Armor penetration's marginal value RISES with the character's own ArP
// total, and the convexity belongs to the TARGET, not the attacker:
// damage taken is A/(A+K), so every point of ArP strips a larger share
// of the armor that is left. That makes the SHAPE the same for every
// physical spec — nine specs swept independently, each normalized to
// its own value at 400 rating, agree inside ~5% across the range:
//
//	ArP total        200   400   600   800  1000  1200
//	Fury Warrior    0.93  1.00  1.09  1.14  1.24  1.34
//	Arms Warrior    0.94  1.00  1.08  1.18  1.27  1.33
//	Marksmanship    0.93  1.00  1.09  1.18  1.28  1.40
//	Beast Mastery   0.93  1.00  1.08  1.16  1.26  1.38
//	Survival        0.92  1.00  1.09  1.20  1.32  1.46
//	Retribution     0.92  1.00  1.08  1.19  1.31  1.45
//	Combat Rogue    0.93  1.00  1.07  1.16  1.25  1.36
//	Enhancement     0.92  1.00  1.08  1.18  1.30  1.43
//	Feral / DK      0.94  1.00  1.08   ——  proc breakpoint, see below
//
// Only the LEVEL differs, set by how much of a spec's damage the armor
// actually touches: fury runs 2.56 → 3.66 EP/point over that range,
// enhancement 0.80 → 1.24. That spread is why each spec ships its own
// points; the shape agreement is what says the measurements are sound. So each spec ships its own measured points
// rather than a shared shape — the shape agreement is the CHECK that
// the measurements are sound, not a modeling shortcut.
//
// Past the last point the slope holds flat to the 1400 hard cap, where
// the cap zeroes it. Readings nearer the cap straddle it and come out
// low, so they are not shipped.
//
// PROC BREAKPOINT — the one place this model is knowingly incomplete: a
// passive ArP PROC in the build caps the useful passive total well below
// 1400, and the engine does not model that yet (plan.md task 4b). The
// feral sweep shows it precisely, because its preset carries Mjolnir
// Runestone (+751 ArP): a clean convex rise to 600 passive, then a
// collapse to 71% of trend at 800 — 751 + 800 = 1551 against a 1400 cap,
// so the proc's window wastes the surplus — and a resumed rise after.
// Frost DK's preset carries Mjolnir too. Both therefore ship the
// proc-FREE curve (their clean points, extended on the pooled shape),
// which over-values passive ArP above ~650 for a character actually
// wearing one of those trinkets.
//
// Harness for all of it: wowsims/wotlk at the pinned commit, driven
// headless (protoc-gen the protos, build -tags with_db, call
// core.StatWeights — the sim's own EP routine, normalized to the same
// stat the shipped table uses), each spec's own test preset, 15k
// iterations (30k for the warrior), seed 101, 300s single target, full
// raid buffs/debuffs. Player.BonusStats sweeps the character along the
// stat so every point is a LOCAL slope at a known total.
var (
	furyArPCurve = StatCurve{
		{At: 199, Weight: 2.56}, {At: 399, Weight: 2.74}, {At: 599, Weight: 2.99},
		{At: 799, Weight: 3.12}, {At: 999, Weight: 3.40}, {At: 1199, Weight: 3.66},
	}
	armsArPCurve = StatCurve{
		{At: 200, Weight: 2.45}, {At: 400, Weight: 2.61}, {At: 600, Weight: 2.82},
		{At: 800, Weight: 3.09}, {At: 1000, Weight: 3.31}, {At: 1200, Weight: 3.48},
	}
	bmArPCurve = StatCurve{
		{At: 200, Weight: 1.76}, {At: 400, Weight: 1.89}, {At: 600, Weight: 2.04},
		{At: 800, Weight: 2.20}, {At: 1000, Weight: 2.39}, {At: 1200, Weight: 2.61},
	}
	mmArPCurve = StatCurve{
		{At: 200, Weight: 1.64}, {At: 400, Weight: 1.76}, {At: 600, Weight: 1.91},
		{At: 800, Weight: 2.07}, {At: 1000, Weight: 2.25}, {At: 1200, Weight: 2.47},
	}
	svArPCurve = StatCurve{
		{At: 200, Weight: 1.12}, {At: 400, Weight: 1.22}, {At: 600, Weight: 1.33},
		{At: 800, Weight: 1.46}, {At: 1000, Weight: 1.61}, {At: 1200, Weight: 1.78},
	}
	retArPCurve = StatCurve{
		{At: 200, Weight: 0.87}, {At: 400, Weight: 0.95}, {At: 600, Weight: 1.03},
		{At: 800, Weight: 1.13}, {At: 1000, Weight: 1.24}, {At: 1200, Weight: 1.38},
	}
	// feral and frost DK: measured clean to 600, then extended on the
	// pooled shape (×1.175 / ×1.280 / ×1.393 of the 400 anchor) because
	// their presets carry Mjolnir Runestone and the raw readings past
	// 600 are the proc breakpoint, not the slope — see the note above.
	feralArPCurve = StatCurve{
		{At: 200, Weight: 2.82}, {At: 400, Weight: 3.01}, {At: 600, Weight: 3.24},
		{At: 800, Weight: 3.54}, {At: 1000, Weight: 3.85}, {At: 1200, Weight: 4.19},
	}
	rogueArPCurve = StatCurve{
		{At: 200, Weight: 1.41}, {At: 400, Weight: 1.51}, {At: 600, Weight: 1.62},
		{At: 800, Weight: 1.75}, {At: 1000, Weight: 1.89}, {At: 1200, Weight: 2.05},
	}
	enhArPCurve = StatCurve{
		{At: 200, Weight: 0.80}, {At: 400, Weight: 0.87}, {At: 600, Weight: 0.94},
		{At: 800, Weight: 1.03}, {At: 1000, Weight: 1.13}, {At: 1200, Weight: 1.24},
	}
	dkArPCurve = StatCurve{
		{At: 200, Weight: 2.27}, {At: 400, Weight: 2.46}, {At: 600, Weight: 2.67},
		{At: 800, Weight: 2.89}, {At: 1000, Weight: 3.15}, {At: 1200, Weight: 3.43},
	}
)

// Fill-to-cap slopes, MEASURED below each spec's cap with tools/eptool
// (sessions 32/33/35). These are the numbers ui/<spec>/sim.ts
// structurally cannot supply: its weights are measured on preset gear
// that already sits AT the hit and expertise caps, so they report what
// the points PAST the cap are worth, while the engine values these
// stats BELOW the cap and applies the cap itself. Sweeping the sim's
// own preset off the stat gives the slope that belongs there.
//
// Every WotLK melee and ranged DPS spec is now measured. 30k iterations
// for the warrior and hunter sims, 15k for the rest, three totals clear
// of each cap; "was" is the EFFECTIVE weight before, i.e. the published
// weight or the old guessed floor, whichever won:
//
//	spec                    hit   was    expertise   was
//	Fury Warrior           4.53   1.80        3.73  3.00
//	Arms Warrior           4.44   1.90        1.50  2.55
//	Beast Mastery Hunter   5.06   2.30           –     –
//	Marksmanship Hunter    4.13   2.30           –     –
//	Survival Hunter        5.74   2.80           –     –
//	Feral Druid            3.60   2.51        3.40  2.44
//	Enhancement Shaman     2.65   1.90        2.26  1.80
//	Combat Rogue           2.18   2.00        1.69  2.00
//	Retribution Paladin    2.07   1.96        1.60  2.70
//	Frost Death Knight     1.67   1.92        1.29  3.40
//
// Three things the measurements settle, none of which a guess got right:
//
//   - Hit's pre-cap value is SPEC-SPECIFIC and spans 3.4× — hunters and
//     warriors 4.1–5.7, feral 3.6, enhancement 2.7, rogue 2.2, ret 2.1,
//     frost DK 1.7. The specs at the bottom lean on damage that cannot
//     miss (diseases, Censure), so a missed swing costs them less. Any
//     single "hit floor" convention is wrong for most of the roster.
//   - A measurement may land BELOW the published weight (DK hit, ret
//     expertise), so measured specs use simWWith (replace), not simW
//     (max). Arms shipped 1.50 expertise in session 33 that the engine
//     never used because max() kept the table's 2.55.
//   - Session 14's "expertise floor above Strength so the engine seeks
//     the dodge cap" was a heuristic, and for half the roster it is
//     backwards: frost DK measures 1.29 against a 3.40 floor and ret
//     1.60 against 2.70. Only fury (3.73) and feral (3.40) genuinely
//     want expertise above their primary stat.
//
// Sweeps must land clear of the spec's TALENT-REDUCED cap: rogue, DK,
// ret and enhancement presets all sit above their own caps (rogue 419
// hit rating, enhancement 571), and sweeping there measures the cap
// rather than the slope — the first pass at those four read 1.4–1.9 for
// exactly that reason. The per-tree numbers are per SIM where only one
// tree was swept (rogue's three trees share the "rogue" sim and its
// measurement, DK's three share "deathknight"); hunters were swept per
// tree because each has its own preset and rotation.
//
// Readings at or just under a cap come out low (the sim's finite
// difference straddles it) and are not shipped — that is the cap, which
// the engine already models.
// Caster spell hit, measured the same way (session 38) but by DIRECT
// DPS differences against spell power — the numeraire its published
// tables already use, so the numbers land in their units untouched.
// Session 37 refused to ship these and was right to at the time; what
// the sim's own EP routine gets wrong for casters is INTELLECT (2.51
// against a hand-measured 0.34), not hit, and for hit the two methods
// agree inside 5% on every spec:
//
//	spec             direct  StatWeights  published   was
//	Balance Druid      3.46         3.13          –  1.60
//	Elemental          2.99         2.84          –  1.60
//	Mage               2.77         2.74       0.38  1.40
//	Warlock            1.87         1.82       0.93  1.70
//	Shadow Priest      1.65         1.60       0.87  1.50
//
// Same story as every other fill-to-cap stat: the published number is
// what hit is worth AT the cap, the engine needs its value below one,
// and the floors standing in for it were roughly half the truth.
// Measured on one tree per sim and adopted by its siblings, exactly as
// the weights themselves already are.
const (
	mageSpellHitEP    = 2.77
	warlockSpellHitEP = 1.87
	spriestSpellHitEP = 1.65
	eleSpellHitEP     = 2.99
	balanceSpellHitEP = 3.46
)

const (
	furyHitEP        = 4.53
	furyExpertiseEP  = 3.73
	armsHitEP        = 4.44
	armsExpertiseEP  = 1.50
	bmHitEP          = 5.06
	mmHitEP          = 4.13
	svHitEP          = 5.74
	rogueHitEP       = 2.18
	rogueExpertiseEP = 1.69
	dkHitEP          = 1.67
	dkExpertiseEP    = 1.29
	// the rest of the DK table, re-derived: its published sim.ts entry
	// does not describe its own preset (see the note above)
	dkStrengthEP     = 2.79
	dkAgilityEP      = 1.49
	dkCritEP         = 1.76
	dkHasteEP        = 1.68
	dkSpellHitEP     = 0.58
	retHitEP         = 2.07
	retExpertiseEP   = 1.60
	enhHitEP         = 2.65
	enhExpertiseEP   = 2.26
	feralHitEP       = 3.60
	feralExpertiseEP = 3.40
)

// Shared cap sets. Expertise soft cap = 26 expertise (dodge) ≈ 214
// rating; melee specs care, ranged/casters don't.
var (
	meleeDPSCaps = []StatCap{{Stat: db.ArmorPenetration, Cap: 1400}, {Stat: db.Expertise, Cap: 214}}
	rangedCaps   = []StatCap{{Stat: db.ArmorPenetration, Cap: 1400}}
	tankCaps     = []StatCap{{Stat: db.Defense, Cap: 689}, {Stat: db.Expertise, Cap: 214}} // 540 defense: crit immune
)

// Shared weapon/relic proposal filters.
var (
	plateWeapons  = []db.WeaponType{db.WeaponAxe, db.WeaponMace, db.WeaponSword, db.WeaponPolearm}
	oneHanders    = []db.WeaponType{db.WeaponAxe, db.WeaponMace, db.WeaponSword, db.WeaponDagger, db.WeaponFist}
	hunterWeapons = []db.WeaponType{db.WeaponAxe, db.WeaponDagger, db.WeaponFist, db.WeaponPolearm, db.WeaponStaff, db.WeaponSword}
	rogueWeapons  = []db.WeaponType{db.WeaponDagger, db.WeaponFist, db.WeaponMace, db.WeaponSword, db.WeaponAxe}
	shamanWeapons = []db.WeaponType{db.WeaponAxe, db.WeaponMace, db.WeaponFist, db.WeaponDagger, db.WeaponStaff}
	druidWeapons  = []db.WeaponType{db.WeaponMace, db.WeaponStaff, db.WeaponPolearm, db.WeaponDagger, db.WeaponFist}
	casterWeapons = []db.WeaponType{db.WeaponMace, db.WeaponDagger, db.WeaponStaff}
	mageWeapons   = []db.WeaponType{db.WeaponSword, db.WeaponDagger, db.WeaponStaff}

	hunterRanged = []db.RangedWeaponType{db.RangedBow, db.RangedCrossbow, db.RangedGun}
	physRanged   = []db.RangedWeaponType{db.RangedBow, db.RangedCrossbow, db.RangedGun, db.RangedThrown}
	wandOnly     = []db.RangedWeaponType{db.RangedWand}
	relicOnly    = []db.RangedWeaponType{db.RangedIdol, db.RangedLibram, db.RangedTotem, db.RangedSigil}
)

// Shared weapon-layout tables (see WeaponLayout). Layouts a build CAN
// play, not what's currently meta — BestSet solves each and keeps the
// winner, so a dominated layout costs solve time, never correctness.
var (
	twoHandOnly   = []WeaponLayout{LayoutTwoHand}
	dualWieldOnly = []WeaponLayout{LayoutDualWield}
	twoHandOrDual = []WeaponLayout{LayoutTwoHand, LayoutDualWield}
	furyLayouts   = []WeaponLayout{LayoutTitanGrip, LayoutDualWield}
	shieldOnly    = []WeaponLayout{LayoutWeaponShield}
	staffOrShield = []WeaponLayout{LayoutTwoHand, LayoutWeaponShield}
	staffOrHeld   = []WeaponLayout{LayoutTwoHand, LayoutWeaponHeld}
)

// Slug converts a display name to a stable machine identifier for
// config keys: lowercase, word runs joined by "_" ("Beast Mastery" →
// "beast_mastery", "Focused Aim" → "focused_aim"). Display strings can
// then change (or be translated) without orphaning saved settings.
func Slug(name string) string {
	var b []rune
	pending := false
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			if pending && len(b) > 0 {
				b = append(b, '_')
			}
			pending = false
			b = append(b, r)
		default:
			pending = true
		}
	}
	return string(b)
}

// Key returns the profile's stable config key, e.g.
// "wotlk/hunter/survival" — game-prefixed so same-named specs of
// different versions never share user overrides.
// SetBonusAudience is what a role-scoped set bonus (SetBonus.Role)
// matches against: the profile's role, or its tree's slug when it has
// none.
func (p *Profile) SetBonusAudience() string {
	if p.Role != "" {
		return p.Role
	}
	return Slug(p.Tree)
}

func (p *Profile) Key() string {
	k := p.rules().Game + "/" + p.Class.Slug() + "/" + Slug(p.Tree)
	if p.Role != "" {
		k += "/" + p.Role
	}
	return k
}

// simW composes a profile's stat weights: the WoWSims sim defaults
// verbatim, raised to per-stat floors where given (the hit-cap stat's
// pre-cap value — see the registry comment).
func simW(sim string, floors db.Stats) db.Stats {
	w := simWeights[sim].Stats
	for st, f := range floors {
		if f > w[st] {
			w[st] = f
		}
	}
	return w
}

// simWWith is simW for specs whose fill-to-cap stats have been MEASURED
// below their cap (see the block above meleeDPSCaps): the measurement
// REPLACES the published weight instead of flooring it. It has to —
// a measurement may land below the table (frost DK reads 1.67 hit
// against a published 1.92, ret 1.60 expertise against 1.80, because
// the published number was itself taken at some other gear point), and
// simW's max() would silently discard exactly those. Arms shipped a
// measured 1.50 expertise in session 33 that the engine never used for
// that reason.
func simWWith(sim string, measured db.Stats) db.Stats {
	w := simWeights[sim].Stats
	for st, v := range measured {
		if v != 0 {
			w[st] = v
		}
	}
	return w
}

// simMeleeDPS is the sim's EP per point of melee weapon DPS; dual-wield
// sims price main and off hand separately, averaged here (the profile
// carries one weight for both hands).
func simMeleeDPS(sim string) float64 {
	s := simWeights[sim]
	if s.OH > 0 {
		return (s.MH + s.OH) / 2
	}
	return s.MH
}

// simRangedDPS is the sim's EP per point of ranged weapon DPS.
func simRangedDPS(sim string) float64 {
	return simWeights[sim].Ranged
}

// Find returns the profile for a class and talent tree, or nil if the
// build isn't supported yet.
func Find(class db.Class, tree string) *Profile {
	return FindIn(profiles, class, tree)
}

// FindIn resolves a class/tree build in any version's profile set —
// the game modules keep their own sets and share the lookup.
func FindIn(profiles []*Profile, class db.Class, tree string) *Profile {
	for _, p := range profiles {
		if p.Class == class && p.Tree == tree {
			return p
		}
	}
	return nil
}
