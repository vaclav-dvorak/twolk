package score

// Joint optimization: gems, enchants and reforges are all per-item
// edits coupled through the same capped set totals — solved separately
// they seesaw (a better enchant drops 5 hit, the gem pass re-gems to
// compensate, the reforge pass undoes it). Optimize round-robins the
// three domain solvers on a working copy of the set, each pass
// conditioned on the others' current choices, until a full round
// changes nothing. Versions without reforging (WotLK) simply get an
// empty reforge section.
//
// Reforges go FIRST in the round: when two domains can fill the same
// gap (reforge to the hit cap vs socket hit gems), the greedy order
// decides which one claims it, and reforging costs pocket change at
// the arcane reforger while gems and enchants burn real mats — near-
// equivalent plans should lean on the cheap edit.

// OptimizePlan is the joint result. Each domain's tips are diffed
// against the ORIGINAL worn state of that domain, conditioned on the
// FINAL state of the other two — applying all three sections yields
// Set. Delta is the exact cap-aware set-score gain of the whole plan.
type OptimizePlan struct {
	Gems     GemPlan
	Enchants []EnchantTip
	Reforges []ReforgeTip
	Set      []Worn
	Delta    float64
}

// mixWorn assembles a hybrid worn set: items and detail flags from
// base, each domain's state taken from its designated source set.
func mixWorn(base, gems, ench, ref []Worn) []Worn {
	out := append([]Worn(nil), base...)
	for i := range out {
		out[i].Gems = gems[i].Gems
		out[i].EnchantID = ench[i].EnchantID
		out[i].Reforge = ref[i].Reforge
	}
	return out
}

// Optimize computes the joint gem+enchant+reforge plan for the worn
// set. Detail-less items are untouched throughout — they already
// score under best-case assumptions.
func (s *Scorer) Optimize(ws []Worn) OptimizePlan {
	if len(ws) == 0 {
		return OptimizePlan{}
	}
	cur := append([]Worn(nil), ws...)
	for range 3 {
		changed := false
		for _, t := range s.OptimizeReforges(cur) {
			cur[t.ItemIdx].Reforge = t.To
			changed = true
		}
		gp := s.OptimizeGems(cur)
		if len(gp.Tips) > 0 {
			cur, changed = gp.Set, true
		}
		for _, t := range s.EnchantTips(cur) {
			cur[t.ItemIdx].EnchantID = t.To.EffectID
			changed = true
		}
		if !changed {
			break
		}
	}
	// display tips: each domain re-solved once from its ORIGINAL state
	// with the other two final, so every section reads as "change this
	// on the gear you wear now" and the three sections compose to Set
	plan := OptimizePlan{
		Gems:     s.OptimizeGems(mixWorn(ws, ws, cur, cur)),
		Enchants: s.EnchantTips(mixWorn(ws, cur, ws, cur)),
		Reforges: s.OptimizeReforges(mixWorn(ws, cur, cur, ws)),
	}
	// Set = the worn state with exactly the three displayed sections
	// applied, composed field by field so the summary totals can never
	// disagree with the listed changes. (Taking Gems.Set wholesale
	// leaked cur's enchant/reforge state into Set: each display pass
	// re-solves from the ORIGINAL worn state, and where a re-solve
	// lands on a different local optimum than the round-robin — greedy
	// from a different start — cur's un-tipped moves silently rode
	// along, showing totals the listed changes cannot produce.)
	final := append([]Worn(nil), ws...)
	if gs := plan.Gems.Set; gs != nil {
		for i := range final {
			final[i].Gems = gs[i].Gems
		}
	}
	for _, t := range plan.Enchants {
		final[t.ItemIdx].EnchantID = t.To.EffectID
	}
	for _, t := range plan.Reforges {
		final[t.ItemIdx].Reforge = t.To
	}
	plan.Set = final
	plan.Delta = s.SetScoreWorn(final) - s.SetScoreWorn(ws)
	return plan
}
