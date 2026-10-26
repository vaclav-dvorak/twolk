// Package cata is the Cataclysm Classic (4.4.2 client) game module:
// the embedded WoWSims-cata item database (the Cataclysm Classic
// simulator's data, so 4.4.x-era) bundled with the Cataclysm rules the
// planner needs. It is deliberately minimal — enough for a
// Whitemane character to import and render — and grows scoring rules
// (profiles, weights, reforge modeling) the way the WotLK module did.
//
// FLAVOR: the slug "cata" means Cataclysm CLASSIC — phase-progressed
// 4.4.x with its Classic-only systems (Elemental Rune protocols,
// per-phase currency rotation). Original Cataclysm 4.3.4 is a
// DIFFERENT flavor: if it is ever wanted, it gets its own slug (config
// keys are stable and game-prefixed), its own game.Game, and its own
// generated db overlays — the TCPP TDB 4.3.4 dump that this module
// only approximates vendor data from would be that module's native
// ground truth. Nothing may be shared silently between the flavors.
package cata

import (
	"strings"

	"github.com/vaclav-dvorak/twolk/internal/db"
	"github.com/vaclav-dvorak/twolk/internal/game"
)

// LoadDB parses the embedded cata item database (the module's only
// expensive part; callers attach it off the UI thread, as with wotlk).
func LoadDB() (*db.Database, error) {
	return db.LoadCata()
}

// New builds the module around a given database (nil until LoadDB
// delivers it).
func New(d *db.Database) *game.Game {
	return &game.Game{
		Slug:        "cata",
		Name:        "Cataclysm 4.4.2 (classic)",
		Flavor:      "cata classic",
		Accent:      "#FF8700", // Deathwing fire orange — the expansion's signature (xterm 208)
		PlayerLevel: 85,
		// Same ilvl-based philosophy as the wotlk module: 272 is where
		// Cataclysm leveling drops start (Hyjal/Vashj'ir), so every
		// 80–85 upgrade path stays while WotLK-era gear below ICC
		// heroic drops out.
		MinUsefulILvl: 272,
		Servers: []game.Server{{
			Name:   "Whitemane",
			Slug:   "whitemane",
			Realms: []string{"Gilneas"},
			// fetch-based: api.whitemane.gg answers direct JSON (see
			// whitemane.md), no paste needed
			Import: game.ImportFetch,
			Fetch:  FetchRaw,
		}},
		Phases: []game.Phase{
			{Label: "BWD / BoT / Throne (T11)", Short: "P1 T11"},
			{Label: "Rise of the Zandalari (ZA/ZG)", Short: "P2 Zul"},
			{Label: "Firelands (T12)", Short: "P3 FL"},
			{Label: "Dragon Soul (T13)", Short: "P4 DS"},
		},
		// endgame currency first, like the wotlk module; the fragment
		// currencies are the Classic heroic-plus protocol economy
		Currencies: []db.Emblem{db.EmblemValorPoints, db.EmblemJustice,
			db.EmblemFissure, db.EmblemObsidian},
		// justice and valor exist from launch (the phase POOL shift is
		// RotatePrice's job); the fragment currencies ship with their
		// protocol: Inferno with Firelands, Twilight with Dragon Soul
		CurrencyPhase: map[db.Emblem]int{
			db.EmblemFissure:  3,
			db.EmblemObsidian: 4,
		},
		// the Firelands daily hub opens its vendors (Damek's plans
		// and Flintlocke's Woodchucker schematic, the mark gear) over
		// weeks of dailies
		Unlocks: []game.Unlock{{Slug: "molten-front", Label: "Molten Front vendors (dailies unlock)",
			Short: "Molten Front", Phase: 3, Zone: 5733}},
		TierLabel: tierLabel,
		// hunter builds have profiles (profiles.go); everything else
		// degrades to the read-only gear view
		FindProfile: findProfileFor,
		Rules:       rules85,
		DB:          d,
	}
}

// tierLabel names the tier of a class-set piece, derived from item
// level (each cata tier has fixed ilvls; the PvP class sets are
// excluded — they are seasons, not tiers). "H" marks heroic, "L" the
// Dragon Soul Raid Finder versions.
func tierLabel(it *db.Item) string {
	if it.SetName == "" || len(it.ClassAllowlist) != 1 ||
		strings.Contains(it.SetName, "Gladiator") {
		return ""
	}
	switch it.ILvl {
	case 359:
		return "T11"
	case 372:
		return "T11H"
	case 378:
		return "T12"
	case 391:
		return "T12H"
	case 384:
		return "T13L"
	case 397:
		return "T13"
	case 410:
		return "T13H"
	}
	return ""
}
