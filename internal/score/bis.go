package score

import (
	"sort"

	"github.com/vaclav-dvorak/twolk/internal/db"
)

// Best-in-slot builder: find the best full set reachable under the
// proposal filter. Armor rows follow the worn layout (how many ring/
// trinket slots exist); weapon rows are re-solved per layout the
// profile can play (Profile.Layouts — 2H vs dual-wield vs shield/held
// off-hand), best layout wins.
//
// Coordinate descent: sweep the slots, re-picking each against the rest
// of the set under the full objective (capped set value + weapon DPS +
// set bonuses, over the set's cap-aware reforge plan — AssumedPlan),
// until a sweep changes nothing. The set-bonus term makes
// the landscape non-convex — completing a 4pc crosses swaps that lose
// value on their own — so after the base descent every curated tier set
// in the candidate pool seeds one more descent with its best pieces
// forced in; the free descent keeps the tier pieces only where the
// bonus actually pays, and the best-scoring result wins.

// BestSet solves the best full set reachable under the proposal filter,
// under the same best-case gem/enchant assumptions candidates are
// ranked with. Every weapon layout the profile can play is solved
// independently — two one-handers each score below a two-hander, but
// the pair can win — and the best layout's result is returned.
//
// The returned baseline is ws with the weapon rows re-dealt onto the
// winning layout (kept worn weapons stay, rows the character has
// nothing for hold EmptyHand); the picks align with it. Slots without
// any allowed candidate keep their baseline item — what is already
// owned needs no acquisition route.
func (s *Scorer) BestSet(ws []Worn, allow func(*db.Item) bool) ([]Worn, []*db.Item, float64) {
	if len(ws) == 0 {
		return nil, nil, 0
	}
	ws = fillMissingSlots(ws)
	type shape struct {
		ws    []Worn
		roles []weaponRole
	}
	var shapes []shape
	for _, l := range s.prof.Layouts {
		sws, roles := reshapeWorn(ws, l)
		shapes = append(shapes, shape{sws, roles})
	}
	if len(shapes) == 0 {
		// no layout table: solve the worn shape as-is
		shapes = append(shapes, shape{ws, s.prof.wornWeaponRoles(itemsOf(ws))})
	}
	var bestWs []Worn
	var best []*db.Item
	bestScore := 0.0
	for _, sh := range shapes {
		set, sc := s.solveShape(sh.ws, sh.roles, allow)
		if bestWs == nil || sc > bestScore {
			bestWs, best, bestScore = sh.ws, set, sc
		}
	}
	// a row the layout added but nothing can fill (no worn weapon, no
	// candidate under the filter) is not a shopping-list line — drop it
	outWs, outSet := bestWs[:0:0], best[:0:0]
	for i := range best {
		if bestWs[i].Item.ID == 0 && best[i].ID == 0 {
			continue
		}
		outWs, outSet = append(outWs, bestWs[i]), append(outSet, best[i])
	}
	return outWs, outSet, bestScore
}

// solveShape runs the coordinate descent plus tier-completion seeds for
// one fixed slot shape (see the package comment above).
func (s *Scorer) solveShape(ws []Worn, roles []weaponRole, allow func(*db.Item) bool) ([]*db.Item, float64) {
	set := itemsOf(ws)
	cands := s.slotCandidates(set, roles, allow)

	best := append([]*db.Item(nil), set...)
	s.descend(best, cands)
	bestScore := s.planScore(best)

	// tier-completion seeds: force each curated set's best pieces into a
	// copy of the base result, then descend freely — the bonus-aware
	// objective decides whether the threshold survives
	for _, name := range curatedSetsIn(s.rules(), cands) {
		for _, b := range s.rules().SetBonusesForRole(name, s.prof.Role) {
			if b.EP <= 0 {
				continue
			}
			seed := s.seedTierSet(best, cands, name, b.Pieces)
			if seed == nil {
				continue
			}
			s.descend(seed, cands)
			if sc := s.planScore(seed); sc > bestScore {
				best, bestScore = seed, sc
			}
		}
	}
	alignOwned(ws, roles, best)
	return best, bestScore
}

// itemsOf extracts the plain items of a worn set.
func itemsOf(ws []Worn) []*db.Item {
	set := make([]*db.Item, len(ws))
	for i, w := range ws {
		set[i] = w.Item
	}
	return set
}

// SlotCount is one entry of the standard equipment layout.
type SlotCount struct {
	Slot db.Slot
	N    int
}

// StandardSlots is the full level-80 equipment layout, in the game's
// character-pane order. Weapon rows are absent on purpose: their count
// is the layout solver's business (reshapeWorn), and they sort between
// trinkets and ranged. The BiS solver synthesizes missing slots from
// this table and the gear screen renders their empty rows from it.
var StandardSlots = []SlotCount{
	{db.SlotHead, 1}, {db.SlotNeck, 1}, {db.SlotShoulder, 1},
	{db.SlotBack, 1}, {db.SlotChest, 1}, {db.SlotWrist, 1},
	{db.SlotHands, 1}, {db.SlotWaist, 1}, {db.SlotLegs, 1},
	{db.SlotFeet, 1}, {db.SlotFinger, 2}, {db.SlotTrinket, 2},
	{db.SlotRanged, 1},
}

