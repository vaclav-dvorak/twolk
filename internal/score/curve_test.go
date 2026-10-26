package score

import (
	"math"
	"testing"
	"time"

	"github.com/vaclav-dvorak/twolk/internal/db"
)

// TestStatCurveIntegral pins the curve's two jobs: the marginal weight
// at a total (what a search needs) and the integral (what a total is
// worth). A flat curve must reduce exactly to a linear weight, or every
// existing number moves the day a stat gains a curve.
func TestStatCurveIntegral(t *testing.T) {
	flat := StatCurve{{At: 100, Weight: 2}}
	for _, v := range []float64{0, 50, 100, 500} {
		if got, want := flat.integral(v), 2*v; math.Abs(got-want) > 1e-9 {
			t.Errorf("flat curve integral(%g) = %g, want %g", v, got, want)
		}
	}

	c := StatCurve{{At: 200, Weight: 2}, {At: 400, Weight: 4}}
	for _, tc := range []struct{ at, want float64 }{
		{0, 2}, {200, 2}, {300, 3}, {400, 4}, {900, 4}, // flat outside the ends
	} {
		if got := c.weightAt(tc.at); math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("weightAt(%g) = %g, want %g", tc.at, got, tc.want)
		}
	}
	// 0..200 at 2 = 400; 200..400 trapezoid (2+4)/2*200 = 600; then flat 4
	for _, tc := range []struct{ at, want float64 }{
		{200, 400}, {300, 400 + 250}, {400, 1000}, {500, 1400},
	} {
		if got := c.integral(tc.at); math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("integral(%g) = %g, want %g", tc.at, got, tc.want)
		}
	}
}

// TestCurvedStatIsWorthMoreWhenStacked is the property the curve exists
// for: the same 100 points of a rising stat are worth MORE to a
// character that already has a lot of it. A flat weight cannot express
// that, and it is why the flat 2.17 made the solver walk away from
// armor penetration builds.
func TestCurvedStatIsWorthMoreWhenStacked(t *testing.T) {
	p := testProfile()
	p.Curves = map[db.Stat]StatCurve{db.ArmorPenetration: furyArPCurve}
	p.Caps = []StatCap{{Stat: db.ArmorPenetration, Cap: 1400}}
	s := New(testDatabase(), p)

	var low, high db.Stats
	low[db.ArmorPenetration] = 200
	high[db.ArmorPenetration] = 1000
	lowGain := s.MarginalValue(low, []db.Stat{db.ArmorPenetration}, 0, 100)
	highGain := s.MarginalValue(high, []db.Stat{db.ArmorPenetration}, 0, 100)
	if !(highGain > lowGain*1.15) {
		t.Errorf("ArP gain at 1000 = %.1f, at 200 = %.1f — the curve is not rising", highGain, lowGain)
	}
	// past the cap the points are worthless, curve or not
	var capped db.Stats
	capped[db.ArmorPenetration] = 1400
	if got := s.MarginalValue(capped, []db.Stat{db.ArmorPenetration}, 0, 100); got != 0 {
		t.Errorf("ArP past the 1400 cap scored %.1f, want 0", got)
	}
}

// TestFuryPrefersArPOverFillableExpertise pins the Quel'Delar case that
// drove the curve (session 32): the one-time quest choice is between a
// 91-ArP sword and an 84-expertise polearm, and with armor penetration
// priced flat the expertise piece won — advice every fury guide would
// reject. On the measured curve, at the ArP total a stacking build
// actually plays, the sword wins.
func TestFuryPrefersArPOverFillableExpertise(t *testing.T) {
	p := Find(db.ClassWarrior, "Fury")
	s := New(testDatabase(), p)

	// the set the curve leads the solver to: ArP stacked toward the cap,
	// expertise short of its own. There the sword wins outright.
	var stacked db.Stats
	stacked[db.ArmorPenetration] = 1200
	stacked[db.Expertise] = 98
	arp := s.MarginalValue(stacked, []db.Stat{db.ArmorPenetration}, 0, 91)
	exp := s.MarginalValue(stacked, []db.Stat{db.Expertise}, 0, 84)
	if arp <= exp {
		t.Errorf("stacked build: 91 ArP = %.1f EP, 84 expertise = %.1f EP — the ArP weapon must win", arp, exp)
	}

	// and the second half of the user's argument: expertise is FILLABLE.
	// A 40-expertise meal covers part of the gap, so the last of the
	// weapon's 84 points spill past the dodge cap and buy nothing —
	// which is what stops a fillable stat from outbidding a scarce one.
	var meal db.Stats
	meal[db.Expertise] = 40
	s.SetBaseline(meal)
	var mid db.Stats
	mid[db.ArmorPenetration] = 800
	mid[db.Expertise] = 98 + 40
	if a, e := s.MarginalValue(mid, []db.Stat{db.ArmorPenetration}, 0, 91),
		s.MarginalValue(mid, []db.Stat{db.Expertise}, 0, 84); a <= e {
		t.Errorf("with expertise food: 91 ArP = %.1f, 84 expertise = %.1f — the fillable stat must not win", a, e)
	}
}

