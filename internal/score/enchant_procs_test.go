package score

import (
	"testing"

	"github.com/vaclav-dvorak/twolk/internal/db"
)

// TestWeaponProcEnchantsPerSpec pins the curated weapon PPM enchants
// (db/curated.go enchantAverages). Berserking, Mongoose, Fallen
// Crusader and Black Magic ship with EMPTY stats upstream, so the
// planner used to value them at zero and told every 2H melee to wear
// Massacre. Real database, a two-hander every melee spec can wield.
func TestWeaponProcEnchantsPerSpec(t *testing.T) {
	d, err := db.Load()
	if err != nil {
		t.Fatal(err)
	}
	ench := func(id int) *db.Enchant {
		e := d.EnchantByEffect(id, db.SlotWeapon)
		if e == nil {
			t.Fatalf("enchant %d missing", id)
		}
		return e
	}
	// value head to head: a naked scorer sits below the hit cap, where
	// Accuracy legitimately wins, so BestEnchant is not the probe here
	for _, c := range []struct {
		class      db.Class
		tree       string
		win, loses int
	}{
		{db.ClassWarrior, "Fury", 3789, 3827},        // Berserking > Massacre
		{db.ClassWarrior, "Arms", 3789, 3827},        // Berserking > Massacre
		{db.ClassPaladin, "Retribution", 3789, 3827}, // Berserking > Massacre
		{db.ClassDeathKnight, "Unholy", 3368, 3789},  // Fallen Crusader > Berserking
		{db.ClassWarrior, "Fury", 3789, 3790},        // Black Magic is a spell proc
	} {
		s := New(d, Find(c.class, c.tree))
		w, l := s.gemDot(s.enchantStats(ench(c.win))), s.gemDot(s.enchantStats(ench(c.loses)))
		if w <= l {
			t.Errorf("%s: %s %.0f should beat %s %.0f", c.tree,
				ench(c.win).Name, w, ench(c.loses).Name, l)
		}
	}
	axe := d.Items[50415] // Bryntroll, the Bone Arbiter: 2H axe
	if axe == nil {
		t.Fatal("fixture two-hander missing")
	}
	// a hunter's melee stat-stick never procs a melee enchant
	h := New(d, Find(db.ClassHunter, "Survival"))
	for _, e := range h.enchantCandidates(axe) {
		if e.Trigger == "melee" {
			t.Errorf("hunter offered melee proc enchant %s", e.Name)
		}
	}
}
