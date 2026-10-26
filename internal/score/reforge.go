package score

import (
	"math"
	"sort"

	"github.com/vaclav-dvorak/twolk/internal/db"
)

// Reforge assumption (Cataclysm onward, Rules.Reforging): candidates
// and detail-less worn items are credited the best single reforge —
// 40% (floored) of one native secondary moved into a secondary the
// item lacks — under the CALIBRATED weights, the same convention as
// assumed gems and BestEnchant. Detailed worn items count their
// ACTUAL reforge instead (Worn.Reforge) — the same deliberate
// asymmetry that drives gem/enchant tips.
//
// Capped stats — hard AND soft — are never a reforge TARGET in the
// context-free assumption: with a fill-to-cap floor above every
// secondary, all ~16 reforges would chase the cap and blow far past it
// (the gem policy comparison in Calibrate is binary and can't split a
// set's reforges between hit and the rest). Gems keep chasing the gap
// as calibrated — their magnitude fits under a cap; reforges add pure
// uncapped value. Reforging OUT of a capped stat still happens
// naturally once calibration drops a saturated stat's weight (to zero,
// or to a soft cap's residual). Landing a cap exactly stays the
// optimizer's job.
//
// Contexts that DO know the whole set — the upgrade ranking and its
// compare pane, where exactly one item swaps — use AssumedReforgeOn
// instead: the exact cap-aware pick on the concrete rest-of-set base,
// where reforging into hit under the cap is legal and priced only up
// to the remaining gap.

// reforgeable is the game's fixed list of reforge-legal stats. The
// unified cata ratings live in the canonical melee slots (matching
// cataStatOrder decoding).
var reforgeable = [...]db.Stat{
	db.Spirit, db.Dodge, db.Parry, db.MeleeHit, db.MeleeCrit,
	db.MeleeHaste, db.Expertise, db.Mastery,
}

// AssumedReforge picks the value-maximizing reforge for an item under
// the calibrated weights: source = a native reforgeable stat, target =
// a reforgeable stat the item lacks, amount = 40% of the source
// (floored, the game's rule). Nil when the version has no reforging or
// no reforge gains value.
func (s *Scorer) AssumedReforge(it *db.Item) *db.Reforge {
	if !s.rules().Reforging {
		return nil
	}
	var best *db.Reforge
	bestGain := 1e-9
	for _, from := range reforgeable {
		amt := math.Floor(it.Stats[from] * 0.4)
		if amt <= 0 {
			continue
		}
		for _, to := range reforgeable {
			if it.Stats[to] > 0 || s.cappedStat(to) {
				continue
			}
			if gain := amt * (s.gemWeights[to] - s.gemWeights[from]); gain > bestGain {
				bestGain = gain
				best = &db.Reforge{From: from, To: to, Amount: amt}
			}
		}
	}
	return best
}

// AssumedReforgeOn is the cap-aware assumption for contexts that know
// the whole set: the reforge maximizing the exact set value on top of
// base — the rest of the worn set plus the item's own reforge-free
// contribution. Unlike AssumedReforge, a capped stat IS a legal target
// here; the base says how much of it still pays.
func (s *Scorer) AssumedReforgeOn(it *db.Item, base db.Stats) *db.Reforge {
	if !s.rules().Reforging {
		return nil
	}
	baseVal := s.Value(base)
	var best *db.Reforge
	bestGain := 1e-9
	for _, opt := range reforgeOptions(it) {
		if gain := s.Value(add(base, reforgeDelta(opt))) - baseVal; gain > bestGain {
			bestGain, best = gain, opt
		}
	}
	return best
}

// swapEffective is a candidate's effective stats under a swap context,
// with the reforge picked cap-aware against woSet (the set totals
// without the replaced item): the memoized EffectiveStatsSwap minus its
// context-free reforge, plus the AssumedReforgeOn choice.
func (s *Scorer) swapEffective(cand *db.Item, ctx SwapContext, woSet db.Stats) (db.Stats, *db.Reforge) {
	eff := sub(s.EffectiveStatsSwap(cand, ctx), s.assumedReforgeStats(cand))
	ref := s.AssumedReforgeOn(cand, add(woSet, eff))
	return add(eff, reforgeDelta(ref)), ref
}

