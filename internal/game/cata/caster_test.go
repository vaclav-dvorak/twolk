package cata

import (
	"math"
	"testing"

	"github.com/vaclav-dvorak/twolk/internal/db"
	"github.com/vaclav-dvorak/twolk/internal/score"
)

// TestCasterProfileResolution: every mage and warlock tree the sim
// models resolves by tree alone; frost, which it does not, stays
// unscored.
func TestCasterProfileResolution(t *testing.T) {
	for _, tc := range []struct {
		class db.Class
		tree  string
		want  *score.Profile
	}{
		{db.ClassMage, "Arcane", arcaneProfile},
		{db.ClassMage, "Fire", fireProfile},
		{db.ClassMage, "Frost", nil},
		{db.ClassWarlock, "Affliction", afflictionProfile},
		{db.ClassWarlock, "Demonology", demonologyProfile},
		{db.ClassWarlock, "Destruction", destructionProfile},
	} {
		if p := findProfileFor(tc.class, tc.tree, nil); p != tc.want {
			t.Errorf("%s: got %v, want %v", tc.tree, p, tc.want)
		}
	}
}

// TestCasterHitCap: hit is worth its measured value up to 17% spell
// hit and nothing past it.
func TestCasterHitCap(t *testing.T) {
	prof := *fireProfile
	prof.Caps = prof.EffectiveCaps(nil)
	s := score.New(&db.Database{}, &prof)
	limit := math.Ceil(spellHitCap)
	at := func(hit float64) float64 {
		var tot db.Stats
		tot[db.MeleeHit] = hit
		return s.Value(tot)
	}
	if d := at(limit-100) - at(limit-200); !near(d, 100*fireHitEP) {
		t.Errorf("+100 hit below the cap = %.2f, want %.2f", d, 100*fireHitEP)
	}
	if d := at(limit+200) - at(limit+100); !near(d, 0) {
		t.Errorf("+100 hit past the cap = %.2f, want 0", d)
	}
}

// TestCasterSetBonusesPerTree: the mage, warlock and warrior DPS sets
// list one row per build; a profile sees its own row at each piece
// count and none of the others'.
func TestCasterSetBonusesPerTree(t *testing.T) {
	for _, tc := range []struct {
		prof   *score.Profile
		set    string
		want4  float64
		others int
	}{
		{arcaneProfile, "Firelord's Vestments", 611, 0},
		{fireProfile, "Firelord's Vestments", 889, 0},
		{afflictionProfile, "Shadowflame Regalia", 126, 0},
		{destructionProfile, "Vestments of the Faceless Shroud", 374, 0},
		// warriors scope by role (fury) or tree (arms) the same way
		{armsProfile, "Colossal Dragonplate Battlegear", 648, 0},
		{furyTGProfile, "Molten Giant Warplate", 595, 0},
		{furySMFProfile, "Molten Giant Warplate", 258, 0},
	} {
		bs := rules85.SetBonusesForRole(tc.set, tc.prof.SetBonusAudience())
		if len(bs) != 2 {
			t.Fatalf("%s, %s: %d rows, want one per piece count", tc.prof.Name, tc.set, len(bs))
		}
		if bs[1].Pieces != 4 || bs[1].EP != tc.want4 {
			t.Errorf("%s, %s: 4pc = %+v, want EP %v", tc.prof.Name, tc.set, bs[1], tc.want4)
		}
	}
	// a feral bear still sees the cat-valued rows as notes at EP 0
	for _, b := range rules85.SetBonusesForRole("Stormrider's Battlegarb", bearProfile.SetBonusAudience()) {
		if b.EP != 0 {
			t.Errorf("bear sees cat EP %v on %d pieces", b.EP, b.Pieces)
		}
	}
}

