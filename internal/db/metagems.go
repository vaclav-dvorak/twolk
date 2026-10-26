package db

import (
	"fmt"
	"strings"
)

// MetaCondition is a meta gem's activation requirement: minimum counts
// of red-, yellow- and blue-counting gems, or a "more X than Y color"
// comparison (TBC-era metas). Hybrid gems count toward both parent
// colors and prismatics toward all three, so a Twilight Opal (purple)
// satisfies both a red and a blue requirement at once.
type MetaCondition struct {
	MinRed, MinYellow, MinBlue int
	Greater, Lesser            GemColor // set = "more Greater than Lesser gems"
}

// Met reports whether the condition holds for the given per-category
// gem counts (each hybrid already counted in every category it matches).
func (c MetaCondition) Met(red, yellow, blue int) bool {
	if red < c.MinRed || yellow < c.MinYellow || blue < c.MinBlue {
		return false
	}
	if c.Greater == GemUnknown {
		return true
	}
	n := map[GemColor]int{GemRed: red, GemYellow: yellow, GemBlue: blue}
	return n[c.Greater] > n[c.Lesser]
}

// OneOfEach reports the common 1 red + 1 yellow + 1 blue rule (the case
// a single Nightmare Tear covers).
func (c MetaCondition) OneOfEach() bool {
	return c.MinRed == 1 && c.MinYellow == 1 && c.MinBlue == 1
}

// String renders the requirement, e.g. "at least 2 blue and 1 red gems".
func (c MetaCondition) String() string {
	if c.Greater != GemUnknown {
		return fmt.Sprintf("more %s than %s gems",
			strings.ToLower(c.Greater.String()), strings.ToLower(c.Lesser.String()))
	}
	var parts []string
	for _, p := range []struct {
		n int
		c string
	}{{c.MinRed, "red"}, {c.MinYellow, "yellow"}, {c.MinBlue, "blue"}} {
		if p.n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", p.n, p.c))
		}
	}
	if len(parts) == 0 {
		return "no gem requirement"
	}
	return "at least " + strings.Join(parts, " and ") + " gems"
}

// preCataMetaConditions maps WotLK and TBC meta gem item IDs to their
// activation rules. Transcribed from WoWSims ui/core/proto_utils/gems.ts
// at the wotlk pin (matches the in-game SpellItemEnchantmentCondition
// data). Metas missing here fall back to the one-of-each rule; every
// WotLK and TBC meta in the item pool is covered. Wired into the
// per-version RuleSet (rules.go) — the cata set layers
// cataMetaConditions on top.
var preCataMetaConditions = map[int]MetaCondition{
	// WotLK
	41285: {MinBlue: 2},                          // Chaotic Skyflare Diamond
	41307: {MinRed: 1, MinYellow: 1, MinBlue: 1}, // Destructive Skyflare Diamond
	41333: {MinRed: 3},                           // Ember Skyflare Diamond
	41335: {MinRed: 2, MinYellow: 1},             // Enigmatic Skyflare Diamond
	41339: {MinRed: 1, MinYellow: 2},             // Swift Skyflare Diamond
	41375: {MinRed: 1, MinYellow: 1, MinBlue: 1}, // Tireless Skyflare Diamond
	41376: {MinRed: 2},                           // Revitalizing Skyflare Diamond
	41377: {MinRed: 1, MinBlue: 2},               // Effulgent Skyflare Diamond
	41378: {MinYellow: 2, MinBlue: 1},            // Forlorn Skyflare Diamond
	41379: {MinRed: 2, MinBlue: 1},               // Impassive Skyflare Diamond
	41380: {MinRed: 1, MinBlue: 2},               // Austere Earthsiege Diamond
	41381: {MinYellow: 2, MinBlue: 1},            // Persistent Earthsiege Diamond
	41382: {MinRed: 1, MinYellow: 1, MinBlue: 1}, // Trenchant Earthsiege Diamond
	41385: {MinRed: 1, MinBlue: 2},               // Invigorating Earthsiege Diamond
	41389: {MinRed: 2, MinYellow: 1},             // Beaming Earthsiege Diamond
	41395: {MinRed: 2, MinBlue: 1},               // Bracing Earthsiege Diamond
	41396: {MinRed: 2, MinBlue: 1},               // Eternal Earthsiege Diamond
	41397: {MinBlue: 3},                          // Powerful Earthsiege Diamond
	41398: {MinRed: 1, MinYellow: 1, MinBlue: 1}, // Relentless Earthsiege Diamond
	41400: {MinRed: 1, MinYellow: 1, MinBlue: 1}, // Thundering Skyflare Diamond
	41401: {MinRed: 1, MinYellow: 1, MinBlue: 1}, // Insightful Earthsiege Diamond
	44076: {MinRed: 1, MinYellow: 2},             // Swift Starflare Diamond
	44078: {MinRed: 1, MinYellow: 1, MinBlue: 1}, // Tireless Starflare Diamond
	44081: {MinRed: 2, MinBlue: 1},               // Enigmatic Starflare Diamond
	44082: {MinRed: 1, MinBlue: 2},               // Impassive Starflare Diamond
	44084: {MinYellow: 2, MinBlue: 1},            // Forlorn Starflare Diamond
	44087: {MinBlue: 3},                          // Persistent Earthshatter Diamond
	44088: {MinYellow: 1, MinBlue: 2},            // Powerful Earthshatter Diamond
	44089: {MinRed: 1, MinYellow: 1, MinBlue: 1}, // Trenchant Earthshatter Diamond
	// TBC
	25890: {MinRed: 2, MinYellow: 2, MinBlue: 2}, // Destructive Skyfire Diamond
	25894: {MinRed: 1, MinYellow: 2},             // Swift Skyfire Diamond
	25896: {MinBlue: 3},                          // Powerful Earthstorm Diamond
	25898: {MinBlue: 5},                          // Tenacious Earthstorm Diamond
	25899: {MinRed: 2, MinYellow: 2, MinBlue: 2}, // Brutal Earthstorm Diamond
	25901: {MinRed: 2, MinYellow: 2, MinBlue: 2}, // Insightful Earthstorm Diamond
	28556: {MinRed: 1, MinYellow: 2},             // Swift Windfire Diamond
	28557: {MinRed: 1, MinYellow: 2},             // Swift Starfire Diamond
	32409: {MinRed: 2, MinYellow: 2, MinBlue: 2}, // Relentless Earthstorm Diamond
	32410: {MinRed: 2, MinYellow: 2, MinBlue: 2}, // Thundering Skyfire Diamond
	32641: {MinYellow: 3},                        // Imbued Unstable Diamond
	34220: {MinBlue: 2},                          // Chaotic Skyfire Diamond
	35501: {MinYellow: 1, MinBlue: 2},            // Eternal Earthstorm Diamond
	35503: {MinRed: 3},                           // Ember Skyfire Diamond
	25893: {Greater: GemBlue, Lesser: GemYellow}, // Mystical Skyfire Diamond
	25895: {Greater: GemRed, Lesser: GemYellow},  // Enigmatic Skyfire Diamond
	25897: {Greater: GemRed, Lesser: GemBlue},    // Bracing Earthstorm Diamond
	32640: {Greater: GemBlue, Lesser: GemYellow}, // Potent Unstable Diamond
}

