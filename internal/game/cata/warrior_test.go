package cata

import (
	"math"
	"testing"

	"github.com/vaclav-dvorak/twolk/internal/db"
	"github.com/vaclav-dvorak/twolk/internal/score"
)

// TestWarriorProfileResolution: the fury tree hosts two builds, told
// apart by the capstone talent; arms and prot resolve by tree alone.
func TestWarriorProfileResolution(t *testing.T) {
	for _, tc := range []struct {
		tree  string
		ranks map[int]int
		want  *score.Profile
	}{
		{"Arms", nil, armsProfile},
		{"Protection", nil, protProfile},
		{"Fury", nil, furyTGProfile}, // no talent detail: the sim's default build
		{"Fury", map[int]int{titansGrip: 1}, furyTGProfile},
		{"Fury", map[int]int{singleMindedFury: 1}, furySMFProfile},
	} {
		if p := findProfileFor(db.ClassWarrior, tc.tree, tc.ranks); p != tc.want {
			t.Errorf("%s %v = %v, want %v", tc.tree, tc.ranks, p.Name, tc.want.Name)
		}
	}
	if furyTGProfile.Key() == furySMFProfile.Key() {
		t.Error("TG and SMF must not share weight overrides")
	}
}

// TestFuryHitBreakpoints: Precision's 3% means 601 rating takes yellow
// attacks to the cap; past it hit is worth the sim's white-only 1.23
// up to 24% from gear, and nothing after.
func TestFuryHitBreakpoints(t *testing.T) {
	prof := *furyTGProfile
	prof.Caps = prof.EffectiveCaps(nil)
	s := score.New(&db.Database{}, &prof)
	yellow, white := math.Ceil(furyYellowCap), math.Ceil(furyWhiteCap)
	hitEP := prof.Weights[db.MeleeHit]

	at := func(hit float64) float64 {
		var tot db.Stats
		tot[db.MeleeHit] = hit
		return s.Value(tot)
	}
	for _, tc := range []struct {
		name     string
		from, to float64
		want     float64
	}{
		{"below the yellow cap", yellow - 300, yellow - 200, 100 * hitEP},
		{"white-only region", yellow + 200, yellow + 300, 100 * furyWhiteHitEP},
		{"past the dual-wield cap", white + 100, white + 200, 0},
	} {
		if d := at(tc.to) - at(tc.from); !near(d, tc.want) {
			t.Errorf("%s: +100 hit = %.2f, want %.2f", tc.name, d, tc.want)
		}
	}
	// no flat 8% default stacked on top: 8% of gear hit is still white value
	if d := at(math.Ceil(8*rules85.MeleeHitRatingPerPct)+100) - at(math.Ceil(8*rules85.MeleeHitRatingPerPct)); !near(d, 100*furyWhiteHitEP) {
		t.Errorf("+100 hit past 8%% gear = %.2f, want white value %.2f", d, 100*furyWhiteHitEP)
	}
}

// TestWarriorPresetsScore: the sim's P3 gear sets resolve against the
// cata db and score positive under their own profiles — every item a
// warrior preset wears is in the pool and carries stats.
func TestWarriorPresetsScore(t *testing.T) {
	d, err := LoadDB()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		prof *score.Profile
		ids  []int
	}{
		{furyTGProfile, []int{71430, 71446, 71603, 71227, 71600, 71418, 71601, 71443,
			71602, 71404, 71215, 71433, 69113, 69167, 70723, 70723, 71593}},
		{furySMFProfile, []int{71430, 71446, 71603, 71227, 71600, 71418, 71601, 71250,
			71602, 71404, 71215, 71433, 69113, 69167, 71562, 71562, 71593}},
	} {
		prof := *tc.prof
		prof.Caps = prof.EffectiveCaps(nil)
		var set []*db.Item
		for _, id := range tc.ids {
			it := d.Items[id]
			if it == nil {
				t.Fatalf("%s: preset item %d missing from the db", prof.Name, id)
			}
			if !it.UsableBy(db.ClassWarrior) {
				t.Errorf("%s: preset item %s not usable by warriors", prof.Name, it.Name)
			}
			set = append(set, it)
		}
		s := score.New(d, &prof)
		s.Calibrate(set)
		if v := s.Value(s.AssumedTotals(set)); v <= 0 {
			t.Errorf("%s: preset scores %.0f", prof.Name, v)
		}
	}
}
