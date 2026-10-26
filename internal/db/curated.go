package db

import "strings"

// curatedEffects hand-approximates items whose WoWSims implementation is
// custom sim code we cannot mechanically extract (26 items upstream).
// Unlike extracted effects, Bonus here holds uptime-weighted *average*
// stats, ready to add to an item's passive stats during scoring.
//
// Add entries as specs start needing them; missing items simply score by
// their passive stats. Assumptions are documented per entry — keep them
// honest, this is estimation, not simulation.
var curatedEffects = []*Effect{
	// Deathbringer's Will (ICC, Saurfang): procs one of three 30s buffs,
	// 35% on hit, 105s ICD — verified against the WoWSims implementation
	// (sim/common/wotlk/other_effects.go at the pinned commit): buff
	// magnitude 600 normal / 700 heroic (AP buff 2×); the trio is
	// class-dependent and for hunters is Agility / Crit / AP (NOT haste).
	// Uptime ≈ 30/(105+5) ≈ 0.27, each buff 1/3 of procs → ×0.09.
	// Values assume a hunter; crit is credited on both rating sides like
	// item stats.
	{ItemID: 50362, Name: "Deathbringer's Will", Kind: EffectCurated,
		Bonus: Stats{Agility: 54, MeleeCrit: 54, SpellCrit: 54, AttackPower: 108, RangedAttackPower: 108}},
	{ItemID: 50363, Name: "Deathbringer's Will (H)", Kind: EffectCurated,
		Bonus: Stats{Agility: 63, MeleeCrit: 63, SpellCrit: 63, AttackPower: 126, RangedAttackPower: 126}},

	// Darkmoon Card: Greatness: +300 of your highest base stat for 15s,
	// 35% chance on damage/heal dealt, 45s ICD (verified against the
	// WoWSims implementation, sim/common/wotlk/highest_stat_effects.go;
	// proc aura = spell 60229/60233/60234/60235 per stat). Uptime
	// ≈ 15/(45+5) = 0.30 → +90 average. Each card version is credited
	// in the stat its passive +90 boosts.
	{ItemID: 44253, Name: "Darkmoon Card: Greatness", Kind: EffectCurated, Bonus: Stats{Agility: 90}},
	{ItemID: 42987, Name: "Darkmoon Card: Greatness", Kind: EffectCurated, Bonus: Stats{Strength: 90}},
	{ItemID: 44255, Name: "Darkmoon Card: Greatness", Kind: EffectCurated, Bonus: Stats{Intellect: 90}},
	{ItemID: 44254, Name: "Darkmoon Card: Greatness", Kind: EffectCurated, Bonus: Stats{Spirit: 90}},

	// Dislodged Foreign Object (ICC): damaging spells have a 10% chance
	// (45s ICD) to grant spell power ramping to 1088/1230 over 10s (avg
	// half while active). Uptime ≈ 10/(45+15) ≈ 0.17 → ×0.5 ramp → ~90/102.
	{ItemID: 50353, Name: "Dislodged Foreign Object", Kind: EffectCurated, Bonus: Stats{SpellPower: 90}},
	{ItemID: 50348, Name: "Dislodged Foreign Object (H)", Kind: EffectCurated, Bonus: Stats{SpellPower: 102}},

	// TODO as needed: Corpse Tongue Coin (50352/50349), The General's
	// Heart (45507), Essence of Gossamer (37220), Bauble of True Blood,
	// Althor's Abacus, healer/tank absorbs — add when a spec cares.
}

