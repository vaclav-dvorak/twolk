package cata

import (
	"math"
	"testing"

	"github.com/vaclav-dvorak/twolk/internal/db"
	"github.com/vaclav-dvorak/twolk/internal/score"
)

// TestDruidProfileResolution: the feral tree hosts two roles, told
// apart by bear-only talents; Balance of Power's rank scales how much
// spirit converts.
func TestDruidProfileResolution(t *testing.T) {
	if p := findProfileFor(db.ClassDruid, "Feral Combat", nil); p != catProfile {
		t.Errorf("feral without talent data = %v, want cat", p.Name)
	}
	if p := findProfileFor(db.ClassDruid, "Feral Combat", map[int]int{57880: 2, 80313: 1}); p != bearProfile {
		t.Errorf("feral with Natural Reaction/Pulverize = %v, want bear", p.Name)
	}
	if catProfile.Key() == bearProfile.Key() {
		t.Error("cat and bear must not share weight overrides")
	}
	p := findProfileFor(db.ClassDruid, "Balance", map[int]int{33592: 1})
	if p.Conversions[0].Ratio != 0.5 || p.Weights[db.Spirit] != 0.5*balanceHitEP {
		t.Errorf("BoP 1/2: ratio %v, spirit %v", p.Conversions[0].Ratio, p.Weights[db.Spirit])
	}
	if balanceProfile.Weights[db.Spirit] != balanceHitEP {
		t.Error("the rank variant must not mutate the shared profile")
	}
	if p := findProfileFor(db.ClassDruid, "Balance", map[int]int{33596: 2}); p != balanceProfile {
		t.Error("BoP 2/2 is the registry profile")
	}
}

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

// TestBalanceSpiritFillsHitCap: below the 17% cap spirit is worth what
// hit is (the sim measured them equal), and past it neither is worth
// anything — whichever of the two carried the total over.
func TestBalanceSpiritFillsHitCap(t *testing.T) {
	prof := *balanceProfile
	prof.Caps = prof.EffectiveCaps(nil)
	s := score.New(&db.Database{}, &prof)
	cap := math.Ceil(spellHitCap)
	var tot db.Stats
	tot[db.MeleeHit], tot[db.Spirit] = 300, cap-500 // 200 short of the cap
	base := s.Value(tot)

	more := tot
	more[db.Spirit] += 100
	if d := s.Value(more) - base; !near(d, 100*balanceHitEP) {
		t.Errorf("+100 spirit below the cap = %.2f, want %.2f", d, 100*balanceHitEP)
	}
	over := tot
	over[db.Spirit] += 500 // 300 past the cap
	if d := s.Value(over) - base; !near(d, 200*balanceHitEP) {
		t.Errorf("+500 spirit across the cap = %.2f, want only the 200 to the cap", d)
	}
	capped := over
	capped[db.MeleeHit] += 100
	if d := s.Value(capped) - s.Value(over); !near(d, 0) {
		t.Errorf("hit past a spirit-filled cap = %.2f, want 0", d)
	}
	if h := s.Derived(tot)[db.MeleeHit]; h != cap-200 {
		t.Errorf("derived hit %.0f, want %.0f (hit + spirit)", h, cap-200)
	}
}

// TestBearSteppedSoftCaps: expertise is full value to the dodge cap,
// half to the parry cap, nothing after; hit keeps its spell share
// (0.035) between 8% and 17%.
func TestBearSteppedSoftCaps(t *testing.T) {
	prof := *bearProfile
	s := score.New(&db.Database{}, &prof)
	val := func(st db.Stat, v float64) float64 {
		var tot db.Stats
		tot[st] = v
		return s.Value(tot)
	}
	exp := []struct{ at, slope float64 }{
		{dodgeCap - 100, 0.37}, {dodgeCap + 100, 0.185}, {parryCapBear + 100, 0},
	}
	for _, c := range exp {
		if d := val(db.Expertise, c.at+10) - val(db.Expertise, c.at); !near(d, 10*c.slope) {
			t.Errorf("expertise slope at %.0f = %.3f, want %.3f", c.at, d/10, c.slope)
		}
	}
	hit := []struct{ at, slope float64 }{
		{physHitCap - 100, 0.22}, {physHitCap + 100, 0.035}, {spellHitCap + 100, 0},
	}
	for _, c := range hit {
		if d := val(db.MeleeHit, c.at+10) - val(db.MeleeHit, c.at); !near(d, 10*c.slope) {
			t.Errorf("hit slope at %.0f = %.3f, want %.3f", c.at, d/10, c.slope)
		}
	}
}
