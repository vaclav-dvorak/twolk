// Package db provides the WotLK 3.3.5 item database, sourced from the
// WoWSims WotLK project (github.com/wowsims/wotlk, MIT).
package db

import (
	"encoding/json"
	"fmt"
)

// Stat indexes the positional stat arrays used throughout the planner.
//
// The order is CANONICAL and owned by twolk: existing positions are
// frozen (generated tables like statfix_gen.go index it numerically)
// and new stats are appended before NumStats, never inserted. It is the
// union of every supported game version's stats — a version simply
// never sets the stats it doesn't have (WotLK data never sets Mastery;
// Cata data will never set ArmorPenetration or Defense).
//
// Provenance: the first 35 entries are the WoWSims WotLK proto order,
// the founding upstream. Upstream positional arrays are nevertheless
// decoded through an explicit per-upstream order table (see
// wowsimsWotlkStatOrder) — a future upstream with a different proto
// layout plugs in its own table and fills the same canonical Stats.
type Stat int

const (
	Strength Stat = iota
	Agility
	Stamina
	Intellect
	Spirit
	SpellPower
	MP5
	SpellHit
	SpellCrit
	SpellHaste
	SpellPenetration
	AttackPower
	MeleeHit
	MeleeCrit
	MeleeHaste
	ArmorPenetration
	Expertise
	Mana
	Energy
	Rage
	Armor
	RangedAttackPower
	Defense
	Block
	BlockValue
	Dodge
	Parry
	Resilience
	Health
	ArcaneResistance
	FireResistance
	FrostResistance
	NatureResistance
	ShadowResistance
	BonusArmor
	Mastery // Cataclysm onward; never set by WotLK data
	NumStats
)

var statNames = [NumStats]string{
	"Strength", "Agility", "Stamina", "Intellect", "Spirit",
	"Spell Power", "MP5", "Spell Hit", "Spell Crit", "Spell Haste",
	"Spell Penetration", "Melee AP", "Melee Hit", "Melee Crit", "Melee Haste",
	"Armor Penetration", "Expertise", "Mana", "Energy", "Rage",
	"Armor", "Ranged AP", "Defense", "Block", "Block Value",
	"Dodge", "Parry", "Resilience", "Health", "Arcane Resistance",
	"Fire Resistance", "Frost Resistance", "Nature Resistance",
	"Shadow Resistance", "Bonus Armor", "Mastery",
}

func (s Stat) String() string {
	if s < 0 || s >= NumStats {
		return fmt.Sprintf("Stat(%d)", int(s))
	}
	return statNames[s]
}

// Stats holds one value per Stat, indexed by the Stat constants.
type Stats [NumStats]float64

// Reforge is one applied reforge: Amount rating points moved from the
// From stat to the To stat (Cataclysm onward; the game moves 40% of
// the item's native From stat, floored — the wire carries the exact
// amount, so no math is repeated here).
type Reforge struct {
	From, To Stat
	Amount   float64
}

// itemModStats maps the game's ITEM_MOD_* ids — the vocabulary armory
// APIs speak in reforge descriptors — to canonical Stats. Only the
// reforgeable stats are listed (reforging touches nothing else); the
// unified cata ratings land in the canonical MELEE slots, matching how
// cata item data decodes (see cataStatOrder).
var itemModStats = map[int]Stat{
	6:  Spirit,
	13: Dodge,
	14: Parry,
	31: MeleeHit,
	32: MeleeCrit,
	36: MeleeHaste,
	37: Expertise,
	49: Mastery,
}

// StatByItemMod resolves an ITEM_MOD_* id to its canonical Stat.
func StatByItemMod(id int) (Stat, bool) {
	s, ok := itemModStats[id]
	return s, ok
}

