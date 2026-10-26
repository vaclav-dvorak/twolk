package score

import (
	"math"

	"github.com/vaclav-dvorak/twolk/internal/db"
)

// Rating conversions at level 80. Note melee/ranged and spell hit convert
// differently — 8% physical = 263 rating, 17% spell = 446 rating.
const (
	MeleeHitRatingPerPct = 32.79
	SpellHitRatingPerPct = 26.232
	RatingPerExpertise   = 8.1975 // 1 expertise = 0.25% dodge/parry reduction
)

// CapTalent is a talent that helps reach a cappable stat (hit, expertise),
// effectively lowering the gear cap. The armory export doesn't reveal
// individual talents, so the user sets assumed points in the UI — but
// the export's per-tree point sums bound what is REACHABLE (see
// MaxReachable), and the UI dims what provably isn't.
type CapTalent struct {
	Name           string
	Tree           string
	Max            int
	ReqPoints      int     // tree points required before the first point can go in (5 × its row, rows 0-based)
	Stat           db.Stat // which capped stat it helps
	RatingPerPoint float64 // gear-rating equivalent per talent point
	Per            string  // display: what one point grants

	// SpellIDs are the talent's per-rank spell ids. When the import
	// carries exact talent ranks (API-fetched cata characters), the
	// taken rank is DETECTED through them instead of assumed.
	SpellIDs []int
}

// DetectRank reads the talent's taken rank from imported per-spell
// ranks: the rank when taken, 0 when the (exact) data lacks the
// talent, and -1 when detection is impossible — no ranks imported or
// no SpellIDs known — and the assumption flow applies.
func (t *CapTalent) DetectRank(ranks map[int]int) int {
	if len(ranks) == 0 || len(t.SpellIDs) == 0 {
		return -1
	}
	for _, id := range t.SpellIDs {
		if r, ok := ranks[id]; ok {
			return min(max(r, 1), t.Max)
		}
	}
	return 0
}

// MaxReachable bounds the talent's assumable rank by the character's
// total points in its tree: unlocking the talent's row needs ReqPoints
// in the tree before the first point can go in, so rank + ReqPoints
// can never exceed the tree total. A negative treePts means the tree
// total is unknown — no restriction.
func (t *CapTalent) MaxReachable(treePts int) int {
	if treePts < 0 {
		return t.Max
	}
	return min(max(treePts-t.ReqPoints, 0), t.Max)
}

func hitMelee(pct float64) float64 { return pct * MeleeHitRatingPerPct }
func hitSpell(pct float64) float64 { return pct * SpellHitRatingPerPct }
func expertise(pts float64) float64 {
	return pts * RatingPerExpertise
}

// capTalents lists every cap-affecting talent per WotLK class. Trees
// and ReqPoints (5 × the talent's 0-based row) verified against the
// WoWSims talent trees (ui/core/talents/trees/*.json @ pinned
// 02e0d9fb).
var capTalents = map[db.Class][]CapTalent{
	db.ClassWarrior: {
		{Name: "Precision", Tree: "Fury", Max: 3, ReqPoints: 20, Stat: db.MeleeHit, RatingPerPoint: hitMelee(1), Per: "1% hit"},
		{Name: "Strength of Arms", Tree: "Arms", Max: 2, ReqPoints: 30, Stat: db.Expertise, RatingPerPoint: expertise(2), Per: "2 expertise"},
		{Name: "Vitality", Tree: "Protection", Max: 3, ReqPoints: 35, Stat: db.Expertise, RatingPerPoint: expertise(2), Per: "2 expertise"},
	},
	db.ClassPaladin: {
		{Name: "Combat Expertise", Tree: "Protection", Max: 3, ReqPoints: 35, Stat: db.Expertise, RatingPerPoint: expertise(2), Per: "2 expertise"},
	},
	db.ClassHunter: {
		{Name: "Focused Aim", Tree: "Marksmanship", Max: 3, ReqPoints: 0, Stat: db.MeleeHit, RatingPerPoint: hitMelee(1), Per: "1% hit"},
	},
	db.ClassRogue: {
		{Name: "Precision", Tree: "Combat", Max: 5, ReqPoints: 5, Stat: db.MeleeHit, RatingPerPoint: hitMelee(1), Per: "1% hit"},
		{Name: "Weapon Expertise", Tree: "Combat", Max: 2, ReqPoints: 25, Stat: db.Expertise, RatingPerPoint: expertise(5), Per: "5 expertise"},
	},
	db.ClassPriest: {
		{Name: "Shadow Focus", Tree: "Shadow", Max: 3, ReqPoints: 5, Stat: db.SpellHit, RatingPerPoint: hitSpell(1), Per: "1% spell hit"},
	},
	db.ClassDeathKnight: {
		{Name: "Virulence", Tree: "Unholy", Max: 3, ReqPoints: 0, Stat: db.SpellHit, RatingPerPoint: hitSpell(1), Per: "1% spell hit"},
		{Name: "Nerves of Cold Steel", Tree: "Frost", Max: 3, ReqPoints: 5, Stat: db.MeleeHit, RatingPerPoint: hitMelee(1), Per: "1% hit (1H)"},
	},
	db.ClassShaman: {
		{Name: "Elemental Precision", Tree: "Elemental", Max: 3, ReqPoints: 25, Stat: db.SpellHit, RatingPerPoint: hitSpell(1), Per: "1% spell hit"},
	},
	db.ClassMage: {
		// Frost tree, NOT Arcane — the old entry was wrong (harmless
		// while Tree was display-only; it matters for reachability)
		{Name: "Precision", Tree: "Frost", Max: 3, ReqPoints: 5, Stat: db.SpellHit, RatingPerPoint: hitSpell(1), Per: "1% spell hit"},
	},
	db.ClassWarlock: {
		{Name: "Suppression", Tree: "Affliction", Max: 3, ReqPoints: 0, Stat: db.SpellHit, RatingPerPoint: hitSpell(1), Per: "1% spell hit"},
	},
	db.ClassDruid: {
		{Name: "Balance of Power", Tree: "Balance", Max: 2, ReqPoints: 25, Stat: db.SpellHit, RatingPerPoint: hitSpell(2), Per: "2% spell hit"},
		{Name: "Primal Precision", Tree: "Feral Combat", Max: 2, ReqPoints: 15, Stat: db.Expertise, RatingPerPoint: expertise(5), Per: "5 expertise"},
	},
}

