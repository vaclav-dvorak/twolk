package cata

import (
	"time"

	"github.com/vaclav-dvorak/twolk/internal/db"
	"github.com/vaclav-dvorak/twolk/internal/score"
)

// rules85 is Cataclysm's scoring rule set. Rating conversions are the
// level-85 values from wowsims/cata ui/core/constants/mechanics.ts @
// the pinned commit. Gems assume the RARE tier — the endgame cuts of
// launch content (Inferno Ruby et al.); flip to epic when the server
// unlocks 4.3 (Queen's Garnet). Weapon racials verified against
// wowsims sim/core/racials.go: Cata trimmed expertise racials to +3
// and added dwarf maces and gnome shortblades.
var rules85 = &score.Rules{
	Game:                 "cata",
	MeleeHitRatingPerPct: 120.1088,
	SpellHitRatingPerPct: 102.44574,
	RatingPerExpertise:   30.027197,
	CritRatingPerPct:     179.28004,
	GemQuality:           db.QualityRare,
	MetaQuality:          db.QualityRare,
	Reforging:            true,
	SetBonuses:           setBonuses,
	// The 4.3 talent revamp left only three flat cap talents (from
	// the wowsims sim implementations @ the pin); hunters have none.
	// The caster spirit→hit conversions (Balance of Power, Twisted
	// Faith, Elemental Precision) are a different mechanic — stat
	// dependencies — and come with the caster profiles. API imports
	// carry exact ranks (SpellIDs), so nothing is ever assumed here.
	CapTalents: map[db.Class][]score.CapTalent{
		db.ClassRogue: {{
			Name: "Precision", Tree: "Combat", Max: 3, ReqPoints: 0,
			Stat: db.MeleeHit, RatingPerPoint: 2 * 120.1088, Per: "2% hit",
			SpellIDs: []int{13705, 13832, 13843},
		}},
		db.ClassShaman: {{
			Name: "Unleashed Rage", Tree: "Enhancement", Max: 2, ReqPoints: 20,
			Stat: db.Expertise, RatingPerPoint: 4 * rulesExpertise, Per: "4 expertise",
			SpellIDs: []int{30802, 30808},
		}},
		db.ClassDeathKnight: {{
			Name: "Nerves of Cold Steel", Tree: "Frost", Max: 3, ReqPoints: 0,
			Stat: db.MeleeHit, RatingPerPoint: 120.1088, Per: "1% hit (1H)",
			SpellIDs: []int{49226, 50137, 50138},
		}},
	},
	// tree tab order did not change WotLK -> Cata; the table is shared
	TreeOrder: score.ClassicTreeOrder,
	// No spell-hit raid debuff: 4.0.1 removed the hit from Misery and
	// Improved Faerie Fire on purpose ("a hit cap that isn't variable
	// depending on group composition") — cata casters gear the full 17%.
	SpellHitDebuffPct: 0,
	WeaponRacials:     weaponRacials,
	BuckleName:        "Ebonsteel Belt Buckle",
	// The stats cata data ACTUALLY carries: unified ratings on the
	// melee slots, no spell twins, none of the stats 4.0 removed from
	// gear. The proto reserves an MP5 slot but nothing real sets it
	// (0 items, 0 gems, one legacy wotlk enchant) — excluded, so the
	// weights editor doesn't offer a dead stat.
	Stats: []db.Stat{
		db.Strength, db.Agility, db.Stamina, db.Intellect, db.Spirit,
		db.MeleeHit, db.MeleeCrit, db.MeleeHaste, db.Expertise,
		db.Dodge, db.Parry, db.Mastery, db.AttackPower,
		db.RangedAttackPower, db.SpellPower, db.SpellPenetration,
		db.Resilience, db.ArcaneResistance, db.FireResistance,
		db.FrostResistance, db.NatureResistance, db.ShadowResistance,
		db.Armor, db.BonusArmor, db.Health, db.Mana,
	},
}