// wowsimsWotlkStatOrder maps WoWSims-WotLK proto stat indices — the
// positions of the embedded db.json's stat arrays — to canonical Stats.
// For this founding upstream the mapping is the identity, but it is
// spelled out so decoding never silently tracks the canonical enum as
// it grows: a different upstream (WoWSims Cata) lays its arrays out
// differently and must bring its own order table.
var wowsimsWotlkStatOrder = [...]Stat{
	Strength, Agility, Stamina, Intellect, Spirit,
	SpellPower, MP5, SpellHit, SpellCrit, SpellHaste,
	SpellPenetration, AttackPower, MeleeHit, MeleeCrit, MeleeHaste,
	ArmorPenetration, Expertise, Mana, Energy, Rage,
	Armor, RangedAttackPower, Defense, Block, BlockValue,
	Dodge, Parry, Resilience, Health, ArcaneResistance,
	FireResistance, FrostResistance, NatureResistance,
	ShadowResistance, BonusArmor,
}

// UnmarshalJSON decodes a WoWSims-WotLK positional stat array. Arrays
// of any length are accepted (the source data occasionally carries
// fewer entries); entries past the known order are dropped rather than
// guessed at. A different game version's database must NOT be decoded
// with this method — each upstream needs its own order table.
func (st *Stats) UnmarshalJSON(data []byte) error {
	var raw []float64
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	for i, v := range raw {
		if i < len(wowsimsWotlkStatOrder) {
			st[wowsimsWotlkStatOrder[i]] = v
		}
	}
	return nil
}

// Slot is the equipment slot an item goes into (WoWSims ItemType enum).
type Slot int

const (
	SlotUnknown Slot = iota
	SlotHead
	SlotNeck
	SlotShoulder
	SlotBack
	SlotChest
	SlotWrist
	SlotHands
	SlotWaist
	SlotLegs
	SlotFeet
	SlotFinger
	SlotTrinket
	SlotWeapon
	SlotRanged
	NumSlots
)

var slotNames = [NumSlots]string{
	"?", "Head", "Neck", "Shoulder", "Back", "Chest", "Wrist", "Hands",
	"Waist", "Legs", "Feet", "Finger", "Trinket", "Weapon", "Ranged",
}

func (s Slot) String() string {
	if s < 0 || s >= NumSlots {
		return fmt.Sprintf("Slot(%d)", int(s))
	}
	return slotNames[s]
}

// SlotByName resolves a canonical slot name ("Trinket") back to the
// enum; SlotUnknown for anything else.
func SlotByName(name string) Slot {
	for i, n := range slotNames {
		if n == name && Slot(i) != SlotUnknown {
			return Slot(i)
		}
	}
	return SlotUnknown
}

// ArmorType mirrors the WoWSims ArmorType enum.
type ArmorType int

const (
	ArmorAny ArmorType = iota
	ArmorCloth
	ArmorLeather
	ArmorMail
	ArmorPlate
)

// WeaponType mirrors the WoWSims WeaponType enum.
type WeaponType int

const (
	WeaponUnknown WeaponType = iota
	WeaponAxe
	WeaponDagger
	WeaponFist
	WeaponMace
	WeaponOffHand
	WeaponPolearm
	WeaponShield
	WeaponStaff
	WeaponSword
)

// HandType mirrors the WoWSims HandType enum: which hand(s) a weapon
// occupies. Shields and held-in-off-hand frills are HandOffHand items
// with WeaponType Shield / OffHand.
type HandType int

const (
	HandUnknown  HandType = iota
	HandMainHand          // main hand only
	HandOneHand           // either hand
	HandOffHand           // off hand only
	HandTwoHand
)

// RangedWeaponType mirrors the WoWSims-WotLK RangedWeaponType enum,
// with Cataclysm's unified relic appended (Cata merged Idol/Libram/
// Totem/Sigil and re-numbered its enum — cata data decodes through
// cataRangedOrder, never positionally).
type RangedWeaponType int

