package forever

import (
	"math"
	"testing"

	"github.com/vaclav-dvorak/twolk/internal/db"
	"github.com/vaclav-dvorak/twolk/internal/score"
)

// TestTrinketBallpark: a stat on-use trinket scores its uptime-averaged
// buff, not just its static stats — Earthstrike's +280 attack power for
// 20s every 2 minutes is worth ~47 AP on average.
func TestTrinketBallpark(t *testing.T) {
	d, err := LoadDB()
	if err != nil {
		t.Fatal(err)
	}
	it := d.Items[21180] // Earthstrike
	if it == nil || len(d.Effects[it.ID]) == 0 {
		t.Fatalf("Earthstrike has no effect: %+v", it)
	}
	s := score.New(d, findProfile(db.ClassHunter, "Marksmanship", nil))
	got := s.EffectiveStats(it)[db.RangedAttackPower] - it.Stats[db.RangedAttackPower]
	if want := 280.0 * 20 / 120; math.Abs(got-want) > want*0.15 {
		t.Errorf("Earthstrike averages %.1f RAP, want ~%.0f", got, want)
	}
}

// TestEveryDPSSpecScores: each measured DPS spec finds its profile by
// the armory's tree name, and its weights put the numeraire at 1.
func TestEveryDPSSpecScores(t *testing.T) {
	specs := []struct {
		class db.Class
		tree  string
		num   db.Stat
	}{
		{db.ClassWarrior, "Arms", db.AttackPower}, {db.ClassWarrior, "Fury", db.AttackPower},
		{db.ClassDruid, "Feral Combat", db.AttackPower}, {db.ClassDruid, "Feral", db.AttackPower},
		{db.ClassDruid, "Balance", db.SpellPower},
		{db.ClassMage, "Arcane", db.SpellPower}, {db.ClassMage, "Fire", db.SpellPower}, {db.ClassMage, "Frost", db.SpellPower},
		{db.ClassWarlock, "Affliction", db.SpellPower}, {db.ClassWarlock, "Demonology", db.SpellPower},
		{db.ClassWarlock, "Destruction", db.SpellPower},
		{db.ClassPaladin, "Retribution", db.AttackPower},
		{db.ClassRogue, "Assassination", db.AttackPower}, {db.ClassRogue, "Combat", db.AttackPower},
		{db.ClassRogue, "Subtlety", db.AttackPower},
		{db.ClassShaman, "Enhancement", db.AttackPower}, {db.ClassShaman, "Elemental", db.SpellPower},
		{db.ClassPriest, "Shadow", db.SpellPower},
		{db.ClassHunter, "Beast Mastery", db.RangedAttackPower},
	}
	for _, s := range specs {
		p := findProfile(s.class, s.tree, nil)
		if p == nil {
			t.Errorf("%v %s: no profile", s.class, s.tree)
			continue
		}
		if p.Weights[s.num] != 1 || p.EPPerDPS <= 0 || p.Rules != rules60 {
			t.Errorf("%s: numeraire %.3f, EPPerDPS %.3f", p.Name, p.Weights[s.num], p.EPPerDPS)
		}
		if p.Weights[db.HealingPower] != 0 {
			t.Errorf("%s prices healing power", p.Name)
		}
	}
	// tanks and healers stay unscored until they can be measured
	for _, tree := range []string{"Protection", "Holy", "Restoration", "Discipline"} {
		for _, c := range []db.Class{db.ClassWarrior, db.ClassPaladin, db.ClassPriest, db.ClassShaman, db.ClassDruid} {
			if p := findProfile(c, tree, nil); p != nil {
				t.Errorf("%s has a profile without a measurement", p.Name)
			}
		}
	}
}