// cataCuratedEffects is the Cataclysm curated table (see curatedEffects
// for the ground rules). Kept separate because the two databases carry
// overlapping legacy items and each version's estimates are calibrated
// against its own sim presets.
var cataCuratedEffects = []*Effect{
	// Darkmoon Card: Hurricane — a DAMAGE proc, so the upstream db.json
	// carries no itemEffect for it at all (the implementation is custom
	// sim code, sim/common/cata/damage_procs.go @ the pinned commit):
	// 5250-8750 nature (avg 7000) on a landed melee-or-ranged attack,
	// PPM 1, no ICD. It cannot crit and cannot be avoided — the outcome
	// resolves to OutcomeMeleeSpecialNoBlockDodgeParryNoCrit — and the
	// NoSpellMods/IgnoreModifiers flags keep raid buffs and talents off
	// it, so the damage is genuinely flat.
	//
	// PPM here is per LANDED ATTACK at weaponSpeed*PPM/60, and ranged
	// specials roll it without consuming swing time, so the rate is far
	// above 1/min: measured 3.60 procs/min for an SV hunter on the
	// wowsims p1_sv preset (headless RunRaidSimConcurrent, 50k iters,
	// seed 101, 300s single target). Two independent readings agree —
	// the proc spell's own damage metrics give 420.3 DPS, and the DPS
	// delta against a flat +321 agility control gives 421.0.
	//
	// HUNTER-CALIBRATED: the rate follows ranged weapon speed and the
	// spec's attacks per second. 62049 (the Strength card) is left out
	// deliberately — no melee spec is scored yet, and its rate would
	// have to be measured, not assumed.
	{ItemID: 62051, Name: "Darkmoon Card: Hurricane", Kind: EffectCurated,
		BonusDPS: 420},
}

// overlayCurated adds a curated table's estimates for items that have no
// effect data at all, so improving the extractor never gets shadowed.
func (d *Database) overlayCurated(table []*Effect) {
	if d.Effects == nil {
		d.Effects = map[int][]*Effect{}
	}
	for _, e := range table {
		if len(d.Effects[e.ItemID]) == 0 {
			d.Effects[e.ItemID] = []*Effect{e}
		}
	}
}

// applyCataCurated overlays the Cataclysm curated table. The cata loader
// calls this instead of applyCurated: the WotLK table's enchant and zone
// overlays are that version's, and only the effect estimates carry over
// in spirit.
func (d *Database) applyCataCurated() {
	d.overlayCurated(cataCuratedEffects)
}

// worldZones supplements the upstream zones table (raids/dungeons only)
// with the world zones vendors live in.
var worldZones = map[int]string{
	65:   "Dragonblight",
	210:  "Icecrown",
	394:  "Grizzly Hills",
	3711: "Sholazar Basin",
	4080: "Isle of Quel'Danas",
	4197: "Wintergrasp",
	4395: "Dalaran",
}

// objectZones anchors raid loot containers that the supplemental source
// table recorded without a zone: ZoneID 0 breaks both the zone display
// and — critically — raid classification (IsRaidZone(0) is false, so
// e.g. Malygos loot from Alexstrasza's Gift leaked past the raids-off
// filter as "dungeon" loot).
var objectZones = map[string]int{
	"Alexstrasza's Gift":   4500, // The Eye of Eternity (Malygos)
	"Cache of Innovation":  4273, // Ulduar (Mimiron)
	"Rare Cache of Winter": 4273, // Ulduar (Hodir)
}

// npcZones anchors zone-less NPC drops from the supplemental table the
// same way: wotlkdb has no location for these NPC entries, and ZoneID 0
// would both blank the zone display and leak raid loot past the
// raids-off filter (IsRaidZone(0) is false → "dungeon" route).
var npcZones = map[int]int{
	10184: 2159, // Onyxia (level-60 NPC entry) — Onyxia's Lair
	36538: 2159, // Onyxia (3.2.2 level-80 entry) — Onyxia's Lair
	38549: 4812, // Rotface (heroic entry) — Icecrown Citadel
	37563: 4812, // Spectral Warden (ICC trash) — Icecrown Citadel
	33288: 4273, // Yogg-Saron — Ulduar
	31537: 4272, // Ionar (heroic entry) — Halls of Lightning
	31350: 4196, // Novos the Summoner (heroic entry) — Drak'Tharon Keep
}