// cataMetaConditions maps the Cataclysm meta gems (wowsims/cata
// ui/core/proto_utils/gems.ts @ the cata pin) — post-4.0.6
// requirements: the throughput metas want 3 red, never one-of-each.
// Missing these meant the WotLK one-of-each fallback made the gem
// optimizer buy a blue+yellow "activation" gem a cata meta never asked
// for.
var cataMetaConditions = map[int]MetaCondition{
	52289: {MinYellow: 2},             // Fleet Shadowspirit Diamond
	52291: {MinRed: 3},                // Chaotic Shadowspirit Diamond
	52292: {MinYellow: 1, MinBlue: 1}, // Bracing Shadowspirit Diamond
	52293: {MinBlue: 3},               // Eternal Shadowspirit Diamond
	52294: {MinYellow: 2},             // Austere Shadowspirit Diamond
	52295: {MinRed: 1, MinYellow: 1},  // Effulgent Shadowspirit Diamond
	52296: {MinYellow: 2},             // Ember Shadowspirit Diamond
	52297: {MinYellow: 1, MinBlue: 1}, // Revitalizing Shadowspirit Diamond
	52298: {MinRed: 2},                // Destructive Shadowspirit Diamond
	52299: {MinBlue: 2},               // Powerful Shadowspirit Diamond
	52300: {MinYellow: 1, MinBlue: 1}, // Enigmatic Shadowspirit Diamond
	52301: {MinYellow: 1, MinBlue: 1}, // Impassive Shadowspirit Diamond
	52302: {MinYellow: 1, MinBlue: 1}, // Forlorn Shadowspirit Diamond
	68778: {MinRed: 3},                // Agile Shadowspirit Diamond
	68779: {MinRed: 3},                // Reverberating Shadowspirit Diamond
	68780: {MinRed: 3},                // Burning Shadowspirit Diamond
}

// MetaConditionFor returns the activation rule for a meta gem item ID.
// ok is false for metas the rules table doesn't cover — callers must
// NOT invent a rule then (the old one-of-each fallback silently
// mis-modeled cata metas); they skip activation modeling and surface
// the gap in the UI so the missing entry gets curated.
func (d *Database) MetaConditionFor(gemID int) (MetaCondition, bool) {
	c, ok := d.Rules.MetaConditions[gemID]
	return c, ok
}
