package db

import "testing"

// TestRandomSuffixVariants: suffix rolls get the sim's stats
// (floor(coef × randPropPoints / 10000) on top of the base armor), keep
// the base ID, and replace the stat-less base in the candidate pool.
func TestRandomSuffixVariants(t *testing.T) {
	d, err := LoadCata()
	if err != nil {
		t.Fatal(err)
	}
	var zephyr *Item
	for _, v := range d.VariantsOf(65381) { // Wind Stalker Leggings (372)
		if v.Suffix == "of the Zephyr" {
			zephyr = v
		}
	}
	if zephyr == nil {
		t.Fatalf("no Zephyr roll among %d variants", len(d.VariantsOf(65381)))
	}
	want := map[Stat]float64{Agility: 345, Stamina: 578, MeleeHaste: 238, Mastery: 238, Armor: 1550}
	for st, v := range want {
		if zephyr.Stats[st] != v {
			t.Errorf("%v = %v, want %v", st, zephyr.Stats[st], v)
		}
	}
	if zephyr.ID != 65381 || zephyr.Name != "Wind Stalker Leggings of the Zephyr" || zephyr.ILvl != 372 {
		t.Errorf("variant identity: %d %q ilvl %d", zephyr.ID, zephyr.Name, zephyr.ILvl)
	}
	base, variants := 0, 0
	for _, it := range d.Pool() {
		if it.ID == 65381 {
			if it.Suffix == "" {
				base++
			} else {
				variants++
			}
		}
	}
	if base != 0 || variants != len(d.VariantsOf(65381)) {
		t.Errorf("pool: %d base, %d variants", base, variants)
	}
}
