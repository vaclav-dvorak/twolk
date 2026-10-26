package cata

import "github.com/vaclav-dvorak/twolk/internal/score"

// setBonuses curates the cata tier-set bonuses. Bonus MECHANICS are
// curated for every class's tier line (T11/T12/T13, text condensed
// from the wowhead cata tooltips) so the set drill-down explains what
// a set does instead of "not valued". EP is another matter: an EP is
// only meaningful in some spec's sim-anchored units, and only hunters
// and druids have cata profiles — every other class carries EP 0 (shown as an
// unvalued note, skipped by the BiS tier seeding) until its profile
// lands and brings its sim units; value the bonuses THEN, in the same
// pass (mechanics live in wowsims sim/<class>/*items*.go).
//
// Hunter EP follows the WotLK curation tradition — mechanics from
// wowsims sim/hunter/cata_items.go, EP from uptime/attribution
// reasoning at the ~90 EP per 1% dps anchor of the cata hunter scale.
var setBonuses = map[string][]score.SetBonus{
	// ========================= Hunter (valued) =========================
	// T11. 2pc: +5% crit on Serpent Sting — sting is ~8-10% of damage,
	// so ~0.4% dps. 4pc: -0.2s cast on Cobra AND Steady Shot — ~10%
	// faster foundation casts, ~2% dps (the community consensus "best
	// 4pc of the tier").
	"Lightning-Charged Battlegear": {
		{Pieces: 2, EP: 35, Note: "+5% Serpent Sting crit"},
		{Pieces: 4, EP: 180, Note: "-0.2s Cobra/Steady Shot cast"},
	},
	// T12. 2pc: 10% chance on Steady/Cobra to fire a Flaming Arrow
	// (~80% weapon fire damage) — ~1.5% dps. 4pc: 10% chance on ranged
	// autos for Burning Adrenaline (next shot free) — ~0.7% dps.
	"Flamewaker's Battlegear": {
		{Pieces: 2, EP: 135, Note: "Flaming Arrow proc on Steady/Cobra"},
		{Pieces: 4, EP: 65, Note: "Burning Adrenaline on ranged autos"},
	},
	// T13. 2pc: extra focus from Steady/Cobra — ~1.5% dps. 4pc: 40%
	// chance on Arcane Shot for +30% haste 15s (110s ICD, ~13% uptime
	// of a huge buff) — ~2.5% dps.
	"Wyrmstalker Battlegear": {
		{Pieces: 2, EP: 140, Note: "extra focus from Steady/Cobra Shot"},
		{Pieces: 4, EP: 230, Note: "Arcane Shot proc: +30% haste 15s"},
	},
	// PvP set, flat stats from the sim source (agility × ~3.3 EP);
	// 4pc adds 5% focus regen (~0.5% dps).
	"Gladiator's Pursuit": {
		{Pieces: 2, EP: 230, Note: "+70 agility"},
		{Pieces: 4, EP: 340, Note: "+90 agility, +5% focus regen"},
	},

	// ================== Warrior (notes only, EP 0) =====================
	"Earthen Warplate": {
		{Pieces: 2, Note: "Bloodthirst/Mortal Strike +5% damage"},
		{Pieces: 4, Note: "Overpower/Raging Blow stack +1% AP (3)"},
	},
	"Earthen Battleplate": {
		{Pieces: 2, Note: "Shield Slam +5% damage"},
		{Pieces: 4, Note: "Shield Wall +50% duration"},
	},
	"Molten Giant Warplate": {
		{Pieces: 2, Note: "shouts grant +10% physical damage"},
		{Pieces: 4, Note: "MS/Raging Blow proc a 100% weapon Fire hit"},
	},
	"Molten Giant Battleplate": {
		{Pieces: 2, Note: "Shield Slam +20% as Fire"},
		{Pieces: 4, Note: "+6% parry after Shield Block"},
	},
	"Colossal Dragonplate Battlegear": {
		{Pieces: 2, Note: "Heroic Strike -10 rage during Inner Rage"},
		{Pieces: 4, Note: "Bloodthirst/Mortal Strike proc Colossus Smash"},
	},
	"Colossal Dragonplate Armor": {
		{Pieces: 2, Note: "Revenge grants a 20% absorb shield"},
		{Pieces: 4, Note: "Shield Wall raid-wide at half effect"},
	},

	// ================ Death Knight (notes only, EP 0) ==================
	"Magma Plated Battlegear": {
		{Pieces: 2, Note: "Death Coil/Frost Strike +5% crit"},
		{Pieces: 4, Note: "Death Rune/Killing Machine stack +1% AP (3)"},
	},
	"Magma Plated Battlearmor": {
		{Pieces: 2, Note: "Death Strike +5% damage"},
		{Pieces: 4, Note: "Icebound Fortitude +50% duration"},
	},
	"Elementium Deathplate Battlegear": {
		{Pieces: 2, Note: "Horn of Winter grants 3 runic power per 5s"},
		{Pieces: 4, Note: "Obliterate/Scourge Strike +6% as Fire"},
	},
	"Elementium Deathplate Battlearmor": {
		{Pieces: 2, Note: "melee applies Burning Blood (counts as 2 diseases)"},
		{Pieces: 4, Note: "+15% parry after Dancing Rune Weapon"},
	},
	"Necrotic Boneplate Battlegear": {
		{Pieces: 2, Note: "Sudden Doom/Rime grant double charges"},
		{Pieces: 4, Note: "runic procs grant 710 mastery 12s"},
	},
	"Necrotic Boneplate Armor": {
		{Pieces: 2, Note: "below 35% health a Blood Rune turns Death"},
		{Pieces: 4, Note: "Vampiric Blood raid-wide at half effect"},
	},

	// ================== Paladin (notes only, EP 0) =====================
	"Reinforced Sapphirium Battleplate": {
		{Pieces: 2, Note: "Templar's Verdict +10% damage"},
		{Pieces: 4, Note: "Inquisition counts +1 Holy Power"},
	},
	"Reinforced Sapphirium Battlearmor": {
		{Pieces: 2, Note: "Crusader Strike +10% damage"},
		{Pieces: 4, Note: "Guardian of Ancient Kings +50% duration"},
	},
	"Reinforced Sapphirium Regalia": {
		{Pieces: 2, Note: "Holy Light +5% crit"},
		{Pieces: 4, Note: "540 spirit for 6s after Holy Shock"},
	},
	"Battleplate of Immolation": {
		{Pieces: 2, Note: "Crusader Strike +15% as Fire dot"},
		{Pieces: 4, Note: "Zealotry +15s duration"},
	},
	"Battlearmor of Immolation": {
		{Pieces: 2, Note: "Shield of the Righteous +20% as Fire"},
		{Pieces: 4, Note: "+12% parry after Divine Protection"},
	},
	"Regalia of Immolation": {
		{Pieces: 2, Note: "Holy Shock heals can refund 6% base mana"},
		{Pieces: 4, Note: "big heals splash 10% to a nearby ally"},
	},
	"Battleplate of Radiant Glory": {
		{Pieces: 2, Note: "Judgement generates 1 Holy Power"},
		{Pieces: 4, Note: "+18% damage during Zealotry"},
	},
	"Armor of Radiant Glory": {
		{Pieces: 2, Note: "Judgement grants a 25% absorb shield"},
		{Pieces: 4, Note: "Divine Guardian -60s cooldown, +radius"},
	},
	"Regalia of Radiant Glory": {
		{Pieces: 2, Note: "Divine Favor: heals -25% mana for 15s"},
		{Pieces: 4, Note: "Holy Radiance +5% healing"},
	},

	// =================== Shaman (notes only, EP 0) =====================
	"Battlegear of the Raging Elements": {
		{Pieces: 2, Note: "Lava Lash/Stormstrike +10% damage"},
		{Pieces: 4, Note: "Lightning Bolt +10% crit"},
	},
	"Regalia of the Raging Elements": {
		{Pieces: 2, Note: "Flame Shock +10% crit"},
		{Pieces: 4, Note: "Lightning Bolt cast -10%"},
	},
	"Vestments of the Raging Elements": {
		{Pieces: 2, Note: "Healing Wave +5% crit"},
		{Pieces: 4, Note: "540 spirit for 6s after Riptide"},
	},
	"Volcanic Battlegear": {
		{Pieces: 2, Note: "Lava Lash +5% per Searing Flames stack"},
		{Pieces: 4, Note: "Stormstrike adds +6% fire-ability damage taken"},
	},
	"Volcanic Regalia": {
		{Pieces: 2, Note: "Lightning Bolt cuts Fire Elemental cooldown"},
		{Pieces: 4, Note: "Lava Surge makes Lava Burst instant"},
	},
	"Volcanic Vestments": {
		{Pieces: 2, Note: "Riptide ticks can refund 1% base mana"},
		{Pieces: 4, Note: "Chain Heal keeps Riptide on the primary"},
	},
	"Spiritwalker's Battlegear": {
		{Pieces: 2, Note: "with Maelstrom stacks: bolts and heals +20%"},
		{Pieces: 4, Note: "Feral Spirits grant Maelstrom charges"},
	},
	"Spiritwalker's Regalia": {
		{Pieces: 2, Note: "Elemental Mastery +2000 mastery 15s"},
		{Pieces: 4, Note: "Elemental Overload stacks +250 haste (3)"},
	},
	"Spiritwalker's Vestments": {
		{Pieces: 2, Note: "Mana Tide: heals -25% mana for 15s"},
		{Pieces: 4, Note: "Spiritwalker's Grace +5s and +30% haste"},
	},

	// ===================== Druid (cat + balance valued) =================
	// Valued like the hunter lines (uptime/attribution reasoning) at the
	// MEASURED scales of their profiles (druid.go): cat ~75 EP per 1%
	// DPS (agility = 1), balance ~106 (spell power = 1). Battlegarb is
	// shared with the bear tank, whose survival units have no DPS anchor
	// — Role "cat" keeps the EP off the bear profile. Resto stays notes.
	//
	// T11 cat. 2pc: Rake (and Lacerate) ticks +10% — Rake ~10% of cat
	// damage, ~1%. 4pc: Mangle stacks +1% AP (3 stacks, kept up by the
	// rotation's Mangles) — 3% AP, ~2% dps.
	"Stormrider's Battlegarb": {
		{Pieces: 2, EP: 75, Note: "Rake/Lacerate periodic +10%", Role: "cat"},
		{Pieces: 4, EP: 150, Note: "Mangle stacks +1% AP (3); Survival Instincts +50%", Role: "cat"},
	},
	// T11 balance. 2pc: +5% crit on Insect Swarm/Moonfire — dots ~20%
	// of damage, ~0.5%. 4pc: entering Eclipse grants +15% crit decaying
	// over its window — ~2.5%.
	"Stormrider's Regalia": {
		{Pieces: 2, EP: 55, Note: "Insect Swarm/Moonfire +5% crit"},
		{Pieces: 4, EP: 265, Note: "Eclipse grants +15% spell crit, decaying"},
	},
	"Stormrider's Vestments": {
		{Pieces: 2, Note: "Lifebloom ticks +5% crit"},
		{Pieces: 4, Note: "540 spirit while Harmony is active"},
	},
	// T12 cat. 2pc: Mangle/Shred add 10% as a fire dot — the filler
	// pair is ~35% of damage, ~3%. 4pc: finishers extend Berserk —
	// longer burst windows, ~2%.
	"Obsidian Arborweave Battlegarb": {
		{Pieces: 2, EP: 225, Note: "Mangle/Maul/Shred +10% as Fire dot", Role: "cat"},
		{Pieces: 4, EP: 150, Note: "finishers extend Berserk; Barkskin +10% dodge", Role: "cat"},
	},
	// T12 balance. 2pc: Burning Treant procs on Wrath/Starfire — ~2%.
	// 4pc: extra Eclipse energy outside Eclipse — faster Eclipse
	// cycling, ~2%.
	"Obsidian Arborweave Regalia": {
		{Pieces: 2, EP: 210, Note: "Burning Treant proc on Wrath/Starfire"},
		{Pieces: 4, EP: 210, Note: "+3/+5 Eclipse energy outside Eclipse"},
	},
	"Obsidian Arborweave Vestments": {
		{Pieces: 2, Note: "Lifebloom ticks can refund 1% base mana"},
		{Pieces: 4, Note: "Swiftmend duplicates to a nearby ally"},
	},
	// T13 cat. 2pc: Blood in the Water riders (Ferocious Bite execute)
	// — ~1.5%. 4pc: Tiger's Fury grants Stampede (free Ravages) — ~1.5%.
	"Deep Earth Battlegarb": {
		{Pieces: 2, EP: 110, Note: "Pulverize/Blood in the Water riders", Role: "cat"},
		{Pieces: 4, EP: 110, Note: "raid Frenzied Regen; Tiger's Fury triggers Stampede", Role: "cat"},
	},
	// T13 balance. 2pc: Insect Swarm makes nukes +3% on the target —
	// nukes ~70% of damage, ~2%. 4pc: Starsurge -5s cooldown, +10%
	// damage — ~2%.
	"Deep Earth Regalia": {
		{Pieces: 2, EP: 225, Note: "Insect Swarm: nukes +3% on the target"},
		{Pieces: 4, EP: 210, Note: "Starsurge -5s cooldown, +10% damage"},
	},
	"Deep Earth Vestments": {
		{Pieces: 2, Note: "Innervate: heals -25% mana for 15s"},
		{Pieces: 4, Note: "Rejuv/Regrowth can Timeslip to double duration"},
	},

	// =================== Rogue (notes only, EP 0) ======================
	"Wind Dancer's Regalia": {
		{Pieces: 2, Note: "builders +5% crit"},
		{Pieces: 4, Note: "auto-attacks proc +100% finisher crit"},
	},
	"Vestments of the Dark Phoenix": {
		{Pieces: 2, Note: "melee crits +6% as Fire dot"},
		{Pieces: 4, Note: "Tricks grants +25% to a random rating"},
	},
	"Blackfang Battleweave": {
		{Pieces: 2, Note: "after Tricks: -20% energy costs 6s"},
		{Pieces: 4, Note: "longer Shadow Dance/Adrenaline Rush/Vendetta"},
	},

	// ==================== Mage (notes only, EP 0) ======================
	"Firelord's Vestments": {
		{Pieces: 2, Note: "Arcane Missiles/Ice Lance/Pyroblast +5% crit"},
		{Pieces: 4, Note: "foundation bolts cast -10%"},
	},
	"Firehawk Robes of Conflagration": {
		{Pieces: 2, Note: "Mirror Image proc on foundation bolts"},
		{Pieces: 4, Note: "more Brain Freeze/Hot Streak; cheap Arcane Power"},
	},
	"Time Lord's Regalia": {
		{Pieces: 2, Note: "bolts stack Stolen Time haste (10)"},
		{Pieces: 4, Note: "stacks cut Arcane Power/Combustion/Icy Veins CD"},
	},

	// =================== Warlock (notes only, EP 0) ====================
	"Shadowflame Regalia": {
		{Pieces: 2, Note: "Chaos Bolt/Hand of Gul'dan/Haunt cast -10%"},
		{Pieces: 4, Note: "dots proc two 300% Fel Flames"},
	},
	"Balespider's Burning Vestments": {
		{Pieces: 2, Note: "Fiery Imp proc from periodic damage"},
		{Pieces: 4, Note: "nuke proc: +20% shadow/fire for 8s"},
	},
	"Vestments of the Faceless Shroud": {
		{Pieces: 2, Note: "Doomguard/Infernal +duration, -4min cooldown"},
		{Pieces: 4, Note: "Soulburn +10% SP; Soul Fire refunds its shard"},
	},

	// =================== Priest (notes only, EP 0) =====================
	"Mercurial Regalia": {
		{Pieces: 2, Note: "Mind Flay/Mind Sear +5% crit"},
		{Pieces: 4, Note: "Shadowy Apparitions +30% damage"},
	},
	"Mercurial Vestments": {
		{Pieces: 2, Note: "Heal +5% crit"},
		{Pieces: 4, Note: "Penance/Chakra grant 540 spirit"},
	},
	"Regalia of the Cleansing Flame": {
		{Pieces: 2, Note: "Shadowfiend +20% damage, -75s cooldown"},
		{Pieces: 4, Note: "all three dots up: Mind Blast +15%"},
	},
	"Vestments of the Cleansing Flame": {
		{Pieces: 2, Note: "direct heals regen 2% base mana per 5s"},
		{Pieces: 4, Note: "Cauterizing Flame proc heal"},
	},
	"Regalia of Dying Light": {
		{Pieces: 2, Note: "Shadow Word: Death +55% damage"},
		{Pieces: 4, Note: "Shadowfiend/Apparitions grant Shadow Orbs"},
	},
	"Vestments of Dying Light": {
		{Pieces: 2, Note: "Power Infusion/Divine Hymn: heals -25% mana"},
		{Pieces: 4, Note: "PW:Shield proc; Holy Words +33% duration"},
	},
}
