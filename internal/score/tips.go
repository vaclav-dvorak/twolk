package score

import (
	"sort"
	"strings"

	"github.com/vaclav-dvorak/twolk/internal/db"
)

// GemChange is one socket's recommended swap. From nil = empty socket.
type GemChange struct {
	SocketIdx int
	From, To  *db.Gem
}

// GemTip proposes re-gemming one equipped item: only the sockets that
// actually change are listed. Worn prismatics (meta-keepers) are never
// swapped for colored gems — at most upgraded to the best prismatic
// (Nightmare Tear) — since set-wide meta activation is not modeled.
type GemTip struct {
	ItemIdx int
	Changes []GemChange
	Matched bool // the resulting package keeps the socket bonus
	Delta   float64
}

// wornUniqueGems collects the unique-equipped gem IDs actually socketed
// anywhere in the set (enriched items only).
func (s *Scorer) wornUniqueGems(ws []Worn) map[int]bool {
	return s.UniquesWornOutside(ws, -1)
}

// UniquesWornOutside collects the unique-equipped gem IDs socketed on
// enriched worn items OTHER than slot i (-1 = the whole set) — the gems
// a replacement for slot i cannot assume, because they stay socketed.
func (s *Scorer) UniquesWornOutside(ws []Worn, i int) map[int]bool {
	used := map[int]bool{}
	for j, w := range ws {
		if j == i || !w.HasDetail {
			continue
		}
		for _, id := range w.Gems {
			if g := s.d.ResolveGem(id); g != nil && g.Unique {
				used[g.ID] = true
			}
		}
	}
	return used
}

// GemTips lists profitable re-gemmings for items with actual gem data,
// best first. Detail-less items are skipped — they already score under
// optimal-gem assumptions. A unique-equipped gem already worn anywhere
// in the set (or suggested once by this pass) is never suggested again
// — the socket falls back to the best non-unique gem instead.
//
// An enhancement socket (buckle, blacksmith bracer/gloves) is
// re-gemmable like any other when its gem is actually there; an item
// WITHOUT the enhancement gets no suggestion for it here — adding the
// socket is SocketTips' business.
//
// Jewelcrafters: Dragon's Eyes are suggested where they beat the epic
// cut, at most three set-wide ("Jeweler's Gems (3)") counting the ones
// already worn; sockets past the quota fall back to the epic cut.
func (s *Scorer) GemTips(ws []Worn) []GemTip {
	tot := s.TotalsWorn(ws)
	base := s.Value(tot)
	// unique gem IDs worn (they stay) or suggested by this pass
	used := s.wornUniqueGems(ws)
	// Dragon's Eyes already worn consume the jeweler quota; suggestions
	// spend (and replacing one away refunds) as the pass goes
	jcCount := 0
	for _, w := range ws {
		if !w.HasDetail {
			continue
		}
		for _, id := range w.Gems {
			if isJewelerGem(s.d.ResolveGem(id)) {
				jcCount++
			}
		}
	}
	var tips []GemTip
	for i, w := range ws {
		if !w.HasDetail {
			continue
		}
		rec, recMatched := s.AssumedGems(w.Item)
		if len(rec) == 0 {
			continue
		}
		sockets := s.assumedSockets(w.Item)
		gems := make([]int, max(len(w.Gems), len(rec)))
		copy(gems, w.Gems) // extra (buckle/blacksmith) gems stay as-is
		var changes []GemChange
		matched := true
		for si, g := range rec {
			var cur *db.Gem
			if si < len(w.Gems) && w.Gems[si] != 0 {
				cur = s.d.ResolveGem(w.Gems[si])
			}
			if si >= len(w.Item.GemSockets) && cur == nil {
				// the socket only exists once the buckle is applied
				continue
			}
			// prismatics keep the meta active: upgrade within the
			// prismatic family only
			if cur != nil && cur.Color == db.GemPrismatic {
				g = s.bestPrism
			}
			if isJewelerGem(g) && !isJewelerGem(cur) && jcCount >= jewelerGemLimit {
				// jeweler quota spent: fall back to the epic cut
				switch {
				case recMatched:
					g = s.bestGem[sockets[si]]
				case sockets[si] == db.GemMeta:
					g = s.bestGem[db.GemMeta]
				default:
					g = s.bestAny
				}
			}
			if g != nil && g.Unique && used[g.ID] && (cur == nil || cur.ID != g.ID) {
				// the unique gem is spoken for elsewhere: fall back
				switch {
				case cur != nil && cur.Color == db.GemPrismatic:
					g = s.safePrism // stay in the family (usually nil: keep)
				case recMatched:
					g = s.safeGem[sockets[si]]
				case sockets[si] == db.GemMeta:
					g = s.safeGem[db.GemMeta]
				default:
					g = s.safeAny
				}
			}
			if g == nil {
				continue
			}
			if cur == nil || cur.ID != g.ID {
				gems[si] = g.ID
				changes = append(changes, GemChange{SocketIdx: si, From: cur, To: g})
				if g.Unique {
					used[g.ID] = true
				}
				if isJewelerGem(g) && !isJewelerGem(cur) {
					jcCount++
				}
				if isJewelerGem(cur) && !isJewelerGem(g) {
					jcCount--
				}
			}
			final := g
			if cur != nil && cur.ID == g.ID {
				final = cur
			}
			if !db.ColorMatches(sockets[si], final.Color) {
				matched = false
			}
		}
		if len(changes) == 0 {
			continue
		}
		nw := w
		nw.Gems = gems
		delta := s.Value(add(sub(tot, s.WornStats(w)), s.WornStats(nw))) - base
		if delta > 0.5 {
			tips = append(tips, GemTip{ItemIdx: i, Changes: changes, Matched: matched, Delta: delta})
		}
	}
	sort.Slice(tips, func(a, b int) bool { return tips[a].Delta > tips[b].Delta })
	return tips
}

