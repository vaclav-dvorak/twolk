package db

import (
	"strings"
	"testing"
)

// fixture is a verbatim excerpt of the wowsims db.json document.
const fixture = `{
  "items": [
    {"id": 33900, "name": "Vindicator's Mooncloth Belt", "icon": "inv_belt_17",
     "type": 8, "armorType": 1,
     "stats": [0,0,49,31,0,41,0,0,0,0,0,0,0,0,0,0,0,0,0,0,138,0,0,0,0,0,0,31,0,0,0,0,0,0,70],
     "socketBonus": [0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0],
     "ilvl": 146, "phase": 1, "quality": 4, "classAllowlist": [5, 3, 8]},
    {"id": 45132, "name": "Golden Saronite Dragon", "icon": "x", "type": 2,
     "stats": [], "socketBonus": [], "ilvl": 252, "quality": 4, "gemSockets": [4],
     "sources": [{"drop": {"difficulty": 5, "npcId": 33113, "zoneId": 4273, "category": "Hard Mode"}}]}
  ],
  "gems": [
    {"id": 40002, "name": "Fractured Scarlet Ruby", "icon": "inv_jewelcrafting_gem_28",
     "color": 2,
     "stats": [0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,16,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0],
     "phase": 1, "quality": 3}
  ],
  "enchants": [
    {"effectId": 2654, "itemId": 22539, "spellId": 27945, "name": "Intellect",
     "type": 13, "enchantType": 2,
     "stats": [0,0,0,12,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0],
     "quality": 1}
  ],
  "zones": [{"id": 4273, "name": "Ulduar", "expansion": 3}],
  "npcs": [{"id": 33113, "name": "Flame Leviathan", "zoneId": 4273}]
}`

func TestParse(t *testing.T) {
	d, err := Parse([]byte(fixture))
	if err != nil {
		t.Fatal(err)
	}

	belt := d.Items[33900]
	if belt == nil {
		t.Fatal("belt not found")
	}
	if belt.Slot != SlotWaist {
		t.Errorf("belt slot = %v, want Waist", belt.Slot)
	}
	if belt.Stats[Stamina] != 49 || belt.Stats[SpellPower] != 41 || belt.Stats[Resilience] != 31 {
		t.Errorf("belt stats wrong: stam=%v sp=%v resil=%v", belt.Stats[Stamina], belt.Stats[SpellPower], belt.Stats[Resilience])
	}
	if belt.UsableBy(ClassHunter) {
		t.Error("cloth caster belt must not be usable by hunter")
	}
	if !belt.UsableBy(ClassMage) {
		t.Error("cloth caster belt must be usable by mage")
	}

	// short stats arrays (here: empty) must parse as all-zero
	neck := d.Items[45132]
	if neck == nil || neck.Stats[Stamina] != 0 {
		t.Fatal("neck with empty stats array must parse as zero stats")
	}
	if len(neck.GemSockets) != 1 || neck.GemSockets[0] != GemYellow {
		t.Errorf("neck sockets = %v, want one yellow", neck.GemSockets)
	}
	drop := neck.Sources[0].Drop
	if drop == nil || d.NPCs[drop.NPCID] != "Flame Leviathan" || d.Zones[drop.ZoneID] != "Ulduar" {
		t.Errorf("drop source not resolved: %+v", drop)
	}

	gem := d.Gems[40002]
	if gem == nil || gem.Color != GemRed || gem.Stats[ArmorPenetration] != 16 {
		t.Errorf("gem wrong: %+v", gem)
	}

	if len(d.Enchants) != 1 || d.Enchants[0].Stats[Intellect] != 12 || d.Enchants[0].Slot != SlotWeapon {
		t.Errorf("enchant wrong: %+v", d.Enchants[0])
	}
}

// The full wotlk load extracts stacking buffs from the vendored
// stat_bonus_stacking.go — absent entirely before that file was
// vendored (Herkuml War Token, the Frost-emblem trinket, scored by its
// passive haste alone).
func TestLoadStackingEffects(t *testing.T) {
	d, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	herkuml := d.Effects[50355]
	if len(herkuml) != 1 || herkuml[0].Kind != EffectProc ||
		herkuml[0].MaxStacks != 20 || herkuml[0].Bonus[AttackPower] != 17 {
		t.Fatalf("Herkuml War Token stacking effect wrong: %+v", herkuml)
	}
}