// ClassicTreeOrder is each class's talent-tree tab order — the order
// the armory export's per-tree point sums use (WoWSims tree jsons at
// the pin, matching the in-game tab order). Identical in WotLK and
// Cataclysm, so both versions' Rules reference this one table.
var ClassicTreeOrder = map[db.Class][3]string{
	db.ClassWarrior:     {"Arms", "Fury", "Protection"},
	db.ClassPaladin:     {"Holy", "Protection", "Retribution"},
	db.ClassHunter:      {"Beast Mastery", "Marksmanship", "Survival"},
	db.ClassRogue:       {"Assassination", "Combat", "Subtlety"},
	db.ClassPriest:      {"Discipline", "Holy", "Shadow"},
	db.ClassDeathKnight: {"Blood", "Frost", "Unholy"},
	db.ClassShaman:      {"Elemental", "Enhancement", "Restoration"},
	db.ClassMage:        {"Arcane", "Fire", "Frost"},
	db.ClassWarlock:     {"Affliction", "Demonology", "Destruction"},
	db.ClassDruid:       {"Balance", "Feral Combat", "Restoration"},
}

// RacialHitPct returns the passive hit% a race grants (Draenei Heroic
// Presence, which applies to melee, ranged and spells alike).
func RacialHitPct(race string) float64 {
	if race == "Draenei" {
		return 1
	}
	return 0
}

// HitCapSpec declares that a profile's build is hit-capped: Stat is the
// rating slot the build reads, BasePct the raw cap vs. raid bosses
// (8% physical, 17% spells).
type HitCapSpec struct {
	Stat    db.Stat
	BasePct float64
	// Spell converts BasePct at the spell rate even though the build
	// reads the unified rating slot (cata casters: hit lives in the
	// melee slot, the cap is 17% of SPELL hit)
	Spell bool
}

// baseRating converts the cap percentage using the stat's conversion
// from the version's rules.
func (h *HitCapSpec) baseRating(r *Rules) float64 {
	per := r.MeleeHitRatingPerPct
	if h.Stat == db.SpellHit || h.Spell {
		per = r.SpellHitRatingPerPct
	}
	return math.Ceil(h.BasePct * per)
}

// EffectiveCaps returns the profile's caps with per-stat rating
// reductions (talents + racials) applied — both to static caps (e.g.
// expertise) and to the hit cap.
func (p *Profile) EffectiveCaps(reductions map[db.Stat]float64) []StatCap {
	caps := make([]StatCap, len(p.Caps))
	copy(caps, p.Caps)
	for i := range caps {
		caps[i].Cap = math.Max(caps[i].Cap-reductions[caps[i].Stat], 0)
	}
	if p.HitCap != nil {
		cap := math.Max(p.HitCap.baseRating(p.rules())-reductions[p.HitCap.Stat], 0)
		caps = append(caps, StatCap{Stat: p.HitCap.Stat, Cap: cap})
	}
	return caps
}