// SlotRank orders slots canonically for inserting synthesized rows.
func SlotRank(s db.Slot) int {
	for i, std := range StandardSlots {
		if std.Slot == s {
			return i
		}
	}
	if s == db.SlotWeapon {
		return len(StandardSlots) - 1 // between trinkets and ranged
	}
	return len(StandardSlots)
}

// fillMissingSlots inserts an empty-baseline row for every standard
// slot the worn set lacks (naked slot, missing second ring/trinket), so
// never-worn slots get solved and shopped too. Rows that stay
// unfillable under the filter are dropped again by BestSet. Worn rows
// keep their order; synthesized rows land next to their slot mates, or
// at the canonical position when the slot is entirely absent.
func fillMissingSlots(ws []Worn) []Worn {
	count := map[db.Slot]int{}
	for _, w := range ws {
		count[w.Item.Slot]++
	}
	out := append([]Worn(nil), ws...)
	for _, std := range StandardSlots {
		for count[std.Slot] < std.N {
			count[std.Slot]++
			row := Worn{Item: &db.Item{ID: 0, Name: "(empty)", Slot: std.Slot}}
			at := len(out)
			for i := len(out) - 1; i >= 0; i-- {
				if out[i].Item.Slot == std.Slot {
					at = i + 1 // right after its slot mates
					break
				}
				if SlotRank(out[i].Item.Slot) > SlotRank(std.Slot) {
					at = i // keep sliding toward the canonical spot
				}
			}
			out = append(out[:at], append([]Worn{row}, out[at:]...)...)
		}
	}
	return out
}

// alignOwned permutes the result within interchangeable slot groups
// (rings, trinkets, dual-wielded weapons) so pieces already worn stay
// in their own slot — the descent walks slots in order and can drift an
// owned pick sideways (upgrade ring 1 to the piece worn in ring 2, hand
// ring 2 the piece worn in ring 1), which reads as "acquire the ring
// you already own". Permuting within a slot group never changes the
// score: same items, same totals — but weapon rows carry hand roles, so
// a permutation only lands if every piece stays legal for its row.
func alignOwned(ws []Worn, roles []weaponRole, set []*db.Item) {
	groups := map[db.Slot][]int{}
	for i, w := range ws {
		groups[w.Item.Slot] = append(groups[w.Item.Slot], i)
	}
	for _, idxs := range groups {
		if len(idxs) < 2 {
			continue
		}
		assigned := make([]*db.Item, len(idxs))
		used := make([]bool, len(idxs))
		// anchor picks to the slots that already wear them
		for k, i := range idxs {
			for p, j := range idxs {
				if !used[p] && set[j].ID == ws[i].Item.ID {
					assigned[k], used[p] = set[j], true
					break
				}
			}
		}
		// the rest keep their solver order
		p := 0
		for k := range assigned {
			if assigned[k] != nil {
				continue
			}
			for used[p] {
				p++
			}
			assigned[k], used[p] = set[idxs[p]], true
		}
		legal := true
		for k, i := range idxs {
			legal = legal && roleAllows(roles[i], assigned[k])
		}
		if !legal {
			continue // e.g. the owned main-hand-only blade can't take the off hand
		}
		for k, i := range idxs {
			set[i] = assigned[k]
		}
	}
}

// planScore is the assumed score of a full item set: capped stat value,
// weapon DPS and set bonuses, best-case gems/enchants and the set-level
// cap-aware reforge plan (AssumedSetScore).
func (s *Scorer) planScore(set []*db.Item) float64 {
	return s.AssumedSetScore(set)
}

// SwapDelta is the set-score change from replacing ws[i] with cand,
// set-bonus threshold crossings included — the same MIXED basis the
// upgrade ranking uses (tuned actuals on the worn side, fresh-drop
// assumptions on the candidate), exposed for single swaps. That basis
// answers "is this drop better than what I wear as it is tuned today";
// for pure item quality — both sides on assumptions — use
// ItemSwapDelta, which is what the BiS shopping list prices with.
func (s *Scorer) SwapDelta(ws []Worn, i int, cand *db.Item) float64 {
	tot := s.TotalsWorn(ws)
	woCur := sub(tot, s.WornStats(ws[i]))
	return s.Value(add(woCur, s.EffectiveStats(cand))) - s.Value(tot) +
		s.itemBonusValue(cand) - s.itemBonusValue(ws[i].Item) +
		s.setBonusSwapDelta(setCounts(ws), ws[i].Item.SetName, cand.SetName)
}