func TestDrop335Irrelevant(t *testing.T) {
	src := []Source{
		{Drop: &DropSource{NPCID: 1, Difficulty: 5}},
		{Drop: &DropSource{NPCID: 1, Difficulty: 8}},               // titan rune → out
		{SoldBy: &SoldBySource{NPCName: "Animated Constellation"}}, // classic vendor → out
		{SoldBy: &SoldBySource{NPCName: "Harold Winston"}},
	}
	got := drop335Irrelevant(src)
	if len(got) != 2 {
		t.Fatalf("kept %d sources, want 2: %+v", len(got), got)
	}
	if got[0].Drop == nil || got[0].Drop.Difficulty != 5 || got[1].SoldBy == nil || got[1].SoldBy.NPCName != "Harold Winston" {
		t.Fatalf("wrong sources kept: %+v", got)
	}
}

func TestIsRaidDrop(t *testing.T) {
	d := &Database{Zones: map[int]string{3456: "Naxxramas", 206: "Utgarde Keep"}, Rules: WotlkRules()}
	// T7 raids use dungeon-style difficulty 1 — zone must decide
	naxx := Source{Drop: &DropSource{ZoneID: 3456, Difficulty: 1}}
	if !d.IsRaidDrop(naxx) {
		t.Error("Naxxramas difficulty-1 drop must classify as raid")
	}
	dungeon := Source{Drop: &DropSource{ZoneID: 206, Difficulty: 2}}
	if d.IsRaidDrop(dungeon) {
		t.Error("Utgarde heroic drop must not classify as raid")
	}
	icc := Source{Drop: &DropSource{ZoneID: 9999, Difficulty: 5}}
	if !d.IsRaidDrop(icc) {
		t.Error("25-man difficulty drop must classify as raid regardless of zone")
	}
	if d.IsRaidDrop(Source{SoldBy: &SoldBySource{}}) {
		t.Error("vendor source is not a raid drop")
	}
}

func TestEmblemPrices(t *testing.T) {
	// known 3.3.5 prices from the generated table
	if p, ok := EmblemPriceFor(40684); !ok || p.Emblem != EmblemHeroism || p.Count != 40 {
		t.Errorf("Mirror of Truth price = %+v, want 40x Heroism", p)
	}
	if _, ok := EmblemPriceFor(48954); ok {
		t.Error("gold-sold Kirin Tor ring must not be in the emblem table")
	}
	if EmblemFrost.String() != "Emblem of Frost" {
		t.Errorf("emblem name = %q", EmblemFrost.String())
	}

	// overlay: an emblem-buyable item with no vendor source gets one
	noose := &Item{ID: 50995, Name: "Vengeful Noose"}
	d := &Database{Items: map[int]*Item{noose.ID: noose}}
	d.applyCurated()
	if len(noose.Sources) != 1 || noose.Sources[0].SoldBy == nil {
		t.Fatalf("emblem overlay did not add vendor source: %+v", noose.Sources)
	}
	// idempotent: applying again must not duplicate
	d.applyCurated()
	if len(noose.Sources) != 1 {
		t.Fatalf("overlay not idempotent: %+v", noose.Sources)
	}
}

func TestDropFakeBadgeSales(t *testing.T) {
	// Drape of Icy Intent: real Hodir drop + a nameless Dalaran "sale"
	// (Classic Sidereal Essence) that must go
	drape := &Item{ID: 45461, Sources: []Source{
		{Drop: &DropSource{NPCID: 1, ZoneID: 4273, Difficulty: 5}},
		{SoldBy: &SoldBySource{ZoneID: 4395}},
	}}
	drape.Sources = dropFakeBadgeSales(drape)
	if len(drape.Sources) != 1 || drape.Sources[0].Drop == nil {
		t.Fatalf("fake badge sale not stripped: %+v", drape.Sources)
	}

	// a real emblem item keeps its nameless Dalaran vendor source
	mirror := &Item{ID: 40684, Sources: []Source{
		{SoldBy: &SoldBySource{ZoneID: 4395}},
		{Drop: &DropSource{NPCID: 2, ZoneID: 1, Difficulty: 2}},
	}}
	mirror.Sources = dropFakeBadgeSales(mirror)
	if len(mirror.Sources) != 2 {
		t.Fatalf("emblem item vendor source must survive: %+v", mirror.Sources)
	}

	// an item whose only source is a nameless Dalaran vendor is left alone
	waraxe := &Item{ID: 42236, Sources: []Source{{SoldBy: &SoldBySource{ZoneID: 4395}}}}
	waraxe.Sources = dropFakeBadgeSales(waraxe)
	if len(waraxe.Sources) != 1 {
		t.Fatalf("sole vendor source must survive: %+v", waraxe.Sources)
	}
}