// TestArPCurveWhereverArPIsCapped: armor penetration's convexity is the
// target's armor formula, so it applies to every physical spec, not to
// the warrior it was first measured on (user call, session 36). Any
// profile that carries the 1400 ArP cap must therefore price ArP on a
// measured curve, and that curve must actually rise.
func TestArPCurveWhereverArPIsCapped(t *testing.T) {
	for _, p := range profiles {
		capped := false
		for _, c := range p.Caps {
			capped = capped || c.Stat == db.ArmorPenetration
		}
		if !capped {
			continue
		}
		curve, ok := p.Curves[db.ArmorPenetration]
		if !ok {
			t.Errorf("%s caps ArP but prices it flat — no measured curve", p.Name)
			continue
		}
		s := New(testDatabase(), p)
		var low, high db.Stats
		low[db.ArmorPenetration] = 200
		high[db.ArmorPenetration] = 1000
		lo := s.MarginalValue(low, []db.Stat{db.ArmorPenetration}, 0, 100)
		hi := s.MarginalValue(high, []db.Stat{db.ArmorPenetration}, 0, 100)
		if hi <= lo*1.15 {
			t.Errorf("%s ArP curve barely rises: %.1f at 200 vs %.1f at 1000", p.Name, lo, hi)
		}
		if curve[0].Weight <= 0 {
			t.Errorf("%s ArP curve starts at %.2f", p.Name, curve[0].Weight)
		}
	}
}

// TestArPProcLowersTheUsableCeiling: a passive ArP proc pushes the
// character past the 1400 cap while it is up, so passive points above
// the ceiling it leaves only pay during its downtime. Measured twice in
// session 36 (feral and frost DK, both wearing Mjolnir Runestone: a
// clean convex rise to 600 passive, then a drop to 71–80% of trend).
func TestArPProcLowersTheUsableCeiling(t *testing.T) {
	d := testDatabase()
	// a Mjolnir-shaped trinket: +751 ArP for 10s, 1 PPM
	trinket := item(50, db.SlotTrinket, db.ArmorAny, db.Agility, 0)
	d.Items[trinket.ID] = trinket
	proc := &db.Effect{ItemID: trinket.ID, Name: "ArP proc", Kind: db.EffectProc,
		Duration: 10 * time.Second, PPM: 1}
	proc.Bonus[db.ArmorPenetration] = 751
	d.Effects[trinket.ID] = []*db.Effect{proc}

	p := Find(db.ClassWarrior, "Fury")
	prof := *p
	s := New(d, &prof)

	plain := item(51, db.SlotChest, db.ArmorPlate, db.ArmorPenetration, 0)
	d.Items[plain.ID] = plain

	// without the trinket the curve is the profile's, full value high up
	s.Calibrate([]*db.Item{plain})
	var high db.Stats
	high[db.ArmorPenetration] = 1200
	free := s.MarginalValue(high, []db.Stat{db.ArmorPenetration}, 0, 50)

	// with it, the same points sit past the ceiling the proc leaves
	s.Calibrate([]*db.Item{plain, trinket})
	procced := s.MarginalValue(high, []db.Stat{db.ArmorPenetration}, 0, 50)

	if !(procced < free*0.95) {
		t.Errorf("ArP at 1200 with a 751 proc = %.1f, without = %.1f — the ceiling did not move",
			procced, free)
	}
	// and below the knee nothing changes
	var low db.Stats
	low[db.ArmorPenetration] = 300
	s.Calibrate([]*db.Item{plain})
	lowFree := s.MarginalValue(low, []db.Stat{db.ArmorPenetration}, 0, 50)
	s.Calibrate([]*db.Item{plain, trinket})
	if lowProc := s.MarginalValue(low, []db.Stat{db.ArmorPenetration}, 0, 50); lowProc != lowFree {
		t.Errorf("ArP at 300 changed with the proc: %.2f vs %.2f — the knee is too low", lowProc, lowFree)
	}
}
