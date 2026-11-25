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