// SocketTip proposes adding an item's missing enhancement socket —
// the belt buckle or a blacksmith's Socket Bracer/Gloves (see
// ExtraSocketName for the slot's enhancement) — plus the gem for it.
type SocketTip struct {
	ItemIdx int
	Gem     *db.Gem
	Delta   float64
}

// SocketTips lists missing enhancement sockets on enriched items: the
// scorer assumes the extra socket on every candidate (buckle for
// everyone, bracer/glove sockets for blacksmiths), so a bare worn item
// is genuinely behind — this is the "go add it" nudge, valued as the
// assumed gem dropped into the current set (cap-aware). Detail-less
// imports are skipped: they already score under the assumption. An
// applied-but-empty socket looks identical to a missing one in the
// armory data; the advice (socket a gem there) stays right either way.
// The gem is always the best NON-Dragon's-Eye pick — the jeweler-gem
// quota bookkeeping lives in GemTips and OptimizeGems.
func (s *Scorer) SocketTips(ws []Worn) []SocketTip {
	tot := s.TotalsWorn(ws)
	base := s.Value(tot)
	used := s.wornUniqueGems(ws)
	var tips []SocketTip
	for i, w := range ws {
		if !w.HasDetail || !s.AssumesExtraSocket(w.Item) || HasExtraGem(w) {
			continue
		}
		g := s.bestGem[db.GemPrismatic]
		if g != nil && g.Unique && used[g.ID] {
			g = s.safeGem[db.GemPrismatic]
		}
		if g == nil {
			continue
		}
		native := len(w.Item.GemSockets)
		gems := make([]int, native+1)
		copy(gems, w.Gems)
		gems[native] = g.ID
		nw := w
		nw.Gems = gems
		delta := s.Value(add(sub(tot, s.WornStats(w)), s.WornStats(nw))) - base
		if delta > 0.5 {
			tips = append(tips, SocketTip{ItemIdx: i, Gem: g, Delta: delta})
		}
	}
	sort.Slice(tips, func(a, b int) bool { return tips[a].Delta > tips[b].Delta })
	return tips
}

// enchantProfession names the profession an enchant is locked to, or "".
// The data's own profession lock decides where present; the name table
// below covers WotLK entries that predate the field. Ring enchants are
// enchanter-only in every version.
func enchantProfession(e *db.Enchant) string {
	if e.RequiredProfession != 0 {
		return (&db.CraftedSource{Profession: e.RequiredProfession}).ProfessionName()
	}
	name := e.Name
	switch {
	case strings.Contains(name, "Fur Lining"):
		return "Leatherworking"
	case strings.Contains(name, "Embroidery"):
		return "Tailoring"
	case strings.Contains(name, "Master's Inscription"):
		return "Inscription"
	case name == "Hyperspeed Accelerators" || name == "Nitro Boosts" ||
		name == "Flexweave Underlay" || name == "Reticulated Armor Webbing" ||
		name == "Springy Arachnoweave" || name == "Mind Amplification Dish" ||
		name == "Frag Belt" || strings.Contains(name, "Pulse Generator") ||
		name == "Gnomish X-Ray Scope": // BoP, Requires Engineering (425)
		return "Engineering"
	case e.Slot == db.SlotFinger:
		return "Enchanting" // ring enchants are enchanter-only
	}
	return ""
}