var weaponRacials = []score.WeaponRacial{
	{
		Race: "Orc", Note: "Orc racial: +3 expertise with axes and fist weapons",
		Match: score.MatchWeapon(db.WeaponAxe, db.WeaponFist),
		Stats: score.StatOf(db.Expertise, 3*rulesExpertise),
	},
	{
		Race: "Human", Note: "Human racial: +3 expertise with swords and maces",
		Match: score.MatchWeapon(db.WeaponSword, db.WeaponMace),
		Stats: score.StatOf(db.Expertise, 3*rulesExpertise),
	},
	{
		Race: "Dwarf", Note: "Dwarf racial: +3 expertise with maces",
		Match: score.MatchWeapon(db.WeaponMace),
		Stats: score.StatOf(db.Expertise, 3*rulesExpertise),
	},
	{
		Race: "Gnome", Note: "Gnome racial: +3 expertise with daggers and one-hand swords",
		Match: func(it *db.Item) bool {
			return it.Slot == db.SlotWeapon && (it.WeaponType == db.WeaponDagger ||
				(it.WeaponType == db.WeaponSword && it.HandType != db.HandTwoHand))
		},
		Stats: score.StatOf(db.Expertise, 3*rulesExpertise),
	},
	{
		Race: "Troll", Note: "Troll racial: +1% crit with bows",
		Match: score.MatchRanged(db.RangedBow),
		Stats: score.StatOf(db.MeleeCrit, rulesCrit),
	},
	{
		Race: "Dwarf", Note: "Dwarf racial: +1% crit with guns",
		Match: score.MatchRanged(db.RangedGun),
		Stats: score.StatOf(db.MeleeCrit, rulesCrit),
	},
}

// hunterEPPerDPS and hunterProcInterval are MEASURED against the pinned
// wowsims/cata build, driven headless on the ui/hunter/survival p1_sv
// preset (RunRaidSimConcurrent, 50k iterations, seed 101, 300s single
// target) — see the sim-comparison test in this package.
//
// EPPerDPS: a flat +1000 ranged attack power on that preset is worth
// 1136.5 DPS, so one point of RAP — the hunter weights' numeraire — buys
// 1.136 DPS and one raw DPS is worth 1/1.136 = 0.880 EP. This is the
// P1 figure; the same sweep gives 0.816 at p3_sv and 0.780 at p4_sv,
// because flat damage does not scale while the character does. Revisit
// alongside the preset weights when the server's tier moves.
//
// ProcInterval: an SV hunter lands ~1.2 melee-or-ranged attacks per
// second, so a proc gets a chance roughly every 0.83s — well under the
// 2s engine default, which was costing every chance-based proc a third
// of its uptime. Two independent readings of the same preset agree:
// Darkmoon Card: Hurricane's PPM-1 proc fires 3.60 times per minute
// (a 3.0-speed bow rolls weaponSpeed*PPM/60 = 5% per landed attack →
// 72 attacks/min), and Key to the Endless Chamber's 10%-chance proc
// fires 3.99 times per 300s against a 75s ICD, implying an ~8.3s wait
// for a 10% roll. Both give ~1.2 attacks/sec.
const (
	hunterEPPerDPS     = 0.880
	hunterProcInterval = 830 * time.Millisecond
)

// racial conversion shorthands (keep in sync with rules85)
const (
	rulesExpertise = 30.027197
	rulesCrit      = 179.28004
)

// Hunter proposal filters: mail, every huntable weapon type, real
// ranged weapons (cata has no hunter melee rotation — the melee weapon
// is a stat stick, hence WeaponDPSWeight 0).
var (
	hunterWeapons = []db.WeaponType{db.WeaponAxe, db.WeaponDagger, db.WeaponFist,
		db.WeaponPolearm, db.WeaponStaff, db.WeaponSword}
	hunterRanged  = []db.RangedWeaponType{db.RangedBow, db.RangedCrossbow, db.RangedGun}
	twoHandOrDual = []score.WeaponLayout{score.LayoutTwoHand, score.LayoutDualWield}
)

// hunterProfile builds one hunter spec's profile from the wowsims P1
// preset weights (ui/hunter/<spec>/presets.ts @ the pin — matching the
// server's current tier), with ONE stat measured rather than adopted:
// the pre-cap value of hit.
//
// The old convention floored hit just above agility (SV 3.5 against
// agility 3.27) on the reasoning that the presets sit AT the 8% cap so
// their published hit is deflated. The floor's DIRECTION was right and
// its size was not: measured below the cap, cata hit is worth about
// HALF of agility, not slightly more.
//
// Measured by direct DPS difference rather than the sim's EP routine
// (session 38 — that routine was caught reporting numbers a hand
// measurement does not reproduce), and expressed as a RATIO to agility
// so it lands in the same units as the rest of this table even though
// the harness's absolute basis does not match the published one:
//
//	SV at 316 hit (cap 961): +300 hit = +107.0 DPS, +300 agility =
//	+201.3 DPS → hit = 0.53 agility = 1.73 in this table's units.
//	SV at 616 hit: 0.52 — flat below the cap, as miss reduction must be.
//	MM at 316 hit: 0.72 agility = 2.20; BM 0.64 = 1.70. Marksmanship
//	leans hardest on shots that can miss, beast mastery least (a good
//	share of its damage is the pet's, which the hunter's hit does not
//	touch) — a spread a single convention floor could never express.
//	All three land near HALF what the convention guessed.
//
// It is physically coherent: 300 rating removes ~2.5% of special-attack
// misses and bought 2.17% DPS. The engine stays cap-seeking anyway —
// 1.73 still beats haste (0.89) and mastery (0.88) as a reforge target,
// so the cap gets filled by REFORGING while gems go to agility, which
// is how the spec is actually played.
func hunterProfile(tree string, weights db.Stats, rangedDPS float64) *score.Profile {
	return &score.Profile{
		Name: tree + " Hunter", Class: db.ClassHunter, Tree: tree,
		Weights:         weights,
		HitCap:          &score.HitCapSpec{Stat: db.MeleeHit, BasePct: 8},
		RangedDPSWeight: rangedDPS,
		EPPerDPS:        hunterEPPerDPS,
		ProcInterval:    hunterProcInterval,
		ArmorType:       db.ArmorMail,
		WeaponTypes:     hunterWeapons,
		RangedTypes:     hunterRanged,
		Layouts:         twoHandOrDual,
		Rules:           rules85,
	}
}

