// Package forever is the World of Warcraft: Forever game module —
// Blizzard's official Classic+ branch (level 60 cap, 1.60.x client,
// launched 2026-11-04). Unlike the private-server versions its content
// keeps growing: the embedded WoWSims-Forever item database is
// re-vendored with every twolk release.
//
// Forever is the first version that supports LEVELING characters: the
// planner's job below 60 is "what can I wear now", not endgame BiS.
//
// Scoring covers hunters so far (profiles.go); other classes get the
// read-only gear view.
package forever

import (
	"github.com/vaclav-dvorak/twolk/internal/db"
	"github.com/vaclav-dvorak/twolk/internal/game"
	"github.com/vaclav-dvorak/twolk/internal/score"
)

// LoadDB parses the embedded Forever item database.
func LoadDB() (*db.Database, error) {
	return db.LoadForever()
}

// New builds the module around a given database (nil until LoadDB
// delivers it).
func New(d *db.Database) *game.Game {
	return &game.Game{
		Slug:        "forever",
		Name:        "Forever 1.60 (official)",
		Flavor:      "forever",
		Accent:      "#00D7D7", // the logo disc's teal-cyan, clear of wotlk's frost blue (xterm 44)
		PlayerLevel: 60,
		Leveling:    true,
		Servers: []game.Server{{
			Name: "Blizzard",
			Slug: "blizzard",
			// Forever characters are realmless: the region is all the
			// armory URL needs besides the name
			Realms: []string{"EU", "US"},
			Import: game.ImportFetch,
			Fetch:  FetchRaw,
		}},
		Phases:      []game.Phase{{Label: "Launch", Short: "P1 Launch"}},
		TierLabel:   func(*db.Item) string { return "" },
		FindProfile: findProfile,
		ArmorAt:     armorAt,
		Rules:       rules60,
		DB:          d,
	}
}

// rules60 is the Forever scoring rule set. Gear states combat stats as
// percentages; the item data stores them as fixed level-independent
// ratings (see db.ForeverHitRatingPerPct), so these conversions hold at
// every level.
var rules60 = &score.Rules{
	Game:                 "forever",
	MeleeHitRatingPerPct: db.ForeverHitRatingPerPct,
	SpellHitRatingPerPct: db.ForeverHitRatingPerPct,
	RatingPerExpertise:   db.ForeverExpertiseRatingPerPct,
	CritRatingPerPct:     db.ForeverCritRatingPerPct,
	TreeOrder:            score.ClassicTreeOrder,
	// Artisan's Precision: +1% physical and spell hit (wowsims/forever
	// sim/core/racials.go @ the pin). Forever's other stat racials
	// (Human +1% crit) shift every item equally and rank nothing.
	RacialHit: map[string]float64{"Dwarf": 1},
	PercentRating: map[db.Stat]float64{
		db.MeleeHit:   db.ForeverHitRatingPerPct,
		db.MeleeCrit:  db.ForeverCritRatingPerPct,
		db.MeleeHaste: db.ForeverHasteRatingPerPct,
		db.Expertise:  db.ForeverExpertiseRatingPerPct,
		db.Dodge:      db.ForeverDodgeRatingPerPct,
		db.Parry:      db.ForeverParryRatingPerPct,
		db.Block:      db.ForeverBlockRatingPerPct,
	},
	Stats: []db.Stat{
		db.Strength, db.Agility, db.Stamina, db.Intellect, db.Spirit,
		db.SpellPower, db.MP5, db.SpellPenetration, db.AttackPower,
		db.RangedAttackPower, db.MeleeHit, db.MeleeCrit, db.MeleeHaste,
		db.ArmorPenetration, db.Expertise, db.Armor, db.BonusArmor,
		db.Defense, db.Block, db.BlockValue, db.Dodge, db.Parry,
		db.Health, db.Mana, db.ArcaneResistance, db.FireResistance,
		db.FrostResistance, db.NatureResistance, db.ShadowResistance,
	},
}
