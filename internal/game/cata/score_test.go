package cata

import (
	"strings"
	"testing"

	"github.com/vaclav-dvorak/twolk/internal/db"
	"github.com/vaclav-dvorak/twolk/internal/score"
)

// TestSurvivalScoring exercises the cata scoring path against the real
// embedded database: level-85 caps, rare-tier gem assumptions (the 4.3
// epics in the data must NOT be assumed), JC Chimera's Eyes, and the
// hunter enchant pick.
func TestSurvivalScoring(t *testing.T) {
	p := findProfileFor(db.ClassHunter, "Survival", nil)
	if p == nil {
		t.Fatal("no survival profile")
	}
	if p.Key() != "cata/hunter/survival" {
		t.Fatalf("key = %q, want cata/hunter/survival", p.Key())
	}

	// 8% ranged hit at level 85 = ceil(8 × 120.1088) = 961 rating
	var hit float64
	for _, c := range p.EffectiveCaps(nil) {
		if c.Stat == db.MeleeHit {
			hit = c.Cap
		}
	}
	if hit != 961 {
		t.Fatalf("hit cap = %v, want 961", hit)
	}

	d, err := db.LoadCata()
	if err != nil {
		t.Fatal(err)
	}
	// mirror the TUI's newScorer: the hit cap is baked into Caps
	// before the scorer is built (EffectiveCaps)
	prof := *p
	prof.Caps = p.EffectiveCaps(nil)
	s := score.New(d, &prof)

	// red sockets: a RARE cut — never the 4.3 epic (Queen's Garnet
	// tier) and never the JC-only Chimera's Eye. Under the hit floor
	// the pick is hit-seeking (Glinting Demonseye); calibrating on a
	// hit-saturated set re-picks the pure agility cut.
	red := s.BestGem(db.GemRed)
	if red == nil || red.Quality != db.QualityRare || red.RequiredProfession != 0 {
		t.Fatalf("red gem = %+v, want a rare non-JC cut", red)
	}
	capped := &db.Item{ID: 1, Slot: db.SlotChest}
	capped.Stats[db.MeleeHit] = 1200 // past the 961 cap
	s.Calibrate([]*db.Item{capped})
	if red = s.BestGem(db.GemRed); red == nil || red.Name != "Delicate Inferno Ruby" {
		t.Fatalf("calibrated red gem = %+v, want Delicate Inferno Ruby", red)
	}
	// metas: a rare Shadowspirit Diamond
	meta := s.BestGem(db.GemMeta)
	if meta == nil || meta.Quality != db.QualityRare {
		t.Fatalf("meta gem = %+v, want a rare Shadowspirit Diamond", meta)
	}

	// jewelcrafters get Chimera's Eyes (bypassing the tier gate);
	// non-jewelcrafters never see them
	helm := d.Items[60303] // 2 sockets: meta + yellow
	base := s.GearStats(helm)
	s.SetProfessions(map[string]bool{"Jewelcrafting": true})
	jc := s.GearStats(helm)
	if jc[db.Agility] <= base[db.Agility] {
		t.Fatalf("JC agility %v not above base %v (Chimera's Eye missing)",
			jc[db.Agility], base[db.Agility])
	}
	s.SetProfessions(nil)

	// best head enchant for an agility build: Arcanum of Ramkahen
	if e := s.BestEnchant(helm); e == nil || e.Name != "Arcanum of Ramkahen" {
		t.Fatalf("head enchant = %+v, want Arcanum of Ramkahen", e)
	}

	// tier bonuses price in: 4 worn T11 pieces carry curated EP
	worn := []score.Worn{}
	for _, id := range []int{60303, 60304, 60305, 60307} { // T11 head/chest/gloves/legs
		it := d.Items[id]
		if it == nil {
			t.Fatalf("T11 piece %d missing", id)
		}
		worn = append(worn, score.Worn{Item: it})
	}
	if got := s.SetBonusScore(worn); got != 215 { // 2pc 35 + 4pc 180
		t.Fatalf("T11 4pc bonus EP = %v, want 215", got)
	}
}

// TestUpgradeProposals runs the actual upgrade path for a survival
// hunter wearing a 359 belt: proposals must be slot- and armor-legal
// with positive deltas, and the T11 chest must rank as an upgrade over
// a 346 dungeon chest.
func TestUpgradeProposals(t *testing.T) {
	d, err := db.LoadCata()
	if err != nil {
		t.Fatal(err)
	}
	p := findProfileFor(db.ClassHunter, "Survival", nil)
	prof := *p
	prof.Caps = p.EffectiveCaps(nil)
	s := score.New(d, &prof)

	worn := []score.Worn{
		{Item: d.Items[56539]}, // Corded Viper Belt (waist 359)
		{Item: d.Items[56317]}, // ranged 346
	}
	ups := s.UpgradesWorn(worn, 0, 30, func(it *db.Item) bool {
		return it.ILvl >= 300 && it.ILvl <= 372 && it.UsableBy(db.ClassHunter)
	})
	if len(ups) == 0 {
		t.Fatal("no upgrade proposals for a 359 belt")
	}
	for _, u := range ups {
		if u.Item.Slot != db.SlotWaist {
			t.Fatalf("proposal in wrong slot: %+v", u.Item)
		}
		if u.Item.ArmorType != db.ArmorMail {
			t.Fatalf("non-mail proposed: %s (%v)", u.Item.Name, u.Item.ArmorType)
		}
		if u.Delta <= 0 {
			t.Fatalf("non-positive delta proposed: %s %v", u.Item.Name, u.Delta)
		}
	}

	// chest row: the T11 tunic must be an upgrade over a 346 agility
	// dungeon chest (picked deterministically — lowest id — so the
	// assertion never depends on map iteration order)
	var chest346 *db.Item
	for _, it := range d.Items {
		if it.Slot == db.SlotChest && it.ArmorType == db.ArmorMail &&
			it.ILvl == 346 && it.Stats[db.Agility] > 0 &&
			(chest346 == nil || it.ID < chest346.ID) {
			chest346 = it
		}
	}
	if chest346 == nil {
		t.Fatal("no 346 agility mail chest in the pool")
	}
	worn[0] = score.Worn{Item: chest346}
	if delta := s.SwapDelta(worn, 0, d.Items[60304]); delta <= 0 {
		t.Fatalf("T11 tunic not an upgrade over %s: delta %v", chest346.Name, delta)
	}
}