func TestStatFixes(t *testing.T) {
	// Conqueror's Scourgestalker Headpiece: Classic-buffed to 232 with
	// 87 agi; original 3.3.5 is 226 with 82 agi (verified vs wotlkdb
	// and Rising-Gods)
	helm := &Item{ID: 46143, ILvl: 232}
	helm.Stats[Agility] = 87
	d := &Database{Items: map[int]*Item{helm.ID: helm}, Zones: map[int]string{}, NPCs: map[int]string{}}
	d.applyCurated()
	if helm.ILvl != 226 || helm.Stats[Agility] != 82 || helm.Stats[Armor] != 1067 {
		t.Fatalf("stat fix not applied: ilvl=%d agi=%v armor=%v",
			helm.ILvl, helm.Stats[Agility], helm.Stats[Armor])
	}
	// non-divergent items (ICC) must have no fix entry
	if _, ok := statFixes[50362]; ok {
		t.Error("Deathbringer's Will must not be in the fix table")
	}
	// tank items with equip-spell block value were skipped, not zeroed
	if _, ok := statFixes[46162]; ok {
		t.Error("equip-spell items must be skipped, not lossily fixed")
	}
}

func TestEnchantAverages(t *testing.T) {
	// proc/on-use profession enchants ship with EMPTY stats upstream;
	// the curated averages must fill them at load
	hyper := &Enchant{EffectID: 3604, Name: "Hyperspeed Accelerators", Slot: SlotHands}
	light := &Enchant{EffectID: 3722, Name: "Lightweave Embroidery", Slot: SlotBack}
	// an enchant with real upstream stats must never be overwritten
	real := &Enchant{EffectID: 3728, Name: "Darkglow Embroidery", Slot: SlotBack}
	real.Stats[MP5] = 99
	d := &Database{Enchants: []*Enchant{hyper, light, real},
		Items: map[int]*Item{}, Zones: map[int]string{}, NPCs: map[int]string{}}
	d.applyCurated()
	if hyper.Stats[MeleeHaste] != 68 || hyper.Stats[SpellHaste] != 68 {
		t.Errorf("Hyperspeed haste = %v/%v, want 68/68",
			hyper.Stats[MeleeHaste], hyper.Stats[SpellHaste])
	}
	if light.Stats[SpellPower] != 87 {
		t.Errorf("Lightweave spell power = %v, want 87", light.Stats[SpellPower])
	}
	if real.Stats[MP5] != 99 {
		t.Errorf("upstream stats must win over the curated average: %v", real.Stats[MP5])
	}
	// damage procs stay untracked, same stance as the capacitor trinkets
	if _, ok := enchantAverages[3603]; ok {
		t.Error("Hand-Mounted Pyro Rocket must not be valued (damage proc)")
	}
}

// TestEnchantByEffectPrefersSlot: upstream catalogs one enchantment
// effect under several slot-specific names (effect 3222 is "Major
// Agility" on hands AND "Greater Agility" on 2H weapons); resolution
// must return the enchanted item's slot variant, not an arbitrary one.
func TestEnchantByEffectPrefersSlot(t *testing.T) {
	d := &Database{Enchants: []*Enchant{
		{EffectID: 3222, Name: "Greater Agility", Slot: SlotWeapon},
		{EffectID: 3222, Name: "Major Agility", Slot: SlotHands},
	}}
	if e := d.EnchantByEffect(3222, SlotHands); e == nil || e.Name != "Major Agility" {
		t.Errorf("hands variant = %+v, want Major Agility", e)
	}
	if e := d.EnchantByEffect(3222, SlotWeapon); e == nil || e.Name != "Greater Agility" {
		t.Errorf("weapon variant = %+v, want Greater Agility", e)
	}
	if e := d.EnchantByEffect(3222, SlotFeet); e == nil {
		t.Error("no slot match must still fall back to some variant")
	}
	if e := d.EnchantByEffect(9999, SlotHands); e != nil {
		t.Errorf("unknown effect = %+v, want nil", e)
	}
}