// questNameFixes fills quest sources the upstream data left nameless, for
// endgame-relevant items (leveling greens keep the generic fallback).
// The Quel'Delar chain ends in a faction-mirrored turn-in quest; the
// Ashen Verdict "quests" are the ICC reputation ring handins.
var questNameFixes = map[int]string{
	// Quel'Delar restoration (The Battered Hilt chain)
	50046: "A Victory for the Sunreavers / the Silver Covenant (Quel'Delar)",
	50047: "A Victory for the Sunreavers / the Silver Covenant (Quel'Delar)",
	50048: "A Victory for the Sunreavers / the Silver Covenant (Quel'Delar)",
	50049: "A Victory for the Sunreavers / the Silver Covenant (Quel'Delar)",
	50050: "A Victory for the Sunreavers / the Silver Covenant (Quel'Delar)",
	50051: "A Victory for the Sunreavers / the Silver Covenant (Quel'Delar)",
	50052: "A Victory for the Sunreavers / the Silver Covenant (Quel'Delar)",
	// Ashen Verdict rings, by reputation tier
	50375: "Ashen Verdict ring (Friendly)", 50376: "Ashen Verdict ring (Friendly)",
	50377: "Ashen Verdict ring (Friendly)", 50378: "Ashen Verdict ring (Friendly)",
	52569: "Ashen Verdict ring (Friendly)",
	50384: "Ashen Verdict ring (Honored)", 50386: "Ashen Verdict ring (Honored)",
	50387: "Ashen Verdict ring (Honored)", 50388: "Ashen Verdict ring (Honored)",
	52570: "Ashen Verdict ring (Honored)",
	50397: "Ashen Verdict ring (Revered)", 50399: "Ashen Verdict ring (Revered)",
	50401: "Ashen Verdict ring (Revered)", 50403: "Ashen Verdict ring (Revered)",
	52571: "Ashen Verdict ring (Revered)",
	50398: "Ashen Verdict ring (Exalted)", 50400: "Ashen Verdict ring (Exalted)",
	50402: "Ashen Verdict ring (Exalted)", 50404: "Ashen Verdict ring (Exalted)",
	52572: "Ashen Verdict ring (Exalted)",
}

// raidTokenVendorNPCs are the Argent Tournament tier vendors whose
// entire stock is priced in raid-dropped tokens (Trophy of the Crusade
// for ilvl 245, ToGC Regalia tokens for 258) — "vendor" sources that a
// player without a raid group cannot actually use.
var raidTokenVendorNPCs = map[int]bool{
	35575: true, // Champion Isimode (258)
	35576: true, // Champion Faesrol (258)
	35577: true, // Valiant Laradia (245)
	35578: true, // Valiant Bressia (245)
}

// raidQuestNames are quests completed inside raids (boss-kill turn-ins
// and legendary chains) whose rewards therefore need a raid group.
// Curated from the full list of epic quest sources in the data; the
// Darkmoon decks are deliberately absent (buyable on the AH).
var raidQuestNames = map[string]bool{
	"All Is Well That Ends Well":             true, // Yogg-Saron 10
	"Heroic: All Is Well That Ends Well":     true, // Yogg-Saron 25
	"Judgment at the Eye of Eternity":        true, // Malygos 10
	"Heroic Judgment at the Eye of Eternity": true, // Malygos 25
	"Val'anyr, Hammer of Ancient Kings":      true, // legendary (Ulduar)
	"Shadow's Edge":                          true, // legendary chain (ICC)
	"Shadowmourne...":                        true, // the legendary itself (ICC; quest really is named that)
	"A Most Puzzling Circumstance":           true, // Head of Onyxia turn-in (jewelry)
}

// WorldDrop is the DropSource.OtherName marking zone-less open-world
// drops (BoE trash loot spread across many mobs): not dungeon loot —
// the "quests & other" acquisition route.
const WorldDrop = "world drop"

