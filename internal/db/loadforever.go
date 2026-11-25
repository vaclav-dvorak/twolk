package db

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// The World of Warcraft: Forever item database is vendored like the
// other versions (see load.go), from the WoWSims Forever simulator
// (github.com/wowsims/forever, MIT), whose bot rebuilds it from the
// game client and Blizzard's hotfixes. Forever keeps adding items, so
// every twolk release re-vendors: run tools/fetch_upstream_forever.sh
// after bumping the pin.
//
// Two files, one item pool: db.json holds what a level-60 sim cares
// about, leftover_db.json everything else (low-level greens, white
// gear). Leveling characters wear the leftovers, so both are loaded.
const pinnedForeverCommit = "9c0decc1560fc19e090cdaec8b4efd9c5363b63f"

// LoadForever builds the Forever item database from the embedded
// upstream data.
func LoadForever() (*Database, error) {
	main, err := readGzip("data/forever-db.json.gz", "embedded forever db")
	if err != nil {
		return nil, err
	}
	leftover, err := readGzip("data/forever-leftover-db.json.gz", "embedded forever leftover db")
	if err != nil {
		return nil, err
	}
	return ParseForever(main, leftover)
}

// Forever gear states hit, crit and the other combat percentages AS
// percentages ("+1% crit"); the WoWSims dump stores them as fixed
// internal ratings — level-independent, unlike real ratings. Values
// from wowsims/forever sim/core/base_stats_auto_gen.go @ the pin.
const (
	ForeverHitRatingPerPct       = 10.0
	ForeverCritRatingPerPct      = 14.0
	ForeverHasteRatingPerPct     = 10.0
	ForeverExpertiseRatingPerPct = 10.0
	ForeverDodgeRatingPerPct     = 12.0
	ForeverParryRatingPerPct     = 15.0
	ForeverBlockRatingPerPct     = 5.0
)

// foreverStatOrder maps WoWSims-Forever proto stat indices to canonical
// Stats; -1 drops a stat the canonical set has no slot for. The shared
// hit/crit/haste ratings decode into the MELEE slots, the cata
// convention. Spell damage and healing power both land on SpellPower
// (Forever splits them, like vanilla; a later scoring pass may need
// them apart). School-specific damage and feral AP are dropped.
var foreverStatOrder = [...]Stat{
	Strength, Agility, Stamina, Intellect,
	SpellPower, SpellPower, // healing power, spell damage
	-1, -1, -1, -1, -1, -1, // arcane/fire/frost/holy/nature/shadow damage
	SpellPenetration, Spirit,
	AttackPower, RangedAttackPower, -1, // feral AP
	MeleeHit, MeleeCrit, MeleeHaste,
	ArmorPenetration, Expertise,
	Defense, Block, BlockValue, Dodge, Parry, Armor, BonusArmor,
	Health, Mana, MP5,
	ArcaneResistance, FireResistance, FrostResistance, NatureResistance, ShadowResistance,
	-1,         // physical damage
	BonusArmor, // armor untouched by armor modifiers
}

// foreverDecode adds one upstream stat. Healing power and spell damage
// share SpellPower: the larger of the two wins rather than their sum,
// so an item carrying both doesn't read as double.
func foreverDecode(dst *Stats, i int, v float64) {
	if i < 0 || i >= len(foreverStatOrder) || foreverStatOrder[i] < 0 {
		return
	}
	s := foreverStatOrder[i]
	if s == SpellPower {
		dst[s] = max(dst[s], v)
		return
	}
	dst[s] += v
}

// foreverStats decodes a positional stat array (enchants).
type foreverStats Stats

func (fs *foreverStats) UnmarshalJSON(data []byte) error {
	var raw []float64
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	for i, v := range raw {
		foreverDecode((*Stats)(fs), i, v)
	}
	return nil
}

// foreverStatMap decodes the sparse {"<index>": value} item stats.
type foreverStatMap Stats

func (fs *foreverStatMap) UnmarshalJSON(data []byte) error {
	var raw map[string]float64
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	for k, v := range raw {
		if i, err := strconv.Atoi(k); err == nil {
			foreverDecode((*Stats)(fs), i, v)
		}
	}
	return nil
}

// foreverRangedOrder maps the WoWSims-Forever RangedWeaponType enum
// (thrown and wand moved up, relics split per class) onto ours.
var foreverRangedOrder = [...]RangedWeaponType{
	RangedUnknown, RangedBow, RangedCrossbow, RangedGun,
	RangedThrown, RangedWand, RangedIdol, RangedLibram, RangedTotem, RangedSigil,
}