// TestCasterTrinketEPvsSim pins the caster proc pricing (EPPerDPS,
// ProcInterval, stacking) against the pinned sim, driven headless on
// each spec's P3 preset — Dragonwrath swapped for the Runestaff, as for
// the weights — with the second trinket slot emptied (tools/eptool-cata
// -replace 14=71085 -at hit=15 -trinkets, 50k iterations, seed 101).
// Each want is the measured DPS gain of filling that slot, divided by
// the measured DPS per spell power on the same gear.
//
// Most land within 12%. The rest are STRUCTURAL misses of a time-average
// model, pinned at a wider band (tol) so a regression still shows:
//
//   - Big procs on a long cooldown (Stump of Time, Theralion's Mirror,
//     Bell) read up to 40% LOW for warlocks and fire: the buff's uptime
//     matches (Stump: 20.0% in the sim, ~19% here) but its window is
//     worth more than its average — warlock DoTs and pets keep the spell
//     power they were cast with, and fire's Combustion copies the dots
//     ticking at the time. Stump's window is worth 1.76× its average
//     spell power to a demonology warlock.
//   - On-use spell power (Soul Casket) 15-27% low: the same snapshotting,
//     plus the 300s fight opening with the cooldown ready and the use
//     lined up with Demon Soul / Combustion / Arcane Power.
//   - Affliction reads haste and mastery procs (Witching Hourglass,
//     Theralion's Mirror) 12-25% HIGH: its haste weight is the
//     1200-rating window average, and a 1710 rating burst lands past the
//     breakpoints that average was taken over.
//
// Do not close these by tuning ProcInterval or EPPerDPS; they need a
// snapshot-aware proc model.
func TestCasterTrinketEPvsSim(t *testing.T) {
	d, err := LoadDB()
	if err != nil {
		t.Fatal(err)
	}
	type trinket struct {
		id   int
		want float64
		tol  float64 // 0: the default 12%
	}
	lockSet := []int{71595, 71472, 71598, 71434, 71597, 71471, 71614, 71613,
		71596, 71447, 71217, 71449, 69110, 71085, 71575}
	for _, tc := range []struct {
		prof *score.Profile
		set  []int
		want []trinket
	}{
		{arcaneProfile, []int{71508, 71472, 71511, 71434, 71510, 71471, 71614, 71613,
			71509, 71447, 71217, 71449, 65105, 71085, 71575},
			[]trinket{{65053, 714.8, 0}, {69139, 722.9, 0}, {69000, 534.6, 0}, {58183, 664.7, 0.25},
				{62465, 397.6, 0.2}, {56320, 677.5, 0}, {65110, 589.0, 0}, {70142, 847.8, 0}, {59519, 828.2, 0}}},
		{fireProfile, []int{71508, 71472, 71511, 71434, 71510, 71471, 71614, 71613,
			71509, 71447, 71217, 71449, 69110, 71085, 71575},
			[]trinket{{65105, 846.8, 0.2}, {65053, 831.4, 0}, {69139, 835.1, 0}, {69000, 492.2, 0}, {58183, 634.0, 0.25},
				{62465, 640.7, 0.25}, {56320, 703.3, 0}, {65110, 692.4, 0}, {70142, 736.8, 0.2}, {59519, 750.6, 0.2}}},
		{afflictionProfile, lockSet,
			[]trinket{{65105, 580.3, 0.2}, {65053, 707.3, 0}, {69139, 743.5, 0}, {69000, 455.6, 0}, {58183, 637.8, 0.3},
				{62465, 563.9, 0.4}, {56320, 467.5, 0.3}, {65110, 618.3, 0}, {70142, 647.3, 0.2}, {59519, 501.1, 0.2}}},
		{demonologyProfile, lockSet,
			[]trinket{{65105, 1249.1, 0.35}, {65053, 853.9, 0.25}, {69139, 878.4, 0}, {69000, 471.0, 0}, {58183, 711.3, 0.2},
				{62465, 678.9, 0.45}, {56320, 640.6, 0}, {65110, 660.0, 0}, {70142, 746.1, 0}, {59519, 1104.4, 0.35}}},
		{destructionProfile, []int{71595, 71472, 71598, 60232, 71597, 71471, 71614, 71613,
			71596, 71447, 71217, 71449, 69110, 71085, 71575},
			[]trinket{{65105, 690.9, 0}, {65053, 724.3, 0}, {69139, 792.3, 0}, {69000, 451.5, 0}, {58183, 624.9, 0.25},
				{62465, 449.7, 0.25}, {56320, 585.9, 0}, {65110, 618.1, 0}, {70142, 652.9, 0}, {59519, 606.3, 0}}},
	} {
		prof := *tc.prof
		prof.Caps = prof.EffectiveCaps(nil)
		var set []*db.Item
		for _, id := range tc.set {
			it := d.Items[id]
			if it == nil {
				t.Fatalf("%s: preset item %d missing from the db", prof.Name, id)
			}
			set = append(set, it)
		}
		s := score.New(d, &prof)
		s.Calibrate(set)
		for _, tr := range tc.want {
			it := d.Items[tr.id]
			got := s.AssumedContribution(append(set, it), len(set))
			tol := tr.tol
			if tol == 0 {
				tol = 0.12
			}
			if off := (got - tr.want) / tr.want; math.Abs(off) > tol {
				t.Errorf("%s, %s: planner EP %.0f, sim measured %.0f (%.1f%% off, tolerance %.0f%%)",
					prof.Name, it.Name, got, tr.want, 100*off, 100*tol)
			}
		}
	}
}
