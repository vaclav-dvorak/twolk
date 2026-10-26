package db

import "testing"

// TestRecipeCoverage: every crafted item in the WotLK pool has a recipe
// route, and the known routes resolve the way the game hands them out.
func TestRecipeCoverage(t *testing.T) {
	d, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	missing := 0
	for _, it := range d.Items {
		for _, s := range it.Sources {
			if s.Crafted == nil || it.ILvl < 200 {
				continue
			}
			if _, ok := d.Recipe(s.Crafted.SpellID); !ok {
				missing++
				t.Logf("no recipe route: %s (spell %d)", it.Name, s.Crafted.SpellID)
			}
		}
	}
	if missing > 1 { // Deflecting Bracers: neither trained nor a recipe item
		t.Errorf("%d crafted items without a recipe route", missing)
	}

	for _, c := range []struct {
		spell int
		want  RecipeSource
	}{
		{60730, RecipeSource{"Pattern: Swiftarrow Hauberk", RecipeVendor, "Braeg Stoutbeard", "", 0}},
		{70557, RecipeSource{"Pattern: Footpads of Impending Death", RecipeVendor, "Alchemist Finklestein", "The Ashen Verdict — Honored", 0}},
		{63194, RecipeSource{"Pattern: Belt of Dragons", RecipeDrop, "Ulduar", "", 0}},
		// script-summoned bosses, located via their encounter's map
		{67086, RecipeSource{"Pattern: Knightbane Carapace", RecipeDrop, "Trial of the Crusader", "", 0}},
		// trained crafts win over a dead recipe item for the same spell
		{56024, RecipeSource{"", RecipeTrainer, "", "", 0}},
	} {
		if got, _ := d.Recipe(c.spell); got != c.want {
			t.Errorf("spell %d: got %+v, want %+v", c.spell, got, c.want)
		}
	}
}

// TestCataRecipes: the cata supplemental crafted sources carry their
// craft spell, and it resolves to the recipe's route.
func TestCataRecipes(t *testing.T) {
	d, err := LoadCata()
	if err != nil {
		t.Fatal(err)
	}
	for item, want := range map[int]RecipeSource{
		56564: {"Pattern: Dragonkiller Tunic", RecipeVendor, "Bombus Finespindle / Jillian Tanner", "", 0},
		69939: {"Pattern: Dragonfire Gloves", RecipeDrop, "Firelands", "", 0},
		52318: {"Design: Band of Blades", RecipeVendor, "Isabel Jones / Laida Gembold", "", 0},
	} {
		it := d.Items[item]
		if it == nil || len(it.Sources) == 0 || it.Sources[0].Crafted == nil {
			t.Fatalf("item %d: not a crafted pool item", item)
		}
		if got, _ := d.Recipe(it.Sources[0].Crafted.SpellID); got != want {
			t.Errorf("%s: got %+v, want %+v", it.Name, got, want)
		}
	}
	// the scope's schematic is sold in the Molten Front (area 5733)
	for _, e := range d.Enchants {
		if e.Name == "Flintlocke's Woodchucker" {
			r, ok := d.EnchantRecipe(e)
			if !ok || r.Kind != RecipeVendor || r.Zone != 5733 {
				t.Errorf("Woodchucker recipe = %+v, %v", r, ok)
			}
		}
	}
}