// RaidGated reports whether obtaining the item requires raiding despite
// not being a raid drop itself: tier bought with raid tokens (WotLK ToC
// Trophy/Regalia vendors, cata token trades), ICC "Sanctified" upgrades
// (Marks of Sanctification, sourceless in the upstream data), and
// raid-quest rewards (boss-kill turn-ins, legendaries).
// UpgradeBase is the item traded in (with a Crystallized Firestone)
// for an upgrade-bought item; nil for everything else.
func (d *Database) UpgradeBase(itemID int) *Item {
	if b, ok := d.Rules.Upgrades[itemID]; ok {
		return d.Items[b]
	}
	return nil
}

func (d *Database) RaidGated(it *Item) bool {
	if d.Rules.RaidGatedPrefix != "" && strings.HasPrefix(it.Name, d.Rules.RaidGatedPrefix) {
		return true
	}
	if d.Rules.RaidGatedItems[it.ID] {
		return true
	}
	for _, s := range it.Sources {
		if s.SoldBy != nil && d.Rules.RaidTokenVendors[s.SoldBy.NPCID] {
			return true
		}
		if s.Quest != nil && d.Rules.RaidQuests[s.Quest.Name] {
			return true
		}
	}
	return false
}

// seasonalBosses marks holiday-event encounters: their loot only drops
// during the event window each year. Suggestions stay visible (the
// gear is real, and verified un-retuned vs original 3.3.5 — see
// sessions.md) but the source line labels the limited availability.
var seasonalBosses = map[int]string{
	25740: "Midsummer Fire Festival", // (Frost Lord) Ahune — The Slave Pens
	23682: "Hallow's End",            // Headless Horseman — Scarlet Monastery
	23872: "Brewfest",                // Coren Direbrew — Blackrock Depths
	36296: "Love is in the Air",      // Apothecary Hummel — Shadowfang Keep
}

// SeasonalEvent returns the holiday during which the boss (and so its
// loot) is available, or "" for year-round sources.
func (d *Database) SeasonalEvent(npcID int) string { return d.Rules.SeasonalBosses[npcID] }

// WorldBoss reports whether the NPC is an outdoor world boss.
func (d *Database) WorldBoss(npcID int) bool { return d.Rules.WorldBosses[npcID] }

// pvpNamed matches item families that are PvP-acquired in every season
// and tier regardless of resilience: arena "Gladiator's" gear and the
// honor-bought Battlemaster's trinkets. It backstops the generated
// table for items missing from AzerothCore's vendor data (e.g. the
// old-season Gladiator relic-slot Idols/Librams/Totems/Sigils have no
// npc_vendor rows at all). Audited against the full item pool: every
// match is PvP, no false positives.
func pvpNamed(name string) bool {
	return strings.Contains(name, "Gladiator's") ||
		strings.HasPrefix(name, "Battlemaster's")
}

// PvPItem reports whether the item is ONLY obtainable through PvP
// (honor, arena points, battleground marks) even when it carries no
// resilience: the version's generated vendor-cost tables (complete —
// items with a gold or emblem route are excluded there) plus the name
// families backstopping gaps in the vendor data.
func (d *Database) PvPItem(it *Item) bool {
	return d.Rules.PvPItems[it.ID] || pvpNamed(it.Name)
}

// Reagent is one crafting material of a recipe (see craftcosts_gen.go).
type Reagent struct {
	ID    int
	Name  string
	Count int
}

// CraftCost returns a crafted item's reagent list, or nil when the
// recipe is unknown (display then falls back to the profession line).
func CraftCost(itemID int) []Reagent {
	return craftCosts[itemID]
}

// UniqueGroup returns the item's category-exclusivity group, or "".
func (d *Database) UniqueGroup(itemID int) string {
	return d.Rules.UniqueGroups[itemID]
}

// statFix is one original-3.3.5 correction from the generated table
// (statfix_gen.go): the true item level, full stat array, and weapon
// damage (zero for non-weapons).
type statFix struct {
	ILvl                  int
	Stats                 Stats
	DmgMin, DmgMax, Speed float64
}

