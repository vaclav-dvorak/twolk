package score

import (
	"sort"

	"github.com/vaclav-dvorak/twolk/internal/db"
)

// Whole-set gem optimizer: finds the best gem assignment across every
// socket of the equipped set AT ONCE, which per-socket reasoning cannot
// do — the classic case is sitting just under the hit cap, where the
// optimum is one hybrid gem (exactly enough hit) instead of a pure hit
// gem (overflow wasted) or a pure agility gem (cap lost).
//
// Sockets couple through four things only, and the search handles each
// exactly:
//   - the hit-cap total (dynamic programming dimension),
//   - socket-bonus matching (per-item package enumeration),
//   - meta-gem activation (color counts in the DP state, checked against
//     the worn meta's real condition),
//   - unique-equipped gems (Nightmare Tear): a bitmask of used unique
//     gems in the DP state keeps each to one socket set-wide,
//   - jewelcrafter Dragon's Eyes: a count in the DP state enforces the
//     in-game "Jeweler's Gems (3)" limit set-wide (duplicates of one
//     cut are legal, so a bitmask can't express it).
//
// Expertise is a SECOND capped dimension (dpKey.exp), handled exactly
// like hit: when its below-cap weight beats the alternatives the DP
// fills it to the dodge cap and then stops, instead of trading capped
// expertise away for a nominally heavier stat (Str) and dropping the
// character back under the cap.
//
// ArP is DELIBERATELY not a cap-seeking dimension despite also being
// capped: unlike hit/expertise (linear to their caps) ArP is convex
// (marginal value rises toward 1400) and build-dependent (only stacked
// with a passive ArP-proc trinket, which caps below 1400). A floor would
// wrongly force every build toward the cap. It stays valued linearly at
// its sim weight during the search — gems rarely straddle it, and its
// weight is below Str so the search never gems it up anyway — with the
// reported delta computed exactly (cap-clamped) on the final assignment.

// GemPlan is the optimizer's result: per-item re-gemming tips (changed
// sockets only, same shape the quick-tips pane renders), the full
// optimized set (for exact stat diffs), and the exact set-score
// improvement of applying everything.
type GemPlan struct {
	Tips  []GemTip
	Set   []Worn // the worn set with the plan applied
	Delta float64
	// UnknownMeta is the worn meta whose activation rule the rules
	// table doesn't cover: the search ran WITHOUT an activation
	// constraint (no rule is invented) and the UI must say so.
	UnknownMeta *db.Gem
}

// gemPack is one candidate gem assignment for a single item's sockets.
type gemPack struct {
	gems    []*db.Gem
	v0      float64 // linear value excluding the capped DP stats (hit, expertise), bonus included
	hit     int
	exp     int // expertise rating in the pack (the second capped dimension)
	r, y, b int // meta-activation color counts (clamped later)
	uniq    int // bitmask of unique-equipped gems used (see uniqBits)
	jc      int // Dragon's Eyes in the pack (jeweler quota)
	changes int // sockets that differ from the currently worn gems
}

// dpKey couples items: hit from gems (clamped at what the set can still
// use), meta color counts (clamped at the largest requirement), the
// unique-equipped gems already spent, and the jeweler-gem count.
type dpKey struct {
	hit     int
	exp     int
	r, y, b int
	uniq    int
	jc      int
}

const metaCountClamp = 5 // largest MetaCondition requirement (Tenacious: 5 blue)