func TestQuestNameFixes(t *testing.T) {
	// Quel'Delar, Ferocity of the Scorned ships with a nameless quest
	// source; the curated table must fill it
	quel := &Item{ID: 50049, Sources: []Source{{Quest: &QuestSource{}}}}
	ring := &Item{ID: 50402, Sources: []Source{{Quest: &QuestSource{}}}}
	named := &Item{ID: 50046, Sources: []Source{{Quest: &QuestSource{Name: "Already Named"}}}}
	d := &Database{Items: map[int]*Item{quel.ID: quel, ring.ID: ring, named.ID: named},
		Zones: map[int]string{}, NPCs: map[int]string{}}
	d.applyCurated()
	if got := quel.Sources[0].Quest.Name; !strings.Contains(got, "Sunreavers") {
		t.Errorf("Quel'Delar quest name = %q", got)
	}
	if got := ring.Sources[0].Quest.Name; got != "Ashen Verdict ring (Exalted)" {
		t.Errorf("Ashen ring quest name = %q", got)
	}
	if named.Sources[0].Quest.Name != "Already Named" {
		t.Error("existing quest names must not be overwritten")
	}
}

func TestSupplementalSources(t *testing.T) {
	// 45704 Shawl of the Shattered Giant: upstream only has a Titan Rune
	// source (stripped), the generated table must restore its true 3.3.5
	// origin as a drop
	gs, ok := supplementalSources[45704]
	if !ok {
		t.Fatal("Shawl of the Shattered Giant missing from generated sources")
	}
	if gs.Kind != "object" || gs.Name != "Cache of Living Stone" {
		t.Fatalf("shawl source = %+v, want Cache of Living Stone object", gs)
	}

	shawl := &Item{ID: 45704, Name: "Shawl of the Shattered Giant"}
	d := &Database{Items: map[int]*Item{shawl.ID: shawl}, Zones: map[int]string{}, NPCs: map[int]string{}}
	d.applyCurated()
	if len(shawl.Sources) != 1 || shawl.Sources[0].Drop == nil || shawl.Sources[0].Drop.OtherName != "Cache of Living Stone" {
		t.Fatalf("overlay did not restore source: %+v", shawl.Sources)
	}
	// idempotent
	d.applyCurated()
	if len(shawl.Sources) != 1 {
		t.Fatalf("overlay not idempotent: %+v", shawl.Sources)
	}

	// items with surviving upstream sources must not be touched
	for id := range supplementalSources {
		it := &Item{ID: id, Sources: []Source{{Quest: &QuestSource{ID: 1}}}}
		d2 := &Database{Items: map[int]*Item{id: it}, Zones: map[int]string{}, NPCs: map[int]string{}}
		d2.applyCurated()
		if len(it.Sources) != 1 {
			t.Fatalf("overlay must not touch items with sources: %d", id)
		}
		break
	}
}

func TestUsableByFaction(t *testing.T) {
	horde := &Item{Faction: FactionHordeOnly}
	alliance := &Item{Faction: FactionAllianceOnly}
	both := &Item{}
	if horde.UsableByFaction("Alliance") || alliance.UsableByFaction("Horde") {
		t.Error("faction mirror loot must be restricted")
	}
	if !horde.UsableByFaction("Horde") || !alliance.UsableByFaction("Alliance") || !both.UsableByFaction("Horde") {
		t.Error("own-faction and unrestricted items must be usable")
	}
	if !horde.UsableByFaction("") {
		t.Error("unknown faction must allow everything")
	}
}

func TestClassByName(t *testing.T) {
	if ClassByName("Hunter") != ClassHunter {
		t.Error("Hunter should resolve")
	}
	if ClassByName("Death Knight") != ClassDeathKnight {
		t.Error("Death Knight should resolve")
	}
	if ClassByName("Boomkin") != ClassUnknown {
		t.Error("unknown names resolve to ClassUnknown")
	}
}