// enchantCandidates lists every enchant applicable to the item for
// this profile, honoring class locks, two-hand/shield/staff
// restrictions and profession-locked enchants (see SetProfessions).
func (s *Scorer) enchantCandidates(it *db.Item) []*db.Enchant {
	var out []*db.Enchant
	for _, e := range s.d.Enchants {
		if e.Slot != it.Slot {
			continue
		}
		if s.serverPhase > 0 && e.Phase > s.serverPhase {
			continue
		}
		if s.enchantOK != nil && !s.enchantOK(e) {
			continue
		}
		if !classAllowed(e.ClassAllowlist, s.prof.Class) || !s.prof.CanTriggerEnchant(e) {
			continue
		}
		switch e.EnchantType {
		case 1: // two-hand only
			if it.HandType != db.HandTwoHand {
				continue
			}
		case 2: // shield only
			if it.WeaponType != db.WeaponShield {
				continue
			}
		case 4: // staff only
			if it.WeaponType != db.WeaponStaff {
				continue
			}
		}
		if p := enchantProfession(e); p != "" && !s.professions[p] {
			continue
		}
		out = append(out, e)
	}
	return out
}

// BestEnchant picks the highest-valued enchant applicable to the item
// for this profile. It values by the calibrated weights (see
// Calibrate), so a set at or near the hit cap stops assuming hit
// enchants just like it stops assuming hit gems.
func (s *Scorer) BestEnchant(it *db.Item) *db.Enchant {
	var best *db.Enchant
	var bestV float64
	for _, e := range s.enchantCandidates(it) {
		if v := s.gemDot(s.enchantStats(e)); v > bestV {
			best, bestV = e, v
		}
	}
	return best
}

func classAllowed(allow []db.Class, c db.Class) bool {
	if len(allow) == 0 {
		return true
	}
	for _, a := range allow {
		if a == c {
			return true
		}
	}
	return false
}

// wornMetaAndColors scans an enriched set's actual gems: the worn meta
// gem (nil when none) and the per-category color counts (hybrids count
// toward both parent colors, prismatics toward all three).
func (s *Scorer) wornMetaAndColors(ws []Worn) (meta *db.Gem, r, y, b int) {
	for _, w := range ws {
		if !w.HasDetail {
			continue
		}
		for _, id := range w.Gems {
			g := s.d.ResolveGem(id)
			if g == nil {
				continue
			}
			if g.Color == db.GemMeta {
				meta = g
				continue
			}
			cr, cy, cb := gemColorCounts(g)
			r, y, b = r+cr, y+cy, b+cb
		}
	}
	return meta, r, y, b
}

// gemColorCounts is a single gem's contribution to meta activation
// counts: 0 or 1 per category (hybrids count both parents, prismatics
// all three, metas none).
func gemColorCounts(g *db.Gem) (r, y, b int) {
	if g == nil || g.Color == db.GemMeta {
		return 0, 0, 0
	}
	if db.ColorMatches(db.GemRed, g.Color) {
		r = 1
	}
	if db.ColorMatches(db.GemYellow, g.Color) {
		y = 1
	}
	if db.ColorMatches(db.GemBlue, g.Color) {
		b = 1
	}
	return r, y, b
}

// InactiveMeta returns the worn meta gem whose own activation rule the
// actual gems fail to meet, with that rule — or nil when no meta is
// worn, it is active, or its rule is unknown (no rule is invented; see
// UnknownMeta). Only meaningful for enriched imports.
func (s *Scorer) InactiveMeta(ws []Worn) (*db.Gem, db.MetaCondition) {
	meta, r, y, b := s.wornMetaAndColors(ws)
	if meta == nil {
		return nil, db.MetaCondition{}
	}
	if cond, ok := s.d.MetaConditionFor(meta.ID); ok && !cond.Met(r, y, b) {
		return meta, cond
	}
	return nil, db.MetaCondition{}
}

// UnknownMeta returns the worn meta gem whose activation rule the
// rules table doesn't cover — activation can be neither confirmed nor
// denied, and the UI surfaces it so the missing entry gets curated.
func (s *Scorer) UnknownMeta(ws []Worn) *db.Gem {
	meta, _, _, _ := s.wornMetaAndColors(ws)
	if meta == nil {
		return nil
	}
	if _, ok := s.d.MetaConditionFor(meta.ID); ok {
		return nil
	}
	return meta
}