const (
	RangedUnknown RangedWeaponType = iota
	RangedBow
	RangedCrossbow
	RangedGun
	RangedIdol
	RangedLibram
	RangedThrown
	RangedTotem
	RangedWand
	RangedSigil
	RangedRelic // Cataclysm onward; never set by WotLK data
)

// GemColor mirrors the WoWSims GemColor enum (identical in WotLK and
// Cata; Cata appends Cogwheel).
type GemColor int

const (
	GemUnknown GemColor = iota
	GemMeta
	GemRed
	GemBlue
	GemYellow
	GemGreen
	GemOrange
	GemPurple
	GemPrismatic
	GemCogwheel // Cataclysm onward (engineering goggles); never set by WotLK data
)

// ColorMatches reports whether a gem of color g satisfies a socket of
// color socket (the game rule used for socket bonuses).
func ColorMatches(socket, g GemColor) bool {
	switch socket {
	case GemMeta:
		return g == GemMeta
	case GemRed:
		return g == GemRed || g == GemOrange || g == GemPurple || g == GemPrismatic
	case GemYellow:
		return g == GemYellow || g == GemOrange || g == GemGreen || g == GemPrismatic
	case GemBlue:
		return g == GemBlue || g == GemPurple || g == GemGreen || g == GemPrismatic
	case GemPrismatic:
		return g != GemMeta && g != GemUnknown && g != GemCogwheel
	case GemCogwheel:
		return g == GemCogwheel
	}
	return false
}

var gemColorNames = map[GemColor]string{
	GemUnknown: "?", GemMeta: "Meta", GemRed: "Red", GemBlue: "Blue",
	GemYellow: "Yellow", GemGreen: "Green", GemOrange: "Orange",
	GemPurple: "Purple", GemPrismatic: "Prismatic", GemCogwheel: "Cogwheel",
}

func (g GemColor) String() string {
	if n, ok := gemColorNames[g]; ok {
		return n
	}
	return fmt.Sprintf("GemColor(%d)", int(g))
}

// epicGemFamily names the 3.3.5 epic jewelcrafting gem per color. The
// planner assumes sockets are filled from this tier (the exact cut within
// a family is chosen by stat weights). Metas have no epic tier in WotLK;
// the rare-quality diamonds are the endgame standard there.
var epicGemFamily = map[GemColor]string{
	GemRed:    "Cardinal Ruby",
	GemYellow: "King's Amber",
	GemBlue:   "Majestic Zircon",
	GemOrange: "Ametrine",
	GemPurple: "Dreadstone",
	GemGreen:  "Eye of Zul",
	GemMeta:   "Earthsiege/Skyflare Diamond",
}

// EpicFamily returns the assumed endgame gem family for the color, or ""
// for colors that have none (e.g. prismatic sockets take any gem).
func (g GemColor) EpicFamily() string {
	return epicGemFamily[g]
}

// Class mirrors the WoWSims Class enum.
type Class int

const (
	ClassUnknown Class = iota
	ClassDruid
	ClassHunter
	ClassMage
	ClassPaladin
	ClassPriest
	ClassRogue
	ClassShaman
	ClassWarlock
	ClassWarrior
	ClassDeathKnight
)

var classNames = map[string]Class{
	"Druid": ClassDruid, "Hunter": ClassHunter, "Mage": ClassMage,
	"Paladin": ClassPaladin, "Priest": ClassPriest, "Rogue": ClassRogue,
	"Shaman": ClassShaman, "Warlock": ClassWarlock, "Warrior": ClassWarrior,
	"Death Knight": ClassDeathKnight, "Deathknight": ClassDeathKnight,
}

// ClassByName maps a display name (as the armory exports it) to a
// Class, returning ClassUnknown for names it does not recognize.
func ClassByName(name string) Class {
	return classNames[name]
}

