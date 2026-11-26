package forever

import (
	"github.com/vaclav-dvorak/twolk/internal/db"
	"github.com/vaclav-dvorak/twolk/internal/score"
)

// capTalents are Forever's talents that lower a hit or expertise cap,
// read from the client's spell data through the pinned sim: every
// talent whose spell carries A_MOD_HIT_CHANCE (54), A_MOD_SPELL_HIT_CHANCE
// (55) or A_MOD_EXPERTISE (240), with its per-rank value. Several differ
// from classic — Nature's Reach and Tidal Focus are hit talents now,
// mage Precision covers every school, Suppression melee too.
//
// SpellIDs hold the armory's talent ids (the talent DEFINITION id, as
// in the sim's ui/sim/talents/trees), so an imported character's exact
// rank is detected, never assumed. ReqPoints is 5 × the row.
//
// Left out: Fury's Furious Precision (+4/7/10% hit on off-hand attacks
// only — the Fury hit sweep's half-value tail already prices it) and
// the Shadow Focus school limit (a Shadow Priest's damage is all
// shadow, so its hit counts in full).
var capTalents = map[db.Class][]score.CapTalent{
	db.ClassPaladin: {
		{Name: "Precision", Tree: "Protection", Max: 3, ReqPoints: 5, Stat: db.MeleeHit,
			RatingPerPoint: 10, Per: "1% hit", SpellIDs: []int{135163}},
	},
	db.ClassHunter: {
		{Name: "Surefooted", Tree: "Survival", Max: 3, ReqPoints: 10, Stat: db.MeleeHit,
			RatingPerPoint: 10, Per: "1% hit", SpellIDs: []int{134452}},
	},
	db.ClassRogue: {
		{Name: "Precision", Tree: "Combat", Max: 3, ReqPoints: 5, Stat: db.MeleeHit,
			RatingPerPoint: 10, Per: "1% hit", SpellIDs: []int{135267}},
		{Name: "Weapon Expertise", Tree: "Combat", Max: 2, ReqPoints: 15, Stat: db.Expertise,
			RatingPerPoint: 10, Per: "1% expertise", SpellIDs: []int{135256}},
	},
	db.ClassDruid: {
		{Name: "Nature's Reach", Tree: "Balance", Max: 2, ReqPoints: 5, Stat: db.MeleeHit,
			RatingPerPoint: 20, Per: "2% hit", SpellIDs: []int{134390}},
	},
	db.ClassShaman: {
		{Name: "Tidal Focus", Tree: "Restoration", Max: 5, ReqPoints: 0, Stat: db.MeleeHit,
			RatingPerPoint: 10, Per: "1% hit", SpellIDs: []int{134190}},
	},
	db.ClassMage: {
		{Name: "Precision", Tree: "Frost", Max: 3, ReqPoints: 0, Stat: db.MeleeHit,
			RatingPerPoint: 20, Per: "2% spell hit", SpellIDs: []int{135308}},
	},
	db.ClassWarlock: {
		{Name: "Suppression", Tree: "Affliction", Max: 5, ReqPoints: 0, Stat: db.MeleeHit,
			RatingPerPoint: 10, Per: "1% hit", SpellIDs: []int{135456}},
	},
	db.ClassPriest: {
		{Name: "Shadow Focus", Tree: "Shadow", Max: 5, ReqPoints: 0, Stat: db.MeleeHit,
			RatingPerPoint: 10, Per: "1% shadow hit", SpellIDs: []int{142597}},
	},
}
