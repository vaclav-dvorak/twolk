package cata

import (
	"math"
	"testing"

	"github.com/vaclav-dvorak/twolk/internal/db"
	"github.com/vaclav-dvorak/twolk/internal/score"
)

// p1SVPreset is the wowsims ui/hunter/survival p1_sv gear set with the
// second trinket left out — that slot is the swap subject below.
var p1SVPreset = []int{65206, 69880, 65208, 69884, 65204, 65028, 65205,
	65132, 60230, 65063, 65082, 65367, 65140, 65139, 65058}

// p1SVSuffixes are the preset's random-suffix rolls (p1_sv.gear.json):
// Mistral Pendant and Drape -135, Mistral Circle -133.
var p1SVSuffixes = map[int]int{69880: -135, 69884: -135, 65367: -133}

// TestTrinketEPvsSim pins our trinket valuations against the pinned
// wowsims build, driven headless on the preset above (50k iterations,
// seed 101, 300s single target). Each want is the measured DPS delta of
// adding that trinket to the empty slot, divided by the measured DPS per
// point of ranged attack power (1.13644) to land in our EP units.
//
// The tolerance is deliberately loose: a linear EP model cannot track a
// sim exactly — a 1710-agility burst is worth slightly more than its
// time-average because agility's returns on this preset are mildly
// superlinear (+1710 flat measures 3.34 EP/point against +321 flat's
// 3.24). Within ~15% means the uptime model is honest, not that it is
// the sim.
func TestTrinketEPvsSim(t *testing.T) {
	d, err := LoadDB()
	if err != nil {
		t.Fatal(err)
	}
	prof := findProfile(db.ClassHunter, "Survival")
	prof.Caps = prof.EffectiveCaps(nil)

	set := make([]*db.Item, 0, len(p1SVPreset))
	for _, id := range p1SVPreset {
		it := d.Items[id]
		if it == nil {
			t.Fatalf("preset item %d missing from the db", id)
		}
		if sid, ok := p1SVSuffixes[id]; ok {
			it = nil
			for _, v := range d.VariantsOf(id) {
				if v.SuffixID == sid {
					it = v
				}
			}
			if it == nil {
				t.Fatalf("preset item %d has no roll %d", id, sid)
			}
		}
		set = append(set, it)
	}
	s := score.New(d, prof)
	s.Calibrate(set)

	// The preset is reforged past the 8% hit cap, so a trinket's hit
	// rating is worth nothing here — the sim agrees exactly: flat +285
	// and +321 hit both measured 0.0 DPS on it.
	var hitCap float64
	for _, c := range prof.Caps {
		if c.Stat == db.MeleeHit {
			hitCap = c.Cap
		}
	}
	// on the SET-level plan, which is where the cap is landed: the
	// per-item assumption never reforges INTO a capped stat, so
	// Totals() reads only the hit the items themselves carry. The plan
	// lands WITHIN A REFORGE STEP of the cap rather than exactly on it
	// (reforges move fixed chunks, and since session 38 priced pre-cap
	// hit at its measured 1.73 the planner stops rather than overshoot);
	// the trinkets compared below carry 285-321 hit, so at most a
	// handful of their points can count, far inside the tolerance.
	if hit := s.AssumedTotals(set)[db.MeleeHit]; hit < hitCap-50 {
		t.Fatalf("preset hit %.0f more than a reforge step below cap %.0f: the comparison assumes hit is saturated",
			hit, hitCap)
	}

	for _, tc := range []struct {
		id   int
		name string
		want float64
		tol  float64
	}{
		{62051, "Darkmoon Card: Hurricane", 1407.8, 0.03},
		{58181, "Fluid Death", 1212.2, 0.03},
		{59473, "Essence of the Cyclone", 1443.1, 0.03},
		{59441, "Prestor's Talisman of Machination", 1367.2, 0.03},
		// Key is the model's known worst case, ~16% low, and the gap is
		// structural rather than a wrong constant. Two parts: the sim's
		// 300s fight opens with the ICD ready, so it fits four 15s procs
		// (19.9% uptime) where the steady state we model is 15/(75+8.3)
		// = 18.0%; and a 1710-agility burst is worth more than its
		// time-average, since agility's returns on this preset are
		// mildly superlinear (+1710 flat measures 3.34 EP/point against
		// +321 flat's 3.24). Prestor's Talisman shares the identical
		// 10%/75s/15s trigger and lands within 1%, which is what rules
		// the uptime cadence itself out as the culprit — its 1926-haste
		// burst just happens to be sublinear by about as much as the
		// fight-start effect gains. Do not close this by tuning
		// ProcInterval: that would push every other trinket off.
		{56328, "Key to the Endless Chamber", 1195.7, 0.20},
	} {
		it := d.Items[tc.id]
		if it == nil {
			t.Fatalf("%s (%d) missing from the db", tc.name, tc.id)
		}
		// AssumedContribution is the marginal EP the planner credits the
		// trinket with on the set-level plan — the basis that knows the
		// preset is hit-capped, and frozen, like the sim run it is
		// compared against (the sim measured flat stats on fixed gear).
		got := s.AssumedContribution(append(set, it), len(set))
		if off := math.Abs(got-tc.want) / tc.want; off > tc.tol {
			t.Errorf("%s: planner EP %.0f, sim measured %.0f (%.1f%% off, tolerance %.0f%%)",
				tc.name, got, tc.want, 100*(got-tc.want)/tc.want, tc.tol*100)
		}
	}
}

// TestCapReachedByReforgingStopsHitGems is the real-data half of
// score's TestCalibrateCountsEverythingButGems: in Cataclysm the hit
// cap is landed by REFORGING, so a set whose items carry only 378 hit
// can still sit at 968/961 once its reforges are planned. The gem
// assumption has to see that — otherwise every candidate is priced with
// hit gems the character cannot use, which is what turned a +141 tier
// legs upgrade into a −121 downgrade (session 34).
func TestCapReachedByReforgingStopsHitGems(t *testing.T) {
	d, err := db.LoadCata()
	if err != nil {
		t.Fatal(err)
	}
	prof := findProfile(db.ClassHunter, "Survival")
	prof.Caps = prof.EffectiveCaps(nil)
	var set []*db.Item
	for _, id := range p1SVPreset {
		set = append(set, d.Items[id])
	}
	s := score.New(d, prof)
	s.Calibrate(set)

	var cap float64
	for _, c := range prof.Caps {
		if c.Stat == db.MeleeHit {
			cap = c.Cap
		}
	}
	items := s.Totals(set)[db.MeleeHit]
	planned := s.AssumedTotals(set)[db.MeleeHit]
	// the gap the bug lived in: the items carry a fraction of the hit
	// the planned set actually has. The plan lands within a reforge step
	// of the cap, not exactly on it (see TestTrinketEPvsSim).
	if !(items < cap/2 && planned >= cap-50) {
		t.Fatalf("fixture no longer covers the case: items %.0f, planned %.0f, cap %.0f",
			items, planned, cap)
	}
	for _, c := range []db.GemColor{db.GemRed, db.GemYellow, db.GemBlue} {
		if g := s.BestGem(c); g != nil && g.Stats[db.MeleeHit] > 0 {
			t.Errorf("%v socket assumes %s on a set the reforge plan already caps", c, g.Name)
		}
	}
}
