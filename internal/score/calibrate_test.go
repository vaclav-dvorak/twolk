package score

import (
	"testing"

	"github.com/vaclav-dvorak/twolk/internal/db"
)

// TestCalibrateCountsEverythingButGems: the saturation test decides
// whether a capped stat's gems are worth assuming, so it has to run on
// the set's REAL no-gem totals. Item stats alone are not that — they
// miss the enchant, the reforge plan and the consumables baseline — and
// a set that is capped through those reads as having headroom, which
// puts a dead hit gem in every socket of every candidate (user report,
// session 34: a hit-capped cata hunter saw every T11 piece scored with
// two worthless hit gems, turning real upgrades into downgrades).
func TestCalibrateCountsEverythingButGems(t *testing.T) {
	d := testDatabase()
	d.Gems[10] = gem(10, "Rigid Fixture Gem", db.GemRed, db.QualityEpic, db.MeleeHit, 20)

	p := testProfile()
	p.Caps = []StatCap{{Stat: db.MeleeHit, Cap: 100}}
	s := New(d, p)

	// a set 20 hit short of the cap on its items alone
	it := item(1, db.SlotChest, db.ArmorMail, db.MeleeHit, 80)
	it.GemSockets = []db.GemColor{db.GemRed}
	d.Items[it.ID] = it
	set := []*db.Item{it}

	s.Calibrate(set)
	if g := s.BestGem(db.GemRed); g == nil || g.ID != 10 {
		t.Fatalf("under the cap the assumed red gem = %v, want the hit gem", g)
	}

	// the same set, with a meal that covers the gap: the gear is capped
	// as the character actually plays it, so hit gems buy nothing
	var meal db.Stats
	meal[db.MeleeHit] = 40
	s.SetBaseline(meal)
	s.Calibrate(set)
	if g := s.BestGem(db.GemRed); g != nil && g.ID == 10 {
		t.Errorf("hit gem still assumed for a set the meal already caps (%v)", g.Name)
	}
}
