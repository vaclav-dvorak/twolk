package db

import "math"

// Random-suffix items ("Mistral Circle of the Wind") carry no stats of
// their own in the dump: each drop rolls one of the item's suffixes,
// whose stats are allocation coefficients scaled by the item's
// random-property budget — floor(coef × randPropPoints / 10000), the
// sim's own formula (wowsims sim/core/database.go). The stats are
// ordinary, reforgeable item stats.
//
// Every suffix becomes a VARIANT: a copy of the finished item that keeps
// its ID (sources, prices, uniqueness and BoE flags all key on it) and
// carries the suffix's stats and name. The candidate pool lists the
// variants instead of the stat-less base; a WORN suffix item resolves
// through VariantsOf, since imports name the item but not its roll.

type randomSuffix struct {
	Name  string
	Stats Stats // coefficients ×10000 per point of budget
}

type suffixRoll struct {
	Options []int // suffix ids the item can roll
	Points  int   // random-property budget at the item's level
}

func (d *Database) expandSuffixes(rolls map[int]suffixRoll, suffixes map[int]randomSuffix) {
	for id, roll := range rolls {
		base := d.Items[id]
		if base == nil {
			continue // dropped by an overlay
		}
		for _, sid := range roll.Options {
			rs, ok := suffixes[sid]
			if !ok {
				continue
			}
			v := *base
			v.Name = base.Name + " " + rs.Name
			v.Suffix, v.SuffixID = rs.Name, sid
			for st, coef := range rs.Stats {
				if coef != 0 {
					v.Stats[st] += math.Floor(coef * float64(roll.Points) / 10000)
				}
			}
			if d.Variants == nil {
				d.Variants = map[int][]*Item{}
			}
			d.Variants[id] = append(d.Variants[id], &v)
		}
	}
}

// VariantsOf lists a random-suffix item's possible rolls (nil for
// ordinary items).
func (d *Database) VariantsOf(id int) []*Item { return d.Variants[id] }

// Pool lists every candidate item: ordinary items as they are and each
// random-suffix item as its variants (the stat-less base never
// competes).
func (d *Database) Pool() []*Item {
	if d.pool != nil {
		return d.pool
	}
	return d.buildPool()
}

// buildPool assembles Pool's list; the loaders cache it once the item
// set is final (hand-built test databases fall back to rebuilding).
func (d *Database) buildPool() []*Item {
	out := make([]*Item, 0, len(d.Items))
	for id, it := range d.Items {
		if vs := d.Variants[id]; len(vs) > 0 {
			out = append(out, vs...)
			continue
		}
		out = append(out, it)
	}
	return out
}
