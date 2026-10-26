// Package wotlk is the WotLK 3.3.5a (build 12340) game module: the
// embedded WoWSims-derived item database bundled with the 3.3.5 rules
// the planner needs. It is the founding module — rule tables that
// still live in the db and score packages migrate here incrementally
// (plan.md task 5).
package wotlk

import (
	"net/url"

	"github.com/vaclav-dvorak/twolk/internal/db"
	"github.com/vaclav-dvorak/twolk/internal/game"
	"github.com/vaclav-dvorak/twolk/internal/score"
)

// LoadDB parses the embedded item database. It is the module's only
// expensive part (~100ms) — callers construct the module with New
// immediately and attach the database off the UI thread.
func LoadDB() (*db.Database, error) {
	return db.Load()
}

// New builds the module around a given database (nil until LoadDB
// delivers it; tests inject small fixture databases).
func New(d *db.Database) *game.Game {
	return &game.Game{
		Slug:        "wotlk",
		Name:        "WotLK 3.3.5a (legacy)",
		Flavor:      "wotlk legacy",
		Accent:      "#5FD7FF", // frost blue — the expansion's signature (xterm 81)
		PlayerLevel: 80,
		// Deliberately ilvl-based, not expansion-based — the upstream
		// expansion tag is unreliable (missing on half the items, and
		// e.g. Frostscythe of Lord Ahune, ilvl 232, is tagged TBC).
		// 130 keeps every Northrend leveling drop and all TBC endgame
		// curiosities (Thori'dal 164, DMC: Greatness 200) while
		// dropping vanilla gear and low-level trash.
		MinUsefulILvl: 130,
		Servers: []game.Server{{
			Name:   "Warmane",
			Slug:   "warmane",
			Realms: []string{"Onyxia", "Icecrown"},
			// paste-based: the armory site sits behind a browser bot
			// check, and the enriched export (real gems/enchants)
			// needs the browser console snippet anyway
			Import: game.ImportPaste,
			PageURL: func(realm, name string) string {
				return "https://armory.warmane.com/character/" +
					url.PathEscape(name) + "/" + url.PathEscape(realm) + "/summary"
			},
		}},
		Phases: []game.Phase{
			{Label: "Naxxramas / OS / EoE (T7)", Short: "P1 Naxx"},
			{Label: "Ulduar (T8)", Short: "P2 Ulduar"},
			{Label: "Trial of the Crusader (T9)", Short: "P3 ToC"},
			{Label: "Icecrown Citadel (T10)", Short: "P4 ICC"},
			{Label: "Ruby Sanctum (mini-phase after ICC)", Short: "P5 RS"},
		},
		Currencies: []db.Emblem{db.EmblemFrost, db.EmblemTriumph,
			db.EmblemConquest, db.EmblemValor, db.EmblemHeroism},
		// each badge ships with its raid patch: Heroism/Valor at launch,
		// Conquest with Ulduar (3.1), Triumph with ToC (3.2), Frost with
		// ICC (3.3)
		CurrencyPhase: map[db.Emblem]int{
			db.EmblemConquest: 2,
			db.EmblemTriumph:  3,
			db.EmblemFrost:    4,
		},
		TierLabel:   tierLabel,
		FindProfile: func(c db.Class, tree string, _ map[int]int) *score.Profile { return score.Find(c, tree) },
		Rules:       score.WotLK,
		DB:          d,
	}
}

// tierLabel names the tier of a class-set piece ("T8.5"), derived from
// phase + item level. Class restriction separates true tier sets from
// crafted/world sets, which carry set names but no class lock.
func tierLabel(it *db.Item) string {
	if it.SetName == "" || len(it.ClassAllowlist) != 1 {
		return ""
	}
	switch min(max(it.Phase, 1), 5) {
	case 1:
		if it.ILvl <= 205 {
			return "T7"
		}
		return "T7.5"
	case 2:
		// original 3.3.5 ilvls (statFixes applied): Valorous 219,
		// Conqueror's 226
		if it.ILvl < 223 {
			return "T8"
		}
		return "T8.5"
	case 3:
		switch {
		case it.ILvl < 240:
			return "T9"
		case it.ILvl < 250:
			return "T9.5"
		default:
			return "T9.75"
		}
	case 4:
		switch {
		case it.ILvl < 258:
			return "T10"
		case it.ILvl < 270:
			return "T10.5"
		default:
			return "T10.75"
		}
	}
	return ""
}