// EnchantTip proposes adding or replacing an item's enchant.
type EnchantTip struct {
	ItemIdx int
	From    *db.Enchant // nil = no enchant
	To      *db.Enchant
	Delta   float64
}

// EnchantTips lists profitable enchant changes for items with actual
// enchant data, best first. Every applicable candidate is priced by
// its exact cap-aware set delta — a single weight-ranked pick would
// die at the hit cap (the hit enchant ranks first on the calibrated
// weights but gains ~nothing on a capped set) and hide the runner-ups
// that still gain plenty. When the change moves a CAPPED stat the
// delta is instead taken jointly with a reforge re-solve: enchanted
// hit lets the reforger un-buy hit elsewhere, so at the cap a +88 hit
// scope is worth the 88 points of secondaries it frees, not zero —
// and dropping a worn hit enchant isn't punished for a gap a cheap
// reforge refills. Candidates that leave capped totals alone can't
// shift the reforge optimum (weights are linear; only caps couple),
// so the plain delta is already exact there.
func (s *Scorer) EnchantTips(ws []Worn) []EnchantTip {
	tot := s.TotalsWorn(ws)
	base := s.Value(tot)
	resolvedBase, resolved := 0.0, false
	var tips []EnchantTip
	for i, w := range ws {
		if !w.HasDetail {
			continue
		}
		cur := s.d.EnchantByEffect(w.EnchantID, w.Item.Slot)
		var best *db.Enchant
		bestDelta := 0.5 // the churn floor: below it no tip is worth acting on
		for _, e := range s.enchantCandidates(w.Item) {
			if cur != nil && cur.EffectID == e.EffectID {
				continue
			}
			nw := w
			nw.EnchantID = e.EffectID
			var delta float64
			if s.rules().Reforging && s.enchantMovesCapped(cur, e) {
				if !resolved {
					resolvedBase, resolved = s.reforgedValue(ws), true
				}
				nws := append([]Worn(nil), ws...)
				nws[i] = nw
				delta = s.reforgedValue(nws) - resolvedBase
			} else {
				delta = s.Value(add(sub(tot, s.WornStats(w)), s.WornStats(nw))) - base
			}
			if delta > bestDelta {
				best, bestDelta = e, delta
			}
		}
		if best != nil {
			tips = append(tips, EnchantTip{ItemIdx: i, From: cur, To: best, Delta: bestDelta})
		}
	}
	sort.Slice(tips, func(a, b int) bool { return tips[a].Delta > tips[b].Delta })
	return tips
}

// enchantMovesCapped reports whether swapping cur (nil = none) for e
// changes the set total of any capped stat — the one case where an
// enchant choice and the reforge optimum couple.
func (s *Scorer) enchantMovesCapped(cur, e *db.Enchant) bool {
	var curSt db.Stats
	if cur != nil {
		curSt = s.enchantStats(cur)
	}
	eSt := s.enchantStats(e)
	for st := range eSt {
		if eSt[st] != curSt[st] && s.cappedStat(db.Stat(st)) {
			return true
		}
	}
	return false
}

// reforgedValue is the set's cap-aware value after letting the reforge
// solver respond to the (possibly hypothetical) worn state.
func (s *Scorer) reforgedValue(ws []Worn) float64 {
	rts := s.OptimizeReforges(ws)
	if len(rts) == 0 {
		return s.Value(s.TotalsWorn(ws))
	}
	nws := append([]Worn(nil), ws...)
	for _, t := range rts {
		nws[t.ItemIdx].Reforge = t.To
	}
	return s.Value(s.TotalsWorn(nws))
}

// EnchantGain is the exact cap-aware set-score gain of applying all the
// tips' enchants at once — per-tip deltas price each change alone, so
// near a cap their sum can overstate the combined result.
func (s *Scorer) EnchantGain(ws []Worn, tips []EnchantTip) float64 {
	if len(tips) == 0 {
		return 0
	}
	nws := append([]Worn(nil), ws...)
	for _, t := range tips {
		if t.ItemIdx < len(nws) {
			nws[t.ItemIdx].EnchantID = t.To.EffectID
		}
	}
	return s.Value(s.TotalsWorn(nws)) - s.Value(s.TotalsWorn(ws))
}
