package score

import (
	"testing"

	"github.com/vaclav-dvorak/twolk/internal/db"
)

// TestBestConsumablesFillsTheGap: the auto pick is the meal that closes
// the character's actual gap, not the biggest number on the tooltip. A
// fury set short of the dodge cap wants the expertise food; the same
// set already over the cap must not.
func TestBestConsumablesFillsTheGap(t *testing.T) {
	d := testDatabase()
	s := New(d, Find(db.ClassWarrior, "Fury"))

	// hit over its cap (another fill-to-cap stat, and the one a melee
	// build closes first), so the only gap left for the meal is the
	// dodge cap
	underCap := item(1, db.SlotChest, db.ArmorPlate, db.Expertise, 100)
	underCap.Stats[db.MeleeHit] = 400
	d.Items[underCap.ID] = underCap
	food, flask := s.BestConsumables([]*db.Item{underCap})
	if food == nil || food.Name != "Rhinolicious Wormsteak" {
		t.Errorf("under the dodge cap the auto food = %v, want the expertise meal", nameOf(food))
	}
	if flask == nil || flask.Name != "Flask of Endless Rage" {
		t.Errorf("melee flask = %v, want Flask of Endless Rage", nameOf(flask))
	}

	atCap := item(2, db.SlotChest, db.ArmorPlate, db.Expertise, 230) // past 214
	atCap.Stats[db.MeleeHit] = 400
	d.Items[atCap.ID] = atCap
	if food, _ := s.BestConsumables([]*db.Item{atCap}); food != nil && food.Name == "Rhinolicious Wormsteak" {
		t.Error("expertise food picked for a set already past the dodge cap")
	}
}

// TestConsumablesDropDominatedTiers: a version's table keeps every tier
// it ever shipped (cata data still lists the WotLK meals). Listing a
// +40 crit food next to the +90 one is noise, so only the best of each
// buff survives.
func TestConsumablesDropDominatedTiers(t *testing.T) {
	d := testDatabase()
	d.Consumables = []db.Consumable{
		{Name: "old crit food", Kind: db.ConsumableFood, Stats: stats(db.MeleeCrit, 40)},
		{Name: "new crit food", Kind: db.ConsumableFood, Stats: stats(db.MeleeCrit, 90)},
		{Name: "str food", Kind: db.ConsumableFood, Stats: stats(db.Strength, 90)},
	}
	p := testProfile()
	p.Rules = &Rules{Game: "test", Stats: WotLK.Stats} // no curated table: the db's is all there is
	p.Weights[db.MeleeCrit] = 1
	p.Weights[db.Strength] = 1
	s := New(d, p)

	got := s.Consumables(db.ConsumableFood)
	if len(got) != 2 {
		t.Fatalf("listed %d foods %v, want 2 (the dominated tier dropped)", len(got), consumeNames(got))
	}
	if got[0].Name != "new crit food" && got[1].Name != "new crit food" {
		t.Errorf("listed %v, want the 90-crit food to survive", consumeNames(got))
	}
	for _, c := range got {
		if c.Name == "old crit food" {
			t.Errorf("dominated tier listed: %v", consumeNames(got))
		}
	}
}

// TestBaselineReachesEveryTotal: consumables must enter through the
// stat totals, or caps and curves keep seeing an unbuffed character
// while the score says otherwise.
func TestBaselineReachesEveryTotal(t *testing.T) {
	d := testDatabase()
	it := item(1, db.SlotChest, db.ArmorPlate, db.Strength, 100)
	d.Items[it.ID] = it
	s := New(d, Find(db.ClassWarrior, "Fury"))

	var meal db.Stats
	meal[db.Expertise] = 40
	s.SetBaseline(meal)

	if got := s.Totals([]*db.Item{it})[db.Expertise]; got != 40 {
		t.Errorf("Totals expertise = %g, want the meal's 40", got)
	}
	if got := s.TotalsWorn(wrapWorn([]*db.Item{it}))[db.Expertise]; got != 40 {
		t.Errorf("TotalsWorn expertise = %g, want the meal's 40", got)
	}
	if got := s.AssumedTotals([]*db.Item{it})[db.Expertise]; got != 40 {
		t.Errorf("AssumedTotals expertise = %g, want the meal's 40", got)
	}
	// and the cap sees it: on totals that carry the meal (every total
	// does), gear expertise only counts for another 174 points
	base := s.Totals(nil)
	full := s.MarginalValue(base, []db.Stat{db.Expertise}, 0, 174)
	overflow := s.MarginalValue(base, []db.Stat{db.Expertise}, 174, 214)
	if overflow != 0 || full <= 0 {
		t.Errorf("with a 40-expertise meal the gear cap did not move: full=%g overflow=%g", full, overflow)
	}
}

func stats(st db.Stat, v float64) db.Stats {
	var s db.Stats
	s[st] = v
	return s
}

func nameOf(c *db.Consumable) string {
	if c == nil {
		return "none"
	}
	return c.Name
}

func consumeNames(cs []db.Consumable) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.Name
	}
	return out
}
