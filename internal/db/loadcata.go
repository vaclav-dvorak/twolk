package db

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// The Cataclysm item database is vendored the same way as the WotLK one
// (see load.go), from the WoWSims Cataclysm Classic simulator
// (github.com/wowsims/cata, MIT) — its data tracks the Cataclysm
// Classic client (4.4.x, currently 4.4.2), not original 4.3.4. To bump
// the pin, run tools/fetch_upstream_cata.sh and update the commit here.
//
// Overlay provenance (the flavor each generated table describes):
//   - catasources_gen.go — Classic-native (wowhead /cata gearplanner
//     dump + tooltips, both tracking the 4.4.x client).
//   - catacosts_gen.go — MIXED: cost amounts are Classic-native (wago
//     4.4.x client CSVs), but the item↔cost linkage is ORIGINAL 4.3.4
//     world data (TCPP TDB) — its end-state currency assignment is
//     corrected per phase at query time (RuleSet.RotatePrice).
//   - cataprotocol_gen.go — Classic-native (warcraft.wiki.gg); the
//     protocol economy has no original-cata counterpart at all.
const pinnedCataCommit = "70d87383a9b92f30fb9e370c4676d3ce33b6e6b6"

// LoadCata builds the Cataclysm item database from the embedded
// upstream data: parse (trinket effects included — the cata upstream
// carries them structured, no sim-source extraction needed), then the
// generated wowhead/TDB overlays (sources, phases, prices).
//
// Two things the structured dump does NOT carry, living only in the
// sim sources and merged in from vendored files exactly like the wotlk
// extraction path:
//   - the proc trigger (melee/ranged/spell/heal), from
//     stat_bonus_procs.go — effects without a sim entry keep an empty
//     ProcMask (= trigger unknown, scored ungated);
//   - stack counts, from stat_bonus_stacking.go — the dump states a
//     stacking buff's PER-STACK bonus with no stack count at all, so
//     without the merge Fluid Death reads as one 38-agility stack
//     instead of ten.
func LoadCata() (*Database, error) {
	raw, err := readGzip("data/cata-db.json.gz", "embedded cata db")
	if err != nil {
		return nil, err
	}
	d, err := ParseCata(raw)
	if err != nil {
		return nil, err
	}
	procs, err := upstream.ReadFile("data/cata_stat_bonus_procs.go.txt")
	if err != nil {
		return nil, fmt.Errorf("embedded cata effects: %w", err)
	}
	for _, pe := range ParseProcEffects(procs) {
		for _, e := range d.Effects[pe.ItemID] {
			if e.Kind == EffectProc {
				e.Callback, e.ProcMask, e.Outcome = pe.Callback, pe.ProcMask, pe.Outcome
			}
		}
	}
	stacking, err := upstream.ReadFile("data/cata_stat_bonus_stacking.go.txt")
	if err != nil {
		return nil, fmt.Errorf("embedded cata effects: %w", err)
	}
	for _, se := range ParseStackingEffects(stacking) {
		merged := false
		for _, e := range d.Effects[se.ItemID] {
			if e.Kind == se.Kind {
				// the dump's per-version Bonus/Duration/chance stay
				// authoritative; the sim entry adds what the dump lacks
				e.MaxStacks = se.MaxStacks
				e.Callback, e.ProcMask, e.Outcome = se.Callback, se.ProcMask, se.Outcome
				merged = true
			}
		}
		if !merged {
			d.Effects[se.ItemID] = append(d.Effects[se.ItemID], se)
		}
	}
	for id, b := range cataBespokeStacking {
		for _, e := range d.Effects[id] {
			if e.Kind == EffectProc {
				e.MaxStacks = b.MaxStacks
				e.Callback, e.ProcMask, e.Outcome = b.Callback, b.ProcMask, b.Outcome
				if e.Cooldown == 0 {
					e.Cooldown = b.Cooldown
				}
			}
		}
	}
	for id, e := range cataBespokeOnUse {
		e := e
		d.Effects[id] = append(d.Effects[id], &e)
	}
	d.pool = d.buildPool()
	return d, nil
}

