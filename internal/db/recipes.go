package db

// RecipeKind is how a crafter learns a craft (see recipes_gen.go).
type RecipeKind int

const (
	RecipeUnknown   RecipeKind = iota // a recipe item nothing hands out
	RecipeTrainer                     // taught by the profession trainer
	RecipeVendor                      // recipe sold by a vendor
	RecipeDrop                        // recipe drops (named mobs / instance)
	RecipeQuest                       // recipe is a quest reward
	RecipeWorldDrop                   // recipe drops anywhere
)

// RecipeSource describes where the recipe for a craft spell comes from.
type RecipeSource struct {
	Name  string // recipe item ("Pattern: Swiftarrow Hauberk"); "" for trainer
	Kind  RecipeKind
	Where string // vendor(s), drop location or quest title
	Rep   string // required standing ("The Ashen Verdict — Revered")
	Zone  int    // area id the recipe vendor stands in (vendor routes; 0 = several)
}

// EnchantRecipe returns how the recipe for an item-based enchant's
// carrier (a scope, an armor kit) is learned — false when the enchant
// isn't crafted or its craft has no known route.
func (d *Database) EnchantRecipe(e *Enchant) (RecipeSource, bool) {
	sp, ok := d.Rules.EnchantCrafts[e.ItemID]
	if !ok || e.ItemID == 0 {
		return RecipeSource{}, false
	}
	return d.Recipe(sp)
}

// Recipe returns how the craft spell's recipe is learned.
func (d *Database) Recipe(spellID int) (RecipeSource, bool) {
	r, ok := d.Rules.Recipes[spellID]
	return r, ok
}
