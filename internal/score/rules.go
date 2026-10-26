package score

import "github.com/vaclav-dvorak/twolk/internal/db"

// Rules bundles one game version's scoring constants and policies —
// what the engine needs beyond a profile and an item database. Every
// profile carries the Rules of the version that registered it; WotLK
// is the default for profiles built without one (all of this package's
// own tables and tests).
type Rules struct {
	// Game is the version's config-key prefix ("wotlk"); it namespaces
	// user overrides so same-named specs of different versions never
	// share weights.
	Game string

	// Rating conversions at the version's endgame level.
	MeleeHitRatingPerPct float64
	SpellHitRatingPerPct float64
	RatingPerExpertise   float64 // 1 expertise = 0.25% dodge/parry reduction
	CritRatingPerPct     float64

	// Reforging enables the version's reforge mechanic (Cataclysm
	// onward): worn reforges count, candidates get an assumed optimal
	// reforge (see AssumedReforge).
	Reforging bool

	// GemQuality is the EXACT quality tier assumed for colored sockets
	// — the version's obtainable endgame cuts (WotLK: epic; Cata launch
	// content: rare, flip to epic when 4.3 unlocks). An exact match,
	// not a floor: the cata database already carries the unobtainable
	// 4.3 epics, and a floor would assume them. Metas have their own
	// tier (WotLK/Cata metas are rare-only). Jewelcrafter-only cuts
	// bypass the tier — their profession lock is the gate.
	GemQuality  db.Quality
	MetaQuality db.Quality

	// SetBonuses values tier bonuses by set name (curated EP).
	SetBonuses map[string][]SetBonus

	// CapTalents lists cap-affecting talents per class; TreeOrder is
	// each class's talent tab order (how the armory export sums them).
	CapTalents map[db.Class][]CapTalent
	TreeOrder  map[db.Class][3]string

	// SpellHitDebuffPct is the spell-hit% the version's raid debuff
	// grants the whole raid (WotLK: Misery / Improved Faerie Fire, 3%,
	// the two never stack). Raid composition, not character data, so it
	// applies only through the user's raid-debuff toggle. Zero means the
	// version has no such debuff — 4.0.1 removed both on purpose, so
	// Cataclysm casters gear the full unreduced cap.
	SpellHitDebuffPct float64

	// WeaponRacials are race bonuses tied to a weapon type, as virtual
	// gear stats (already converted with this version's ratings).
	WeaponRacials []WeaponRacial

	// BuckleName is the version's universal belt-buckle enhancement
	// (display only; the extra waist socket is assumed either way).
	BuckleName string

	// Consumables lists the version's assumable food and flasks for
	// versions whose data does not carry them (see consumes.go); the
	// database's own table is merged with it. Empty on both sides
	// leaves scoring unbuffed — the behavior before consumables.
	Consumables []db.Consumable

	// Stats lists the canonical stats this version's data can carry —
	// what the weights editor offers and marginal displays show.
	// WotLK has everything but Mastery; Cata drops the stats 4.0
	// removed (ArP, Defense, block value) and unifies the spell/melee
	// rating pairs onto the melee slots.
	Stats []db.Stat
}

// HasStat reports whether any of the given stats occurs in this
// version's data.
func (r *Rules) HasStat(stats ...db.Stat) bool {
	for _, want := range stats {
		for _, have := range r.Stats {
			if have == want {
				return true
			}
		}
	}
	return false
}

// WotLK is the founding version's rule set; tables live in their
// original files (talents.go, racials.go, setbonus.go).
var WotLK = &Rules{
	Game:                 "wotlk",
	MeleeHitRatingPerPct: MeleeHitRatingPerPct,
	SpellHitRatingPerPct: SpellHitRatingPerPct,
	RatingPerExpertise:   RatingPerExpertise,
	CritRatingPerPct:     critRatingPerPct,
	GemQuality:           db.QualityEpic,
	MetaQuality:          db.QualityRare,
	SetBonuses:           setBonuses,
	CapTalents:           capTalents,
	TreeOrder:            ClassicTreeOrder,
	SpellHitDebuffPct:    3,
	WeaponRacials:        wotlkWeaponRacials,
	BuckleName:           BuckleName,
	Consumables:          wotlkConsumables(),
	Stats: []db.Stat{
		db.Strength, db.Agility, db.Stamina, db.Intellect, db.Spirit,
		db.SpellPower, db.MP5, db.SpellHit, db.SpellCrit, db.SpellHaste,
		db.SpellPenetration, db.AttackPower, db.MeleeHit, db.MeleeCrit,
		db.MeleeHaste, db.ArmorPenetration, db.Expertise, db.Mana,
		db.Energy, db.Rage, db.Armor, db.RangedAttackPower, db.Defense,
		db.Block, db.BlockValue, db.Dodge, db.Parry, db.Resilience,
		db.Health, db.ArcaneResistance, db.FireResistance,
		db.FrostResistance, db.NatureResistance, db.ShadowResistance,
		db.BonusArmor,
	},
}

// rules resolves the profile's version rules (WotLK when unset).
func (p *Profile) rules() *Rules {
	if p.Rules != nil {
		return p.Rules
	}
	return WotLK
}

func (s *Scorer) rules() *Rules { return s.prof.rules() }

// CapTalentsFor returns the version's cap-affecting talents of a class.
func (r *Rules) CapTalentsFor(class db.Class) []CapTalent {
	return r.CapTalents[class]
}

// TreePoints returns the character's total points in the named tree,
// given the armory export's per-tree sums in tab order. Unknown
// classes, tree names or malformed sums return -1 (unknown — callers
// must not restrict on it).
func (r *Rules) TreePoints(class db.Class, tree string, points []int) int {
	order, ok := r.TreeOrder[class]
	if !ok {
		return -1
	}
	for i, name := range order {
		if name == tree {
			if i < len(points) {
				return points[i]
			}
			return -1
		}
	}
	return -1
}

// SetBonusesFor lists the known bonuses of a set name (nil if uncurated).
func (r *Rules) SetBonusesFor(setName string) []SetBonus {
	return r.SetBonuses[setName]
}

// SetBonusesForRole is SetBonusesFor as a profile of the role sees it:
// bonuses valued for another role carry EP 0 (see SetBonus.Role).
func (r *Rules) SetBonusesForRole(setName, role string) []SetBonus {
	bs := r.SetBonuses[setName]
	if bs == nil {
		return nil
	}
	out := make([]SetBonus, len(bs))
	for i, b := range bs {
		if b.Role != "" && b.Role != role {
			b.EP = 0
		}
		out[i] = b
	}
	return out
}

// bonusAt is the EP a set contributes at n worn pieces for the role.
func (r *Rules) bonusAt(name string, n int, role string) float64 {
	var total float64
	for _, b := range r.SetBonusesForRole(name, role) {
		if n >= b.Pieces {
			total += b.EP
		}
	}
	return total
}

// RacialWeaponStats returns the virtual stats a race gains while
// wielding this item (zero for non-matching race/weapon combinations).
func (r *Rules) RacialWeaponStats(race string, it *db.Item) db.Stats {
	for _, w := range r.WeaponRacials {
		if w.Race == race && w.Match(it) {
			return w.Stats
		}
	}
	return db.Stats{}
}

// RacialWeaponNote describes the applying weapon racial for display, or "".
func (r *Rules) RacialWeaponNote(race string, it *db.Item) string {
	for _, w := range r.WeaponRacials {
		if w.Race == race && w.Match(it) {
			return w.Note
		}
	}
	return ""
}
