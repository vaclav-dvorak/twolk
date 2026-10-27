package score

// Tier-set 2pc/4pc bonuses for EVERY class, valued as flat score (EP
// in the wearing spec's sim-native units — only the set's own class can
// equip it, so per-set values are safe). MECHANICS are sourced from the
// WoWSims implementations at the pinned commit (sim/<class>/*items.go);
// where the bonus lives in an ability file upstream, the note is a
// short summary and the EP a role-lore estimate — refine those entries
// when their spec starts mattering. Bonuses WoWSims marks TODO or
// leaves empty are valued 0, never guessed. EP anchors: ~30 EP ≈ 1% for
// SP=1 sims (casters/healers), ~65 for AP=1 melee, ~80 for the hunter.
// TBC sets (all below ilvl 165) are omitted entirely.
//
// Faction-mirrored T9 sets share bonuses under two names (WoWSims
// AlternativeName); both keys are listed. WotLK feral sets cover cat
// AND bear in one tooltip — valued as cat, matching the registry's
// Feral Combat = cat DPS reading.

// SetBonus is one piece-count threshold of a tier set.
type SetBonus struct {
	Pieces int
	EP     float64
	Note   string // short bonus description for the UI
	// Role limits EP to one audience (Profile.SetBonusAudience): a role
	// ("cat") for sets shared by two roles of one tree (cata feral
	// Battlegarb), valued in that role's sim units only — others see the
	// note with EP 0; or a tree slug ("arcane") for sets shared by two
	// trees, each listing its own row per piece count (cata mage sets)
	Role string
}