// unreforgedWorn is an equipped instance's stats with its reforge taken
// back out: worn actuals for a detailed item, the context-free
// assumption for one without detail. It is the basis a re-plan starts
// from — every slot's reforge is then re-picked against the whole set.
func (s *Scorer) unreforgedWorn(w Worn) db.Stats {
	st := s.WornStats(w)
	if w.HasDetail {
		return sub(st, reforgeDelta(w.Reforge))
	}
	return sub(st, s.assumedReforgeStats(w.Item))
}

// replan re-picks every slot's reforge against the whole set, given each
// slot's stats WITHOUT any reforge applied. It is AssumedPlan's descent
// on an arbitrary basis, so a worn set can be re-planned while keeping
// its real gems and enchants — only the reforges move.
//
// Versions without reforging return the bare totals and a nil plan,
// which makes every caller a no-op there.
func (s *Scorer) replan(unreforged []db.Stats, items []*db.Item) (db.Stats, []*db.Reforge) {
	refs := make([]*db.Reforge, len(items))
	var tot db.Stats
	for _, st := range unreforged {
		tot = add(tot, st)
	}
	if !s.rules().Reforging {
		return tot, refs
	}
	for range 8 {
		improved := false
		for i, it := range items {
			without := sub(tot, reforgeDelta(refs[i]))
			r := s.AssumedReforgeOn(it, without)
			if sameReforge(r, refs[i]) {
				continue
			}
			refs[i], tot = r, add(without, reforgeDelta(r))
			improved = true
		}
		if !improved {
			break
		}
	}
	return tot, refs
}

// ReplanWorn re-picks every worn slot's reforge against the whole set,
// keeping the real gems and enchants — what the gear is worth once the
// reforge advisor has had another pass at it.
func (s *Scorer) ReplanWorn(ws []Worn) (db.Stats, []*db.Reforge) {
	unreforged := make([]db.Stats, len(ws))
	items := make([]*db.Item, len(ws))
	for i, w := range ws {
		unreforged[i], items[i] = s.unreforgedWorn(w), w.Item
	}
	return s.replan(unreforged, items)
}

// AssumedReforgeWorn is the reforge assumed for a candidate replacing
// slot i of the worn set — the same cap-aware pick the upgrade ranking
// prices, for displays that must agree with it.
func (s *Scorer) AssumedReforgeWorn(ws []Worn, i int, cand *db.Item) *db.Reforge {
	woSet := sub(s.TotalsWorn(ws), s.WornStats(ws[i]))
	_, ref := s.swapEffective(cand, s.SwapContextFor(ws, i), woSet)
	return ref
}

// Reforging reports whether the profile's game version has reforging
// (gates the advisor and every reforge display).
func (s *Scorer) Reforging() bool { return s.rules().Reforging }

// ReforgeTip proposes changing one worn item's reforge. From nil =
// currently unreforged, To nil = clear the reforge.
type ReforgeTip struct {
	ItemIdx int
	From    *db.Reforge
	To      *db.Reforge
	Delta   float64 // this change alone vs the worn set, cap-aware
}

// reforgeOptions lists every legal reforge of an item: source native
// and reforgeable, target reforgeable and NOT native (the game forbids
// reforging into a stat the item already has).
func reforgeOptions(it *db.Item) []*db.Reforge {
	var opts []*db.Reforge
	for _, from := range reforgeable {
		amt := math.Floor(it.Stats[from] * 0.4)
		if amt <= 0 {
			continue
		}
		for _, to := range reforgeable {
			if it.Stats[to] > 0 {
				continue
			}
			opts = append(opts, &db.Reforge{From: from, To: to, Amount: amt})
		}
	}
	return opts
}

// CanReforge reports whether the item has any legal reforge (some
// native reforgeable stat and a reforgeable stat it lacks).
func CanReforge(it *db.Item) bool {
	return len(reforgeOptions(it)) > 0
}

// reforgeDelta is the stat shift a reforge applies (zero for nil).
func reforgeDelta(r *db.Reforge) db.Stats {
	var st db.Stats
	if r != nil {
		st[r.From] -= r.Amount
		st[r.To] += r.Amount
	}
	return st
}

func sameReforge(a, b *db.Reforge) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// minReforgeGain is the least set-score improvement a greedy move must
// bring to be suggested at all. Weights of neighboring secondaries sit
// ~0.01 apart, so without a floor the optimizer offers churn — "move
// 22 rating from mastery to haste, +0.2" — that no player should act
// on (the enchant advisor gates the same way at 0.5).
const minReforgeGain = 1.0