var classSlugs = map[Class]string{
	ClassDruid: "druid", ClassHunter: "hunter", ClassMage: "mage",
	ClassPaladin: "paladin", ClassPriest: "priest", ClassRogue: "rogue",
	ClassShaman: "shaman", ClassWarlock: "warlock", ClassWarrior: "warrior",
	ClassDeathKnight: "death_knight",
}

// Slug returns the class as a stable machine identifier for config keys
// (never shown to the user, safe against display-name changes).
func (c Class) Slug() string {
	if s, ok := classSlugs[c]; ok {
		return s
	}
	return "unknown"
}

// FactionRestriction mirrors the WoWSims UIItem faction enum. Faction
// mirror-loot (e.g. Trial of the Crusader drops) exists as two items with
// different names, one per faction.
type FactionRestriction int

const (
	FactionAny FactionRestriction = iota
	FactionAllianceOnly
	FactionHordeOnly
)

// Quality is the item quality (0 poor ... 5 legendary).
type Quality int

const (
	QualityPoor Quality = iota
	QualityCommon
	QualityUncommon
	QualityRare
	QualityEpic
	QualityLegendary
)

// Source describes one way to obtain an item. Exactly one field is set.
type Source struct {
	Crafted *CraftedSource `json:"crafted,omitempty"`
	Drop    *DropSource    `json:"drop,omitempty"`
	Quest   *QuestSource   `json:"quest,omitempty"`
	SoldBy  *SoldBySource  `json:"soldBy,omitempty"`
	Rep     *RepSource     `json:"rep,omitempty"`
}

// RepSource is a reputation-vendor route: the item sells at a faction
// quartermaster once the given standing is reached. The cata upstream
// carries these natively (115 items); the wotlk data instead attributes
// quartermaster gear as plain SoldBy vendors.
type RepSource struct {
	FactionID int `json:"repFactionId"`
	Level     int `json:"repLevel"` // client ReputationRank enum, 8 = Exalted
}

// repFactionNames covers the factions that gate gear in the cata pool.
var repFactionNames = map[int]string{
	1135: "The Earthen Ring",
	1158: "Guardians of Hyjal",
	1171: "Therazane",
	1172: "Dragonmaw Clan",
	1173: "Ramkahen",
	1174: "Wildhammer Clan",
	1177: "Baradin's Wardens",
	1178: "Hellscream's Reach",
	1204: "Avengers of Hyjal",
}

// FactionName resolves the faction's display name ("" when uncurated —
// callers fall back to a generic label rather than inventing one).
func (r *RepSource) FactionName() string { return repFactionNames[r.FactionID] }

var repStandings = [...]string{"", "Hated", "Hostile", "Unfriendly",
	"Neutral", "Friendly", "Honored", "Revered", "Exalted"}

// Standing names the required reputation rank ("Exalted").
func (r *RepSource) Standing() string {
	if r.Level > 0 && r.Level < len(repStandings) {
		return repStandings[r.Level]
	}
	return ""
}

type CraftedSource struct {
	Profession int `json:"profession"`
	SpellID    int `json:"spellId"`
}

// ProfessionName returns the crafting profession's display name.
func (c *CraftedSource) ProfessionName() string {
	names := [...]string{"?", "Alchemy", "Blacksmithing", "Enchanting",
		"Engineering", "Herbalism", "Inscription", "Jewelcrafting",
		"Leatherworking", "Mining", "Skinning", "Tailoring"}
	if c.Profession > 0 && c.Profession < len(names) {
		return names[c.Profession]
	}
	return names[0]
}

type DropSource struct {
	NPCID      int    `json:"npcId"`
	ZoneID     int    `json:"zoneId"`
	Difficulty int    `json:"difficulty"`
	Category   string `json:"category"`
	OtherName  string `json:"otherName"`
}

// DifficultyLabel is the short display form of the WoWSims
// DungeonDifficulty enum ("" for normal 5-mans and unknown).
func (d *DropSource) DifficultyLabel() string {
	switch d.Difficulty {
	case 2:
		return "HC"
	case 3:
		return "10"
	case 4:
		return "10H"
	case 5:
		return "25"
	case 6:
		return "25H"
	}
	return ""
}