// enchantAverages values the proc/on-use profession enchants that the
// upstream data carries with EMPTY stats (their effects live in WoWSims
// sim code, not the enchant table). Without these the planner valued
// them at 0 — and actively advised replacing a worn Hyperspeed with a
// flat stat enchant. Uptime-weighted averages in the curatedEffects
// tradition: assumptions documented per entry, using the score
// package's proc conventions (expected wait ≈ 2s trigger cadence /
// proc chance; uptime = duration / (ICD + wait)). AP and haste entries
// fill both unified-rating sides, matching how items carry them.
// Damage procs (Hand-Mounted Pyro Rocket 3603, Frag Belt 3601) stay at
// zero — the same untracked stance as the capacitor trinkets.
// Keyed by enchantment effect ID.
//
// Weapon PPM procs: chance per landed hit = PPM × weapon speed / 60, on
// white AND yellow hits with that hand, and a proc refreshes rather than
// stacks, so uptime = 1 − e^(−procs/min × duration/60). Assumed cadence:
// a melee DPS weapon lands ~1.8 procs/min at 1 PPM, averaged over hands
// (a 3.6-speed main hand swinging white + abilities ~2.4, an off hand
// ~1.3). Berserking and Mongoose keep SEPARATE main/off-hand auras in
// the sim, so two copies stack and the per-weapon average is additive.
// Trigger gates them to specs that actually swing: a hunter's stat-stick
// never procs Berserking, a warrior never procs Black Magic.
var enchantAverages = map[int]enchantAverage{
	// Hyperspeed Accelerators: +340 haste for 12s, 60s cooldown (on-use)
	// → 340 × 12/60 = 68.
	3604: {Stats: Stats{MeleeHaste: 68, SpellHaste: 68}},
	// Lightweave Embroidery: +295 SP for 15s, 35% on harmful spell,
	// 45s ICD → wait ≈ 5.7s, uptime ≈ 15/50.7 ≈ 0.30 → ≈ 87.
	3722: {Stats: Stats{SpellPower: 87}},
	// Darkglow Embroidery: 400 mana restored, 35% on cast, 45s ICD
	// → 400 mana / ~50.7s ≈ 7.9 mana/s → ≈ 39 MP5.
	3728: {Stats: Stats{MP5: 39}},
	// Swordguard Embroidery: +400 AP for 15s, 20% on hit, 55s ICD
	// → wait ≈ 10s, uptime ≈ 15/65 ≈ 0.23 → ≈ 92.
	3730: {Stats: Stats{AttackPower: 92, RangedAttackPower: 92}},

	// Berserking: +400 AP for 15s, 1 PPM per hand (sim/common/wotlk/
	// enchant_effects.go) → 1.8/min, uptime ≈ 0.36 → ≈ 145 AP. Melee AP
	// only: it cannot proc from ranged attacks. (Its −5% armor is the
	// wearer's own and costs a DPS nothing.)
	3789: {Stats: Stats{AttackPower: 145}, Trigger: "melee"},
	// Mongoose: +120 Agi and +30 haste rating for 15s, 0.73 PPM per hand
	// (sim/common/tbc/enchant_effects.go) → 1.3/min, uptime ≈ 0.28.
	2673: {Stats: Stats{Agility: 34, MeleeHaste: 8}, Trigger: "melee"},
	// Executioner: +120 ArP for 15s, 1 PPM → uptime ≈ 0.36 → ≈ 43.
	3225: {Stats: Stats{ArmorPenetration: 43}, Trigger: "melee"},
	// Rune of the Fallen Crusader: +15% Strength for 15s, 2 PPM, ONE aura
	// shared by both hands (sim/deathknight/items.go) → ~3.6 procs/min,
	// uptime ≈ 0.6; 15% of a raid-buffed ~1700 Str × 0.6 ≈ 150. Without
	// it the DK runeforges scored zero and lost to Berserking.
	3368: {Stats: Stats{Strength: 150}, Trigger: "melee"},
	// Black Magic: +250 haste for 10s, 35% on damaging spell, 35s ICD →
	// wait ≈ 5.7s, uptime ≈ 10/40.7 ≈ 0.25 → ≈ 61. (The sim also lets
	// Rend/Deep Wounds proc it; for a warrior that is ~70 melee haste,
	// still below Berserking, so the spell gate does not change advice.)
	3790: {Stats: Stats{MeleeHaste: 61, SpellHaste: 61}, Trigger: "spell"},
}

