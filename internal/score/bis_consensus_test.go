package score

import (
	"testing"

	"github.com/vaclav-dvorak/twolk/internal/db"
)

// TestHunterT10FourPieceParity pins the BiS solver to the sim-tuned
// consensus: at full 277 ilvl parity (heroic Sanctified accessible)
// every hunter spec's best set runs 4pc T10 — the wowsims P4/P5
// presets do, and the session-21 review found our old 4pc EP (70,
// priced at unbuffed AP) made the solver stop at 2pc. Real database,
// naked solve, epic ≥245 non-PvP pool.
func TestHunterT10FourPieceParity(t *testing.T) {
	d, err := db.Load()
	if err != nil {
		t.Fatal(err)
	}
	allow := func(it *db.Item) bool {
		return it.Quality >= db.QualityEpic && it.ILvl >= 245 &&
			it.Stats[db.Resilience] == 0 && !d.PvPItem(it)
	}
	for _, spec := range []string{"Beast Mastery", "Marksmanship", "Survival"} {
		s := New(d, Find(db.ClassHunter, spec))
		_, set, _ := s.BestSet(wrapWorn([]*db.Item{{ID: 1, Name: "(naked)", Slot: db.SlotHead}}), allow)
		tier := 0
		for _, it := range set {
			if it.SetName == "Ahn'Kahar Blood Hunter's Battlegear" {
				tier++
			}
		}
		if tier != 4 {
			t.Errorf("%s BiS carries %d T10 pieces, want 4 (sim-consensus parity)", spec, tier)
		}
	}
}

// TestWotlkSetBonusCoverage: every endgame set in the real database
// (ilvl ≥ 200 — TBC sets are omitted by policy) must have a curated
// bonus entry, so BiS solves never silently ignore a set. Zero-EP
// entries are fine (some bonuses are genuinely worthless); MISSING
// entries are not.
func TestWotlkSetBonusCoverage(t *testing.T) {
	d, err := db.Load()
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, it := range d.Items {
		if it.SetName == "" || it.ILvl < 200 || seen[it.SetName] {
			continue
		}
		seen[it.SetName] = true
		if _, ok := setBonuses[it.SetName]; !ok {
			t.Errorf("set %q (ilvl %d+) has no bonus entry — curate it in setbonus.go", it.SetName, it.ILvl)
		}
	}
}

// TestQuelDelarOneRewardPerCharacter pins the one-time quest choice:
// the Battered Hilt chain ends in a single reward pick, so a set may
// never carry two Quel'Delar weapons. Titan's Grip is the case that
// breaks without the group — two 251 two-handers from that list beat
// anything else a non-raider can hold, and each is only unique-equipped
// against its OWN id.
func TestQuelDelarOneRewardPerCharacter(t *testing.T) {
	d, err := db.Load()
	if err != nil {
		t.Fatal(err)
	}
	rewards := map[int]bool{50046: true, 50047: true, 50048: true,
		50049: true, 50050: true, 50051: true, 50052: true}
	allow := func(it *db.Item) bool { return rewards[it.ID] }
	s := New(d, Find(db.ClassWarrior, "Fury"))
	_, set, _ := s.BestSet(wrapWorn([]*db.Item{{ID: 1, Name: "(naked)", Slot: db.SlotHead}}), allow)
	var picked []string
	for _, it := range set {
		if rewards[it.ID] {
			picked = append(picked, it.Name)
		}
	}
	if len(picked) > 1 {
		t.Errorf("BiS carries %d Quel'Delar rewards %v, want at most 1 (one quest, one choice)", len(picked), picked)
	}
}