// cataBespokeStacking curates the stacking trinkets the sim writes as
// hand-rolled item effects in sim/common/cata/other_effects.go (@ the
// pin) rather than as StackingStatBonusEffect blocks, so
// stat_bonus_stacking.go — and with it the loader above — never sees
// their stack counts or triggers. Without them the dump's per-stack
// bonus scored as ONE stack: Vessel of Acceleration's crit read 136 EP
// on a fury warrior where the sim measures ~700 (session 47), because
// it sits at 5 stacks nearly all fight. Normal and heroic ids alike.
//
// Left out on purpose: Apparatus of Khaz'goroth and Fury of Angerforge
// (stacks that an on-use converts), Jaws of Defeat (cost reduction, no
// stats) and Kiril, Fury of Beasts (a 15% proc OPENS a window in which
// hits build stacks — neither proc model here describes that).
var cataBespokeStacking = map[int]Effect{
	68995: vesselStacks, 69167: vesselStacks, // Vessel of Acceleration
	59514: heartStacks, 65110: heartStacks, // Heart of Ignacious
	59354: jarStacks, 65029: jarStacks, // Jar of Ancient Remedies
}

var (
	vesselStacks = Effect{MaxStacks: 5, Callback: "OnSpellHitDealt", ProcMask: "MeleeOrMeleeProc", Outcome: "Crit"}
	heartStacks  = Effect{MaxStacks: 5, Callback: "OnSpellHitDealt", ProcMask: "SpellDamage", Outcome: "Landed",
		Cooldown: 2 * time.Second}
	jarStacks = Effect{MaxStacks: 5, Callback: "OnHealDealt", ProcMask: "SpellHealing", Outcome: "Landed",
		Cooldown: 2 * time.Second}
)

// cataBespokeOnUse adds the on-use half of a hand-rolled trinket the
// dump carries only the proc of. Heart of Ignacious (other_effects.go
// @ the pin): the use consumes the spell power stacks for 363 haste
// each (321 normal), 20s, 2 min cooldown — at the 5 stacks it sits at,
// a plain haste on-use. Without it the heart priced ~35% under the sim
// for every caster (session 48). The ~10s it takes to rebuild the
// consumed stacks is ignored.
var cataBespokeOnUse = map[int]Effect{
	59514: {ItemID: 59514, Name: "Heart's Judgement", Kind: EffectOnUse,
		Bonus: Stats{MeleeHaste: 5 * 321}, Duration: 20 * time.Second, Cooldown: 2 * time.Minute},
	65110: {ItemID: 65110, Name: "Heart's Judgement", Kind: EffectOnUse,
		Bonus: Stats{MeleeHaste: 5 * 363}, Duration: 20 * time.Second, Cooldown: 2 * time.Minute},
}

// Consumable types in the cata dump's ConsumableType enum.
const (
	cataConsumableFlask = 2
	cataConsumableFood  = 3
)

// cataStatOrder maps WoWSims-Cata proto stat indices to canonical
// Stats. Cataclysm unified the melee/spell rating pairs into single
// ratings; those decode into the canonical MELEE slots (MeleeHit,
// MeleeCrit, MeleeHaste) by convention — cata rules must read hit/crit/
// haste from there, and the spell slots stay unset by cata data.
var cataStatOrder = [...]Stat{
	Strength, Agility, Stamina, Intellect, Spirit,
	MeleeHit, MeleeCrit, MeleeHaste, Expertise, Dodge, Parry, Mastery,
	AttackPower, RangedAttackPower, SpellPower, SpellPenetration, Resilience,
	ArcaneResistance, FireResistance, FrostResistance, NatureResistance,
	ShadowResistance, Armor, BonusArmor, Health, Mana, MP5,
}

// cataStats decodes a positional stat array laid out in the cata proto
// order (socket bonuses, gems, enchants). Entries past the known order
// are dropped rather than guessed at.
type cataStats Stats

func (cs *cataStats) UnmarshalJSON(data []byte) error {
	var raw []float64
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	for i, v := range raw {
		if i < len(cataStatOrder) {
			cs[cataStatOrder[i]] = v
		}
	}
	return nil
}

// cataStatMap decodes the sparse {"<cata stat index>": value} objects
// used inside item scalingOptions. Unknown indices are dropped.
type cataStatMap Stats

func (cs *cataStatMap) UnmarshalJSON(data []byte) error {
	var raw map[string]float64
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	for k, v := range raw {
		if i, err := strconv.Atoi(k); err == nil && i >= 0 && i < len(cataStatOrder) {
			cs[cataStatOrder[i]] = v
		}
	}
	return nil
}