// IsRaidZone reports whether the zone is a raid instance.
func (d *Database) IsRaidZone(zoneID int) bool {
	return d.Rules.RaidZones[d.Zones[zoneID]]
}

// IsRaidDrop reports whether the source is a drop inside a raid.
func (d *Database) IsRaidDrop(s Source) bool {
	if s.Drop == nil {
		return false
	}
	return (s.Drop.Difficulty >= 3 && s.Drop.Difficulty <= 6) || d.IsRaidZone(s.Drop.ZoneID)
}

type QuestSource struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type SoldBySource struct {
	NPCID   int    `json:"npcId"`
	NPCName string `json:"npcName"`
	ZoneID  int    `json:"zoneId"`
}

// Item is one equippable item.
type Item struct {
	ID             int                `json:"id"`
	Name           string             `json:"name"`
	Icon           string             `json:"icon"`
	Slot           Slot               `json:"type"`
	ArmorType      ArmorType          `json:"armorType"`
	WeaponType     WeaponType         `json:"weaponType"`
	HandType       HandType           `json:"handType"`
	RangedType     RangedWeaponType   `json:"rangedWeaponType"`
	Stats          Stats              `json:"stats"`
	WeaponDmgMin   float64            `json:"weaponDamageMin"`
	WeaponDmgMax   float64            `json:"weaponDamageMax"`
	WeaponSpeed    float64            `json:"weaponSpeed"`
	GemSockets     []GemColor         `json:"gemSockets"`
	SocketBonus    Stats              `json:"socketBonus"`
	ILvl           int                `json:"ilvl"`
	Phase          int                `json:"phase"`
	Quality        Quality            `json:"quality"`
	Unique         bool               `json:"unique"`
	Heroic         bool               `json:"heroic"`
	SetName        string             `json:"setName"`
	ClassAllowlist []Class            `json:"classAllowlist"`
	Faction        FactionRestriction `json:"factionRestriction"`
	Sources        []Source           `json:"sources"`
	// Suffix names a random-suffix variant's roll ("of the Wind"); ""
	// for ordinary items and for the stat-less base (see suffix.go).
	Suffix   string `json:"-"`
	SuffixID int    `json:"-"` // the roll's random-suffix id (negative, as in the dump)
}

// UsableByFaction reports whether the item is available to a faction,
// given as the armory exports it ("Alliance"/"Horde"). Unknown faction
// strings allow everything.
func (i *Item) UsableByFaction(faction string) bool {
	switch i.Faction {
	case FactionAllianceOnly:
		return faction != "Horde"
	case FactionHordeOnly:
		return faction != "Alliance"
	}
	return true
}

// UsableBy reports whether the item is available to the given class.
func (i *Item) UsableBy(c Class) bool {
	if len(i.ClassAllowlist) == 0 {
		return true
	}
	for _, a := range i.ClassAllowlist {
		if a == c {
			return true
		}
	}
	return false
}

// Gem is a socketable gem.
type Gem struct {
	ID      int      `json:"id"`
	Name    string   `json:"name"`
	Icon    string   `json:"icon"`
	Color   GemColor `json:"color"`
	Stats   Stats    `json:"stats"`
	Phase   int      `json:"phase"`
	Quality Quality  `json:"quality"`
	Unique  bool     `json:"unique"` // unique-equipped: at most one worn (Nightmare Tear)
	// RequiredProfession locks the gem to a profession (CraftedSource
	// enum; 7 = the jewelcrafter-only Dragon's Eye / Chimera's Eye).
	RequiredProfession int `json:"requiredProfession"`
}

