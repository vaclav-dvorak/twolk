package score

import (
	"sort"

	"github.com/vaclav-dvorak/twolk/internal/db"
)

// Consumables — the stats a raiding character carries that are not on
// its gear. Food and flasks cost a few gold and last through a wipe, so
// they are not a choice the planner should make the player pay for in
// GEAR: whether a weapon's expertise is precious depends on whether a
// 40-expertise meal already covers the gap, and an ArP food moves the
// character along a curve where every further point is worth more.
// Modeling them is therefore a SCORING change, not a display one.
//
// They enter as a baseline added to the character's stat totals
// (Scorer.SetBaseline), so caps and curves see the buffed character
// while linear stats just shift every score by the same constant. That
// is the whole mechanism: a consumable can only change WHICH GEAR wins
// through a cap or a curve, which is exactly how it works in game.
//
// Where the version's own data carries the table, it is loaded from
// there (Cataclysm's db.json); WotLK's dump has none, so the table
// below is transcribed from the pinned sim's consumes.go — never from
// memory of a tooltip.

// wotlkConsumables is the WotLK endgame table, from wowsims/wotlk
// sim/core/consumes.go at the pinned commit. Only what a level-80
// raider actually uses is listed: the feasts, the full Northrend
// +40-stat food line, and the four Northrend flasks. The stamina every
// meal carries is included for tank profiles that weight it.
func wotlkConsumables() []db.Consumable {
	food := func(name string, st db.Stats) db.Consumable {
		st[db.Stamina] = 40
		return db.Consumable{Name: name, Kind: db.ConsumableFood, Stats: st}
	}
	twin := func(a, b db.Stat, v float64) db.Stats {
		var st db.Stats
		st[a], st[b] = v, v
		return st
	}
	return []db.Consumable{
		// feasts: the raid default, no stat choice
		{Name: "Fish Feast", Kind: db.ConsumableFood, Stats: db.Stats{
			db.AttackPower: 80, db.RangedAttackPower: 80, db.SpellPower: 46, db.Stamina: 40}},
		{Name: "Great Feast", Kind: db.ConsumableFood, Stats: db.Stats{
			db.AttackPower: 60, db.RangedAttackPower: 60, db.SpellPower: 35, db.Stamina: 40}},
		// the +40 line: one stat each — the reason food can close a cap
		food("Dragonfin Filet", db.Stats{db.Strength: 40}),
		food("Blackened Dragonfin", db.Stats{db.Agility: 40}),
		food("Hearty Rhino", db.Stats{db.ArmorPenetration: 40}),
		food("Rhinolicious Wormsteak", db.Stats{db.Expertise: 40}),
		food("Snapper Extreme", twin(db.MeleeHit, db.SpellHit, 40)),
		food("Spiced Worm Burger", twin(db.MeleeCrit, db.SpellCrit, 40)),
		food("Imperial Manta Steak", twin(db.MeleeHaste, db.SpellHaste, 40)),
		food("Mega Mammoth Meal", db.Stats{db.AttackPower: 80, db.RangedAttackPower: 80}),
		food("Firecracker Salmon", db.Stats{db.SpellPower: 46}),
		food("Cuttlesteak", db.Stats{db.Spirit: 40}),
		food("Mighty Rhino Dogs", db.Stats{db.MP5: 16}),
		{Name: "Flask of Endless Rage", Kind: db.ConsumableFlask, Stats: db.Stats{
			db.AttackPower: 180, db.RangedAttackPower: 180}},
		{Name: "Flask of the Frost Wyrm", Kind: db.ConsumableFlask, Stats: db.Stats{db.SpellPower: 125}},
		{Name: "Flask of Stoneblood", Kind: db.ConsumableFlask, Stats: db.Stats{db.Health: 1300}},
		{Name: "Flask of Pure Mojo", Kind: db.ConsumableFlask, Stats: db.Stats{db.MP5: 45}},
	}
}