// TestCanonicalStatContract pins the canonical Stat enum contract:
// positions are frozen (generated tables and the WotLK order table
// index them numerically) and new stats are append-only. If this test
// fails, something REORDERED or INSERTED into the enum — that breaks
// statfix_gen.go's numeric keys and must be reverted, not "fixed" here.
func TestCanonicalStatContract(t *testing.T) {
	anchors := map[Stat]int{
		Strength: 0, SpellPower: 5, ArmorPenetration: 15, Armor: 20,
		Resilience: 27, BonusArmor: 34, Mastery: 35,
	}
	for st, want := range anchors {
		if int(st) != want {
			t.Errorf("%s = %d, want %d (canonical positions are frozen)", st, int(st), want)
		}
	}
	for st := Stat(0); st < NumStats; st++ {
		if statNames[st] == "" {
			t.Errorf("stat %d has no name — statNames not extended with the enum", int(st))
		}
	}
	// The WoWSims-WotLK wire order is the identity over the canonical
	// prefix — the founding-upstream coincidence the decoder spells out.
	if len(wowsimsWotlkStatOrder) != 35 {
		t.Fatalf("wotlk order has %d entries, want 35 (the upstream proto is frozen)", len(wowsimsWotlkStatOrder))
	}
	for i, st := range wowsimsWotlkStatOrder {
		if int(st) != i {
			t.Errorf("wotlk wire index %d maps to %s (%d) — upstream order must not track enum growth", i, st, int(st))
		}
	}
}

// TestStatsDecodeIgnoresUnknownTail guards the positional trap: an
// upstream array longer than the known wire order must drop the tail,
// never spill into canonical stats appended after the WotLK era
// (Mastery sits at index 35, exactly where a naive copy would put a
// 36th upstream entry).
func TestStatsDecodeIgnoresUnknownTail(t *testing.T) {
	raw := make([]byte, 0, 128)
	raw = append(raw, '[')
	for i := 0; i < 36; i++ {
		if i > 0 {
			raw = append(raw, ',')
		}
		raw = append(raw, '7')
	}
	raw = append(raw, ']')
	var st Stats
	if err := st.UnmarshalJSON(raw); err != nil {
		t.Fatal(err)
	}
	if st[BonusArmor] != 7 {
		t.Errorf("BonusArmor = %v, want 7 (last known wire index must decode)", st[BonusArmor])
	}
	if st[Mastery] != 0 {
		t.Errorf("Mastery = %v, want 0 (unknown upstream tail must be dropped)", st[Mastery])
	}
}

func TestEmbeddedLoad(t *testing.T) {
	d, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Items) < 8000 || len(d.Gems) < 300 || len(d.Enchants) < 200 {
		t.Fatalf("embedded db incomplete: %d items, %d gems, %d enchants",
			len(d.Items), len(d.Gems), len(d.Enchants))
	}
	if len(d.Effects) == 0 {
		t.Fatal("no effects extracted from embedded sim sources")
	}

	// zoneless raid containers are anchored: Black Ice comes from
	// Alexstrasza's Gift (Malygos) and must classify as a RAID drop —
	// with zone 0 it leaked past the raids-off filter as dungeon loot
	blackIce := d.Items[40497]
	if blackIce == nil || len(blackIce.Sources) == 0 {
		t.Fatal("Black Ice missing or sourceless")
	}
	drop := blackIce.Sources[0].Drop
	if drop == nil || drop.ZoneID != 4500 {
		t.Fatalf("Black Ice zone = %+v, want The Eye of Eternity (4500)", drop)
	}
	if !d.IsRaidDrop(blackIce.Sources[0]) {
		t.Fatal("Malygos container loot must classify as raid drop")
	}
}