// OptimizeReforges solves the whole worn set's reforges at once — the
// set-level view the per-item assumption cannot take: reforges into a
// capped stat pay only until the SET total lands the cap, so which
// items reforge into hit and which take an uncapped secondary is one
// joint choice. Greedy ascent on the exact cap-aware set value: apply
// the single best reforge change until none improves by at least
// minReforgeGain (the same pragmatism as the BiS descent). Detail-less
// items are skipped — they already score under assumptions. Returned
// tips are the items whose optimal reforge differs from the worn one,
// best first, each priced alone against the current set (near a cap
// the combined gain is ReforgeGain's business).
func (s *Scorer) OptimizeReforges(ws []Worn) []ReforgeTip {
	if !s.rules().Reforging {
		return nil
	}
	type slot struct {
		idx  int
		cur  *db.Reforge
		opts []*db.Reforge
	}
	var slots []slot
	for i, w := range ws {
		if !w.HasDetail {
			continue
		}
		opts := reforgeOptions(w.Item)
		if w.Reforge == nil && len(opts) == 0 {
			continue
		}
		slots = append(slots, slot{idx: i, cur: w.Reforge, opts: append(opts, nil)})
	}
	tot := s.TotalsWorn(ws)
	base := s.Value(tot)
	for range 64 {
		bestGain, bestSlot := minReforgeGain, -1
		var bestOpt *db.Reforge
		for si, sl := range slots {
			without := sub(tot, reforgeDelta(sl.cur))
			for _, opt := range sl.opts {
				if sameReforge(opt, sl.cur) {
					continue
				}
				if gain := s.Value(add(without, reforgeDelta(opt))) - base; gain > bestGain {
					bestGain, bestSlot, bestOpt = gain, si, opt
				}
			}
		}
		if bestSlot < 0 {
			break
		}
		sl := &slots[bestSlot]
		tot = add(sub(tot, reforgeDelta(sl.cur)), reforgeDelta(bestOpt))
		sl.cur = bestOpt
		base = s.Value(tot)
	}
	origTot := s.TotalsWorn(ws)
	origBase := s.Value(origTot)
	var tips []ReforgeTip
	for _, sl := range slots {
		worn := ws[sl.idx].Reforge
		if sameReforge(sl.cur, worn) {
			continue
		}
		alone := s.Value(add(sub(origTot, reforgeDelta(worn)), reforgeDelta(sl.cur))) - origBase
		tips = append(tips, ReforgeTip{ItemIdx: sl.idx, From: worn, To: sl.cur, Delta: alone})
	}
	sort.Slice(tips, func(a, b int) bool { return tips[a].Delta > tips[b].Delta })
	return tips
}

// ReforgeGain is the exact cap-aware set-score gain of applying all
// the tips at once — per-tip deltas price each change alone, so near a
// cap their sum can overstate the combined result.
func (s *Scorer) ReforgeGain(ws []Worn, tips []ReforgeTip) float64 {
	if len(tips) == 0 {
		return 0
	}
	tot := s.TotalsWorn(ws)
	base := s.Value(tot)
	for _, t := range tips {
		tot = add(sub(tot, reforgeDelta(t.From)), reforgeDelta(t.To))
	}
	return s.Value(tot) - base
}

// cappedStat reports whether the profile caps the stat (static caps
// or the hit-cap spec).
func (s *Scorer) cappedStat(st db.Stat) bool {
	for _, c := range s.prof.Caps {
		if c.Stat == st {
			return true
		}
	}
	// a stat feeding a capped one (spirit → hit) fills that cap too
	for _, cv := range s.prof.Conversions {
		if cv.From == st && cv.To != st && s.cappedStat(cv.To) {
			return true
		}
	}
	return s.prof.HitCap != nil && s.prof.HitCap.Stat == st
}

// assumedReforgeStats is the stat delta of the assumed reforge (zero
// when none applies).
func (s *Scorer) assumedReforgeStats(it *db.Item) db.Stats {
	return reforgeDelta(s.AssumedReforge(it))
}