// Enchant is a permanent item enhancement.
type Enchant struct {
	EffectID       int     `json:"effectId"`
	ItemID         int     `json:"itemId"`
	SpellID        int     `json:"spellId"`
	Name           string  `json:"name"`
	Slot           Slot    `json:"type"`
	EnchantType    int     `json:"enchantType"`
	Stats          Stats   `json:"stats"`
	Quality        Quality `json:"quality"`
	ClassAllowlist []Class `json:"classAllowlist"`
	// Phase is the content phase the enchant arrives in (0 = launch).
	// The upstream carries no availability data on enchants — their
	// carrier items aren't gear, so the phase overlay never reaches
	// them; post-launch arrivals are curated per loader.
	Phase int `json:"-"`
	// RequiredProfession locks the enchant to its own profession
	// (CraftedSource enum: ring enchants 3 Enchanting, tinkers 4
	// Engineering, embroideries 11 Tailoring, ...).
	RequiredProfession int `json:"requiredProfession"`
	// Trigger is what a curated proc enchant needs the wearer to do for
	// its averaged Stats to be real: "melee" (weapon PPM procs) or
	// "spell" (damaging spells). Empty = no restriction.
	Trigger string `json:"-"`
}

// Database is the parsed item database.
// ConsumableKind separates the buffs a character can carry at once:
// one food and one flask.
type ConsumableKind int

const (
	ConsumableFood ConsumableKind = iota
	ConsumableFlask
)

// Consumable is one food or flask, with the stats it grants. Kept in
// the item database's shape (not the scorer's) because a version whose
// dump carries the table parses straight into it.
type Consumable struct {
	ID    int
	Name  string
	Kind  ConsumableKind
	Stats Stats
}

type Database struct {
	Items    map[int]*Item
	Gems     map[int]*Gem
	Enchants []*Enchant
	Zones    map[int]string
	NPCs     map[int]string
	// Effects holds special item effects by item ID: proc/on-use data
	// extracted from WoWSims sim sources, plus curated approximations
	// for custom-coded items.
	Effects map[int][]*Effect

	// Variants holds each random-suffix item's rolls by item ID (see
	// suffix.go); Pool lists them in place of the stat-less base.
	Variants map[int][]*Item
	pool     []*Item // cached Pool, set by the loaders

	// Consumables lists the version's assumable food and flasks, where
	// the version's data carries them (Cataclysm's db.json does; the
	// WotLK dump has no such table and the score package curates one
	// from the sim source instead).
	Consumables []Consumable

	// Rules is the game version's classification table set, attached by
	// the version's loader (see RuleSet).
	Rules RuleSet

	// Protocol maps items obtainable through the version's heroic-plus
	// dungeon economy (Cataclysm Classic's Elemental Rune protocols) to
	// that route's price and availability; nil for versions without one.
	Protocol map[int]ProtocolPrice

	enchantByEffect map[int][]*Enchant // lazy index; rebuilt per process
}

// ProtocolFor reports the heroic-plus dungeon route of an item: the
// fragment price (Count 0 when the vendor stocks it but the exact cost
// is not in our data), the phase the route opens, and whether the item
// also DROPS inside protocol runs (Dragon Soul LFR gear).
func (d *Database) ProtocolFor(itemID int) (ProtocolPrice, bool) {
	p, ok := d.Protocol[itemID]
	return p, ok
}

// ResolveGem returns the gem for an ID that may be either a gem item ID
// (our database's keys) or a gem SpellItemEnchantment ID (how armory
// item links encode socketed gems).
func (d *Database) ResolveGem(id int) *Gem {
	if g := d.Gems[id]; g != nil {
		return g
	}
	if item, ok := gemEnchantToItem[id]; ok {
		return d.Gems[item]
	}
	return nil
}