// foreverDoc is the shape shared by db.json and leftover_db.json.
type foreverDoc struct {
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
		ReqLevel       int                `json:"reqLvl"`
		Phase          int                `json:"phase"`
		Quality        Quality            `json:"quality"`
		Unique         bool               `json:"unique"`
		SetName        string             `json:"setName"`
		ClassAllowlist []Class            `json:"classAllowlist"`
		Faction        FactionRestriction `json:"factionRestriction"`
		Sources        []Source           `json:"sources"`
		// on-use and proc buffs; only the stat-granting ones (a
		// scalingOptions stat block) are priced — absorbs, extra attacks
		// and pets have no stats to average and load display-only
		ItemEffects []struct {
			BuffName       string `json:"buffName"`
			DurationMs     int    `json:"effectDurationMs"`
			ScalingOptions map[string]struct {
				Stats foreverStatMap `json:"stats"`
			} `json:"scalingOptions"`
			Proc *struct {
				ProcChance float64 `json:"procChance"`
				PPM        float64 `json:"ppm"`
				IcdMs      int     `json:"icdMs"`
			} `json:"proc"`
			OnUse *struct {
				CooldownMs int `json:"cooldownMs"`
			} `json:"onUse"`
		} `json:"itemEffects"`
		ScalingOptions map[string]struct {
			Stats        foreverStatMap `json:"stats"`
			ILvl         int            `json:"ilvl"`
			WeaponDmgMin float64        `json:"weaponDamageMin"`
			WeaponDmgMax float64        `json:"weaponDamageMax"`
		} `json:"scalingOptions"`
	} `json:"items"`
	Enchants []struct {
		EffectID       int          `json:"effectId"`
		ItemID         int          `json:"itemId"`
		SpellID        int          `json:"spellId"`
		Name           string       `json:"name"`
		Slot           Slot         `json:"type"`
		ExtraSlots     []Slot       `json:"extraTypes"`
		ReqLevel       int          `json:"lvl"`
		EnchantType    int          `json:"enchantType"`
		Stats          foreverStats `json:"stats"`
		Quality        Quality      `json:"quality"`
		ClassAllowlist []Class      `json:"classAllowlist"`
	} `json:"enchants"`
	Zones []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"zones"`
	NPCs []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"npcs"`
	Factions []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"factions"`
}

// ParseForever decodes the two raw WoWSims Forever documents into one
// database; an item in both keeps the main db's entry.
func ParseForever(main, leftover []byte) (*Database, error) {
	d := &Database{
		Items:    map[int]*Item{},
		Gems:     map[int]*Gem{},
		Zones:    map[int]string{},
		NPCs:     map[int]string{},
		Factions: map[int]string{},
		Effects:  map[int][]*Effect{},
	}
	seenEnchant := map[int]bool{}
	for _, part := range []struct {
		raw   []byte
		label string
	}{{main, "forever item database"}, {leftover, "forever leftover database"}} {
		var doc foreverDoc
		if err := json.Unmarshal(part.raw, &doc); err != nil {
			return nil, fmt.Errorf("parsing %s: %w", part.label, err)
		}
		for _, raw := range doc.Items {
			// slotless entries are cosmetics (shirts, tabards): never
			// gear rows, as in the other versions' data
			if d.Items[raw.ID] != nil || raw.Slot == SlotUnknown {
				continue
			}
			it := &Item{
				ID:             raw.ID,
				Name:           raw.Name,
				Icon:           raw.Icon,
				Slot:           raw.Slot,
				ArmorType:      raw.ArmorType,
				WeaponType:     raw.WeaponType,
				HandType:       raw.HandType,
				WeaponSpeed:    raw.WeaponSpeed,
				ReqLevel:       raw.ReqLevel,
				Phase:          raw.Phase,
				Quality:        raw.Quality,
				Unique:         raw.Unique,
				SetName:        raw.SetName,
				ClassAllowlist: raw.ClassAllowlist,
				Faction:        raw.Faction,
				Sources:        raw.Sources,
			}
			if raw.RangedType >= 0 && raw.RangedType < len(foreverRangedOrder) {
				it.RangedType = foreverRangedOrder[raw.RangedType]
			}
			if so, ok := raw.ScalingOptions["0"]; ok {
				it.Stats = Stats(so.Stats)
				it.ILvl = so.ILvl
				it.WeaponDmgMin = so.WeaponDmgMin
				it.WeaponDmgMax = so.WeaponDmgMax
			}
			for _, ie := range raw.ItemEffects {
				e := &Effect{
					ItemID:   it.ID,
					Name:     strings.TrimSpace(strings.Split(ie.BuffName, "(")[0]),
					Duration: time.Duration(ie.DurationMs) * time.Millisecond,
				}
				// a stat buff needs a duration to average over; without
				// stats (absorbs, summons) the effect is shown, not
				// priced — a zero Bonus scores zero everywhere
				if so, ok := ie.ScalingOptions["0"]; ok && ie.DurationMs > 0 {
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
				default:
					continue
				}
				d.Effects[it.ID] = append(d.Effects[it.ID], e)
			}
			d.Items[it.ID] = it
		}
		for _, raw := range doc.Enchants {
			if seenEnchant[raw.EffectID] {
				continue
			}
			seenEnchant[raw.EffectID] = true
			// one enchant per slot it fits: armor kits name chest as
			// their type and hands/legs/feet as extra types
			for _, slot := range append([]Slot{raw.Slot}, raw.ExtraSlots...) {
				d.Enchants = append(d.Enchants, &Enchant{
					EffectID: raw.EffectID, ItemID: raw.ItemID, SpellID: raw.SpellID,
					Name: raw.Name, Slot: slot, EnchantType: raw.EnchantType,
					Stats: Stats(raw.Stats), Quality: raw.Quality,
					ClassAllowlist: raw.ClassAllowlist, ReqLevel: raw.ReqLevel,
				})
			}
		}
		for _, z := range doc.Zones {
			d.Zones[z.ID] = z.Name
		}
		for _, n := range doc.NPCs {
			d.NPCs[n.ID] = n.Name
		}
		for _, f := range doc.Factions {
			d.Factions[f.ID] = f.Name
		}
	}
	d.CraftSkills, d.BoPCrafts = foreverCraftSkills, foreverBoPCrafts
	// the upstream spells its world drops "World Drop"; the planner's
	// marker is WorldDrop, and anything else reads as dungeon loot
	for _, it := range d.Items {
		for _, s := range it.Sources {
			if s.Drop != nil && strings.EqualFold(s.Drop.OtherName, WorldDrop) {
				s.Drop.OtherName = WorldDrop
			}
		}
	}
	// open-world loot the sim data leaves sourceless, from QuestieDB
	// (tools/gen_forever_worlddrops.py): the same source the upstream
	// itself writes for the world drops it does record
	for id := range foreverWorldDrops {
		if it := d.Items[id]; it != nil && len(it.Sources) == 0 {
			it.Sources = []Source{{Drop: &DropSource{OtherName: WorldDrop}}}
		}
	}
	d.Rules = ForeverRules()
	d.classifyForeverPvP()
	d.pool = d.buildPool()
	return d, nil
}

// ForeverRules is the Forever classification table set; the PvP table
// is filled from the data itself (classifyForeverPvP).
func ForeverRules() RuleSet {
	return RuleSet{
		PvPItems:     map[int]bool{},
		Recipes:      foreverRecipes,
		RecipesHorde: foreverRecipesHorde,
		Legend: []Abbrev{
			{"ACA", "Azeroth Commerce Authority", true},
			{"DSL", "Durotar Supply and Logistics", true},
			{"Favor", "Merchant's Favor", false},
		},
	}
}

// classifyForeverPvP marks the PvP-only items: everything the upstream
// routes through a bare PvP source (that route then goes — it carries
// nothing to render), and the faction insignia family, which
// the upstream sometimes leaves sourceless ("Greater Insignia of the
// Horde"). The insignias are also locked to their faction: the client
// lets every race equip them, but only one side's honor vendor sells
// each.
func (d *Database) classifyForeverPvP() {
	for id, it := range d.Items {
		kept := it.Sources[:0]
		for _, s := range it.Sources {
			if s.PvP != nil {
				d.Rules.PvPItems[id] = true
				continue
			}
			kept = append(kept, s)
		}
		it.Sources = kept
		// battleground and honor gear: every route through a PvP
		// vendor or a battleground faction (gear that also drops or
		// comes from a quest stays PvE)
		pvpOnly := len(it.Sources) > 0
		for _, s := range it.Sources {
			pvpOnly = pvpOnly && foreverPvPRoute(s)
		}
		if pvpOnly {
			d.Rules.PvPItems[id] = true
		}
		switch {
		case strings.HasSuffix(it.Name, "Insignia of the Horde"):
			d.Rules.PvPItems[id] = true
			it.Faction = FactionHordeOnly
		case strings.HasSuffix(it.Name, "Insignia of the Alliance"):
			d.Rules.PvPItems[id] = true
			it.Faction = FactionAllianceOnly
		}
	}
}

// Forever's PvP vendors: the honor officers and the battleground
// quartermasters (NPC ids from the upstream's own vendor records).
var foreverPvPVendors = map[int]bool{
	12785: true, // Sergeant Major Clate (Alliance honor)
	12781: true, // Master Sergeant Biggins (Alliance honor)
	12799: true, // Sergeant Ba'sha (Horde honor)
	14753: true, // Illiyana Moonblaze (Silverwing Sentinels, Warsong Gulch)
	14754: true, // Kelm Hargunth (Warsong Outriders, Warsong Gulch)
	15126: true, // Rutherford Twing (Defilers, Arathi Basin)
	15127: true, // Samuel Hawke (League of Arathor, Arathi Basin)
	13217: true, // Thanthaldis Snowgleam (Stormpike Guard, Alterac Valley)
	13218: true, // Grunnda Wolfheart (Frostwolf Clan, Alterac Valley)
	13219: true, // Jekyll Flandring (Frostwolf Clan, Alterac Valley)
}

// Battleground reputation factions: League of Arathor, Defilers,
// Stormpike Guard, Frostwolf Clan, Warsong Outriders, Silverwing
// Sentinels.
var foreverPvPFactions = map[int]bool{509: true, 510: true, 730: true, 729: true, 889: true, 890: true}

func foreverPvPRoute(s Source) bool {
	return (s.SoldBy != nil && foreverPvPVendors[s.SoldBy.NPCID]) ||
		(s.Rep != nil && foreverPvPFactions[s.Rep.FactionID])
}
