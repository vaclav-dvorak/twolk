package db

import "testing"

func TestLoadForever(t *testing.T) {
	d, err := LoadForever()
	if err != nil {
		t.Fatal(err)
	}
	// ~7.4k sim items + ~3.7k leftovers at the pin
	if len(d.Items) < 10000 || len(d.Enchants) < 250 {
		t.Fatalf("suspiciously small db: %d items, %d enchants", len(d.Items), len(d.Enchants))
	}

	// Hillman's Shoulders — a low-level green only leftover_db.json
	// carries; leveling characters wear these
	sh := d.Items[4251]
	if sh == nil {
		t.Fatal("leftover item 4251 missing")
	}
	if sh.ReqLevel != 21 || sh.Stats[Stamina] != 5 || sh.Stats[Spirit] != 3 || sh.Stats[Armor] != 65 {
		t.Errorf("Hillman's Shoulders: req %d, stats %v", sh.ReqLevel, sh.Stats)
	}

	// Brain Hacker: "+1% crit" stored as 14 fixed rating, decoded into
	// the shared melee slot
	if bh := d.Items[1263]; bh == nil || bh.Stats[MeleeCrit] != 14 || bh.Stats[Stamina] != 16 {
		t.Errorf("Brain Hacker = %+v", bh)
	}

	// Golemsight Long Gun: the Forever ranged enum moved guns to 3
	if g := d.Items[273029]; g == nil || g.RangedType != RangedGun {
		t.Errorf("Golemsight Long Gun = %+v", g)
	}

	// Heavy Armor Kit fits chest, hands, legs and feet (extra types)
	for _, slot := range []Slot{SlotChest, SlotHands, SlotLegs, SlotFeet} {
		if e := d.EnchantByEffect(8484, slot); e == nil || e.Slot != slot || e.Stats[Stamina] != 3 {
			t.Errorf("Heavy Armor Kit on %v = %+v", slot, e)
		}
	}
}

// Shirts and tabards carry no item type upstream; like the other
// versions' data, the Forever db leaves them out.
func TestForeverSkipsCosmetics(t *testing.T) {
	d, err := LoadForever()
	if err != nil {
		t.Fatal(err)
	}
	if it := d.Items[148]; it != nil { // Rugged Trapper's Shirt
		t.Errorf("shirt loaded as gear: %+v", it)
	}
	for _, it := range d.Items {
		if it.Slot == SlotUnknown {
			t.Fatalf("slotless item %d %s loaded", it.ID, it.Name)
		}
	}
}

// TestForeverCraftSkills: learn levels come from the pattern item when
// one teaches the craft (exact), else an estimate from the trainer
// recipe's thresholds; bind-on-pickup crafts are marked.
func TestForeverCraftSkills(t *testing.T) {
	d, err := LoadForever()
	if err != nil {
		t.Fatal(err)
	}
	// Pattern: Skulker's Leather Belt requires Leatherworking 150 —
	// confirmed in game (a 152 leatherworker made it)
	if cs := d.CraftSkills[1255080]; cs.Learn != 150 || cs.Estimated {
		t.Errorf("Skulker's Leather Belt = %+v, want exact 150", cs)
	}
	if cs := d.CraftSkills[3770]; !cs.Estimated || cs.Learn == 0 { // Toughened Leather Gloves: trainer
		t.Errorf("Toughened Leather Gloves = %+v, want an estimate", cs)
	}
	if !d.BoPCrafts[252520] || d.BoPCrafts[4253] {
		t.Error("Skulker's belt binds on pickup, the gloves don't")
	}
	if len(d.CraftSkills) < 1300 {
		t.Errorf("only %d craft skills", len(d.CraftSkills))
	}
}

// TestForeverWorldDrops: sourceless items QuestieDB shows dropping from
// many NPCs or chests become world drops; items without such data stay
// sourceless rather than guessed at.
func TestForeverWorldDrops(t *testing.T) {
	d, err := LoadForever()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []int{3020, 4048, 4052} { // Enduring Cap, Emblazoned Hat, Insignia Cap
		it := d.Items[id]
		if it == nil || len(it.Sources) != 1 || it.Sources[0].Drop == nil || it.Sources[0].Drop.OtherName != WorldDrop {
			t.Errorf("item %d: sources %+v, want one world drop", id, it)
		}
	}
	if it := d.Items[6204]; it == nil || len(it.Sources) != 0 { // Tribal Worg Helm: no drop data
		t.Errorf("Tribal Worg Helm got a source: %+v", it)
	}
}

// TestForeverUpstreamWorldDrops: the upstream's own "World Drop"
// spelling normalizes to the planner's marker (else the filter files
// them under dungeon loot).
func TestForeverUpstreamWorldDrops(t *testing.T) {
	d, err := LoadForever()
	if err != nil {
		t.Fatal(err)
	}
	it := d.Items[252512] // Brawler's Leather Helm: world drop and leatherworking craft
	found := false
	for _, s := range it.Sources {
		found = found || (s.Drop != nil && s.Drop.OtherName == WorldDrop)
	}
	if !found {
		t.Errorf("Brawler's Leather Helm sources %+v, want a WorldDrop", it.Sources)
	}
}

// TestForeverFactionNames: every reputation route names its faction
// from the upstream's own faction list ("faction 21" was Booty Bay).
func TestForeverFactionNames(t *testing.T) {
	d, err := LoadForever()
	if err != nil {
		t.Fatal(err)
	}
	reps := 0
	for _, it := range d.Items {
		for _, s := range it.Sources {
			if s.Rep == nil {
				continue
			}
			reps++
			if d.FactionName(s.Rep.FactionID) == "" {
				t.Errorf("%s (%d): rep faction %d unnamed", it.Name, it.ID, s.Rep.FactionID)
			}
		}
	}
	if reps == 0 {
		t.Fatal("no reputation routes at all")
	}
	if got := d.FactionName(21); got != "Booty Bay" {
		t.Errorf("faction 21 = %q, want Booty Bay", got)
	}
}

// TestForeverPvP: the upstream's bare PvP routes and the faction
// insignia family classify as PvP; insignias lock to their faction.
func TestForeverPvP(t *testing.T) {
	d, err := LoadForever()
	if err != nil {
		t.Fatal(err)
	}
	for id, faction := range map[int]FactionRestriction{
		274444: FactionHordeOnly,    // Greater Insignia of the Horde (sourceless upstream)
		274445: FactionAllianceOnly, // Greater Insignia of the Alliance
		209619: FactionHordeOnly,    // Insignia of the Horde ({"pvp": {}} route)
	} {
		it := d.Items[id]
		if it == nil || !d.PvPItem(it) || it.Faction != faction {
			t.Errorf("item %d = %+v, want PvP and faction %v", id, it, faction)
		}
		for _, s := range it.Sources {
			if s.PvP != nil {
				t.Errorf("%s keeps its bare PvP route", it.Name)
			}
		}
	}
	// battleground quartermaster gear: Rune of Perfection sells only at
	// Illiyana Moonblaze (Silverwing Sentinels, Warsong Gulch)
	if it := d.Items[21565]; !d.PvPItem(it) {
		t.Error("Rune of Perfection not classified PvP")
	}
	// PvE controls: a craft, and a quest reward
	for _, id := range []int{4253, 9624} { // Toughened Leather Gloves, Triprunner Dungarees
		if it := d.Items[id]; d.PvPItem(it) {
			t.Errorf("%s classified PvP", it.Name)
		}
	}
}