// enchantAverage is one curated proc enchant: its uptime-averaged
// stats and the action that triggers it (see Enchant.Trigger).
type enchantAverage struct {
	Stats   Stats
	Trigger string
}

// applyCurated overlays the curated tables onto d. Extracted effects win:
// a curated entry is only added for items with no effect data at all, so
// improving the extractor never gets shadowed by an estimate.
func (d *Database) applyCurated() {
	d.overlayCurated(curatedEffects)
	// proc/on-use profession enchants: curated averages onto empty
	// stats only — if the upstream data ever grows real numbers for
	// them, those win.
	for _, e := range d.Enchants {
		if fix, ok := enchantAverages[e.EffectID]; ok && e.Stats == (Stats{}) {
			e.Stats, e.Trigger = fix.Stats, fix.Trigger
		}
	}
	if d.Zones == nil {
		d.Zones = map[int]string{}
	}
	for id, name := range worldZones {
		if d.Zones[id] == "" {
			d.Zones[id] = name
		}
	}
	// Items the upstream data cannot source get their true 3.3.5 source
	// from the generated wotlkdb table (sources_gen.go): stripped-orphan
	// Classic redistributions plus upstream-sourceless quest rewards,
	// raid trash drops and world drops.
	d.addSupplementalSources(supplementalSources)

	// Items neither the upstream data nor wotlkdb can source: data-only
	// leftovers and WotLK-Classic-era additions (211xxx Hallow's End
	// gear). Nothing on a 3.3.5 server can drop them — out of the pool.
	for _, id := range unobtainableItems {
		delete(d.Items, id)
	}

	// Restore original 3.3.5 ilvl+stats where the WoWSims (Classic) data
	// is re-tuned (mainly Classic's Ulduar loot buff).
	for id, f := range statFixes {
		if it := d.Items[id]; it != nil {
			it.ILvl = f.ILvl
			it.Stats = f.Stats
			if f.Speed > 0 {
				it.WeaponDmgMin = f.DmgMin
				it.WeaponDmgMax = f.DmgMax
				it.WeaponSpeed = f.Speed
			}
		}
	}

	// Anchor zoneless raid loot containers and NPC drops to their zones.
	for _, it := range d.Items {
		for _, s := range it.Sources {
			if s.Drop != nil && s.Drop.ZoneID == 0 {
				if z, ok := objectZones[s.Drop.OtherName]; ok {
					s.Drop.ZoneID = z
				}
				if z, ok := npcZones[s.Drop.NPCID]; ok {
					s.Drop.ZoneID = z
				}
			}
		}
	}

	// Fill nameless quest sources for known items.
	for id, name := range questNameFixes {
		it := d.Items[id]
		if it == nil {
			continue
		}
		for _, s := range it.Sources {
			if s.Quest != nil && s.Quest.Name == "" {
				s.Quest.Name = name
			}
		}
	}

	// Emblem-buyable items (per the generated 3.3.5 price table) that the
	// upstream data gives no vendor source get one, so acquisition
	// filtering and source display treat them as vendor gear.
	const dalaran = 4395
	for id := range emblemPrices {
		it := d.Items[id]
		if it == nil {
			continue
		}
		hasVendor := false
		for _, s := range it.Sources {
			if s.SoldBy != nil {
				hasVendor = true
				break
			}
		}
		if !hasVendor {
			it.Sources = append(it.Sources, Source{SoldBy: &SoldBySource{ZoneID: dalaran}})
		}
	}
}