// OptimizeGems computes the best whole-set gem assignment for the worn
// set (enriched imports only: items with actual gem data and sockets).
// A belt's buckle socket is part of the search: its worn gem is
// re-gemmable like any other, and a belt WITHOUT the buckle offers it
// as an optional extra socket — a gem there means "apply the buckle
// first" (GemChange.SocketIdx past the item's own sockets).
func (s *Scorer) OptimizeGems(ws []Worn) GemPlan {
	hitStat, hitCap, hasHit := s.capTarget()
	expCap, hasExp := s.expTarget()

	// items with sockets we can re-gem (incl. a belt's buckle socket)
	var idxs []int
	for i, w := range ws {
		if w.HasDetail && len(s.assumedSockets(w.Item)) > 0 {
			idxs = append(idxs, i)
		}
	}
	if len(idxs) == 0 {
		return GemPlan{}
	}

	// base = the set with every optimizable socket emptied (extra gems
	// beyond the optimizable sockets — blacksmith bracer/glove sockets —
	// stay; they are not modeled)
	base := make([]Worn, len(ws))
	copy(base, ws)
	for _, i := range idxs {
		w := ws[i]
		gems := make([]int, len(w.Gems))
		copy(gems, w.Gems)
		for si := 0; si < min(len(gems), len(s.assumedSockets(w.Item))); si++ {
			gems[si] = 0
		}
		nw := w
		nw.Gems = gems
		base[i] = nw
	}
	baseTot := s.TotalsWorn(base)
	baseHit := 0.0
	if hasHit {
		baseHit = baseTot[hitStat]
	}
	// hit from gems is only worth anything up to this much
	hitNeed := 0
	if hitCap > baseHit {
		hitNeed = int(hitCap - baseHit)
	}
	// same for expertise (the second capped dimension)
	baseExp := 0.0
	if hasExp {
		baseExp = baseTot[db.Expertise]
	}
	expNeed := 0
	if hasExp && expCap > baseExp {
		expNeed = int(expCap - baseExp)
	}

	// the worn meta's activation condition constrains color counts; a
	// meta with no curated rule constrains NOTHING — inventing one
	// (the old one-of-each fallback) made the search buy activation
	// gems the meta never asked for — and is reported on the plan
	var metaCond *db.MetaCondition
	var unknownMeta *db.Gem
	for _, i := range idxs {
		for si, c := range ws[i].Item.GemSockets {
			if c != db.GemMeta {
				continue
			}
			g := s.metaCandidate(ws[i], si)
			if g == nil {
				continue
			}
			if cond, ok := s.d.MetaConditionFor(g.ID); ok {
				metaCond = &cond
			} else {
				unknownMeta = g
			}
		}
	}

	// unique-equipped gems in play each get a mask bit; the DP refuses
	// to spend one twice. Gems that stay put (extras beyond the
	// optimizable sockets, i.e. blacksmith bracer/glove sockets) are
	// pre-spent in the start state; a belt's buckle gem is NOT — the DP
	// owns that socket.
	shared := s.socketCandidates(hitStat, hasHit, hasExp)
	uniqBit := map[int]int{}
	register := func(g *db.Gem) {
		if g != nil && g.Unique {
			if _, ok := uniqBit[g.ID]; !ok {
				uniqBit[g.ID] = len(uniqBit)
			}
		}
	}
	register(s.bestGem[db.GemMeta])
	for _, g := range shared {
		register(g)
	}
	startMask := 0
	startJC := 0 // Dragon's Eyes frozen outside the optimizable sockets
	for _, w := range ws {
		if !w.HasDetail {
			continue
		}
		for gi, id := range w.Gems {
			g := s.d.ResolveGem(id)
			register(g)
			if gi >= len(s.assumedSockets(w.Item)) {
				if g != nil && g.Unique {
					startMask |= 1 << uniqBit[g.ID]
				}
				if isJewelerGem(g) {
					startJC++
				}
			}
		}
	}

	// curved stats (ArP) enter the DP's linear proxy at the slope they
	// have HERE — the search cannot represent an integral, and the
	// table weight would make it trade ArP away exactly where the
	// character values it most. The winner is re-scored exactly below.
	lw := s.localWeights(baseTot)
	packs := make([][]gemPack, len(idxs))
	for k, i := range idxs {
		packs[k] = s.enumeratePacks(ws[i], shared, uniqBit, hitStat, hasHit, hitNeed, hasExp, expNeed, lw)
	}

	// DP across items. Many distinct assignments tie at the optimum
	// (equal gems across equal sockets); ties break toward the FEWEST
	// changed sockets so the plan is minimal and, run to run, stable —
	// map iteration order must never pick the representative.
	const eps = 1e-9
	type cell struct {
		v0      float64
		changes int
		prev    dpKey
		choice  int
		set     bool
	}
	better := func(v float64, ch int, old cell, ok bool) bool {
		if !ok {
			return true
		}
		if v > old.v0+eps {
			return true
		}
		return v > old.v0-eps && ch < old.changes
	}
	cur := map[dpKey]cell{{uniq: startMask, jc: startJC}: {set: true}}
	trace := make([]map[dpKey]cell, len(idxs))
	for k := range idxs {
		next := map[dpKey]cell{}
		for key, c := range cur {
			for pi, p := range packs[k] {
				if key.uniq&p.uniq != 0 {
					continue // a unique gem is already socketed elsewhere
				}
				if key.jc+p.jc > jewelerGemLimit {
					continue // out of Dragon's Eyes
				}
				nk := dpKey{
					hit:  min(key.hit+p.hit, hitNeed),
					exp:  min(key.exp+p.exp, expNeed),
					r:    min(key.r+p.r, metaCountClamp),
					y:    min(key.y+p.y, metaCountClamp),
					b:    min(key.b+p.b, metaCountClamp),
					uniq: key.uniq | p.uniq,
					jc:   key.jc + p.jc,
				}
				nv := c.v0 + p.v0
				nch := c.changes + p.changes
				if old, ok := next[nk]; better(nv, nch, old, ok) {
					next[nk] = cell{v0: nv, changes: nch, prev: key, choice: pi, set: true}
				}
			}
		}
		trace[k] = next
		cur = next
	}

	// pick the best feasible terminal state; the capped DP stats count at
	// their weights up to their needs (already clamped into the key)
	hitW := 0.0
	if hasHit {
		hitW = s.prof.Weights[hitStat]
	}
	expW := 0.0
	if hasExp {
		expW = s.prof.Weights[db.Expertise]
	}
	bestKey, found := dpKey{}, false
	bestVal, bestCh := 0.0, 0
	pick := func(constrained bool) {
		for key, c := range cur {
			if constrained && metaCond != nil && !metaCond.Met(key.r, key.y, key.b) {
				continue
			}
			v := c.v0 + hitW*float64(key.hit) + expW*float64(key.exp)
			if !found || v > bestVal+eps || (v > bestVal-eps && c.changes < bestCh) {
				bestKey, bestVal, bestCh, found = key, v, c.changes, true
			}
		}
	}
	pick(true)
	if !found {
		// no assignment satisfies the meta condition: fall back to the
		// unconstrained optimum rather than returning nothing
		pick(false)
	}

	// reconstruct choices
	choices := make([]int, len(idxs))
	key := bestKey
	for k := len(idxs) - 1; k >= 0; k-- {
		c := trace[k][key]
		choices[k] = c.choice
		key = c.prev
	}

	// build the new set and the exact delta
	opt := make([]Worn, len(ws))
	copy(opt, ws)
	for k, i := range idxs {
		p := packs[k][choices[k]]
		w := ws[i]
		gems := make([]int, max(len(w.Gems), len(p.gems)))
		copy(gems, w.Gems)
		for si, g := range p.gems {
			if g != nil {
				gems[si] = g.ID
			}
		}
		nw := w
		nw.Gems = gems
		opt[i] = nw
	}
	plan := GemPlan{Set: opt, Delta: s.Value(s.TotalsWorn(opt)) - s.Value(s.TotalsWorn(ws)),
		UnknownMeta: unknownMeta}
	// the DP optimizes a linear proxy; where that proxy disagrees with
	// the exact valuation (a curved stat), its answer can come out
	// WORSE than what is already socketed. Never hand out a losing
	// plan — the current gems stand.
	if plan.Delta <= 0 {
		return GemPlan{UnknownMeta: unknownMeta}
	}

	// per-item tips: changed sockets only, delta = applying just this
	// item's change to the current set (matches GemTips semantics; the
	// per-item deltas need not sum to the total around the cap)
	curTot := s.TotalsWorn(ws)
	curVal := s.Value(curTot)
	for k, i := range idxs {
		p := packs[k][choices[k]]
		sockets := s.assumedSockets(ws[i].Item)
		var changes []GemChange
		matched := true
		for si, g := range p.gems {
			if g == nil {
				continue
			}
			var have *db.Gem
			if si < len(ws[i].Gems) && ws[i].Gems[si] != 0 {
				have = s.d.ResolveGem(ws[i].Gems[si])
			}
			if have == nil || have.ID != g.ID {
				changes = append(changes, GemChange{SocketIdx: si, From: have, To: g})
			}
			if !db.ColorMatches(sockets[si], g.Color) {
				matched = false
			}
		}
		if len(changes) == 0 {
			continue
		}
		delta := s.Value(add(sub(curTot, s.WornStats(ws[i])), s.WornStats(opt[i]))) - curVal
		plan.Tips = append(plan.Tips, GemTip{ItemIdx: i, Changes: changes, Matched: matched, Delta: delta})
	}
	sort.Slice(plan.Tips, func(a, b int) bool { return plan.Tips[a].Delta > plan.Tips[b].Delta })
	return plan
}

