// Package game defines the seam between the version-agnostic planner
// and one game version's data and rules. A Game value bundles the item
// database with everything the planner may only know per version; the
// TUI reads such rules exclusively from the active Game, never from
// version literals. The active character's game version selects which
// module builds the value (internal/game/wotlk today, a Cataclysm
// module later) — one version is active at a time, and there is no
// cross-version logic anywhere.
package game

import (
	"context"

	"github.com/vaclav-dvorak/twolk/internal/db"
	"github.com/vaclav-dvorak/twolk/internal/score"
)

// Phase is one step of a progressively-unlocked server's content.
type Phase struct {
	Label string // filter-modal line, e.g. "Ulduar (T8)"
	Short string // status-line summary, e.g. "P2 Ulduar"
}

// ImportMethod selects the import-flow wiring for a server: armories
// are per-server software, not a standard.
type ImportMethod int

const (
	// ImportPaste: the user copies JSON from the armory site (browser
	// snippet or plain API response) and pastes it into the modal.
	ImportPaste ImportMethod = iota + 1
	// ImportFetch: the planner calls the armory API directly.
	ImportFetch
)

// Server is one supported private server offering this game version,
// with the wiring the import flow needs.
type Server struct {
	Name   string   // display name, e.g. "Warmane"
	Slug   string   // stable config identifier, never shown
	Realms []string // supported realms

	Import ImportMethod

	// PageURL renders a character's armory page for ImportPaste
	// servers (nil otherwise) — the page the export snippet runs on.
	PageURL func(realm, name string) string

	// Fetch imports a character straight from the server's armory API
	// for ImportFetch servers (nil otherwise), returning the export in
	// the planner's armory JSON format — the same bytes a paste would
	// have stored.
	Fetch func(ctx context.Context, realm, name string) ([]byte, error)
}

// Game is one game version's module: the item database plus the rules
// the planner needs. Fields are data where data suffices; behavior
// that data can't express (tier labeling, profile lookup) is a func.
type Game struct {
	Slug string // stable config-key prefix; never shown to the user
	Name string // display name, "<expansion> <patch> (<flavor>)", e.g. "WotLK 3.3.5a (legacy)"

	// Flavor is the short version tag shown next to realm rows in the
	// import wizard, e.g. "wotlk legacy", "cata classic".
	Flavor string

	// Accent is the version's signature color (hex): the TUI re-tints
	// its title, borders and hotkeys with it and paints the expansion
	// stripe, so which version is active is visible at a glance.
	Accent string

	// PlayerLevel is the only supported character level — weights,
	// caps and the candidate pool all assume endgame characters.
	PlayerLevel int

	// MinUsefulILvl floors upgrade candidates: below it nothing can
	// matter to an endgame character. Worn items are unaffected — it
	// only gates what the planner suggests.
	MinUsefulILvl int

	// Servers lists the supported private servers running this
	// version; the import wizard offers their realms.
	Servers []Server

	// Phases models the server's content unlocks; Phases[0] is
	// phase 1, the last entry the version's endgame.
	Phases []Phase

	// Currencies lists the vendor currencies the BiS shopping list
	// prices items in, most endgame first.
	Currencies []db.Emblem

	// CurrencyPhase is the server phase each currency starts existing
	// in (its vendors and earn routes ship with that content patch);
	// currencies not listed are open from phase 1. Spending planners
	// must not price items in a currency the phase filter says is not
	// on the server yet — the item's own phase tag is about the
	// CONTENT, not the currency, and the two can disagree.
	CurrencyPhase map[db.Emblem]int

	// Unlocks are progression gates INSIDE a phase that servers open
	// over weeks (Molten Front dailies unlocking its vendors): the
	// filter lets the player close one until their realm gets there.
	Unlocks []Unlock

	// TierLabel names the tier of a class-set piece ("T8.5"), "" for
	// items that aren't class tier pieces.
	TierLabel func(*db.Item) string

	// FindProfile resolves the build profile for a class and talent
	// tree. ranks are the exact talent ranks when the import carries
	// them (nil otherwise): trees hosting two roles (cata feral: cat or
	// bear) and talent-scaled mechanics (Balance of Power) read them.
	FindProfile func(c db.Class, tree string, ranks map[int]int) *score.Profile

	// Rules is the version's scoring rule set (rating conversions,
	// gem-tier policy, set bonuses, cap talents, weapon racials);
	// every profile FindProfile returns carries the same value.
	Rules *score.Rules

	// DB is the version's item database — the module's only expensive
	// part, attached asynchronously after construction. Everything
	// above is static rule data, usable immediately.
	DB *db.Database
}

// Unlock is one in-phase progression gate. Zone is the area its
// vendors stand in: gear they sell, crafted gear whose recipe they
// sell, and enchants whose recipe they sell all hang on it.
type Unlock struct {
	Slug  string // stable config key ("molten-front")
	Label string // filter row label
	Short string // filter summary word when closed
	Phase int    // the phase that brings it
	Zone  int    // vendor area id
}

// MaxPhase is the number of the version's last content phase.
func (g *Game) MaxPhase() int { return len(g.Phases) }

// CurrencyOpen reports whether a currency exists on a server at the
// given phase (see CurrencyPhase; unlisted currencies always exist).
func (g *Game) CurrencyOpen(e db.Emblem, phase int) bool {
	return phase >= g.CurrencyPhase[e]
}

// ClampPhase forces p into the version's valid phase range (upstream
// item data occasionally carries phase 0).
func (g *Game) ClampPhase(p int) int { return min(max(p, 1), len(g.Phases)) }
