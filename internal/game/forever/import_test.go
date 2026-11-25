package forever

import (
	"testing"

	"github.com/vaclav-dvorak/twolk/internal/armory"
	"github.com/vaclav-dvorak/twolk/internal/armory/forever"
	"github.com/vaclav-dvorak/twolk/internal/db"
)

// TestConvert runs an armory document through the export format and
// back through armory.Parse, the path every stored character takes.
func TestConvert(t *testing.T) {
	var ch forever.Character
	ch.Name, ch.Surname, ch.Level = "Brannoc", "Ironvale", 32
	ch.Race.Name, ch.Class.Name, ch.Faction.Name = "Dwarf", "Hunter", "Alliance"
	item := func(id int, slot string, ench int) forever.Item {
		var it forever.Item
		it.ID, it.Slot.Type = id, slot
		if ench != 0 {
			it.Enchantments = append(it.Enchantments, struct {
				EnchantmentID int `json:"enchantment_id"`
				Slot          struct {
					Type string `json:"type"`
				} `json:"enchantment_slot"`
			}{EnchantmentID: ench})
			it.Enchantments[0].Slot.Type = "PERMANENT"
		}
		return it
	}
	ch.Gear = map[string]forever.Item{
		"weapon": item(281296, "MAIN_HAND", 0),
		"hand":   item(4253, "HANDS", 8484),
		"shirt":  item(148, "SHIRT", 0),
	}
	ch.Specs = []forever.Spec{
		{Name: "Marksmanship", SpentPoints: 5, Talents: []forever.Talent{{ID: 2, Rank: 5}}},
		{Name: "Beast Mastery", Active: true, SpentPoints: 18, Talents: []forever.Talent{{ID: 1, Rank: 3}}},
	}
	raw, err := convert(&ch, "eu")
	if err != nil {
		t.Fatal(err)
	}
	c, err := armory.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if c.Name != "Brannoc Ironvale" || c.Realm != "EU" || c.Level != 32 || c.Class != db.ClassHunter {
		t.Errorf("character = %q @ %q level %d class %v", c.Name, c.Realm, c.Level, c.Class)
	}
	if c.Spec() != "Beast Mastery" {
		t.Errorf("spec = %q, want the active tree", c.Spec())
	}
	if c.TalentRanks[1] != 3 || c.TalentRanks[2] != 5 {
		t.Errorf("talent ranks = %v, want every tree's", c.TalentRanks)
	}
	// paper-doll order, real detail (worn enchant, no assumed gems)
	want := []struct {
		id, ench int
		slot     db.Slot
	}{{148, 0, db.SlotUnknown}, {4253, 8484, db.SlotHands}, {281296, 0, db.SlotWeapon}}
	if len(c.Equipment) != len(want) {
		t.Fatalf("equipment = %+v", c.Equipment)
	}
	for i, w := range want {
		eq := c.Equipment[i]
		if eq.ItemID != w.id || eq.EnchantID != w.ench || eq.Slot != w.slot || !eq.HasDetail {
			t.Errorf("equipment[%d] = %+v, want item %d ench %d slot %v", i, eq, w.id, w.ench, w.slot)
		}
	}
}