// capTarget picks the capped stat the DP trades against: the profile's
// hit-cap stat when it is capped, else the first capped stat any
// eligible gem carries. ok is false when gems cannot touch any cap.
func (s *Scorer) capTarget() (db.Stat, float64, bool) {
	if h := s.prof.HitCap; h != nil {
		for _, c := range s.prof.Caps {
			if c.Stat == h.Stat && s.gemsCarry(c.Stat) {
				return c.Stat, c.Cap, true
			}
		}
	}
	for _, c := range s.prof.Caps {
		if s.gemsCarry(c.Stat) {
			return c.Stat, c.Cap, true
		}
	}
	return 0, 0, false
}

// expTarget reports the expertise cap when the profile caps expertise and
// gems can carry it — the second DP dimension, so the optimizer fills
// expertise to the dodge cap (when its weight beats the alternatives)
// rather than trading capped expertise away for Strength. Distinct from
// capTarget, which owns the hit dimension; a melee DPS profile has both.
func (s *Scorer) expTarget() (float64, bool) {
	if hs, _, ok := s.capTarget(); ok && hs == db.Expertise {
		return 0, false // capTarget already owns expertise as its dimension
	}
	for _, c := range s.prof.Caps {
		if c.Stat == db.Expertise && s.gemsCarry(db.Expertise) {
			return c.Cap, true
		}
	}
	return 0, false
}

