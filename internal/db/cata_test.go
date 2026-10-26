package db

import (
	"strings"
	"testing"
	"time"
)

// TestLoadCata parses the embedded cata database and spot-checks items
// whose values are known from wowhead.com/cata — most importantly the
// scalingOptions decode (stats as sparse cata-index maps, ilvl and
// weapon damage nested per scaling level) and the cata→canonical stat
// mapping (unified ratings land in the melee slots).
func TestLoadCata(t *testing.T) {
	d, err := LoadCata()
	if err != nil {
		t.Fatal(err)
	}
	// ~6.6k upstream minus the generated unobtainable drop list
	if len(d.Items) < 5500 || len(d.Gems) < 200 || len(d.Enchants) < 500 {
		t.Fatalf("suspiciously small db: %d items, %d gems, %d enchants",
			len(d.Items), len(d.Gems), len(d.Enchants))
	}

	// Lightning-Charged Headguard — hunter T11 helm
	helm := d.Items[60303]
	if helm == nil {
		t.Fatal("item 60303 missing")
	}
	if helm.Name != "Lightning-Charged Headguard" || helm.Slot != SlotHead ||
		helm.ILvl != 359 || helm.Quality != QualityEpic {
		t.Fatalf("helm basics wrong: %+v", helm)
	}
	if helm.Stats[Agility] != 281 || helm.Stats[MeleeCrit] != 228 ||
		helm.Stats[Mastery] != 168 || helm.Stats[Armor] != 1977 {
		t.Fatalf("helm stats wrong: %v", helm.Stats)
	}
	if len(helm.GemSockets) != 2 || helm.GemSockets[0] != GemMeta ||
		helm.GemSockets[1] != GemYellow || helm.SocketBonus[Agility] != 30 {
		t.Fatalf("helm sockets wrong: %v bonus %v", helm.GemSockets, helm.SocketBonus)
	}
	if helm.SetName != "Lightning-Charged Battlegear" || len(helm.ClassAllowlist) != 1 ||
		helm.ClassAllowlist[0] != ClassHunter {
		t.Fatalf("helm set/class wrong: %+v", helm)
	}

	// weapon damage decodes from scalingOptions
	mace := d.Items[50603] // Cryptmaker, 938-1407 @ 3.6
	if mace == nil || mace.WeaponDmgMin != 938 || mace.WeaponDmgMax != 1407 ||
		mace.WeaponSpeed != 3.6 || mace.WeaponType != WeaponMace ||
		mace.HandType != HandTwoHand {
		t.Fatalf("weapon decode wrong: %+v", mace)
	}

	// the generated overlays (catasources_gen.go) are applied:
	// sim-only test items are gone, orphans got sources, and the phase
	// overlay corrects the upstream's everything-is-phase-1 tags
	if d.Items[32466] != nil {
		t.Fatal("sim test item (Bland Blade) not dropped")
	}
	sourced := 0
	for _, it := range d.Items {
		if len(it.Sources) > 0 {
			sourced++
		}
	}
	if sourced < 5000 {
		t.Fatalf("only %d items sourced — overlay not applied?", sourced)
	}
	for id, want := range map[int]int{
		68915: 3, // Scales of Life — Firelands
		69600: 2, // Belt of Slithering Serpents — Zul'Gurub
		77188: 4, // No'Kaled — Dragon Soul
		60303: 1, // Lightning-Charged Headguard — T11 stays P1
		65074: 1, // Spaulders of the Scarred Lady [H] — heroic BWD is launch content
		65208: 1, // Lightning-Charged Spaulders [H] — heroic tier token, ilvl 372
	} {
		if got := d.Items[id].Phase; got != want {
			t.Errorf("phase of %d = %d, want %d", id, got, want)
		}
	}
	// nothing but heroic T11 gear is ilvl 372 in cata — all of it P1
	for _, it := range d.Items {
		if it.ILvl == 372 && it.Phase != 1 {
			t.Errorf("372 item mis-phased: %s phase=%d", it.Name, it.Phase)
		}
	}
	// elite PvP variants are classified, not dropped
	if !d.PvPItem(d.Items[73589]) { // Cataclysmic Gladiator's Links of Accuracy
		t.Error("elite Gladiator piece not PvP-classified")
	}
	// the five outdoor rares are world bosses; a raid boss is not
	if !d.WorldBoss(50009) { // Mobus
		t.Error("Mobus not classified as a world boss")
	}
	if d.WorldBoss(41570) { // Magmaw, Blackwing Descent
		t.Error("raid boss classified as a world boss")
	}
	// tooltip-derived unique-equipped overlay: the DS ring variants
	// share a family group, so the solver can wear at most one
	for _, id := range []int{78011, 78413, 78421} { // Signet of Grasping Mouths LFR/N/H
		if g := d.UniqueGroup(id); g != "Signet of Grasping Mouths" {
			t.Errorf("Signet %d unique group = %q", id, g)
		}
	}
	// the dump omits the two-hand flag on "Enchant 2H Weapon" lines;
	// the loader restores it (else 1H stat sticks assume +130 agi)
	for _, e := range d.Enchants {
		if strings.Contains(e.Name, "2H Weapon") && e.EnchantType != 1 {
			t.Errorf("%s: EnchantType = %d, want 1 (two-hand only)", e.Name, e.EnchantType)
		}
	}
	// proc enchants fold their uptime-averaged buff into Stats:
	// X-Ray Scope 800 RAP × 10s/(40s ICD + 60s wait at 1 PPM) = 80;
	// Landslide 1000 AP × 12s/60s = 200 — 0 would mean the planner
	// advises replacing them with flat enchants
	procAvg := map[int]struct {
		stat Stat
		want float64
	}{
		4175: {RangedAttackPower, 80},    // Gnomish X-Ray Scope
		4099: {AttackPower, 200},         // Enchant Weapon - Landslide
		4097: {Intellect, 500 * 12 / 55}, // Enchant Weapon - Power Torrent
	}
	for _, e := range d.Enchants {
		if want, ok := procAvg[e.EffectID]; ok {
			if got := e.Stats[want.stat]; got < want.want-1 || got > want.want+1 {
				t.Errorf("%s: averaged stat = %.1f, want ≈ %.1f", e.Name, got, want.want)
			}
		}
	}
	// curated legendary class locks (the upstream carries none)
	if cs := d.Items[77949].ClassAllowlist; len(cs) != 1 || cs[0] != ClassRogue {
		t.Errorf("Golad allowlist = %v, want rogue only", cs)
	}
	if cs := d.Items[71086].ClassAllowlist; len(cs) != 5 {
		t.Errorf("Dragonwrath allowlist = %v, want the five caster classes", cs)
	}

	// cata meta activation conditions — an uncovered meta would fall
	// back to the WotLK one-of-each rule and make the gem optimizer
	// chase a blue gem an Agile Shadowspirit (3 red) never asked for
	if c, ok := d.MetaConditionFor(68778); !ok || c.MinRed != 3 || c.MinBlue != 0 || c.MinYellow != 0 {
		t.Errorf("Agile Shadowspirit condition = %+v (known=%v), want 3 red", c, ok)
	}
	for id, g := range d.Gems {
		if g.Color == GemMeta {
			if _, ok := d.Rules.MetaConditions[id]; !ok {
				t.Errorf("meta %d (%s) has no activation condition", id, g.Name)
			}
		}
	}

	// vendor economics (catacosts_gen.go, TCPP TDB + ItemExtendedCost):
	// point-buyable tier carries its justice price...
	if p, ok := EmblemPriceFor(71054); !ok || p.Emblem != EmblemJustice || p.Count != 2200 {
		t.Errorf("Flamewaker's Tunic price = %+v %v, want 2200 Justice Points", p, ok)
	}
	// ...and PriceFor rotates the currency to the server's phase: the
	// raw table is the ORIGINAL game's 4.3.4 end-state, but on a P1
	// Classic server the T11-era vendor gear costs Valor, T12 tier is
	// Valor while Firelands is current, and amounts never change
	for _, tc := range []struct {
		id, phase int
		want      Emblem
		count     int
	}{
		{58181, 1, EmblemValorPoints, 1650}, // Fluid Death (359): P1 top tier
		{58181, 3, EmblemJustice, 1650},     // rotates down when FL opens
		{71054, 3, EmblemValorPoints, 2200}, // Flamewaker's Tunic (378): current in P3
		{71054, 4, EmblemJustice, 2200},     // 4.3.4 end-state again
		{58103, 1, EmblemJustice, 2200},     // Helm of the Proud (346): dungeon tier never rotates up
	} {
		p, ok := d.PriceFor(tc.id, tc.phase)
		if !ok || p.Emblem != tc.want || p.Count != tc.count {
			t.Errorf("PriceFor(%d, P%d) = %+v %v, want %d× %v",
				tc.id, tc.phase, p, ok, tc.count, tc.want)
		}
	}
	if EmblemJustice.String() != "Justice Points" {
		t.Errorf("currency name = %q", EmblemJustice)
	}
	// ...while token-only tier (T12 head/shoulders, all heroic pieces)
	// is raid content in disguise
	for _, id := range []int{71051, 71501} { // Headguard normal, Tunic heroic
		if it := d.Items[id]; it != nil && !d.RaidGated(it) {
			t.Errorf("token-only tier piece %d (%s) not raid-gated", id, it.Name)
		}
	}

	// the Elemental Rune protocol economy (Cataclysm Classic 4.4.1/2):
	// Protocol Inferno sells T11 raid gear for Fissure Stone Fragments
	// from phase 3 — the hunter tier head via its Forlorn token, boss
	// loot directly
	if p, ok := d.ProtocolFor(60303); !ok || p.Emblem != EmblemFissure ||
		p.Count != 32 || p.Phase != 3 {
		t.Errorf("Lightning-Charged Headguard protocol = %+v %v, want 32 fragments P3", p, ok)
	}
	// ...and the vendor stocks the HEROIC armor/trinket variants (the
	// wowhead snapshot corrected the wiki's normal-variant reading)
	if p, ok := d.ProtocolFor(65053); !ok || p.Count != 50 { // Bell of Enraging Resonance [H]
		t.Errorf("Bell [H] protocol = %+v %v, want 50 fragments", p, ok)
	}
	if _, ok := d.ProtocolFor(59326); ok { // the normal Bell is raid-only
		t.Error("normal Bell of Enraging Resonance wrongly fragment-priced")
	}
	// Classic has no Raid Finder: the LFR-ilvl Dragon Soul pieces drop
	// from Protocol Twilight final bosses — dungeon content, so they
	// must NOT stay raid-gated, and their source names the protocol
	lfr := d.Items[78793] // Wyrmstalker's Headguard 384
	if p, ok := d.ProtocolFor(78793); !ok || p.Emblem != EmblemObsidian ||
		p.Phase != 4 || !p.Drops {
		t.Errorf("LFR T13 protocol = %+v %v, want obsidian P4 drop", p, ok)
	}
	if d.RaidGated(lfr) {
		t.Error("LFR T13 piece still raid-gated")
	}
	if len(lfr.Sources) != 1 || lfr.Sources[0].Drop == nil ||
		lfr.Sources[0].Drop.OtherName != ProtocolTwilight {
		t.Errorf("LFR T13 source = %+v, want the protocol drop", lfr.Sources)
	}
	// the normal (397) and heroic (410) T13 pieces stay raid content
	for _, id := range []int{77030, 78698} {
		if _, ok := d.ProtocolFor(id); ok {
			t.Errorf("non-LFR T13 %d has a protocol route", id)
		}
		if !d.RaidGated(d.Items[id]) {
			t.Errorf("non-LFR T13 %d not raid-gated", id)
		}
	}
	// Twilight vendor stock with itemized costs (the vendored wowhead
	// sells snapshot): normal FL weapons, the heroic Flickering
	// satchel, an LFR weapon (the 390 variant, not 403/416), the LFR
	// tier head via its Corrupted token, and a heroic Rag trinket
	for id, want := range map[int]int{
		69897: 45, // Fandral's Flamescythe (FL normal)
		70922: 30, // Mandible of Beth'tilac (FL normal)
		71421: 30, // Satchel of the Flickering Cowl (heroic 391)
		78481: 70, // No'Kaled — the 390 Raid Finder variant
		78793: 55, // Wyrmstalker's Headguard via Crown of the Corrupted Protector
		69110: 80, // Variable Pulse Lightning Capacitor [H] (397)
	} {
		if p, ok := d.ProtocolFor(id); !ok || p.Emblem != EmblemObsidian ||
			p.Phase != 4 || p.Count != want {
			t.Errorf("obsidian price of %d = %+v %v, want %d", id, p, ok, want)
		}
	}
	// quest rewards carry their quest name from the wowhead dump's
	// sourcemore (t=5) — both the dungeon-chain gear and the raid
	// quests (which render as "Raid quest: <name>")
	for id, want := range map[int]string{
		72876: "The Path to the Dragon Soul", // Ironfeather Longbow
		77947: "Victory in the Depths",       // The Sleeper (raid-gated)
	} {
		q := d.Items[id].Sources[0].Quest
		if q == nil || q.Name != want {
			t.Errorf("quest name of %d = %+v, want %q", id, q, want)
		}
	}

	// End Time's random-Echo drops collapse to ONE source under the
	// owning boss (upstream lists all four Echo NPCs per drop)
	bow := d.Items[72810] // Windrunner's Bow — Echo of Sylvanas's loot
	if len(bow.Sources) != 1 || bow.Sources[0].Drop == nil ||
		bow.Sources[0].Drop.OtherName != "Echo of Sylvanas" {
		t.Errorf("Echo collapse wrong: %+v", bow.Sources)
	}
	scepter := d.Items[72804] // Dragonshrine Scepter — shared Echo loot
	if len(scepter.Sources) != 1 || scepter.Sources[0].Drop.OtherName != "Any boss" {
		t.Errorf("shared Echo collapse wrong: %+v", scepter.Sources)
	}

	// the 18 dedicated final-boss drops carry their assigned dungeon
	// (renders "Protocol Twilight · <dungeon>"); the token-bought tier
	// stays generic — its tokens drop randomly in any protocol run
	kiril := d.Items[78482].Sources[0].Drop
	if kiril == nil || d.Zones[kiril.ZoneID] != "Blackrock Caverns" {
		t.Errorf("Kiril dungeon attribution wrong: %+v", kiril)
	}
	if lfrDrop := d.Items[78793].Sources[0].Drop; lfrDrop.ZoneID != 0 ||
		lfrDrop.OtherName != ProtocolTwilight {
		t.Errorf("tier piece attribution should stay generic: %+v", lfrDrop)
	}
	// NOT vendor stock: BoE raid trash (Ranseur of Hatred) has no
	// protocol route — the AH is its raids-off path
	for _, id := range []int{71361, 71557} {
		if p, ok := d.ProtocolFor(id); ok && !p.Drops {
			t.Errorf("Ranseur of Hatred %d wrongly stocked: %+v", id, p)
		}
	}
	// rewards of quests completed INSIDE raids gate on the Raids
	// toggle (the upstream sources them namelessly, so this is the
	// curated cataRaidQuestItems table)...
	for _, id := range []int{77947, 77945, 71084} { // Sleeper, Fear, Branch of Nordrassil
		if !d.RaidGated(d.Items[id]) {
			t.Errorf("raid-quest reward %d (%s) not gated", id, d.Items[id].Name)
		}
	}
	// ...while dungeon-quest and Darkmoon quest gear stays open
	for _, id := range []int{72873, 76152, 74035} { // Bronze Blaster, Cowl of Destiny, Master Pit Fighter
		if d.RaidGated(d.Items[id]) {
			t.Errorf("non-raid quest reward %d (%s) wrongly gated", id, d.Items[id].Name)
		}
	}

	// Delicate Inferno Ruby — red, +40 agility
	gem := d.Gems[52212]
	if gem == nil || gem.Color != GemRed || gem.Stats[Agility] != 40 {
		t.Fatalf("gem decode wrong: %+v", gem)
	}

	// Arcanum of Ramkahen — head enchant, 60 agi + 35 haste
	ench := d.EnchantByEffect(4209, SlotHead)
	if ench == nil || ench.Stats[Agility] != 60 || ench.Stats[MeleeHaste] != 35 {
		t.Fatalf("enchant decode wrong: %+v", ench)
	}

	// re-numbered ranged enum: no item may decode into the WotLK-only
	// positions (Idol/Libram/Totem/Sigil sit where cata has none)
	for _, it := range d.Items {
		switch it.RangedType {
		case RangedIdol, RangedLibram, RangedTotem, RangedSigil:
			t.Fatalf("item %d decoded a WotLK-only ranged type %v", it.ID, it.RangedType)
		}
	}

	// cata raids classify as raid zones
	if !d.IsRaidDrop(Source{Drop: &DropSource{ZoneID: zoneIDByName(d, "Blackwing Descent")}}) {
		t.Fatal("Blackwing Descent not a raid zone")
	}
	// ...but Zul'Aman must NOT: the 4.x zone of that name is the
	// level-85 5-man heroic, not the TBC raid — a raids-off filter has
	// to keep its loot visible as dungeon drops
	za := d.Items[69576] // Headdress of Sharpened Vision — Daakara, ZA
	if za == nil || len(za.Sources) == 0 {
		t.Fatal("ZA headdress missing or sourceless")
	}
	for _, s := range za.Sources {
		if s.Drop != nil && d.IsRaidDrop(s) {
			t.Errorf("Zul'Aman drop classified as raid: %+v", s.Drop)
		}
	}
	// no dead classification rules: every raid-zone entry must be
	// referenced by at least one item in THIS pool. Zones matching
	// nothing are pure name-collision hazard (the ZA lesson) — either
	// the entry is era-carryover the pool outgrew, or the zone was
	// renamed upstream and the rule silently stopped classifying.
	zoneRef := map[string]bool{}
	for _, it := range d.Items {
		for _, s := range it.Sources {
			if s.Drop != nil {
				zoneRef[d.Zones[s.Drop.ZoneID]] = true
			}
		}
	}
	for z := range d.Rules.RaidZones {
		if !zoneRef[z] {
			t.Errorf("RaidZones entry %q matches no cata item — dead rule", z)
		}
	}
	// the two deliberate pre-cata carryovers stay load-bearing (ICC/RS
	// heroic gear is above the 272 floor and must gate on Raids)
	for _, z := range []string{"Icecrown Citadel", "The Ruby Sanctum"} {
		if !d.Rules.RaidZones[z] {
			t.Errorf("pre-cata carryover %q missing from RaidZones", z)
		}
	}

	// trinket effects decode from the structured itemEffect data:
	// Key to the Endless Chamber [H] procs 1710 agility (10%, 75s ICD),
	// Unsolvable Riddle is a 1605-agility on-use (120s)
	key := d.Effects[56328]
	if len(key) != 1 || key[0].Kind != EffectProc || key[0].Bonus[Agility] != 1710 ||
		key[0].ProcChance != 0.1 || key[0].Cooldown != 75*time.Second ||
		key[0].Duration != 15*time.Second {
		t.Fatalf("Key to the Endless Chamber effect wrong: %+v", key)
	}
	// proc triggers merge in from the vendored sim source (the
	// structured dump has none): the Key fires from melee AND ranged;
	// Schnottz's Medallion of Command is melee-ONLY — the fact that
	// lets scoring zero it for hunters
	if key[0].ProcMask != "MeleeOrRanged" || key[0].Callback != "OnSpellHitDealt" {
		t.Fatalf("Key trigger wrong: mask %q cb %q", key[0].ProcMask, key[0].Callback)
	}
	schnottz := d.Effects[65805]
	if len(schnottz) != 1 || schnottz[0].ProcMask != "Melee" {
		t.Fatalf("Schnottz's Medallion trigger wrong: %+v", schnottz)
	}
	riddle := d.Effects[62463]
	if len(riddle) != 1 || riddle[0].Kind != EffectOnUse || riddle[0].Bonus[Agility] != 1605 ||
		riddle[0].Cooldown != 120*time.Second {
		t.Fatalf("Unsolvable Riddle effect wrong: %+v", riddle)
	}
	// stack counts merge in from the vendored stat_bonus_stacking.go —
	// the structured dump states Fluid Death's PER-STACK 38 agility
	// with no stack count, which alone would bury a near-BiS trinket
	fluid := d.Effects[58181]
	if len(fluid) != 1 || fluid[0].Kind != EffectProc || fluid[0].MaxStacks != 10 ||
		fluid[0].Bonus[Agility] != 38 || fluid[0].ProcMask != "MeleeOrRanged" {
		t.Fatalf("Fluid Death stacking merge wrong: %+v", fluid)
	}
	// non-stacking effects stay untouched by the merge
	if key[0].MaxStacks != 0 {
		t.Fatalf("Key to the Endless Chamber gained stacks: %+v", key[0])
	}
}

