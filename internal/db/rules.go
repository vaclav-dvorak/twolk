package db

// RuleSet bundles the version-scoped classification tables the planner
// consults at QUERY time: raid/PvP/seasonal gating, category
// exclusivity, meta-gem activation. Each game version's loader attaches
// its own set to the Database it builds (Load → WotlkRules, LoadCata →
// CataRules), and shared code reads them only through the Database —
// never through package globals — so nothing version-specific bleeds
// across the game seam. Load-time overlay tables (stat fixes, source
// supplements, price tables) are NOT here: those are private to their
// version's loader by construction and never consulted afterwards.
//
// Tables for content that exists identically in both versions (the
// seasonal bosses, the WotLK unique-ring groups a fresh character can
// still wear) are shared by reference; era-layered tables (raid zones,
// meta conditions, PvP items) give Cataclysm the union of every era in
// its item pool, while WotLK sees nothing from its future.
type RuleSet struct {
	// RaidZones names the version's raid instances. Needed because the
	// upstream data marks some raid drops (WotLK tier-7 Naxx/OS/EoE)
	// with dungeon-style difficulty values 1/2 — the difficulty field
	// alone cannot separate raids from 5-mans there.
	RaidZones map[string]bool

	// UniqueGroups maps items to exclusivity groups — at most one per
	// set, even though the item IDs differ. Two sources: an in-game
	// unique-equipped category ("Unique-Equipped: Ashen Verdict band")
	// and a one-time quest that hands out a choice of rewards
	// (questChoiceGroups), which the item flags cannot express at all.
	UniqueGroups map[int]string

	// MetaConditions maps meta gem item IDs to their activation rules;
	// metas missing here fall back to the one-of-each rule.
	MetaConditions map[int]MetaCondition

	// SeasonalBosses marks holiday-event encounters by NPC ID: their
	// loot only drops during the event window each year.
	SeasonalBosses map[int]string

	// WorldBosses marks outdoor boss encounters by NPC ID — rare
	// multi-day spawns tuned for groups, not farmable like a dungeon.
	WorldBosses map[int]bool

	// PvPItems flags items ONLY obtainable through PvP (honor, arena
	// points, battleground marks) even when they carry no resilience;
	// the name families in pvpNamed backstop gaps in this table.
	PvPItems map[int]bool

	// Raid content in disguise (see Database.RaidGated): tier priced in
	// raid-dropped tokens — by item ID (RaidGatedItems) or by the
	// vendor's NPC ID (RaidTokenVendors) — rewards of quests completed
	// inside raids (RaidQuests), and sourceless raid upgrades matched
	// by name prefix (RaidGatedPrefix).
	RaidGatedItems   map[int]bool
	RaidTokenVendors map[int]bool
	RaidQuests       map[string]bool
	RaidGatedPrefix  string

	// RotatePrice adjusts a vendor price for the server's content phase.
	// The generated price tables are a single-snapshot dump (for cata:
	// TDB 4.3.4, the ORIGINAL game's final state), but Cataclysm Classic
	// rotates point currencies each phase — the current raid tier's
	// vendor gear costs Valor, and it drops to Justice (same amount)
	// when the next tier opens. nil means prices never rotate (WotLK).
	RotatePrice func(p EmblemPrice, ilvl, phase int) EmblemPrice

	// Recipes maps craft spell IDs to how the recipe is learned
	// (trainer, vendor, drop, quest) — generated per version.
	Recipes map[int]RecipeSource

	// EnchantCrafts maps item-based enchants' carrier items (scopes,
	// armor kits) to the craft spell that makes them.
	EnchantCrafts map[int]int

	// Upgrades maps an item bought by trading another item up (cata:
	// heroic Firelands gear = the normal item + a Crystallized
	// Firestone at Lurah Wrathvine) to that base item.
	Upgrades map[int]int
}

// WotlkRules is the 3.3.5 classification table set.
func WotlkRules() RuleSet {
	return RuleSet{
		RaidZones:        preCataRaidZones,
		UniqueGroups:     merged(uniqueRingGroups(), questChoiceGroups()),
		MetaConditions:   preCataMetaConditions,
		SeasonalBosses:   seasonalBosses,
		PvPItems:         pvpCurrencyItems,
		RaidTokenVendors: raidTokenVendorNPCs,
		RaidQuests:       raidQuestNames,
		RaidGatedPrefix:  "Sanctified ",
		Recipes:          recipeSources,
		EnchantCrafts:    enchantCrafts,
	}
}