// metaCandidate picks the gem for a meta socket: the worn meta if any,
// else the assumed best meta.
func (s *Scorer) metaCandidate(w Worn, si int) *db.Gem {
	if si < len(w.Gems) && w.Gems[si] != 0 {
		if g := s.d.ResolveGem(w.Gems[si]); g != nil && g.Color == db.GemMeta {
			return g
		}
	}
	return s.bestGem[db.GemMeta]
}

// enumeratePacks lists Pareto-worthy gem assignments for one item's
// sockets: the cartesian product of per-socket candidates, reduced to
// the best v0 per (hit, colors, uniques) signature. Socket bonus
// applies when every socket matches its color. Packs socketing the
// same unique gem twice are never emitted.
//
// On waist items the socket list includes the buckle's extra socket
// (assumedSockets). Its worn gem, when present, is re-gemmable like any
// other; when absent, nil ("don't apply the buckle") is a candidate —
// it never breaks the socket-bonus match and costs no change, so the
// optimizer only proposes the buckle when a gem there actually pays.
func (s *Scorer) enumeratePacks(w Worn, shared []*db.Gem, uniqBit map[int]int, hitStat db.Stat, hasHit bool, hitNeed int, hasExp bool, expNeed int, lw db.Stats) []gemPack {
	sockets := s.assumedSockets(w.Item)
	native := len(w.Item.GemSockets)
	cands := make([][]*db.Gem, len(sockets))
	for si, c := range sockets {
		if c == db.GemMeta {
			if g := s.metaCandidate(w, si); g != nil {
				cands[si] = []*db.Gem{g}
			} else {
				cands[si] = []*db.Gem{nil}
			}
			continue
		}
		if c == db.GemCogwheel {
			// engineer-only socket: regular gems don't fit, so the
			// worn cogwheel (or nothing) is the only candidate
			var worn *db.Gem
			if si < len(w.Gems) && w.Gems[si] != 0 {
				worn = s.d.ResolveGem(w.Gems[si])
			}
			if worn != nil {
				cands[si] = []*db.Gem{worn}
			} else {
				cands[si] = []*db.Gem{nil}
			}
			continue
		}
		cands[si] = shared
		if si >= native && (si >= len(w.Gems) || w.Gems[si] == 0) {
			// unapplied buckle socket: leaving it out must be representable
			cands[si] = append([]*db.Gem{nil}, shared...)
		}
		// the currently worn gem is always a candidate for its own
		// socket, even when an equal-value twin shadowed it in the
		// shared pool — "keep what you have" must be representable
		if si < len(w.Gems) && w.Gems[si] != 0 {
			if g := s.d.ResolveGem(w.Gems[si]); g != nil && g.Color != db.GemMeta {
				present := false
				for _, c := range shared {
					if c.ID == g.ID {
						present = true
						break
					}
				}
				if !present {
					cands[si] = append(append([]*db.Gem(nil), shared...), g)
				}
			}
		}
	}

	var packs []gemPack
	var rec func(si int, cur []*db.Gem)
	rec = func(si int, cur []*db.Gem) {
		if si == len(sockets) {
			p := gemPack{gems: append([]*db.Gem(nil), cur...)}
			for _, g := range cur {
				if isJewelerGem(g) {
					p.jc++
				}
				if g == nil || !g.Unique {
					continue
				}
				bit := 1 << uniqBit[g.ID]
				if p.uniq&bit != 0 {
					return // same unique gem twice in one item: illegal
				}
				p.uniq |= bit
			}
			if p.jc > jewelerGemLimit {
				return // more Dragon's Eyes than the game allows at all
			}
			for i, g := range cur {
				var have int
				if i < len(w.Gems) && w.Gems[i] != 0 {
					if hg := s.d.ResolveGem(w.Gems[i]); hg != nil {
						have = hg.ID
					}
				}
				if g != nil && g.ID != have {
					p.changes++
				}
			}
			var st db.Stats
			matched := true
			for i, g := range cur {
				if g == nil {
					if i < native {
						matched = false // a real empty socket breaks the bonus
					}
					continue // an unfilled buckle socket doesn't
				}
				st = add(st, g.Stats)
				if !db.ColorMatches(sockets[i], g.Color) {
					matched = false
				}
				switch g.Color {
				case db.GemRed:
					p.r++
				case db.GemYellow:
					p.y++
				case db.GemBlue:
					p.b++
				case db.GemOrange:
					p.r++
					p.y++
				case db.GemPurple:
					p.r++
					p.b++
				case db.GemGreen:
					p.y++
					p.b++
				case db.GemPrismatic:
					p.r++
					p.y++
					p.b++
				}
			}
			if matched && native > 0 {
				st = add(st, w.Item.SocketBonus)
			}
			if hasHit {
				p.hit = int(st[hitStat])
				st[hitStat] = 0
				// zero the twin side too: items/gems duplicate unified
				// ratings and the profile weights only one side anyway
				st[twinOf(hitStat)] = 0
			}
			if hasExp {
				// expertise is scored via the DP dimension, not v0
				p.exp = int(st[db.Expertise])
				st[db.Expertise] = 0
			}
			p.v0 = dotWith(st, lw)
			packs = append(packs, p)
			return
		}
		for _, g := range cands[si] {
			rec(si+1, append(cur, g))
		}
	}
	rec(0, nil)

	// keep the best v0 per (hit, r, y, b, uniq, jc) signature; ties keep
	// the pack closest to what is already socketed
	best := map[dpKey]int{}
	var out []gemPack
	for _, p := range packs {
		k := dpKey{hit: min(p.hit, hitNeed+20), exp: min(p.exp, expNeed+20), r: min(p.r, metaCountClamp),
			y: min(p.y, metaCountClamp), b: min(p.b, metaCountClamp), uniq: p.uniq, jc: p.jc}
		if j, ok := best[k]; ok {
			if p.v0 > out[j].v0+1e-9 ||
				(p.v0 > out[j].v0-1e-9 && p.changes < out[j].changes) {
				out[j] = p
			}
			continue
		}
		best[k] = len(out)
		out = append(out, p)
	}
	return out
}