// slotCandidates lists the allowed candidates per slot, sorted by ID so
// the descent is deterministic. Slots sharing a db.Slot and hand role
// (rings, trinkets) share one list; weapon rows only see items legal
// for their role.
func (s *Scorer) slotCandidates(set []*db.Item, roles []weaponRole, allow func(*db.Item) bool) [][]*db.Item {
	type key struct {
		slot db.Slot
		role weaponRole
	}
	lists := map[key][]*db.Item{}
	for i, cur := range set {
		lists[key{cur.Slot, roles[i]}] = nil
	}
	for _, cand := range s.d.Pool() {
		if !cand.UsableBy(s.prof.Class) || !s.typeAllowed(cand) {
			continue
		}
		if allow != nil && !allow(cand) {
			continue
		}
		for k := range lists {
			if k.slot == cand.Slot && roleAllows(k.role, cand) {
				lists[k] = append(lists[k], cand)
			}
		}
	}
	for _, list := range lists {
		// suffix variants share an ID: the name breaks the tie, so the
		// descent stays deterministic
		sort.Slice(list, func(a, b int) bool {
			if list[a].ID != list[b].ID {
				return list[a].ID < list[b].ID
			}
			return list[a].Name < list[b].Name
		})
	}
	out := make([][]*db.Item, len(set))
	for i, cur := range set {
		out[i] = lists[key{cur.Slot, roles[i]}]
	}
	return out
}

// descend runs coordinate descent in place: each slot re-picked against
// the rest of the set until a full sweep improves nothing. Candidates
// are priced exactly like the upgrade ranking — caps on set totals,
// weapon DPS, set-bonus crossings, and the candidate's own reforge
// chosen cap-aware against the rest (swapEffective) over the set's
// current reforge plan, which is re-solved after every accepted pick.
// Unique-equipped and category exclusivity are checked against the
// evolving set.
func (s *Scorer) descend(set []*db.Item, cands [][]*db.Item) {
	const eps = 1e-9
	for pass := 0; pass < 12; pass++ {
		improved := false
		tot, refs := s.AssumedPlan(set)
		for i := range set {
			if len(cands[i]) == 0 {
				continue
			}
			ws := wrapWorn(set)
			base := s.Value(tot)
			woCur := sub(tot, s.plannedStats(set[i], refs[i]))
			counts := setCounts(ws)
			bestDelta := eps
			var bestCand *db.Item
			for _, cand := range cands[i] {
				if cand.ID == set[i].ID {
					continue
				}
				if cand.Unique && wornElsewhere(set, i, cand.ID) {
					continue
				}
				if g := s.d.UniqueGroup(cand.ID); g != "" && s.groupWornElsewhere(set, i, g) {
					continue
				}
				eff, _ := s.swapEffective(cand, SwapContext{}, woCur)
				delta := s.Value(add(woCur, eff)) - base +
					s.itemBonusValue(cand) - s.itemBonusValue(set[i]) +
					s.setBonusSwapDelta(counts, set[i].SetName, cand.SetName)
				if delta > bestDelta {
					bestDelta, bestCand = delta, cand
				}
			}
			if bestCand != nil {
				set[i] = bestCand
				improved = true
				tot, refs = s.AssumedPlan(set) // the plan shifts with the pick
			}
		}
		if !improved {
			return
		}
	}
}

// curatedSetsIn lists the tier sets with curated bonuses that have
// pieces in the candidate pool, in deterministic order.
func curatedSetsIn(r *Rules, cands [][]*db.Item) []string {
	seen := map[string]bool{}
	var names []string
	for _, list := range cands {
		for _, it := range list {
			if it.SetName == "" || seen[it.SetName] || r.SetBonusesFor(it.SetName) == nil {
				continue
			}
			seen[it.SetName] = true
			names = append(names, it.SetName)
		}
	}
	sort.Strings(names)
	return names
}

// seedTierSet copies the set with the n best pieces of the named tier
// set forced in — best meaning the smallest standalone loss versus the
// slot's current pick, so the seed starts as close to optimal as the
// threshold allows. Nil when the pool cannot field n pieces.
func (s *Scorer) seedTierSet(set []*db.Item, cands [][]*db.Item, name string, n int) []*db.Item {
	type option struct {
		slot int
		it   *db.Item
		loss float64 // current pick's value minus the tier piece's
	}
	var opts []option
	for i := range set {
		var pick *db.Item
		for _, cand := range cands[i] {
			if cand.SetName != name {
				continue
			}
			if pick == nil || s.dot(s.EffectiveStats(cand)) > s.dot(s.EffectiveStats(pick)) {
				pick = cand
			}
		}
		if pick == nil || pick.ID == set[i].ID {
			if pick != nil {
				n-- // already wearing it: counts toward the threshold
			}
			continue
		}
		opts = append(opts, option{slot: i, it: pick,
			loss: s.dot(s.EffectiveStats(set[i])) - s.dot(s.EffectiveStats(pick))})
	}
	if n <= 0 {
		return nil // threshold already met: the base descent covered it
	}
	if len(opts) < n {
		return nil
	}
	sort.Slice(opts, func(a, b int) bool {
		if opts[a].loss != opts[b].loss {
			return opts[a].loss < opts[b].loss
		}
		return opts[a].slot < opts[b].slot
	})
	seed := append([]*db.Item(nil), set...)
	for _, o := range opts[:n] {
		seed[o.slot] = o.it
	}
	return seed
}