func zoneIDByName(d *Database, name string) int {
	for id, n := range d.Zones {
		if n == name {
			return id
		}
	}
	return 0
}

// TestCataVendorSources pins the vendor-source curation: the hub
// zones the upstream list lacks resolve to names, wowhead's phantom
// 4.4 catch-up vendors are never named (any other route outranks
// them — the Crucible quest reward beats the "Weapons Vendor" listing
// nobody found in-game), and real boss drops stay drops.
func TestCataVendorSources(t *testing.T) {
	d, err := LoadCata()
	if err != nil {
		t.Fatal(err)
	}
	for z, want := range map[int]string{1637: "Orgrimmar", 5733: "Molten Front"} {
		if got := d.Zones[z]; got != want {
			t.Errorf("Zones[%d] = %q, want %q", z, got, want)
		}
	}
	// Big Bendy Blasting Bow: quest reward, not the phantom vendor
	if it := d.Items[63791]; it == nil || len(it.Sources) == 0 || it.Sources[0].Quest == nil {
		t.Errorf("Big Bendy Blasting Bow must be quest-sourced, got %+v", d.Items[63791].Sources)
	}
	// no source anywhere may carry a phantom catch-up vendor id
	for id, it := range d.Items {
		for _, src := range it.Sources {
			if sb := src.SoldBy; sb != nil && sb.NPCID >= 200000 {
				t.Errorf("item %d sold by phantom npc %d (%s)", id, sb.NPCID, sb.NPCName)
			}
		}
	}
	// a drop+vendor item whose only claim is the phantom seller is the
	// 4.4 "drops from any boss" BoE gear: a world drop, AH-buyable
	if it := d.Items[66973]; it == nil || len(it.Sources) == 0 ||
		it.Sources[0].Drop == nil || it.Sources[0].Drop.OtherName != WorldDrop {
		t.Errorf("Ravenheart Longbow must be a world drop, got %+v", d.Items[66973].Sources)
	}
	if !IsBoE(66973) {
		t.Error("Ravenheart Longbow must be BoE")
	}
	// drop+vendor disambiguation: the Baradin Hold boss DROPS its loot
	if it := d.Items[60245]; it == nil || len(it.Sources) == 0 ||
		it.Sources[0].Drop == nil || it.Sources[0].Drop.NPCID != 47120 {
		t.Errorf("Argaloth loot must stay a boss drop, got %+v", d.Items[60245].Sources)
	}
}