// twinOf maps a unified rating to its duplicated other side.
func twinOf(st db.Stat) db.Stat {
	switch st {
	case db.MeleeHit:
		return db.SpellHit
	case db.SpellHit:
		return db.MeleeHit
	}
	return st
}

// socketCandidates returns the gems worth considering for a colored
// socket: per (color, hit-content) the highest-valued endgame gem. Pure
// value picks, pure hit picks and hybrids all survive, so the DP can
// trade hit for value at every granularity. When the set cannot use any
// more hit, hit-carrying gems are still offered — the DP simply values
// their hit at zero. Jewelcrafters additionally get the Dragon's Eye
// cuts, keyed apart from the epic cuts so the epic alternative survives
// for sockets past the jeweler quota.
func (s *Scorer) socketCandidates(hitStat db.Stat, hasHit, hasExp bool) []*db.Gem {
	type key struct {
		c   db.GemColor
		hit int
		exp int
		jc  bool
	}
	best := map[key]*db.Gem{}
	for _, g := range s.d.Gems {
		// metas and cogwheels never enter the colored-socket pool:
		// metas have their own socket, cogwheels only fit engineer
		// cogwheel sockets
		if g.Color == db.GemMeta || g.Color == db.GemCogwheel {
			continue
		}
		if isJewelerGem(g) {
			if !s.jc() {
				continue
			}
		} else if g.Quality != s.rules().GemQuality {
			continue
		}
		hit, exp := 0, 0
		var st db.Stats
		st = add(st, g.Stats)
		if hasHit {
			hit = int(st[hitStat])
			st[hitStat] = 0
			st[twinOf(hitStat)] = 0
		}
		if hasExp {
			exp = int(st[db.Expertise])
			st[db.Expertise] = 0
		}
		k := key{c: g.Color, hit: hit, exp: exp, jc: isJewelerGem(g)}
		cur := best[k]
		if cur == nil {
			best[k] = g
			continue
		}
		var cst db.Stats
		cst = add(cst, cur.Stats)
		if hasHit {
			cst[hitStat] = 0
			cst[twinOf(hitStat)] = 0
		}
		if hasExp {
			cst[db.Expertise] = 0
		}
		// deterministic: better value, then lower ID (standard cuts)
		if v, cv := s.dot(st), s.dot(cst); v > cv || (v == cv && g.ID < cur.ID) {
			best[k] = g
		}
	}
	out := make([]*db.Gem, 0, len(best))
	for _, g := range best {
		out = append(out, g)
	}
	sort.Slice(out, func(a, b int) bool { return out[a].ID < out[b].ID })
	return out
}