var setBonuses = map[string][]SetBonus{
	// ================= Hunter (RAP=1 units, ~80 EP/1%) =================
	// T7: 2pc pet damage ×1.05 (pet ≈ 15% of DPS → ~0.75%). 4pc grants
	// +20% ranged haste ONLY while Aspect of the Viper is active
	// (wotlkdb itemset -180) — Viper halves damage, nobody DPSes in it,
	// so the correct DPS value IS zero (it only softens mana downtime).
	"Cryptstalker Battlegear": {
		{2, 60, "pet damage +5%", ""},
		{4, 0, "+20% haste in Aspect of the Viper only", ""},
	},
	// T8: 2pc Serpent Sting +10% damage (SS ≈ 7% of DPS). 4pc 600 AP/RAP
	// 15s, 10% on Steady, 45s ICD → uptime ≈ 0.23 → ~138 avg RAP.
	"Scourgestalker Battlegear": {
		{2, 55, "Serpent Sting +10% damage", ""},
		{4, 140, "600 AP proc on Steady (est. 23% uptime)", ""},
	},
	// T9: 2pc Serpent Sting can crit (~7% of DPS, ~40% crit at 2.09× meta
	// → ≈ +2.2% DPS). 4pc pet 600 AP proc 35% on ranged, 45s ICD.
	"Windrunner's Battlegear": {
		{2, 180, "Serpent Sting can crit", ""},
		{4, 45, "pet 600 AP proc", ""},
	},
	"Windrunner's Pursuit": {
		{2, 180, "Serpent Sting can crit", ""},
		{4, 45, "pet 600 AP proc", ""},
	},
	// T10: 2pc 5% on Auto Shot → +15% ALL damage 10s, no ICD (~25%
	// uptime ≈ +3.7% DPS). 4pc 5% on SS tick → +10% AP 10s (~14%
	// uptime): ~850 RAID-BUFFED AP × 0.14 ≈ 120 direct, and the 2pc's
	// +15% windows multiply it — consensus ≈ +2% DPS. The old 70 priced
	// it at UNBUFFED AP and made the BiS solver stop at 2pc, preferring
	// 277 off-pieces, while the sim-tuned wowsims P4/P5 presets run 4pc
	// at full 277 parity (session 21 margin probe: 4pc needed >118 MM /
	// >169 SV to win).
	"Ahn'Kahar Blood Hunter's Battlegear": {
		{2, 300, "+15% damage proc from Auto Shot", ""},
		{4, 160, "+10% AP proc from Serpent Sting", ""},
	},

	// ============== Warrior DPS (AP=1 units, ~65 EP/1%) ================
	// Mechanics sourced from sim/warrior/items.go.
	"Dreadnaught Battlegear": {
		{2, 60, "Slam +10% damage", ""},
		{4, 20, "bleed procs save 5 rage", ""},
	},
	"Siegebreaker Battlegear": {
		{2, 65, "+150 haste proc from HS/Slam crits", ""},
		{4, 160, "Bloodthirst/Mortal Strike +10% crit", ""},
	},
	// T9 2pc Berserker Stance +2% crit ≈ 92 crit-rating equivalent.
	"Wrynn's Battlegear": {
		{2, 190, "Berserker Stance +2% crit", ""},
		{4, 150, "Slam and Heroic Strike +5% crit", ""},
	},
	"Hellscream's Battlegear": {
		{2, 190, "Berserker Stance +2% crit", ""},
		{4, 150, "Slam and Heroic Strike +5% crit", ""},
	},
	// T10 2pc 3% on Deep Wounds tick → +16% AP 10s (~30% uptime).
	"Ymirjar Lord's Battlegear": {
		{2, 300, "+16% AP proc from Deep Wounds", ""},
		{4, 130, "Bloodsurge/Sudden Death double procs", ""},
	},

	// ========== Warrior protection (mixed tank units, modest) ==========
	// Tank bonuses are mostly defensive CD tweaks our score can't model
	// per-fight; valued as small nudges so tier pieces tie-break right.
	"Dreadnaught Plate":    {{2, 25, "Shield Slam +10% damage", ""}, {4, 30, "Shield Block +10% value", ""}},
	"Siegebreaker Plate":   {{2, 30, "Devastate +5% crit", ""}, {4, 40, "Shield Block grants damage reflect", ""}},
	"Wrynn's Plate":        {{2, 30, "Devastate/Revenge +5% damage", ""}, {4, 40, "Shield Wall -20s cooldown", ""}},
	"Hellscream's Plate":   {{2, 30, "Devastate/Revenge +5% damage", ""}, {4, 40, "Shield Wall -20s cooldown", ""}},
	"Ymirjar Lord's Plate": {{2, 50, "Shield Slam/Shockwave +20% crit", ""}, {4, 60, "Bloodrage grants 20% damage reduction", ""}},

	// ============ Death Knight DPS (AP=1 units, ~65 EP/1%) =============
	// Mechanics sourced from sim/deathknight/items.go.
	"Scourgeborne Battlegear": {
		{2, 45, "Obliterate/Scourge/Death Strike +5% crit", ""},
		{4, 35, "strikes generate +5 runic power", ""},
	},
	"Darkruned Battlegear": {
		{2, 50, "Death Coil/Frost Strike +8% crit", ""},
		{4, 130, "+20% per-disease damage on strikes", ""},
	},
	"Thassarian's Battlegear": {
		{2, 60, "180 strength proc from Blood/Heart Strike", ""},
		{4, 65, "Blood Plague can crit", ""},
	},
	"Koltira's Battlegear": {
		{2, 60, "180 strength proc from Blood/Heart Strike", ""},
		{4, 65, "Blood Plague can crit", ""},
	},
	"Scourgelord's Battlegear": {
		{2, 160, "Obliterate/Scourge Strike +10%, Heart Strike +7%", ""},
		{4, 100, "+3% damage while all runes on cooldown", ""},
	},

	// ============== Death Knight tank (Stamina=1 units) ================
	"Scourgeborne Plate":  {{2, 15, "Plague Strike +10% crit", ""}, {4, 30, "Icebound Fortitude +3s", ""}},
	"Darkruned Plate":     {{2, 25, "Rune Strike +10% damage", ""}, {4, 40, "Anti-Magic Shell -10% physical damage", ""}},
	"Thassarian's Plate":  {{2, 20, "Blood/Heart Strike +5%, Dark Command -2s", ""}, {4, 50, "defensive cooldowns -10s", ""}},
	"Koltira's Plate":     {{2, 20, "Blood/Heart Strike +5%, Dark Command -2s", ""}, {4, 50, "defensive cooldowns -10s", ""}},
	"Scourgelord's Plate": {{2, 25, "Death and Decay +20% damage", ""}, {4, 50, "Blood Tap grants 12% damage reduction", ""}},

	// =============== Rogue (AP=1 units, ~65 EP/1%) =====================
	// Mechanics sourced from sim/rogue/items.go.
	"Bonescythe Battlegear": {
		{2, 40, "Rupture +10% damage", ""},
		{4, 60, "combo builders -5% energy", ""},
	},
	"Terrorblade Battlegear": {
		{2, 70, "Deadly Poison ticks grant 1 energy", ""},
		{4, 80, "Rupture +20% damage", ""},
	},
	"VanCleef's Battlegear": {
		{2, 80, "Rupture procs 40-energy discount", ""},
		{4, 90, "builders +5% crit", ""},
	},
	"Garona's Battlegear": {
		{2, 80, "Rupture procs 40-energy discount", ""},
		{4, 90, "builders +5% crit", ""},
	},
	"Shadowblade's Battlegear": {
		{2, 90, "Tricks of the Trade grants 15 energy", ""},
		{4, 120, "finishers 13% chance for +3 combo points", ""},
	},

	// ========= Paladin retribution (AP=1 units, ~65 EP/1%) =============
	// Bonus effects live in ability files upstream; EP from role lore —
	// refine notes when a ret alt appears.
	"Redemption Battlegear": {{2, 45, "Divine Storm bonus", ""}, {4, 60, "Judgement bonus", ""}},
	"Aegis Battlegear":      {{2, 40, "Exorcism/Hammer of Wrath bonus", ""}, {4, 80, "Divine Storm/Crusader Strike bonus", ""}},
	"Turalyon's Battlegear": {{2, 60, "Righteous Vengeance bonus", ""}, {4, 80, "seal damage bonus", ""}},
	"Liadrin's Battlegear":  {{2, 60, "Righteous Vengeance bonus", ""}, {4, 80, "seal damage bonus", ""}},
	"Lightsworn Battlegear": {{2, 90, "tier bonus", ""}, {4, 110, "seal damage bonus", ""}},

	// ====== Paladin holy (SP=1 units, HPS-anchored; bonus text from
	// wowhead wotlk tooltips — wowsims has no holy tier bonuses) ======
	"Redemption Regalia": {{2, 15, "Holy Shock +10% crit", ""}, {4, 30, "Holy Light -5% cost", ""}},
	"Aegis Regalia":      {{2, 25, "Holy Shock crits add 15% HoT", ""}, {4, 50, "Sacred Shield triggers every 4s", ""}},
	"Turalyon's Garb":    {{2, 20, "Judgements +10s duration", ""}, {4, 55, "Flash of Light HoT +100% with Sacred Shield", ""}},
	"Liadrin's Garb":     {{2, 20, "Judgements +10s duration", ""}, {4, 55, "Flash of Light HoT +100% with Sacred Shield", ""}},
	// T10 2pc: +35% healing during Divine Illumination (15s/3min ≈ 8%
	// uptime ≈ +2.9% healing) — the famous holy 2pc.
	"Lightsworn Garb": {{2, 85, "+35% healing during Divine Illumination", ""}, {4, 40, "Holy Shock hastens next Holy Light", ""}},

	// ============== Paladin protection (modest tank nudges) ============
	"Redemption Plate": {{2, 25, "Hammer of the Righteous bonus", ""}, {4, 30, "Divine Shield/Protection bonus", ""}},
	"Aegis Plate":      {{2, 30, "Seal of Vengeance bonus", ""}, {4, 40, "Shield of Righteousness bonus", ""}},
	"Turalyon's Plate": {{2, 30, "Hammer of the Righteous bonus", ""}, {4, 40, "Divine Protection bonus", ""}},
	"Liadrin's Plate":  {{2, 30, "Hammer of the Righteous bonus", ""}, {4, 40, "Divine Protection bonus", ""}},
	// T10 4pc (wotlkdb itemset -412): Divine Plea on CD → 12% dodge for
	// 10s/60s ≈ +2% avg dodge ≈ 79 dodge rating × 0.46 weight ≈ 36 EP —
	// the famous prot-paladin bonus worth keeping over higher ilvl.
	"Lightsworn Plate": {{2, 30, "Hammer of the Righteous bonus", ""}, {4, 36, "Divine Plea grants 12% dodge", ""}},

	// ============ Priest shadow (SP=1 units, ~30 EP/1%) ================
	"Garb of Faith":             {{2, 20, "Mind Blast cost bonus", ""}, {4, 40, "Shadow Word: Death bonus", ""}},
	"Sanctification Garb":       {{2, 45, "Devouring Plague bonus", ""}, {4, 55, "240 haste proc", ""}},
	"Zabra's Regalia":           {{2, 50, "Vampiric Touch bonus", ""}, {4, 60, "Mind Flay bonus", ""}},
	"Velen's Regalia":           {{2, 50, "Vampiric Touch bonus", ""}, {4, 60, "Mind Flay bonus", ""}},
	"Crimson Acolyte's Regalia": {{2, 75, "dot crit bonus", ""}, {4, 90, "Mind Flay/SW:P bonus", ""}},

	// ============ Priest healing (SP=1 units, HPS-anchored) ============
	"Regalia of Faith":          {{2, 20, "Prayer of Mending bonus", ""}, {4, 40, "Greater Heal bonus", ""}},
	"Sanctification Regalia":    {{2, 30, "tier bonus", ""}, {4, 60, "250 spell power proc", ""}},
	"Zabra's Raiment":           {{2, 40, "Prayer of Mending bonus", ""}, {4, 55, "Renew bonus", ""}},
	"Velen's Raiment":           {{2, 40, "Prayer of Mending bonus", ""}, {4, 55, "Renew bonus", ""}},
	"Crimson Acolyte's Raiment": {{2, 55, "tier bonus", ""}, {4, 75, "PW:Shield/Circle of Healing bonus", ""}},

	// ================ Mage (SP=1 units, ~30 EP/1%) =====================
	// T7 4pc: +5% crit BONUS damage (wotlkdb itemset -198). Talented
	// crit multiplier ≈ 2.0× → crits deal +2.5%; at ~40% damage-weighted
	// crit ≈ +1% DPS.
	"Frostfire Garb": {{2, 15, "mana gem bonus", ""}, {4, 30, "+5% crit bonus damage", ""}},
	// T8 2pc: 350 SP proc 15s/45s ICD → ~25% uptime ≈ 90 avg SP (sourced).
	"Kirin Tor Garb":       {{2, 90, "350 spell power proc", ""}, {4, 40, "instant-cast proc", ""}},
	"Kirin'dor Garb":       {{2, 90, "350 spell power proc", ""}, {4, 40, "instant-cast proc", ""}},
	"Khadgar's Regalia":    {{2, 50, "tier bonus", ""}, {4, 60, "tier bonus", ""}},
	"Sunstrider's Regalia": {{2, 50, "tier bonus", ""}, {4, 60, "tier bonus", ""}},
	"Bloodmage's Regalia":  {{2, 60, "Mirror Image bonus", ""}, {4, 90, "tier bonus", ""}},

	// =============== Warlock (SP=1 units, ~30 EP/1%) ===================
	"Plagueheart Garb":     {{2, 40, "spell power proc", ""}, {4, 45, "300 spirit proc", ""}},
	"Deathbringer Garb":    {{2, 50, "tier bonus", ""}, {4, 70, "tier bonus", ""}},
	"Gul'dan's Regalia":    {{2, 45, "pet +10% crit", ""}, {4, 60, "tier bonus", ""}},
	"Kel'Thuzad's Regalia": {{2, 45, "pet +10% crit", ""}, {4, 60, "tier bonus", ""}},
	// T10 4pc ×1.1 damage proc 10s (sourced multiplier).
	"Dark Coven's Regalia": {{2, 70, "tier bonus", ""}, {4, 100, "+10% damage proc", ""}},

	// ========== Shaman elemental (SP=1 units, ~30 EP/1%) ===============
	"Earthshatter Garb":     {{2, 15, "Lightning Bolt -5% cost", ""}, {4, 40, "Lava Burst +10% crit damage", ""}},
	"Worldbreaker Garb":     {{2, 45, "shock bonus", ""}, {4, 55, "Lightning Bolt bonus", ""}},
	"Thrall's Regalia":      {{2, 50, "shock bonus", ""}, {4, 65, "Lava Burst bonus", ""}},
	"Nobundo's Regalia":     {{2, 50, "shock bonus", ""}, {4, 65, "Lava Burst bonus", ""}},
	"Frost Witch's Regalia": {{2, 60, "Elemental Mastery bonus", ""}, {4, 80, "Flame Shock haste ticks", ""}},

	// ====== Shaman restoration (SP=1 units, HPS-anchored; bonus text
	// from wowhead wotlk tooltips — wowsims has no resto tier bonuses) ==
	"Earthshatter Regalia": {{2, 15, "Water Shield +10%", ""}, {4, 60, "Chain Heal/Healing Wave +5%", ""}},
	"Worldbreaker Regalia": {{2, 30, "Riptide cooldown -1s", ""}, {4, 60, "Chain Heal cast -0.2s", ""}},
	"Thrall's Garb":        {{2, 45, "Riptide +20% healing", ""}, {4, 25, "Chain Heal +5% crit", ""}},
	"Nobundo's Garb":       {{2, 45, "Riptide +20% healing", ""}, {4, 25, "Chain Heal +5% crit", ""}},
	"Frost Witch's Garb":   {{2, 65, "Riptide grants 20% haste for next cast", ""}, {4, 40, "Chain Heal crits add 25% HoT", ""}},

	// ========= Shaman enhancement (AP=1 units, ~65 EP/1%) ==============
	"Earthshatter Battlegear": {{2, 25, "Lightning Shield +10% damage", ""}, {4, 60, "Flurry +5% haste", ""}},
	"Worldbreaker Battlegear": {{2, 130, "Stormstrike/Lava Lash +20% damage", ""}, {4, 100, "Maelstrom +20% proc rate", ""}},
	"Thrall's Battlegear":     {{2, 40, "Static Shock +3% proc rate", ""}, {4, 90, "shocks +25% damage", ""}},
	"Nobundo's Battlegear":    {{2, 40, "Static Shock +3% proc rate", ""}, {4, 90, "shocks +25% damage", ""}},
	// T10 (wotlkdb itemset -391): 2pc Shamanistic Rage (1min CD, kept on
	// CD with this bonus) adds +12% damage for 15s → 25% uptime ≈ +3%
	// DPS at the ~65 EP/1% melee anchor. 4pc: each Maelstrom 5-stack
	// (~every 10s) has 15% chance of +20% AP 10s → proc ~every 67s,
	// ~15% uptime; at ~5500 raid-buffed AP ≈ +165 avg AP.
	"Frost Witch's Battlegear": {
		{2, 195, "Shamanistic Rage adds +12% damage window", ""},
		{4, 165, "+20% AP proc from Maelstrom 5-stacks", ""},
	},

	// ====== Druid balance (SP=1 units; mechanics sourced comments) =====
	"Dreamwalker Garb":    {{2, 15, "Insect Swarm +10% damage", ""}, {4, 35, "Wrath/Starfire +5% crit", ""}},
	"Nightsong Garb":      {{2, 45, "Eclipse bonus +7%", ""}, {4, 60, "Insect Swarm procs instant Starfire", ""}},
	"Malfurion's Regalia": {{2, 30, "Moonfire periodic can crit", ""}, {4, 75, "Starfire/Wrath +4% damage", ""}},
	"Runetotem's Regalia": {{2, 30, "Moonfire periodic can crit", ""}, {4, 75, "Starfire/Wrath +4% damage", ""}},
	"Lasherweave Regalia": {{2, 45, "+15% nature/arcane damage on Clearcasting", ""}, {4, 60, "Starfire/Wrath crits languish +7%", ""}},

	// == Druid feral (valued as CAT per the registry's Feral=DPS pick) ==
	// Feral sets carry both cat and bear bonuses; EP reflects the cat
	// side (AP=1 units). Bear value unmodeled until a tank spec matters.
	"Dreamwalker Battlegear": {{2, 50, "tier bonus", ""}, {4, 60, "tier bonus", ""}},
	"Nightsong Battlegear":   {{2, 60, "Rake/Rip procs Clearcasting", ""}, {4, 50, "Savage Roar bonus", ""}},
	"Malfurion's Battlegear": {{2, 60, "tier bonus", ""}, {4, 70, "tier bonus", ""}},
	"Runetotem's Battlegear": {{2, 60, "tier bonus", ""}, {4, 70, "tier bonus", ""}},
	"Lasherweave Battlegear": {{2, 90, "tier bonus", ""}, {4, 110, "tier bonus", ""}},

	// ====== Druid restoration (SP=1 units, HPS-anchored; bonus text
	// from wowhead wotlk tooltips — wowsims has no resto tier bonuses) ==
	"Dreamwalker Regalia": {{2, 15, "Lifebloom -5% cost", ""}, {4, 35, "Nourish +5% per HoT on target", ""}},
	// T8 4pc: Rejuvenation also heals instantly on cast — the famous
	// resto bonus kept deep into T9.
	"Nightsong Regalia": {{2, 20, "Swiftmend +10%", ""}, {4, 60, "Rejuvenation also heals instantly", ""}},
	"Malfurion's Garb":  {{2, 25, "Nourish +5% crit", ""}, {4, 50, "Rejuvenation can crit", ""}},
	"Runetotem's Garb":  {{2, 25, "Nourish +5% crit", ""}, {4, 50, "Rejuvenation can crit", ""}},
	"Lasherweave Garb":  {{2, 60, "Wild Growth decays 30% less", ""}, {4, 45, "Rejuvenation ticks can jump to a new target", ""}},

	// == Shard of the Gods (ToC trinket pairs; sourced from the WoWSims
	// implementation, sim/common/wotlk/item_sets.go): 2pc grants flat
	// spell power plus a damage AND healing proc (25%, 50s ICD) — the
	// flat SP dominates, the proc adds a modest nudge. ==
	"Purified Shard of the Gods": {{2, 240, "+222 spell power + heal/damage proc", ""}},
	"Shiny Shard of the Gods":    {{2, 270, "+250 spell power + heal/damage proc", ""}},

	// ====== Gladiator sets: flat stats straight from the source ========
	// (2pc +50 AP or +29 SP, 4pc +150 AP or +88 SP; resilience ignored —
	// the PvP filter gates these anyway.)
	"Gladiator's Pursuit":     {{2, 50, "+50 attack power", ""}, {4, 150, "+150 attack power", ""}},
	"Gladiator's Battlegear":  {{2, 50, "+50 attack power", ""}, {4, 150, "+150 attack power", ""}},
	"Gladiator's Vestments":   {{2, 50, "+50 attack power", ""}, {4, 150, "+150 attack power", ""}},
	"Gladiator's Vindication": {{2, 50, "+50 attack power", ""}, {4, 150, "+150 attack power", ""}},
	"Gladiator's Earthshaker": {{2, 50, "+50 attack power", ""}, {4, 150, "+150 attack power", ""}},
	"Gladiator's Sanctuary":   {{2, 50, "+50 attack power", ""}, {4, 150, "+150 attack power", ""}},
	"Gladiator's Regalia":     {{2, 29, "+29 spell power", ""}, {4, 88, "+88 spell power", ""}},
	"Gladiator's Investiture": {{2, 29, "+29 spell power", ""}, {4, 88, "+88 spell power", ""}},
	"Gladiator's Raiment":     {{2, 29, "+29 spell power", ""}, {4, 88, "+88 spell power", ""}},
	"Gladiator's Wildhide":    {{2, 29, "+29 spell power", ""}, {4, 88, "+88 spell power", ""}},
	"Gladiator's Wartide":     {{2, 29, "+29 spell power", ""}, {4, 88, "+88 spell power", ""}},
	"Gladiator's Felshroud":   {{2, 29, "+29 spell power", ""}, {4, 88, "+88 spell power", ""}},
	"Gladiator's Desecration": {{2, 50, "+50 attack power", ""}, {4, 150, "+150 attack power", ""}},
	// Thunderfist is the shaman set with SPELL power bonuses per the
	// wowhead tooltip (item 40987) — curated as the data says, not the
	// spec lore.
	"Gladiator's Thunderfist": {{2, 29, "+29 spell power", ""}, {4, 88, "+88 spell power", ""}},
	"Gladiator's Redemption":  {{2, 29, "+29 spell power", ""}, {4, 88, "+88 spell power", ""}},
	"Gladiator's Refuge":      {{2, 29, "+29 spell power", ""}, {4, 88, "+88 spell power", ""}},
}

// setCounts tallies worn pieces per set name.
func setCounts(ws []Worn) map[string]int {
	counts := map[string]int{}
	for _, w := range ws {
		if w.Item.SetName != "" {
			counts[w.Item.SetName]++
		}
	}
	return counts
}

// SetBonusScore is the total EP of all active set bonuses in the set.
func (s *Scorer) SetBonusScore(ws []Worn) float64 {
	var total float64
	for name, n := range setCounts(ws) {
		total += s.rules().bonusAt(name, n, s.prof.SetBonusAudience())
	}
	return total
}

// setBonusSwapDelta is the set-bonus EP change from replacing an item
// of set curSet with one of candSet, given the current per-set counts.
func (s *Scorer) setBonusSwapDelta(counts map[string]int, curSet, candSet string) float64 {
	if curSet == candSet {
		return 0
	}
	r := s.rules()
	aud := s.prof.SetBonusAudience()
	var d float64
	if curSet != "" {
		n := counts[curSet]
		d += r.bonusAt(curSet, n-1, aud) - r.bonusAt(curSet, n, aud)
	}
	if candSet != "" {
		n := counts[candSet]
		d += r.bonusAt(candSet, n+1, aud) - r.bonusAt(candSet, n, aud)
	}
	return d
}