// TestTierLabels checks the ilvl-derived cata tier tags; PvP class
// sets stay unlabeled (seasons, not tiers).
func TestTierLabels(t *testing.T) {
	d, err := db.LoadCata()
	if err != nil {
		t.Fatal(err)
	}
	for id, want := range map[int]string{
		60303: "T11",  // Lightning-Charged Headguard 359
		71501: "T12H", // Flamewaker's Tunic heroic 391
		71050: "T12",  // Flamewaker's Gloves 378
	} {
		if got := tierLabel(d.Items[id]); got != want {
			t.Errorf("tierLabel(%d) = %q, want %q", id, got, want)
		}
	}
	for _, it := range d.Items {
		if strings.Contains(it.SetName, "Gladiator") && tierLabel(it) != "" {
			t.Fatalf("PvP set piece labeled a tier: %s", it.Name)
		}
	}
}

// TestCapTalentDetection: API imports carry exact ranks — detection
// beats assumptions, absence means not taken, and paste imports (nil
// ranks) fall back to the assumption flow.
func TestCapTalentDetection(t *testing.T) {
	precision := rules85.CapTalents[db.ClassRogue][0]
	taken := map[int]int{13832: 2} // rank-2 spell of Precision
	if got := precision.DetectRank(taken); got != 2 {
		t.Errorf("detected rank = %d, want 2", got)
	}
	if got := precision.DetectRank(map[int]int{99999: 1}); got != 0 {
		t.Errorf("untaken talent rank = %d, want 0 (exact data)", got)
	}
	if got := precision.DetectRank(nil); got != -1 {
		t.Errorf("paste import rank = %d, want -1 (assume)", got)
	}
}

// TestCataTierBonusCoverage: every class tier line in the cata pool
// must have a setBonuses entry — mechanics notes at minimum (EP may
// be 0 until the class gets a profile with sim units). The list is
// the tier lines only; PvP/craft/legendary sets are curated when
// their audience exists.
func TestCataTierBonusCoverage(t *testing.T) {
	tiers := []string{
		// T11
		"Battlegear of the Raging Elements", "Regalia of the Raging Elements", "Vestments of the Raging Elements",
		"Earthen Battleplate", "Earthen Warplate", "Firelord's Vestments",
		"Lightning-Charged Battlegear",
		"Magma Plated Battlearmor", "Magma Plated Battlegear",
		"Mercurial Regalia", "Mercurial Vestments",
		"Reinforced Sapphirium Battlearmor", "Reinforced Sapphirium Battleplate", "Reinforced Sapphirium Regalia",
		"Shadowflame Regalia", "Stormrider's Battlegarb", "Stormrider's Regalia", "Stormrider's Vestments",
		"Wind Dancer's Regalia",
		// T12
		"Balespider's Burning Vestments", "Battlearmor of Immolation", "Battleplate of Immolation", "Regalia of Immolation",
		"Elementium Deathplate Battlearmor", "Elementium Deathplate Battlegear",
		"Firehawk Robes of Conflagration", "Flamewaker's Battlegear",
		"Molten Giant Battleplate", "Molten Giant Warplate",
		"Obsidian Arborweave Battlegarb", "Obsidian Arborweave Regalia", "Obsidian Arborweave Vestments",
		"Regalia of the Cleansing Flame", "Vestments of the Cleansing Flame", "Vestments of the Dark Phoenix",
		"Volcanic Battlegear", "Volcanic Regalia", "Volcanic Vestments",
		// T13
		"Armor of Radiant Glory", "Battleplate of Radiant Glory", "Regalia of Radiant Glory",
		"Blackfang Battleweave", "Colossal Dragonplate Armor", "Colossal Dragonplate Battlegear",
		"Deep Earth Battlegarb", "Deep Earth Regalia", "Deep Earth Vestments",
		"Necrotic Boneplate Armor", "Necrotic Boneplate Battlegear",
		"Regalia of Dying Light", "Vestments of Dying Light", "Vestments of the Faceless Shroud",
		"Spiritwalker's Battlegear", "Spiritwalker's Regalia", "Spiritwalker's Vestments",
		"Time Lord's Regalia", "Wyrmstalker Battlegear",
	}
	for _, name := range tiers {
		bs, ok := setBonuses[name]
		if !ok {
			t.Errorf("tier set %q has no bonus entry", name)
			continue
		}
		for _, b := range bs {
			if b.Note == "" {
				t.Errorf("tier set %q %dpc has an empty note", name, b.Pieces)
			}
		}
	}
}