// CataRules is the Cataclysm-classic classification table set. Its item
// pool reaches back through every earlier era, so the era-layered
// tables take the union; the WotLK raid-token/raid-quest gates are
// absent on purpose (that tier sells for gold in Cataclysm, and it sits
// below the useful-ilvl floor anyway).
func CataRules() RuleSet {
	// Only the pre-cata raids whose loot can still be SUGGESTED at 85
	// carry over: ICC/RS heroic gear (277/284) sits above the module's
	// 272 usefulness floor. Blanket-merging every old raid zone is a
	// name-collision hazard, not a safety net — the 4.x client reuses
	// instance names, and the TBC "Zul'Aman" raid entry once
	// classified the level-85 ZA DUNGEON as a raid (session 23); the
	// other 14 old zones matched nothing in the cata pool at all.
	// TestLoadCata enforces the every-entry-referenced invariant.
	return RuleSet{
		RaidZones: merged(cataRaidZones, map[string]bool{
			"Icecrown Citadel": true,
			"The Ruby Sanctum": true,
		}),
		UniqueGroups:   merged(uniqueRingGroups(), questChoiceGroups(), cataUniqueGroups),
		MetaConditions: merged(preCataMetaConditions, cataMetaConditions),
		SeasonalBosses: seasonalBosses,
		PvPItems:       merged(pvpCurrencyItems, cataPvPItems, cataPvPVendorItems),
		WorldBosses:    cataWorldBosses,
		// a fresh copy: the protocol overlay un-gates the pieces that
		// Classic moved from Raid Finder to protocol dungeons, and that
		// must never mutate the shared generated table
		RaidGatedItems: merged(cataTokenTradeItems, cataRaidQuestItems),
		// ICC "Sanctified" pieces stay sourceless in any era's data
		RaidGatedPrefix: "Sanctified ",
		RotatePrice:     cataRotatePrice,
		Recipes:         cataRecipeSources,
		Upgrades:        cataFirestoneUpgrades,
		EnchantCrafts:   cataEnchantCrafts,
	}
}

// cataRotatePrice maps the TDB 4.3.4 end-state currency assignment back
// to the phase the server is actually in. Vendor gear tiers have fixed
// ilvls (359 = T11 era, 378 = T12 era, 397 = T13 era) and Blizzard
// keeps the amounts when a tier rotates from Valor down to Justice —
// only the currency flips (Classic mirrors the original 4.2/4.3
// rotations). Non-point prices and off-tier ilvls pass through.
func cataRotatePrice(p EmblemPrice, ilvl, phase int) EmblemPrice {
	if p.Emblem != EmblemJustice && p.Emblem != EmblemValorPoints {
		return p
	}
	switch ilvl {
	case 359: // T11-era vendor gear: current tier until Firelands opens
		if phase <= 2 {
			p.Emblem = EmblemValorPoints
		} else {
			p.Emblem = EmblemJustice
		}
	case 378: // T12-era: current tier during Firelands only
		if phase <= 3 {
			p.Emblem = EmblemValorPoints
		} else {
			p.Emblem = EmblemJustice
		}
	case 397: // T13-era: never rotates down within Cataclysm
		p.Emblem = EmblemValorPoints
	}
	return p
}