// EnchantByEffect returns the enchant with the given enchantment effect
// ID (the "ench=" value in armory item links), or nil. Upstream lists
// one effect under several slot-specific catalog names ("Major Agility"
// on hands is effect 3222, same as "Greater Agility" on 2H weapons —
// identical stats), so the entry matching the enchanted item's slot is
// preferred; any variant is a fallback.
func (d *Database) EnchantByEffect(effectID int, slot Slot) *Enchant {
	if effectID == 0 {
		return nil
	}
	if d.enchantByEffect == nil {
		d.enchantByEffect = make(map[int][]*Enchant, len(d.Enchants))
		for _, e := range d.Enchants {
			d.enchantByEffect[e.EffectID] = append(d.enchantByEffect[e.EffectID], e)
		}
	}
	variants := d.enchantByEffect[effectID]
	for _, e := range variants {
		if e.Slot == slot {
			return e
		}
	}
	if len(variants) > 0 {
		return variants[0]
	}
	return nil
}

// classicOnlyVendors sell for currencies that never existed on original
// 3.3.5 (WotLK-Classic Sidereal Essence / Defiler's Scourgestone). Every
// item they carry also has a real drop source, so stripping these loses
// nothing.
var classicOnlyVendors = map[string]bool{
	"Animated Constellation": true,
	"Korralin Hoperender":    true,
	"Kolara Dreamsmasher":    true,
}

// drop335Irrelevant removes sources that don't exist on original 3.3.5
// servers: WoWSims-Classic "Titan Rune" dungeon difficulties (7, 8) and
// Classic-only currency vendors.
func drop335Irrelevant(sources []Source) []Source {
	kept := sources[:0]
	for _, s := range sources {
		if s.Drop != nil && (s.Drop.Difficulty == 7 || s.Drop.Difficulty == 8) {
			continue
		}
		if s.SoldBy != nil && classicOnlyVendors[s.SoldBy.NPCName] {
			continue
		}
		kept = append(kept, s)
	}
	return kept
}

// dropFakeBadgeSales removes nameless Dalaran vendor entries from items
// that are not badge-buyable per the generated 3.3.5 emblem price table.
// Those entries are WotLK-Classic currency sales (e.g. Ulduar hard-mode
// gear for Sidereal Essence) that never existed on 3.3.5. Only applied
// when the item keeps another source, so nothing ends up unexplained.
func dropFakeBadgeSales(it *Item) []Source {
	const dalaran = 4395
	if _, isEmblem := emblemPrices[it.ID]; isEmblem || len(it.Sources) < 2 {
		return it.Sources
	}
	kept := it.Sources[:0]
	for _, s := range it.Sources {
		if s.SoldBy != nil && s.SoldBy.NPCName == "" && s.SoldBy.ZoneID == dalaran {
			continue
		}
		kept = append(kept, s)
	}
	if len(kept) == 0 {
		return it.Sources
	}
	return kept
}

// Parse decodes the raw WoWSims db.json document.
func Parse(raw []byte) (*Database, error) {
	var doc struct {
		Items    []*Item    `json:"items"`
		Gems     []*Gem     `json:"gems"`
		Enchants []*Enchant `json:"enchants"`
		Zones    []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"zones"`
		NPCs []struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"npcs"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parsing item database: %w", err)
	}
	d := &Database{
		Items:    make(map[int]*Item, len(doc.Items)),
		Gems:     make(map[int]*Gem, len(doc.Gems)),
		Enchants: doc.Enchants,
		Zones:    make(map[int]string, len(doc.Zones)),
		NPCs:     make(map[int]string, len(doc.NPCs)),
	}
	for _, it := range doc.Items {
		it.Sources = drop335Irrelevant(it.Sources)
		it.Sources = dropFakeBadgeSales(it)
		d.Items[it.ID] = it
	}
	for _, g := range doc.Gems {
		d.Gems[g.ID] = g
	}
	for _, z := range doc.Zones {
		d.Zones[z.ID] = z.Name
	}
	for _, n := range doc.NPCs {
		d.NPCs[n.ID] = n.Name
	}
	return d, nil
}