func TestSupplementalSourcelessItems(t *testing.T) {
	d, err := Load()
	if err != nil {
		t.Fatal(err)
	}

	// Onyxia's 3.2.2 T2-remake helms carried no upstream source and
	// leaked past the raids-off filter as "quests & other"; they must
	// classify as raid drops from Onyxia's Lair now
	helm := d.Items[49480] // Dragonstalker's Helm (245)
	if helm == nil || len(helm.Sources) == 0 {
		t.Fatal("Dragonstalker's Helm missing or sourceless")
	}
	drop := helm.Sources[0].Drop
	if drop == nil || !d.IsRaidDrop(helm.Sources[0]) || d.Zones[drop.ZoneID] != "Onyxia's Lair" {
		t.Fatalf("Onyxia helm not a raid drop: %+v", helm.Sources[0])
	}

	// the head-quest jewelry and the Shadowmourne chain are raid-gated
	// quest rewards
	for _, id := range []int{49485 /* Onyxia Tooth Pendant */, 49623 /* Shadowmourne */} {
		it := d.Items[id]
		if it == nil || len(it.Sources) == 0 || it.Sources[0].Quest == nil {
			t.Fatalf("item %d must have a quest source: %+v", id, it)
		}
		if !d.RaidGated(it) {
			t.Fatalf("%s must be raid-gated", it.Name)
		}
	}

	// world drops carry the marker source instead of "source unknown"
	bell := d.Items[37835] // Je'Tze's Bell
	if bell == nil || len(bell.Sources) == 0 || bell.Sources[0].Drop == nil ||
		bell.Sources[0].Drop.OtherName != WorldDrop {
		t.Fatalf("Je'Tze's Bell must be a world drop: %+v", bell)
	}

	// WotLK-Classic-era inventions (232 Hallow's End re-releases) and
	// data-only leftovers are dropped from the pool entirely
	for _, id := range []int{211850, 211817, 50204} {
		if d.Items[id] != nil {
			t.Fatalf("unobtainable item %d must be dropped", id)
		}
	}

	// the Sanctified tier (token+mark trades, sourceless upstream AND on
	// wotlkdb) must never be swept into the unobtainable list
	if d.Items[51280] == nil {
		t.Fatal("Sanctified T10 was wrongly dropped")
	}
}

func TestCraftCosts(t *testing.T) {
	// Crusader's Dragonscale Breastplate: the full 3.3.5 reagent bill
	// (verified against wotlkdb spell 67082)
	mats := CraftCost(47595)
	if len(mats) != 5 {
		t.Fatalf("reagents = %+v, want 5 entries", mats)
	}
	want := map[string]int{"Icy Dragonscale": 40, "Eternal Water": 8,
		"Eternal Life": 8, "Arctic Fur": 8, "Crusader Orb": 8}
	for _, r := range mats {
		if want[r.Name] != r.Count {
			t.Fatalf("reagent %s = %d, want %d", r.Name, r.Count, want[r.Name])
		}
	}
	// non-crafted items have no bill
	if CraftCost(50362) != nil {
		t.Fatal("Deathbringer's Will is not crafted")
	}
}

func TestReagentEmblemPrices(t *testing.T) {
	// crafting reagents are priced too: Primordial Saronite (Frost) and
	// Crusader Orb (Triumph) — verified against wotlkdb vendor rows
	if p, ok := EmblemPriceFor(49908); !ok || p.Emblem != EmblemFrost || p.Count != 23 {
		t.Fatalf("Primordial Saronite = %+v, want 23× Frost", p)
	}
	if p, ok := EmblemPriceFor(47556); !ok || p.Emblem != EmblemTriumph || p.Count != 15 {
		t.Fatalf("Crusader Orb = %+v, want 15× Triumph", p)
	}
	// plain trade mats stay unpriced
	if _, ok := EmblemPriceFor(44128); ok {
		t.Fatal("Arctic Fur is not emblem-buyable")
	}
}

// TestWotlkMetaConditionCoverage: every meta the planner can VALUE
// (nonzero stats) must have a curated activation rule — unlisted metas
// are not assumed one-of-each anymore; they run unconstrained and the
// UI flags them. Stat-less data leftovers (Forceful Earthstorm
// Diamond: no stats even upstream, nothing to price) are exempt — the
// worn-side warning covers them. The cata pool has its own check in
// cata_test.go.
func TestWotlkMetaConditionCoverage(t *testing.T) {
	d, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	for id, g := range d.Gems {
		if g.Color != GemMeta || g.Stats == (Stats{}) {
			continue
		}
		if _, ok := d.Rules.MetaConditions[id]; !ok {
			t.Errorf("meta %d (%s) has no activation rule — curate it in metagems.go", id, g.Name)
		}
	}
}