// cataRangedOrder maps the re-numbered WoWSims-Cata RangedWeaponType
// enum (relics unified, no wands gap) onto ours.
var cataRangedOrder = [...]RangedWeaponType{
	RangedUnknown, RangedBow, RangedCrossbow, RangedGun,
	RangedRelic, RangedThrown, RangedWand,
}

// ParseCata decodes the raw WoWSims cata db.json document. The item
// shape differs from the WotLK one: per-item stats, ilvl and weapon
// damage live under scalingOptions (keyed by scaling level — retail
// cata data only ever carries "0"), stats are sparse maps rather than
// positional arrays, and the enums above need their own decode tables.
func ParseCata(raw []byte) (*Database, error) {
	var doc struct {
		Items []struct {
			ID             int                `json:"id"`
			Name           string             `json:"name"`
			Icon           string             `json:"icon"`
			Slot           Slot               `json:"type"`
			ArmorType      ArmorType          `json:"armorType"`
			WeaponType     WeaponType         `json:"weaponType"`
			HandType       HandType           `json:"handType"`
			RangedType     int                `json:"rangedWeaponType"`
			WeaponSpeed    float64            `json:"weaponSpeed"`
			GemSockets     []GemColor         `json:"gemSockets"`
			SocketBonus    cataStats          `json:"socketBonus"`
			Phase          int                `json:"phase"`
			Quality        Quality            `json:"quality"`
			Unique         bool               `json:"unique"`
			Heroic         bool               `json:"heroic"`
			SetName        string             `json:"setName"`
			ClassAllowlist []Class            `json:"classAllowlist"`
			Faction        FactionRestriction `json:"factionRestriction"`
			Sources        []Source           `json:"sources"`
			ScalingOptions map[string]struct {
				Stats          cataStatMap `json:"stats"`
				ILvl           int         `json:"ilvl"`
				WeaponDmgMin   float64     `json:"weaponDamageMin"`
				WeaponDmgMax   float64     `json:"weaponDamageMax"`
				RandPropPoints int         `json:"randPropPoints"`
			} `json:"scalingOptions"`
			RandomSuffixOptions []int `json:"randomSuffixOptions"`
			// stat-buff trinket effects, structured upstream (unlike
			// wotlk, where they are extracted from sim sources)
			ItemEffect *struct {
				BuffName       string `json:"buffName"`
				DurationMs     int    `json:"effectDurationMs"`
				ScalingOptions map[string]struct {
					Stats cataStatMap `json:"stats"`
				} `json:"scalingOptions"`
				Proc *struct {
					ProcChance float64 `json:"procChance"`
					PPM        float64 `json:"ppm"`
					IcdMs      int     `json:"icdMs"`
				} `json:"proc"`
				OnUse *struct {
					CooldownMs int `json:"cooldownMs"`
				} `json:"onUse"`
			} `json:"itemEffect"`
		} `json:"items"`
		Gems []struct {
			ID                 int       `json:"id"`
			Name               string    `json:"name"`
			Icon               string    `json:"icon"`
			Color              GemColor  `json:"color"`
			Stats              cataStats `json:"stats"`
			Phase              int       `json:"phase"`
			Quality            Quality   `json:"quality"`
			Unique             bool      `json:"unique"`
			RequiredProfession int       `json:"requiredProfession"`
		} `json:"gems"`
		Enchants []struct {
			EffectID           int       `json:"effectId"`
			ItemID             int       `json:"itemId"`
			SpellID            int       `json:"spellId"`
			Name               string    `json:"name"`
			Slot               Slot      `json:"type"`
			EnchantType        int       `json:"enchantType"`
			Stats              cataStats `json:"stats"`
			Quality            Quality   `json:"quality"`
			ClassAllowlist     []Class   `json:"classAllowlist"`
			RequiredProfession int       `json:"requiredProfession"`
			// proc/on-use buff, structured like the items' itemEffect.
			// The dump also abuses it (proc, duration -1) to restate
			// passive auras — only positive durations are real procs.
			EnchantEffect *struct {
				DurationMs     int `json:"effectDurationMs"`
				ScalingOptions map[string]struct {
					Stats cataStatMap `json:"stats"`
				} `json:"scalingOptions"`
				Proc *struct {
					ProcChance float64 `json:"procChance"`
					PPM        float64 `json:"ppm"`
					IcdMs      int     `json:"icdMs"`
				} `json:"proc"`
				OnUse *struct {
					CooldownMs int `json:"cooldownMs"`
				} `json:"onUse"`
			} `json:"enchantEffect"`
		} `json:"enchants"`
		Zones []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"zones"`
		RandomSuffixes []struct {
			ID    int       `json:"id"`
			Name  string    `json:"name"`
			Stats cataStats `json:"stats"`
		} `json:"randomSuffixes"`
		NPCs []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"npcs"`
		// the cata dump ships the consumable tables the WotLK one lacks;
		// only flasks (type 2) and food (type 3) are read — potions and
		// elixirs are situational or strictly worse than a flask, and
		// the planner assumes a raiding character
		Consumables []struct {
			ID    int       `json:"id"`
			Name  string    `json:"name"`
			Type  int       `json:"type"`
			Stats cataStats `json:"stats"`
		} `json:"consumables"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parsing cata item database: %w", err)
	}
	d := &Database{
		Items:    make(map[int]*Item, len(doc.Items)),
		Gems:     make(map[int]*Gem, len(doc.Gems)),
		Enchants: make([]*Enchant, 0, len(doc.Enchants)),
		Zones:    make(map[int]string, len(doc.Zones)),
		NPCs:     make(map[int]string, len(doc.NPCs)),
		Effects:  map[int][]*Effect{},
	}
	suffixes := map[int]randomSuffix{}
	for _, rs := range doc.RandomSuffixes {
		suffixes[rs.ID] = randomSuffix{rs.Name, Stats(rs.Stats)}
	}
	rolls := map[int]suffixRoll{} // random-suffix items: options + budget
	for _, raw := range doc.Items {
		it := &Item{
			ID:             raw.ID,
			Name:           raw.Name,
			Icon:           raw.Icon,
			Slot:           raw.Slot,
			ArmorType:      raw.ArmorType,
			WeaponType:     raw.WeaponType,
			HandType:       raw.HandType,
			WeaponSpeed:    raw.WeaponSpeed,
			GemSockets:     raw.GemSockets,
			SocketBonus:    Stats(raw.SocketBonus),
			Phase:          raw.Phase,
			Quality:        raw.Quality,
			Unique:         raw.Unique,
			Heroic:         raw.Heroic,
			SetName:        raw.SetName,
			ClassAllowlist: raw.ClassAllowlist,
			Faction:        raw.Faction,
			Sources:        raw.Sources,
		}
		if raw.RangedType >= 0 && raw.RangedType < len(cataRangedOrder) {
			it.RangedType = cataRangedOrder[raw.RangedType]
		}
		if so, ok := raw.ScalingOptions["0"]; ok {
			it.Stats = Stats(so.Stats)
			it.ILvl = so.ILvl
			it.WeaponDmgMin = so.WeaponDmgMin
			it.WeaponDmgMax = so.WeaponDmgMax
			if len(raw.RandomSuffixOptions) > 0 {
				rolls[it.ID] = suffixRoll{raw.RandomSuffixOptions, so.RandPropPoints}
			}
		}
		if ie := raw.ItemEffect; ie != nil {
			e := &Effect{
				ItemID:   it.ID,
				Name:     ie.BuffName,
				Duration: time.Duration(ie.DurationMs) * time.Millisecond,
			}
			if so, ok := ie.ScalingOptions["0"]; ok {
				e.Bonus = Stats(so.Stats)
			}
			switch {
			case ie.Proc != nil:
				e.Kind = EffectProc
				e.ProcChance = ie.Proc.ProcChance
				e.PPM = ie.Proc.PPM
				e.Cooldown = time.Duration(ie.Proc.IcdMs) * time.Millisecond
			case ie.OnUse != nil:
				e.Kind = EffectOnUse
				e.Cooldown = time.Duration(ie.OnUse.CooldownMs) * time.Millisecond
			}
			if e.Kind != 0 && e.Bonus != (Stats{}) {
				d.Effects[it.ID] = append(d.Effects[it.ID], e)
			}
		}
		d.Items[it.ID] = it
	}
	for _, raw := range doc.Gems {
		d.Gems[raw.ID] = &Gem{
			ID: raw.ID, Name: raw.Name, Icon: raw.Icon, Color: raw.Color,
			Stats: Stats(raw.Stats), Phase: raw.Phase, Quality: raw.Quality,
			Unique: raw.Unique, RequiredProfession: raw.RequiredProfession,
		}
	}
	for _, raw := range doc.Enchants {
		// the cata dump omits the two-hand flag on every "Enchant 2H
		// Weapon" line (staff enchants ARE flagged) — without it
		// BestEnchant lands Mighty Agility (+130 agi, 2H-only) on
		// one-handed stat sticks and dual-wield beats every real 2H
		if raw.EnchantType == 0 && strings.Contains(raw.Name, "2H Weapon") {
			raw.EnchantType = 1
		}
		e := &Enchant{
			EffectID: raw.EffectID, ItemID: raw.ItemID, SpellID: raw.SpellID,
			Name: raw.Name, Slot: raw.Slot, EnchantType: raw.EnchantType,
			Stats: Stats(raw.Stats), Quality: raw.Quality,
			ClassAllowlist:     raw.ClassAllowlist,
			RequiredProfession: raw.RequiredProfession,
		}
		// the dump writes both haste scopes' rating into proto slot 0
		// (Strength): Safety Catch Removal Kit is +88 ranged haste
		// (wowhead item 59596), Sun Scope +40 — without the move a
		// hit-capped hunter has no scope worth anything
		if cataScopeHasteFix[e.EffectID] && e.Stats[MeleeHaste] == 0 {
			e.Stats[MeleeHaste], e.Stats[Strength] = e.Stats[Strength], 0
		}
		e.Phase = cataEnchantPhase[e.EffectID]
		// Proc enchants (Landslide, Power Torrent, the scopes) carry
		// empty flat stats and a structured buff instead: fold the
		// uptime-averaged buff into Stats — the wotlk enchantAverages
		// convention, computed rather than curated, with the score
		// package's proc model (wait 60/PPM, or the 2s trigger cadence
		// / proc chance; uptime = duration / (ICD + wait)).
		if fx := raw.EnchantEffect; fx != nil && fx.DurationMs > 0 && e.Stats == (Stats{}) {
			if so, ok := fx.ScalingOptions["0"]; ok {
				dur := float64(fx.DurationMs) / 1000
				var uptime float64
				switch {
				case fx.Proc != nil && fx.Proc.PPM > 0:
					uptime = dur / (float64(fx.Proc.IcdMs)/1000 + 60/fx.Proc.PPM)
				case fx.Proc != nil && fx.Proc.ProcChance > 0:
					uptime = dur / (float64(fx.Proc.IcdMs)/1000 + 2/fx.Proc.ProcChance)
				case fx.OnUse != nil && fx.OnUse.CooldownMs > 0:
					uptime = float64(fx.DurationMs) / float64(fx.OnUse.CooldownMs)
				}
				for i, v := range so.Stats {
					e.Stats[i] = v * min(uptime, 1)
				}
			}
		}
		d.Enchants = append(d.Enchants, e)
	}
	for _, z := range doc.Zones {
		d.Zones[z.ID] = z.Name
	}
	for _, n := range doc.NPCs {
		d.NPCs[n.ID] = n.Name
	}
	for _, c := range doc.Consumables {
		var kind ConsumableKind
		switch c.Type {
		case cataConsumableFlask:
			kind = ConsumableFlask
		case cataConsumableFood:
			kind = ConsumableFood
		default:
			continue
		}
		if Stats(c.Stats) == (Stats{}) {
			continue // utility consumables (invisibility, speed) carry no stats
		}
		d.Consumables = append(d.Consumables, Consumable{
			ID: c.ID, Name: c.Name, Kind: kind, Stats: Stats(c.Stats)})
	}
	d.applyCataOverlays()
	d.applyCataCurated()
	// a Firestone upgrade exists as soon as its vendor and its base do:
	// Lurah Wrathvine stands at Firelands (phase 3) — the heroic
	// trinkets the dump leaves unsourced would otherwise wait for their
	// phase-4 protocol price
	for up, base := range cataFirestoneUpgrades {
		if it, b := d.Items[up], d.Items[base]; it != nil && b != nil {
			it.Phase = min(it.Phase, max(b.Phase, 3))
		}
	}
	d.Rules = CataRules()
	d.applyCataProtocol()
	// last: variants copy the finished item (sources, phase, overlays)
	d.expandSuffixes(rolls, suffixes)
	return d, nil
}

// cataScopeHasteFix marks the ranged enchants whose haste rating the
// upstream dump misfiles under Strength (see the load loop).
var cataScopeHasteFix = map[int]bool{
	4177: true, // Safety Catch Removal Kit (+88 haste)
	3607: true, // Sun Scope (+40 haste)
}

// cataEnchantPhase curates the arrival phase of post-launch enchants
// (everything else is available from P1). Only entries the optimizer
// could actually propose matter.
var cataEnchantPhase = map[int]int{
	4267: 3, // Flintlocke's Woodchucker — 4.2, Firelands recipe drop
}

// cataLegendaryClasses restores the class locks the upstream data
// omits on the Cataclysm legendaries (Shadowmourne arrives with its
// allowlist; these don't — without them the rogue daggers win any
// spec's BiS weapon slots).
var cataLegendaryClasses = map[int][]Class{
	71086: {ClassDruid, ClassMage, ClassPriest, ClassShaman, ClassWarlock}, // Dragonwrath, Tarecgosa's Rest
	77949: {ClassRogue},                                                    // Golad, Twilight of Aspects
	77950: {ClassRogue},                                                    // Tiriosh, Nightmare of Ages
}

// applyCataOverlays applies the generated wowhead side tables
// (catasources_gen.go): true sources for items the upstream cannot
// source, the PvP-only classification, the unobtainable drop list,
// the content-phase overlay (upstream tags nearly everything phase 1)
// and the tooltip-derived Unique-Equipped flags, plus the curated
// legendary class locks.
func (d *Database) applyCataOverlays() {
	d.addSupplementalSources(cataSupplementalSources)
	// justice/valor-priced gear is vendor stock even when the upstream
	// only knows a drop for it (Argaloth drops the same T11 legs and
	// gloves the quartermaster sells): without a SoldBy source the
	// vendor route neither renders nor passes the raid-less filter.
	for id := range cataVendorPrices {
		it := d.Items[id]
		if it == nil {
			continue
		}
		sold := false
		for _, s := range it.Sources {
			sold = sold || s.SoldBy != nil
		}
		if !sold {
			it.Sources = append(it.Sources, Source{SoldBy: &SoldBySource{}})
		}
	}
	// vendor-hub zones (daily hubs like the Molten Front) the upstream
	// zone list lacks — without names their vendor sources render
	// location-less
	for z, name := range cataVendorZones {
		if d.Zones[z] == "" {
			d.Zones[z] = name
		}
	}
	for _, id := range cataUnobtainable {
		delete(d.Items, id)
	}
	for _, id := range cataUniqueItems {
		if it := d.Items[id]; it != nil {
			it.Unique = true
		}
	}
	for id, classes := range cataLegendaryClasses {
		if it := d.Items[id]; it != nil {
			it.ClassAllowlist = classes
		}
	}
	// data-leftover gems the optimizer must never propose: "Strange
	// Gem" (52070) is a rare-quality agility/stamina oddity with no
	// acquisition route — it isn't cut from any raw gem and no vendor
	// carries it.
	delete(d.Gems, 52070)
	for id, p := range cataPhaseOverride {
		if it := d.Items[id]; it != nil {
			it.Phase = p
		}
	}
	// End Time's first two encounters are two RANDOM Echoes of four,
	// so the upstream lists every Echo drop on all four NPCs and names
	// the true owner in Category — a specific Echo, or "Shared" for
	// either-random-boss loot. Four near-identical rows are display
	// noise: collapse each (zone, category) group to one source under
	// the owner's name. Cata-only: WotLK categories are CONDITIONS
	// ("Hard Mode") where distinct bosses must stay distinct.
	for _, it := range d.Items {
		type ownerKey struct {
			zone int
			cat  string
		}
		seen := map[ownerKey]bool{}
		var out []Source
		for _, s := range it.Sources {
			if s.Drop == nil || s.Drop.Category == "" {
				out = append(out, s)
				continue
			}
			k := ownerKey{s.Drop.ZoneID, s.Drop.Category}
			if seen[k] {
				continue
			}
			seen[k] = true
			who := s.Drop.Category
			if who == "Shared" {
				who = "Any boss"
			}
			out = append(out, Source{Drop: &DropSource{
				OtherName: who, ZoneID: s.Drop.ZoneID, Difficulty: s.Drop.Difficulty}})
		}
		it.Sources = out
	}
}

// applyCataProtocol wires the Elemental Rune Dungeon economy — a
// Cataclysm CLASSIC system (4.4.1/4.4.2) that neither vendor upstream
// can see (TCPP TDB is original 4.3.4, wowhead's dump says only
// "vendor"): raid gear farmable through 5-man heroic-plus runs.
//
//   - Protocol Inferno (phase 3): T11-era raid gear priced in Fissure
//     Stone Fragments — the generated wiki table (cataprotocol_gen.go).
//   - Protocol Twilight (phase 4): Dragon Soul "Raid Finder" gear.
//     Classic has no Raid Finder — the LFR-ilvl pieces (384 armor, 390
//     weapons) drop from protocol final bosses and sell for Obsidian
//     Fragments, so they leave the raid-gated set and their upstream
//     LFR-raid attribution is replaced. The vendor also stocks normal
//     Firelands weapons, Ragnaros loot, T12 head/shoulder tier and the
//     heroic "Flickering" satchels (per-item costs not in our data
//     yet: warcraft.wiki.gg itemizes only the Inferno vendor).
func (d *Database) applyCataProtocol() {
	d.Protocol = map[int]ProtocolPrice{}
	for id, c := range cataFissurePrices {
		if d.Items[id] == nil {
			continue
		}
		d.Protocol[id] = ProtocolPrice{
			Emblem: EmblemFissure, Count: c, Phase: 3, Label: "Protocol Inferno"}
	}
	// the LFR pool: every phase-4 epic at Raid Finder ilvls that isn't
	// PvP gear — 95 T13 "L" set pieces + the DS weapons, trinkets and
	// jewelry. Classic has no Raid Finder: these drop from Protocol
	// Twilight final bosses, so they leave the raid gate and their
	// upstream LFR-raid attribution is replaced. The 18 dedicated
	// weapon/accessory drops carry their assigned dungeon; the rest
	// (tier via random tokens, vendor armor) stay generic.
	zoneID := map[string]int{}
	for id, name := range d.Zones {
		zoneID[name] = id
	}
	for id, it := range d.Items {
		if it.Phase == 4 && (it.ILvl == 384 || it.ILvl == 390) &&
			it.Quality == QualityEpic &&
			it.Stats[Resilience] == 0 && !d.PvPItem(it) &&
			!it.crafted() && !it.quested() {
			d.Protocol[id] = ProtocolPrice{
				Emblem: EmblemObsidian, Phase: 4, Label: "Protocol Twilight", Drops: true}
			drop := &DropSource{OtherName: ProtocolTwilight}
			if dungeon, ok := cataTwilightDungeons[id]; ok {
				drop.OtherName = "Protocol Twilight"
				if z, ok := zoneID[dungeon]; ok {
					drop.ZoneID = z // renders "Protocol Twilight · <dungeon>"
				} else if z, ok := zoneID[strings.TrimPrefix(dungeon, "The ")]; ok {
					drop.ZoneID = z
				} else {
					drop.OtherName += " · " + dungeon
				}
			}
			it.Sources = []Source{{Drop: drop}}
			delete(d.Rules.RaidGatedItems, id)
		}
	}
	// the Twilight vendor's stock with exact Obsidian costs (the
	// vendored wowhead snapshot is the COMPLETE 134-row inventory:
	// normal FL weapons, Ragnaros loot, heroic FL/Rag trinkets, the
	// LFR gear and tier/satchel expansions) — drop-pool entries keep
	// Drops (buyable AND farmable), the rest are plain vendor stock
	for id, c := range cataObsidianPrices {
		if d.Items[id] == nil {
			continue
		}
		p, ok := d.Protocol[id]
		if !ok {
			p = ProtocolPrice{Emblem: EmblemObsidian, Phase: 4, Label: "Protocol Twilight"}
		}
		p.Count = c
		d.Protocol[id] = p
	}
}

// crafted / quested guard the LFR-pool derivation: genuine Raid Finder
// gear carries only anonymous vendor or drop attributions upstream.
func (it *Item) crafted() bool {
	for _, s := range it.Sources {
		if s.Crafted != nil {
			return true
		}
	}
	return false
}

func (it *Item) quested() bool {
	for _, s := range it.Sources {
		if s.Quest != nil {
			return true
		}
	}
	return false
}

// ProtocolTwilight is the drop attribution the LFR pool renders with.
const ProtocolTwilight = "Protocol Twilight (heroic++ final bosses)"