// AssumedPlan is the assumed reforge plan of a full item set: every
// item's reforge re-picked cap-aware against the rest of the set,
// coordinate-descent style, plus the resulting stat totals.
//
// The per-item assumption (AssumedReforge) is cap-blind on purpose — it
// has no set to look at — and that costs a set two ways: it can never
// reforge INTO a capped stat (so a set under the hit cap never assumes
// the reforge that lands it) and it values a capped stat at its full
// selection weight (so a set far OVER the cap never dumps the surplus).
// Both errors are large in Cataclysm, where reforging is how hit lands
// on the cap: a hit-heavy set priced item-by-item carries hundreds of
// wasted rating that any real character would reforge away. Contexts
// that know the whole set — the BiS solver, its shopping deltas — price
// through here instead.
func (s *Scorer) AssumedPlan(set []*db.Item) (db.Stats, []*db.Reforge) {
	refs := make([]*db.Reforge, len(set))
	tot := s.baseline
	for _, it := range set {
		tot = add(tot, sub(s.EffectiveStats(it), s.assumedReforgeStats(it)))
	}
	if !s.rules().Reforging {
		return tot, refs
	}
	// each sweep re-picks every slot's reforge against the current
	// totals; AssumedReforgeOn maximizes set value over the item's
	// options plus "no reforge", so accepted moves never lose value and
	// the descent terminates
	for range 8 {
		improved := false
		for i, it := range set {
			without := sub(tot, reforgeDelta(refs[i]))
			r := s.AssumedReforgeOn(it, without)
			if sameReforge(r, refs[i]) {
				continue
			}
			refs[i], tot = r, add(without, reforgeDelta(r))
			improved = true
		}
		if !improved {
			break
		}
	}
	return tot, refs
}

// plannedStats is an item's contribution to an AssumedPlan total: its
// effective stats with the context-free reforge assumption swapped for
// the plan's cap-aware pick.
func (s *Scorer) plannedStats(it *db.Item, ref *db.Reforge) db.Stats {
	return add(sub(s.EffectiveStats(it), s.assumedReforgeStats(it)), reforgeDelta(ref))
}

// AssumedTotals is AssumedPlan's stat totals.
func (s *Scorer) AssumedTotals(set []*db.Item) db.Stats {
	tot, _ := s.AssumedPlan(set)
	return tot
}

// AssumedSetScore scores a full item set under the fresh-drop
// assumptions — best-case gems and enchant, set-level cap-aware
// reforges — including weapon DPS and set bonuses. This is the basis
// the BiS solver optimizes and the basis its shopping deltas compare
// on; the worn set scored through here is what its own gear would be
// worth as a pile of fresh drops (see SetScoreWorn for the tuned
// actuals).
func (s *Scorer) AssumedSetScore(set []*db.Item) float64 {
	ws := wrapWorn(set)
	score := s.Value(s.AssumedTotals(set)) + s.SetBonusScore(ws)
	for _, it := range set {
		score += s.itemBonusValue(it)
	}
	return score
}

// AssumedContribution is Contribution on the SET-LEVEL plan: how much
// set[i] adds to totals whose reforges are already planned, with the
// plan held FROZEN rather than re-solved around the item. Contribution
// itself sums per-item assumptions, and those never reforge into a
// capped stat — so on a Cataclysm set, where reforging is what lands
// the hit cap, it reads the set as far under a cap it actually sits on
// and credits a candidate's hit rating that the character cannot use.
func (s *Scorer) AssumedContribution(set []*db.Item, i int) float64 {
	rest := make([]*db.Item, 0, len(set)-1)
	rest = append(rest, set[:i]...)
	rest = append(rest, set[i+1:]...)
	tot, _ := s.AssumedPlan(rest) // the rest of the set, reforges planned
	st := s.plannedStats(set[i], nil)
	return s.Value(add(tot, st)) - s.Value(tot) + s.itemBonusValue(set[i])
}

// ItemSwapDelta prices replacing set[i] with cand under AssumedSetScore
// — BOTH sides on fresh-drop assumptions, so the number is pure item
// quality. SwapDelta is the mixed-basis sibling (tuned actuals on the
// worn side) that answers "is this drop worth taking over what I wear
// right now, as my gear is tuned today".
func (s *Scorer) ItemSwapDelta(set []*db.Item, i int, cand *db.Item) float64 {
	swapped := append([]*db.Item(nil), set...)
	swapped[i] = cand
	return s.AssumedSetScore(swapped) - s.AssumedSetScore(set)
}
