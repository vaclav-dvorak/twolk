package score

import (
	"testing"

	"github.com/vaclav-dvorak/twolk/internal/db"
)

// TestFillToCapStatsAreWorthlessPastTheCap: whatever pre-cap value a
// fill-to-cap stat carries — a guessed floor or a measured slope — the
// points past the cap must be worth nothing. This is the one invariant
// that holds for every spec; the pre-cap NUMBER is not (a measurement
// may legitimately land below the published weight, as frost DK's hit
// and ret's expertise do, when the published one was itself taken below
// a cap on different gear).
func TestFillToCapStatsAreWorthlessPastTheCap(t *testing.T) {
	for _, p := range profiles {
		if p.HitCap == nil {
			continue
		}
		prof := *p
		prof.Caps = p.EffectiveCaps(nil)
		s := New(testDatabase(), &prof)

		var capped db.Stats
		for _, c := range prof.Caps {
			if c.Stat == p.HitCap.Stat {
				capped[c.Stat] = c.Cap
			}
		}
		if capped[p.HitCap.Stat] == 0 {
			t.Errorf("%s: hit cap missing from the effective caps", p.Name)
			continue
		}
		if got := s.MarginalValue(capped, []db.Stat{p.HitCap.Stat}, 0, 100); got != 0 {
			t.Errorf("%s: %v past the cap scored %.1f, want 0", p.Name, p.HitCap.Stat, got)
		}
	}
}

// TestFillToCapStatsLeadBelowTheirCap pins what the measured below-cap
// slopes are FOR: while a melee build is short of its hit cap, hit is
// the strongest thing it can gear for — every guide's "cap hit first" —
// and the moment the cap lands the same points are worth nothing. The
// published sim weights say the opposite (they are measured on capped
// preset gear), which is why these are floored from measurement.
func TestFillToCapStatsLeadBelowTheirCap(t *testing.T) {
	// The specs the SIM says lead with hit below their cap. Not every
	// measured spec does — frost DK (1.67 hit against Strength 3.22) and
	// ret (2.07 against 2.53) genuinely do not, because their damage
	// leans on effects that cannot miss, and that asymmetry is the whole
	// reason these are measured per spec instead of floored by
	// convention. This list is the measured YES half.
	for _, tc := range []struct {
		class db.Class
		tree  string
		top   db.Stat // the build's strongest primary stat
	}{
		{db.ClassWarrior, "Fury", db.Strength},
		{db.ClassWarrior, "Arms", db.Strength},
		{db.ClassHunter, "Survival", db.Agility},
		{db.ClassHunter, "Marksmanship", db.Agility},
		{db.ClassHunter, "Beast Mastery", db.Agility},
		{db.ClassDruid, "Feral Combat", db.Agility},
		{db.ClassRogue, "Combat", db.Agility},
		{db.ClassShaman, "Enhancement", db.Agility},
	} {
		base := Find(tc.class, tc.tree)
		if base == nil {
			t.Fatalf("no profile for %v/%s", tc.class, tc.tree)
		}
		// the hit cap is per-character (talents, racials), so it only
		// enters Caps through EffectiveCaps — build the profile the way
		// the app does, or the cap under test is not there at all
		prof := *base
		prof.Caps = base.EffectiveCaps(nil)
		p := &prof
		s := New(testDatabase(), p)
		var empty db.Stats // far below every cap
		hit := s.MarginalValue(empty, []db.Stat{db.MeleeHit}, 0, 100)
		top := s.MarginalValue(empty, []db.Stat{tc.top}, 0, 100)
		if hit <= top {
			t.Errorf("%s: 100 hit = %.0f EP, 100 %v = %.0f — hit must lead below its cap",
				p.Name, hit, tc.top, top)
		}
		// and stop dead at the cap
		var capped db.Stats
		for _, c := range p.Caps {
			if c.Stat == db.MeleeHit {
				capped[db.MeleeHit] = c.Cap
			}
		}
		if capped[db.MeleeHit] == 0 {
			t.Fatalf("%s has no melee hit cap", p.Name)
		}
		if got := s.MarginalValue(capped, []db.Stat{db.MeleeHit}, 0, 100); got != 0 {
			t.Errorf("%s: hit past the cap scored %.1f, want 0", p.Name, got)
		}
	}
}