func hunterWeights(stam, agi, rap, hit, crit, haste, mastery float64) db.Stats {
	var w db.Stats
	w[db.Stamina] = stam
	w[db.Agility] = agi
	w[db.RangedAttackPower] = rap
	// NO AttackPower weight: an in-game "+X Attack Power" line grants
	// melee and ranged AP alike, and the dump encodes it into BOTH stat
	// slots (Landslide 1000/1000, Unheeded Warning 1926/1926). The
	// wowsims convention is that class weights zero the slot that does
	// not apply — the wotlk hunter preset carries only
	// RangedAttackPower too. Weighting both double-counts every AP
	// source; RAP-only sources (X-Ray Scope) land in the RAP slot.
	w[db.MeleeHit] = hit // unified rating: cata data decodes into the melee slots
	w[db.MeleeCrit] = crit
	w[db.MeleeHaste] = haste
	w[db.Mastery] = mastery
	return w
}

// profiles lists the supported cata builds. P1 sim weights with the
// hit floor raised above agility (fill-to-cap convention, see
// hunterProfile).
//
// SV's fabled "757 haste soft cap" (fit 3 Cobras between Explosive
// Shots) is deliberately NOT modeled: a mean-DPS sweep on the pinned
// sim (P1 preset, 60k iterations/point, bonus haste −800..+300) shows
// DPS linear in haste across 168–1268 rating — EP ≈ 1.0 on both sides
// of 757, no kink. Lock and Load resets break the rigid 6s cycle the
// breakpoint math assumes, and haste also scales focus regen (→ more
// Arcane Shots), so surplus haste is never "just autos" — the guide
// claim the cap rests on. The engine supports soft caps
// (StatCap.SoftWeight) if a spec with a real breakpoint ever needs
// one.
// Measured pre-cap hit, per spec, as a ratio of that spec's own agility
// weight (see hunterProfile). Was 3.5 / 3.3 / 2.9 by convention.
const (
	svHitEP = 1.73 // 0.53 × 3.27
	mmHitEP = 2.20 // 0.72 × 3.05
	bmHitEP = 1.70 // 0.64 × 2.65
)

var profiles = []*score.Profile{
	hunterProfile("Beast Mastery",
		hunterWeights(0.5, 2.65, 1.0, bmHitEP, 1.19, 0.97, 0.55), 6.32),
	hunterProfile("Marksmanship",
		hunterWeights(0.5, 3.05, 1.0, mmHitEP, 1.39, 1.33, 1.15), 6.32),
	hunterProfile("Survival",
		hunterWeights(0.5, 3.27, 1.0, svHitEP, 1.17, 0.89, 0.88), 3.75),
	balanceProfile, catProfile, bearProfile, restoProfile,
	armsProfile, furyTGProfile, furySMFProfile, protProfile,
	arcaneProfile, fireProfile,
	afflictionProfile, demonologyProfile, destructionProfile,
}

// findProfileFor resolves the profile with the import's exact talent
// ranks: a feral with bear-only talents is the bear tank, and a balance
// druid's spirit converts to hit at the Balance of Power rank taken.
func findProfileFor(class db.Class, tree string, ranks map[int]int) *score.Profile {
	if class == db.ClassDruid && tree == "Feral Combat" {
		if hasAny(ranks, bearTalents) {
			return bearProfile
		}
		return catProfile
	}
	if class == db.ClassWarrior && tree == "Fury" {
		return furyProfileFor(ranks)
	}
	p := score.FindIn(profiles, class, tree)
	if p == balanceProfile {
		if r := balanceOfPower.DetectRank(ranks); r >= 0 && r < balanceOfPower.Max {
			return balanceWithBoP(r)
		}
	}
	return p
}

func hasAny(ranks map[int]int, ids []int) bool {
	for _, id := range ids {
		if ranks[id] > 0 {
			return true
		}
	}
	return false
}