// TestCataRepSources: the upstream's reputation sources decode and are
// fully curated — every rep-gated item resolves a faction name and a
// standing, the Earthen Ring exalted ring included, and the phase-bound
// Avengers of Hyjal (Firelands) gear never leaks into earlier phases.
func TestCataRepSources(t *testing.T) {
	d, err := LoadCata()
	if err != nil {
		t.Fatal(err)
	}
	repItems := 0
	for _, it := range d.Items {
		for _, s := range it.Sources {
			if s.Rep == nil {
				continue
			}
			repItems++
			if s.Rep.FactionName() == "" {
				t.Errorf("%s (%d): rep faction %d has no curated name", it.Name, it.ID, s.Rep.FactionID)
			}
			if s.Rep.Standing() == "" {
				t.Errorf("%s (%d): rep level %d has no standing name", it.Name, it.ID, s.Rep.Level)
			}
			if s.Rep.FactionID == 1204 && it.Phase < 3 {
				t.Errorf("%s (%d): Avengers of Hyjal gear at phase %d, want >= 3 (Firelands)", it.Name, it.ID, it.Phase)
			}
		}
	}
	if repItems < 100 {
		t.Fatalf("only %d rep-sourced items decoded, want the full upstream set (~115)", repItems)
	}
	ring := d.Items[62362] // Signet of the Elder Council
	if ring == nil {
		t.Fatal("Signet of the Elder Council missing")
	}
	var rep *RepSource
	for _, s := range ring.Sources {
		if s.Rep != nil {
			rep = s.Rep
		}
	}
	if rep == nil || rep.FactionName() != "The Earthen Ring" || rep.Standing() != "Exalted" {
		t.Fatalf("Signet of the Elder Council rep source = %+v, want The Earthen Ring / Exalted", rep)
	}
}