// merged unions tables into a fresh map — constructors only, once per
// database load.
func merged[K comparable, V any](ms ...map[K]V) map[K]V {
	out := map[K]V{}
	for _, m := range ms {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}

// preCataRaidZones identifies the WotLK and TBC raid instances (TBC
// drops also mark dungeon-style difficulties, and a player without a
// raid group can't farm those either).
var preCataRaidZones = map[string]bool{
	// WotLK
	"Naxxramas":             true,
	"The Obsidian Sanctum":  true,
	"The Eye of Eternity":   true,
	"Vault of Archavon":     true,
	"Ulduar":                true,
	"Trial of the Crusader": true,
	"Onyxia's Lair":         true,
	"Icecrown Citadel":      true,
	"The Ruby Sanctum":      true,
	// TBC
	"Karazhan":             true,
	"Gruul's Lair":         true,
	"Magtheridon's Lair":   true,
	"Serpentshrine Cavern": true,
	"Tempest Keep":         true,
	"Hyjal Summit":         true,
	"Black Temple":         true,
	"Zul'Aman":             true,
	"Sunwell Plateau":      true,
}

// cataRaidZones identifies the Cataclysm raid instances (names only in
// the cata database's zone table).
var cataRaidZones = map[string]bool{
	"Blackwing Descent":        true,
	"The Bastion of Twilight":  true,
	"Throne of the Four Winds": true,
	"Baradin Hold":             true,
	"Firelands":                true,
	"Dragon Soul":              true,
}

// cataRaidQuestItems are rewards of quests completed INSIDE a raid
// (kill Ragnaros / Madness of Deathwing, or a legendary chain) — the
// upstream sources them as anonymous "Quest reward", so without this
// curated gate a raids-off filter would top its weapon suggestions
// with heroic-Deathwing kill rewards. Verified against warcraft.wiki
// quest pages; the OTHER high-ilvl quest gear (End Time / Hour of
// Twilight dungeon quests, Darkmoon Faire trinkets, Elemental Bonds
// cloaks) is genuinely solo/5-man and stays open.
var cataRaidQuestItems = map[int]bool{
	71084: true, // Branch of Nordrassil — Ragnaros (normal)
	71085: true, // Runestaff of Nordrassil — Ragnaros (heroic)
	71086: true, // Dragonwrath, Tarecgosa's Rest — legendary chain
	77945: true, // Fear — Madness of Deathwing (normal)
	77946: true, // Vengeance — Madness of Deathwing (normal)
	77947: true, // The Sleeper — Madness of Deathwing (heroic)
	77948: true, // The Dreamer — Madness of Deathwing (heroic)
	77949: true, // Golad, Twilight of Aspects — legendary chain
	77950: true, // Tiriosh, Nightmare of Ages — legendary chain
}

// cataWorldBosses are Cataclysm's five outdoor rare bosses (added in
// 4.0.3a), each dropping ilvl 359 epics.
var cataWorldBosses = map[int]bool{
	50009: true, // Mobus — Abyssal Depths
	50056: true, // Garr — Mount Hyjal
	50061: true, // Xariona — Deepholm
	50063: true, // Akma'hat — Uldum
	50089: true, // Julak-Doom — Twilight Highlands
}

// questChoiceGroups covers exclusivity the item flags cannot express:
// a one-time quest that hands out a CHOICE of rewards. The Battered
// Hilt chain is the endgame case — it ends in a single reforge pick and
// cannot be repeated, so a character owns exactly one Quel'Delar even
// though the seven rewards are seven item ids, each unique-equipped
// only against itself. Without the group a Titan's Grip solve happily
// proposes two of them. Grouping them here is the same mechanic the
// planner already uses for band families: one per set, and the upgrade
// browser hides the rest once one is held.
func questChoiceGroups() map[int]string {
	groups := map[int]string{}
	for _, id := range []int{
		50046, // Quel'Delar, Cunning of the Shadows
		50047, // Quel'Delar, Lens of the Mind
		50048, // Quel'Delar, Might of the Faithful
		50049, // Quel'Delar, Ferocity of the Scorned
		50050, // Cudgel of Furious Justice
		50051, // Hammer of Purified Flame
		50052, // Lightborn Spire
	} {
		groups[id] = "Quel'Delar reward"
	}
	return groups
}

// uniqueRingGroups builds the curated category-exclusivity groups: the
// Ashen Verdict and Kirin Tor ring families ("Unique-Equipped: ... band"
// in-game). The rings — and their exclusivity — exist unchanged in both
// game versions.
func uniqueRingGroups() map[int]string {
	groups := map[int]string{}
	ashen := []int{50375, 50376, 50377, 50378, 50384, 50386, 50387, 50388,
		50397, 50398, 50399, 50400, 50401, 50402, 50403, 50404,
		52569, 52570, 52571, 52572}
	kirin := []int{40585, 40586, 44934, 44935, 45688, 45689, 45690, 45691,
		48954, 48955, 48956, 48957, 51557, 51558, 51559, 51560}
	for _, id := range ashen {
		groups[id] = "Ashen Verdict band"
	}
	for _, id := range kirin {
		groups[id] = "Kirin Tor band"
	}
	return groups
}
