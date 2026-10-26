package cata

import (
	"testing"

	"github.com/vaclav-dvorak/twolk/internal/armory"
	"github.com/vaclav-dvorak/twolk/internal/armory/whitemane"
	"github.com/vaclav-dvorak/twolk/internal/db"
)

// TestConvert round-trips a whitemane character through the armory
// format: what convert emits must parse with armory.Parse and carry
// the details (real gems/enchants) the planner scores.
func TestConvert(t *testing.T) {
	raw, err := convert(&whitemane.Character{
		Profile: whitemane.Profile{
			GUID: 84512, Name: "Zulja", Race: 2, Class: 3,
			Level: 85, Faction: "Horde",
		},
		Realm: "Gilneas",
		Items: []whitemane.EquippedItem{
			{Slot: 0, ItemID: 60303, EnchantID: 4209, Sockets: []int{68778, 52209, 0},
				Reforge: &whitemane.Reforge{Source: 49, Target: 31, Value: 67}},
			{Slot: 15, ItemID: 19970, EnchantID: 846, Sockets: []int{0, 0, 0}},
		},
		Specializations: []whitemane.Specialization{
			{Index: 0, SpecializationID: 809},
			{Index: 1, SpecializationID: 0}, // unused slot must vanish
		},
		Talents: map[string][]whitemane.TalentEntry{
			"0": {
				{TalentID: 9432, SpellID: 52786, Rank: 2},
				{TalentID: 9450, SpellID: 82834, Rank: 1},
				{TalentID: 9472, SpellID: 53299, Rank: 0}, // not taken
			},
		},
		Professions: []whitemane.Profession{
			{ID: 129, Skill: 525}, // First Aid: secondary, must vanish
			{ID: 171, Skill: 525},
			{ID: 202, Skill: 525},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	c, err := armory.Parse(raw)
	if err != nil {
		t.Fatalf("convert output does not parse: %v", err)
	}
	if c.Name != "Zulja" || c.Realm != "Gilneas" || c.Level != 85 ||
		c.Race != "Orc" || c.ClassName != "Hunter" || c.Class != db.ClassHunter {
		t.Fatalf("identity wrong: %+v", c)
	}
	if c.Spec() != "Survival" {
		t.Fatalf("spec = %q, want Survival", c.Spec())
	}
	if len(c.Equipment) != 2 {
		t.Fatalf("equipment wrong: %+v", c.Equipment)
	}
	helm := c.Equipment[0]
	if helm.ItemID != 60303 || helm.EnchantID != 4209 || !helm.HasDetail ||
		len(helm.Gems) != 3 || helm.Gems[0] != 68778 {
		t.Fatalf("worn detail lost: %+v", helm)
	}
	if len(c.Professions) != 2 || c.Professions[0].Name != "Alchemy" ||
		c.Professions[1].Name != "Engineering" || c.Professions[0].Skill != 525 {
		t.Fatalf("professions wrong: %+v", c.Professions)
	}

	// exact talent ranks survive the round-trip: taken ranks keyed by
	// spell id, rank-0 entries dropped (absence = not taken, exactly)
	if len(c.TalentRanks) != 2 || c.TalentRanks[52786] != 2 || c.TalentRanks[82834] != 1 {
		t.Fatalf("talent ranks wrong: %+v", c.TalentRanks)
	}
	if _, ok := c.TalentRanks[53299]; ok {
		t.Fatal("rank-0 talent must not be stored")
	}
}

func TestConvertUnknownClass(t *testing.T) {
	_, err := convert(&whitemane.Character{
		Profile: whitemane.Profile{Name: "X", Class: 10}, // 10 unused in cata
		Items:   []whitemane.EquippedItem{{ItemID: 1}},
	})
	if err == nil {
		t.Fatal("expected an error for an unknown class id")
	}
}