// Consumables lists the version's food or flasks, WORTH-ORDERED for
// this build and with dominated entries dropped: a version's data keeps
// every tier it ever shipped (cata's table still carries the WotLK
// meals), and offering a +40 crit food next to the +90 one is noise.
// Two consumables are the same CHOICE when they buff the same stats;
// only the best of each survives. Empty when the version has no table —
// scoring then runs unbuffed, as it did before consumables existed.
func (s *Scorer) Consumables(kind db.ConsumableKind) []db.Consumable {
	all := s.rules().Consumables
	if s.d != nil {
		all = append(append([]db.Consumable(nil), all...), s.d.Consumables...)
	}
	best := map[db.Stats]db.Consumable{}
	for _, c := range all {
		if c.Kind != kind {
			continue
		}
		// the signature is WHICH stats, not how many: stamina is on
		// every meal and never distinguishes one from another
		sig := db.Stats{}
		for st := db.Stat(0); st < db.NumStats; st++ {
			if c.Stats[st] != 0 && st != db.Stamina {
				sig[st] = 1
			}
		}
		if sig == (db.Stats{}) {
			continue // stamina-only food says nothing to a DPS profile
		}
		if cur, ok := best[sig]; !ok || s.consumableValue(c) > s.consumableValue(cur) {
			best[sig] = c
		}
	}
	out := make([]db.Consumable, 0, len(best))
	for _, c := range best {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		vi, vj := s.consumableValue(out[i]), s.consumableValue(out[j])
		if vi != vj {
			return vi > vj
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// consumableValue ranks consumables for listing and for the
// dominated-entry filter: the profile's linear read of the stats, with
// the raw point total breaking ties between buffs the build doesn't
// weight at all (so the listing still shows the bigger meal).
func (s *Scorer) consumableValue(c db.Consumable) float64 {
	var sum float64
	for st := db.Stat(0); st < db.NumStats; st++ {
		sum += c.Stats[st] * 1e-6 // tie-break on raw points
	}
	return s.dot(c.Stats) + sum
}

// SetBaseline sets the non-gear stats every total carries (consumables).
func (s *Scorer) SetBaseline(st db.Stats) { s.baseline = st }

// Baseline returns the non-gear stats currently assumed.
func (s *Scorer) Baseline() db.Stats { return s.baseline }

// BestConsumables picks the food and flask that score highest ON TOP OF
// a given set — jointly, because they share the caps and curves that
// make the choice interesting (an expertise food is worth nothing once
// the gear alone caps expertise). Returns nil for a kind the version
// has no table for. The set is priced on its assumed basis, the same
// one the BiS screen solves on.
func (s *Scorer) BestConsumables(set []*db.Item) (food, flask *db.Consumable) {
	// AssumedTotals already carries the current baseline; the choice is
	// made against the GEAR, so take it back out
	return s.BestConsumablesFor(sub(s.AssumedTotals(set), s.baseline))
}

// BestConsumablesForWorn is BestConsumables against an EQUIPPED set's
// real gems, enchants and reforges — what the character actually walks
// around with. The worn side must use this: a meal is advice for this
// character as he is tuned today, and the assumed basis can put him on
// the wrong side of a cap (a set the assumption model re-gems below the
// dodge cap would be told to eat expertise it does not need).
func (s *Scorer) BestConsumablesForWorn(ws []Worn) (food, flask *db.Consumable) {
	return s.BestConsumablesFor(sub(s.TotalsWorn(ws), s.baseline))
}

// BestConsumablesFor picks the best food and flask on top of given GEAR
// totals (the baseline excluded), jointly — they share the caps and
// curves that make the choice interesting.
func (s *Scorer) BestConsumablesFor(gear db.Stats) (food, flask *db.Consumable) {
	foods, flasks := s.Consumables(db.ConsumableFood), s.Consumables(db.ConsumableFlask)
	if len(foods) == 0 && len(flasks) == 0 {
		return nil, nil
	}
	pick := func(list []db.Consumable) []db.Consumable {
		if len(list) == 0 {
			return []db.Consumable{{}} // "none", so the other kind still gets picked
		}
		return list
	}
	best := 0.0
	first := true
	for _, f := range pick(foods) {
		for _, fl := range pick(flasks) {
			v := s.Value(add(gear, add(f.Stats, fl.Stats)))
			if !first && v <= best {
				continue
			}
			best, first = v, false
			food, flask = nil, nil
			if f.Name != "" {
				fc := f
				food = &fc
			}
			if fl.Name != "" {
				flc := fl
				flask = &flc
			}
		}
	}
	return food, flask
}
