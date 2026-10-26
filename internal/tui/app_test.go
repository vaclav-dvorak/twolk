package tui

import (
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/vaclav-dvorak/twolk/internal/config"
	"github.com/vaclav-dvorak/twolk/internal/db"
	"github.com/vaclav-dvorak/twolk/internal/game/wotlk"
	"github.com/vaclav-dvorak/twolk/internal/score"
)

const (
	exportChunk1 = `{"name":"Testchar","realm":"Icecrown","level":"80","class":"Hunter","race":"Troll","faction":"Horde",`
	exportChunk2 = `"equipment":[{"name":"X","item":"46143"},{"name":"Shirt","item":"42360"},{"name":"Y","item":"45820"}],"talents":[{"tree":"Survival","points":[0,15,56]}]}`
)

// testDB covers the two resolvable fixture items; the shirt (42360) is
// deliberately absent, mirroring the real DB.
func testDB() *db.Database {
	head := &db.Item{ID: 46143, Name: "Test Headpiece", Slot: db.SlotHead, ILvl: 232,
		Quality: db.QualityEpic, GemSockets: []db.GemColor{db.GemMeta, db.GemRed}}
	head.Stats[db.Agility] = 62
	head.Stats[db.SpellCrit] = 45 // upstream duplicates unified ratings...
	head.Stats[db.MeleeCrit] = 45 // ...into both sides
	neck := &db.Item{ID: 45820, Name: "Test Broach", Slot: db.SlotNeck, ILvl: 200, Quality: db.QualityEpic,
		Sources: []db.Source{{Drop: &db.DropSource{NPCID: 1, ZoneID: 2}}}}
	proc := &db.Effect{ItemID: head.ID, Kind: db.EffectProc,
		Duration: 10 * time.Second, Cooldown: 50 * time.Second,
		ProcChance: 0.10, Outcome: "Crit"}
	proc.Bonus[db.AttackPower] = 1000
	proc.Bonus[db.RangedAttackPower] = 1000

	// two head upgrades: a phase-4 raid drop and a phase-2 crafted piece
	raidHelm := &db.Item{ID: 100, Name: "Raid Helm", Slot: db.SlotHead, ILvl: 264,
		Quality: db.QualityEpic, Phase: 4, ArmorType: db.ArmorMail,
		Sources: []db.Source{
			{Drop: &db.DropSource{NPCID: 1, ZoneID: 2, Difficulty: 3}},
			{Drop: &db.DropSource{NPCID: 1, ZoneID: 2, Difficulty: 5}},
		}}
	raidHelm.Stats[db.Agility] = 250
	raidHelm.SetName = "Test Battlegear"
	raidHelm.ClassAllowlist = []db.Class{db.ClassHunter} // tier sets are class-locked
	craftHelm := &db.Item{ID: 101, Name: "Crafted Helm", Slot: db.SlotHead, ILvl: 226,
		Quality: db.QualityEpic, Phase: 2, ArmorType: db.ArmorMail,
		Sources: []db.Source{{Crafted: &db.CraftedSource{Profession: 8}}}}
	craftHelm.Stats[db.Agility] = 200
	// 40787 is in the generated emblem price table (75× Triumph); it
	// must survive raid filters and be gated by the vendor toggle. The
	// phase tag is deliberately EARLIER than Triumph's shipping phase
	// (3) — the currency-phase gate, not the item filter, must catch it
	emblemHelm := &db.Item{ID: 40787, Name: "Emblem Helm", Slot: db.SlotHead, ILvl: 245,
		Quality: db.QualityEpic, Phase: 2, ArmorType: db.ArmorMail,
		Sources: []db.Source{{SoldBy: &db.SoldBySource{ZoneID: 4395}}}}
	emblemHelm.Stats[db.Agility] = 220
	// 45704 (Shawl of the Shattered Giant) is in the generated BoE table:
	// a raid-zone drop that must be gated by the BoE toggle, not raid tiers
	boeHelm := &db.Item{ID: 45704, Name: "BoE Helm", Slot: db.SlotHead, ILvl: 226,
		Quality: db.QualityEpic, Phase: 2, ArmorType: db.ArmorMail,
		Sources: []db.Source{{Drop: &db.DropSource{ZoneID: 2, OtherName: "Cache"}}}}
	boeHelm.Stats[db.Agility] = 210
	// resilience marks PvP gear: hidden unless the PvP toggle is on
	pvpHelm := &db.Item{ID: 104, Name: "Gladiator Helm", Slot: db.SlotHead, ILvl: 264,
		Quality: db.QualityEpic, Phase: 4, ArmorType: db.ArmorMail,
		Sources: []db.Source{{SoldBy: &db.SoldBySource{NPCID: 9, NPCName: "PvP Vendor"}}}}
	pvpHelm.Stats[db.Agility] = 240
	pvpHelm.Stats[db.Resilience] = 90
	// the other faction's copy of raidHelm (ToC-style mirror loot)
	allianceHelm := &db.Item{ID: 105, Name: "Alliance Helm", Slot: db.SlotHead, ILvl: 264,
		Quality: db.QualityEpic, Phase: 4, ArmorType: db.ArmorMail,
		Faction: db.FactionAllianceOnly,
		Sources: []db.Source{{Drop: &db.DropSource{NPCID: 1, ZoneID: 2, Difficulty: 3}}}}
	allianceHelm.Stats[db.Agility] = 250
	// RAID-TOKEN-GATED mirror loot ("Sanctified" prefix → RaidGated):
	// the faction check must run before allowItem's raid-gated early
	// return — T9 trophy tier leaked to both factions and the BiS
	// solver double-counted the mirrored 2pc bonuses
	gatedAlly := &db.Item{ID: 106, Name: "Sanctified Alliance Helm", Slot: db.SlotHead,
		ILvl: 277, Quality: db.QualityEpic, Phase: 4, ArmorType: db.ArmorMail,
		Faction: db.FactionAllianceOnly}
	gatedAlly.Stats[db.Agility] = 300

	// a belt with one native socket: the buckle tip / warning fixture
	belt := &db.Item{ID: 107, Name: "Test Belt", Slot: db.SlotWaist, ILvl: 245,
		Quality: db.QualityEpic, ArmorType: db.ArmorMail,
		GemSockets: []db.GemColor{db.GemRed}}
	belt.Stats[db.Agility] = 70
	// a socketless bracer: the blacksmith-socket fixture
	bracer := &db.Item{ID: 108, Name: "Test Bracer", Slot: db.SlotWrist, ILvl: 245,
		Quality: db.QualityEpic, ArmorType: db.ArmorMail}
	bracer.Stats[db.Agility] = 60

	meta := &db.Gem{ID: 41398, Name: "Relentless Earthsiege Diamond", Color: db.GemMeta, Quality: db.QualityRare}
	ruby := &db.Gem{ID: 40112, Name: "Delicate Cardinal Ruby", Color: db.GemRed, Quality: db.QualityEpic}
	ruby.Stats[db.Agility] = 20
	scope := &db.Enchant{EffectID: 3817, Name: "Heartseeker Scope"}
	scope.Stats[db.MeleeHaste] = 40

	return &db.Database{
		Items: map[int]*db.Item{head.ID: head, neck.ID: neck,
			raidHelm.ID: raidHelm, craftHelm.ID: craftHelm,
			emblemHelm.ID: emblemHelm, boeHelm.ID: boeHelm, pvpHelm.ID: pvpHelm,
			allianceHelm.ID: allianceHelm, gatedAlly.ID: gatedAlly,
			belt.ID: belt, bracer.ID: bracer},
		Gems:     map[int]*db.Gem{meta.ID: meta, ruby.ID: ruby},
		Enchants: []*db.Enchant{scope},
		Zones:    map[int]string{2: "Ulduar"},
		NPCs:     map[int]string{1: "Flame Leviathan"},
		Effects:  map[int][]*db.Effect{head.ID: {proc}},
		Rules:    db.WotlkRules(),
	}
}

// drive feeds a message to the model and returns the updated Model.
func drive(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	next, _ := m.Update(msg)
	return next.(Model)
}

func keys(t *testing.T, m Model, s string) Model {
	t.Helper()
	for _, r := range s {
		m = drive(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	return m
}

// paste simulates a bracketed-paste of s into the model.
func paste(t *testing.T, m Model, s string) Model {
	t.Helper()
	return drive(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s), Paste: true})
}

// openImport walks the import wizard to the paste step with the
// fixture realm (Warmane — Icecrown) selected and the name skipped.
func openImport(t *testing.T, m Model) Model {
	t.Helper()
	m = keys(t, m, "i")
	m.impCursor = 1 // Warmane — Icecrown
	m.impName.SetValue("")
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // continue
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // skip the name field
	return m
}

func TestImportFlow(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // isolate os.UserConfigDir

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	m := New(cfg)
	m = drive(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})

	if !strings.Contains(m.View(), "no character imported") {
		t.Fatalf("expected empty state, got:\n%s", m.View())
	}

	// i opens the import wizard on the realm step
	m = keys(t, m, "i")
	if m.mode != modeImport {
		t.Fatalf("expected import mode after 'i', got %v", m.mode)
	}
	if !strings.Contains(m.View(), "Warmane — Icecrown") {
		t.Fatalf("realm step missing registry rows:\n%s", m.View())
	}
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyDown}) // Warmane — Icecrown
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // skip the name field

	// typed characters are ignored — the paste step is paste-only
	m = keys(t, m, "hello")
	if len(m.pasted) != 0 {
		t.Fatalf("typing must not enter content, got %q", m.pasted)
	}

	// valid JSON that is not an armory export is rejected on save
	m = paste(t, m, `{"foo": 1}`)
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != modeImport || !strings.Contains(m.errMsg, "armory export") {
		t.Fatalf("expected armory validation error, mode=%v err=%q", m.mode, m.errMsg)
	}

	// c clears the buffer; chunked pastes append; s saves
	m = keys(t, m, "c")
	if len(m.pasted) != 0 {
		t.Fatalf("expected cleared buffer, got %q", m.pasted)
	}
	m = paste(t, m, exportChunk1)
	m = paste(t, m, exportChunk2)
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != modeNormal {
		t.Fatalf("expected normal mode after save, err=%q", m.errMsg)
	}
	view := m.View()
	for _, want := range []string{"Testchar @ Icecrown", "80 Troll", "Survival Hunter", "loading item database"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}

	// the config file on disk round-trips, and a fresh model parses it
	path, err := config.Path()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("config file not written: %v", err)
	}
	fresh, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := fresh.Character()
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != exportChunk1+exportChunk2 {
		t.Fatalf("stored character mismatch: %s", raw)
	}
	m2 := New(fresh)
	if m2.char == nil || m2.char.Name != "Testchar" {
		t.Fatalf("fresh model did not restore character: %+v", m2.char)
	}

	// the refresh flow (r) targets the active character directly: paste
	// step, name fixed, exact armory link — no realm wizard
	m = keys(t, m, "r")
	if !m.impRefresh || m.impStep != 1 {
		t.Fatalf("r: refresh=%v step=%d, want paste step of the refresh flow",
			m.impRefresh, m.impStep)
	}
	view = m.View()
	if !strings.Contains(view, "Refresh character") ||
		!strings.Contains(view, "/character/Testchar/Icecrown/summary") {
		t.Fatalf("refresh modal missing title or personalized API URL:\n%s", view)
	}
	// esc leaves the refresh flow entirely (there is no realm list)
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != modeNormal {
		t.Fatal("esc did not close the refresh modal")
	}
}

func TestStatGroups(t *testing.T) {
	var s db.Stats
	s[db.Agility] = 62
	s[db.Stamina] = 90
	s[db.SpellCrit] = 45
	s[db.MeleeCrit] = 45
	s[db.AttackPower] = 120
	s[db.RangedAttackPower] = 120 // duplicated in data, one line in game

	m := Model{game: wotlk.New(testDB())}
	groups := m.statGroups(s)
	if len(groups) != 2 {
		t.Fatalf("groups = %d, want 2 (base + secondary)", len(groups))
	}
	base, sec := strings.Join(groups[0], "\n"), strings.Join(groups[1], "\n")
	if !strings.Contains(base, "+62 Agility") || !strings.Contains(base, "+90 Stamina") || strings.Contains(base, "Crit") {
		t.Errorf("base group wrong: %q", base)
	}
	if !strings.Contains(sec, "+45 Crit") || !strings.Contains(sec, "+120 Attack Power") || strings.Contains(sec, "Agility") {
		t.Errorf("secondary group wrong: %q", sec)
	}
	if strings.Contains(sec, "Ranged AP") {
		t.Errorf("AP/RAP not collapsed: %q", sec)
	}

	// ranged-only AP (e.g. bows) keeps its explicit label
	var rangedOnly db.Stats
	rangedOnly[db.RangedAttackPower] = 70
	if got := strings.Join(m.statGroups(rangedOnly)[0], "\n"); !strings.Contains(got, "+70 Ranged AP") {
		t.Errorf("ranged-only AP mislabeled: %q", got)
	}

	// only base stats → single group, no empty second group
	var onlyBase db.Stats
	onlyBase[db.Strength] = 10
	if got := m.statGroups(onlyBase); len(got) != 1 {
		t.Errorf("groups = %d, want 1 when no secondary stats", len(got))
	}
}

func TestGearNavigation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.UpsertCharacter([]byte(exportChunk1+exportChunk2), config.StoredCharacter{Game: "wotlk", Server: "warmane", Realm: "Icecrown"}, -1); err != nil {
		t.Fatal(err)
	}
	m := New(cfg)
	m = drive(t, m, tea.WindowSizeMsg{Width: 170, Height: 40})
	m = drive(t, m, dbLoadedMsg{slug: "wotlk", db: testDB()})

	// the shirt is dropped: 3 equipped, 2 navigable rows
	if got := len(m.gearRows()); got != 2 {
		t.Fatalf("gear rows = %d, want 2 (shirt ignored)", got)
	}

	view := m.View()
	if strings.Contains(view, "Shirt") {
		t.Fatal("shirt must not be rendered")
	}
	// first row selected: detail panel shows the head, its stats, sockets
	// with the assumed epic gems, and no source (head has none)
	for _, want := range []string{"+62 Agility", "Meta", "Cardinal Ruby", "assume"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
	// unified rating: one "+45 Crit" line, not spell/melee twins
	if !strings.Contains(view, "+45 Crit") || strings.Contains(view, "Spell Crit") || strings.Contains(view, "Melee Crit") {
		t.Fatalf("crit rating not collapsed:\n%s", view)
	}
	// extracted proc effect renders with merged bonus and trigger details
	for _, want := range []string{"Proc: +1000 Attack Power", "for 10s", "10% chance, on crit, 50s ICD"} {
		if !strings.Contains(view, want) {
			t.Fatalf("effect rendering missing %q:\n%s", want, view)
		}
	}

	// class emblem: the pixel-art class icon renders at this width
	if !strings.Contains(view, "▀") {
		t.Fatalf("class icon missing:\n%s", view)
	}

	// cap readout: the test gear carries no hit or ArP, so the only hit
	// in the totals is the assumed meal's 40 (consumables are always on)
	for _, want := range []string{"Hit 40/263", "ArP 0/1400", "buffs: Snapper Extreme"} {
		if !strings.Contains(view, want) {
			t.Fatalf("cap readout missing %q:\n%s", want, view)
		}
	}

	// ilvl analysis: head 232 + neck 200 → avg 216
	if !strings.Contains(view, "avg ilvl") || !strings.Contains(view, "216") {
		t.Fatalf("avg ilvl missing:\n%s", view)
	}
	// the neck has no upgrades in the fixture DB → ticked best in slot
	// (which outranks its below-average ▲)
	if !strings.Contains(view, "✓ Neck") {
		t.Fatalf("best-in-slot tick missing:\n%s", view)
	}

	// cursor down: detail switches to the neck with its drop source
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyDown})
	view = m.View()
	for _, want := range []string{"Drops: Flame Leviathan · Ulduar"} {
		if !strings.Contains(view, want) {
			t.Fatalf("after down, view missing %q:\n%s", want, view)
		}
	}
	// clamped at the last row
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyDown})
	if m.cursor != 1 {
		t.Fatalf("cursor = %d, want clamped at 1", m.cursor)
	}
	// and back up
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyUp})
	if m.cursor != 0 {
		t.Fatalf("cursor = %d, want 0", m.cursor)
	}
}

func TestUpgradeBrowserAndFilter(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.UpsertCharacter([]byte(exportChunk1+exportChunk2), config.StoredCharacter{Game: "wotlk", Server: "warmane", Realm: "Icecrown"}, -1); err != nil {
		t.Fatal(err)
	}
	m := New(cfg)
	m = drive(t, m, tea.WindowSizeMsg{Width: 140, Height: 40})
	m = drive(t, m, dbLoadedMsg{slug: "wotlk", db: testDB()})
	if m.scorer == nil {
		t.Fatal("scorer must be built for Survival Hunter")
	}

	// enter on the head row opens the upgrade browser with all candidates
	// except PvP gear (hidden by default despite its high budget)
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != modeUpgrades || len(m.upList) != 4 {
		t.Fatalf("mode=%v upgrades=%d, want browser with 4", m.mode, len(m.upList))
	}
	view := m.View()
	if strings.Contains(view, "Gladiator Helm") {
		t.Fatalf("PvP gear must be hidden by default:\n%s", view)
	}
	if strings.Contains(view, "Alliance Helm") {
		t.Fatalf("other faction's mirror loot must be hidden for a Horde character:\n%s", view)
	}
	if strings.Contains(view, "Sanctified Alliance Helm") {
		t.Fatalf("raid-token-gated mirror loot must respect faction too:\n%s", view)
	}
	// enabling the PvP toggle (last row) brings it in on top
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	for range m.filterRows() - 1 {
		m = drive(t, m, tea.KeyMsg{Type: tea.KeyDown})
	}
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if len(m.upList) != 5 || !strings.Contains(m.View(), "incl. PvP") {
		t.Fatalf("PvP toggle not applied: %d ups\n%s", len(m.upList), m.View())
	}
	// turn it back off
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	for range m.filterRows() - 1 {
		m = drive(t, m, tea.KeyMsg{Type: tea.KeyUp})
	}
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if len(m.upList) != 4 {
		t.Fatalf("PvP toggle-off not applied: %d ups", len(m.upList))
	}
	view = m.View()
	for _, want := range []string{"Upgrades — Head", "▲", "Raid Helm", "Flame Leviathan · Ulduar (10/25)",
		"Crafted Helm", "Crafted: Leatherworking", "Emblem Helm", "Vendor: 75× Emblem of Triumph",
		"BoE Helm", "BoE ", "≤ P5 RS"} {
		if !strings.Contains(view, want) {
			t.Fatalf("upgrades view missing %q:\n%s", want, view)
		}
	}
	// tier pieces carry an explicit tier label (phase 4, ilvl 264 →
	// T10.5) and the detail names the set
	for _, want := range []string{"T10.5", "set: ", "Test Battlegear"} {
		if !strings.Contains(view, want) {
			t.Fatalf("tier labeling missing %q:\n%s", want, view)
		}
	}
	// the compare pane shows equipped left, candidate right, per-stat
	// deltas between them — the items' OWN stats (gains agility, loses
	// the crit); the displaced gem's agility prices in the gems section
	for _, want := range []string{"equipped: ", "candidate: ", "Agility", "+188", "-45", "-20"} {
		if !strings.Contains(view, want) {
			t.Fatalf("compare pane missing %q:\n%s", want, view)
		}
	}

	// THE PHASED-SERVER SCENARIO: server on phase 2, player doesn't raid.
	// Selecting phase 2 (row 1) hides the P4 raid helm (item phase) AND
	// the vendor-only emblem helm — a phase-2 ITEM priced in Triumph,
	// which ships with ToC (P3): the currency gate, not the tag.
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	if m.mode != modeFilter {
		t.Fatalf("mode = %v, want filter", m.mode)
	}
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyDown}) // row 1 = phase 2
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if len(m.upList) != 2 {
		t.Fatalf("phase ceiling not applied: %+v", m.upList)
	}
	if !strings.Contains(m.View(), "≤ P2 Ulduar") {
		t.Fatalf("filter summary wrong:\n%s", m.View())
	}

	// no raid group: raid drops off (row 5) — the BoE raid drop STAYS
	// (buyable on the AH), so still crafted + BoE
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	for range 4 { // row 1 → row 5
		m = drive(t, m, tea.KeyMsg{Type: tea.KeyDown})
	}
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if len(m.upList) != 2 {
		t.Fatalf("raids-off must keep the BoE: %+v", m.upList)
	}

	// BoE off too (row 6) → only the crafted helm
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyDown}) // row 5 → 6
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if len(m.upList) != 1 || m.upList[0].Item.ID != 101 {
		t.Fatalf("BoE filter not applied: %+v", m.upList)
	}

	// crafted off (row 9) → nothing → best in slot
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	for range 3 { // row 6 → row 9
		m = drive(t, m, tea.KeyMsg{Type: tea.KeyDown})
	}
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if len(m.upList) != 0 || !strings.Contains(m.View(), "best in slot") {
		t.Fatalf("crafted filter not applied: %+v", m.upList)
	}

	// esc returns to the gear screen
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != modeNormal {
		t.Fatalf("esc should return to normal mode, got %v", m.mode)
	}

	// the filter survives a restart
	fresh, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	m2 := New(fresh)
	if m2.filter.ServerPhase != 2 || m2.filter.Raids || m2.filter.RaidBoE || m2.filter.Crafted {
		t.Fatalf("filter not persisted: %+v", m2.filter)
	}
	if !m2.filter.Dungeon || !m2.filter.Vendor || !m2.filter.Other {
		t.Fatalf("unrelated filter fields changed: %+v", m2.filter)
	}
}

func TestFilterModalRenders(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	m := New(cfg)
	m = drive(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
	// the filter modal works before the item database arrives — phase
	// rows are game-module RULES, available from construction
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	if m.mode != modeFilter {
		t.Fatalf("mode = %v, want filter", m.mode)
	}
	// the modal must render on every cursor row, including the last
	// (crafted) one — this used to panic with index out of range
	for i := 0; i < m.filterRows(); i++ {
		view := m.View()
		for _, want := range []string{"Filter upgrade proposals", "Crafted items", "Ulduar"} {
			if !strings.Contains(view, want) {
				t.Fatalf("filter view (row %d) missing %q:\n%s", i, want, view)
			}
		}
		m = drive(t, m, tea.KeyMsg{Type: tea.KeyDown})
	}
}

func TestILvlStats(t *testing.T) {
	rows := []gearRow{
		{item: &db.Item{ILvl: 232}},
		{item: &db.Item{ILvl: 200}},
		{item: &db.Item{ILvl: 264}},
	}
	if avg := ilvlStats(rows); avg != 232 {
		t.Errorf("avg=%v, want 232", avg)
	}
	if avg := ilvlStats(nil); avg != 0 {
		t.Errorf("empty gear avg = %v, want 0", avg)
	}
}

func TestHitTalentModal(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.UpsertCharacter([]byte(exportChunk1+exportChunk2), config.StoredCharacter{Game: "wotlk", Server: "warmane", Realm: "Icecrown"}, -1); err != nil {
		t.Fatal(err)
	}
	m := New(cfg)
	m = drive(t, m, tea.WindowSizeMsg{Width: 140, Height: 40})
	m = drive(t, m, dbLoadedMsg{slug: "wotlk", db: testDB()})

	if !strings.Contains(m.View(), "Hit 40/263") {
		t.Fatalf("default cap wrong:\n%s", m.View())
	}

	// t opens the talent modal listing Focused Aim with a slider
	m = keys(t, m, "t")
	if m.mode != modeTalents {
		t.Fatalf("mode = %v, want talents", m.mode)
	}
	view := m.View()
	for _, want := range []string{"Talent assumptions", "Focused Aim", "Marksmanship", "0/3", "+1% hit each", "effective caps:", "Hit 263"} {
		if !strings.Contains(view, want) {
			t.Fatalf("talent modal missing %q:\n%s", want, view)
		}
	}

	// two points of Focused Aim → 6% → 197 rating
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyRight})
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyRight})
	if !strings.Contains(m.View(), "2/3") {
		t.Fatalf("slider not at 2/3:\n%s", m.View())
	}
	// clamped at max
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyRight})
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyRight})
	if !strings.Contains(m.View(), "3/3") {
		t.Fatalf("slider must clamp at 3/3:\n%s", m.View())
	}
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyLeft}) // back to 2
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != modeNormal || !strings.Contains(m.View(), "Hit 40/197") {
		t.Fatalf("cap after 2 talent points wrong:\n%s", m.View())
	}

	// persisted per talent, restored by a fresh model
	fresh, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if fresh.TalentPoints["wotlk/hunter/focused_aim"] != 2 {
		t.Fatalf("talent points not persisted: %+v", fresh.TalentPoints)
	}
	m2 := New(fresh)
	m2 = drive(t, m2, tea.WindowSizeMsg{Width: 140, Height: 40})
	m2 = drive(t, m2, dbLoadedMsg{slug: "wotlk", db: testDB()})
	if !strings.Contains(m2.View(), "Hit 40/197") {
		t.Fatalf("restored cap wrong:\n%s", m2.View())
	}
}

func TestWeightsEditor(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.UpsertCharacter([]byte(exportChunk1+exportChunk2), config.StoredCharacter{Game: "wotlk", Server: "warmane", Realm: "Icecrown"}, -1); err != nil {
		t.Fatal(err)
	}
	m := New(cfg)
	m = drive(t, m, tea.WindowSizeMsg{Width: 140, Height: 50})
	m = drive(t, m, dbLoadedMsg{slug: "wotlk", db: testDB()})
	baseScore := m.scorer.SetScoreWorn(wornOf(m.gearRows()))

	// w opens the editor showing the profile defaults
	m = keys(t, m, "w")
	if m.mode != modeWeights {
		t.Fatalf("mode = %v, want weights", m.mode)
	}
	view := m.View()
	for _, want := range []string{"Stat weights — Survival Hunter", "Agility", "2.65", "Hit", "Armor Penetration"} {
		if !strings.Contains(view, want) {
			t.Fatalf("weights view missing %q:\n%s", want, view)
		}
	}

	// bump agility by 0.5 (row 1): down, shift+right — applied live
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyDown})
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyShiftRight})
	if got := m.scorer.Profile().Weights[db.Agility]; got != 3.15 {
		t.Fatalf("live agility weight = %v, want 3.15", got)
	}
	if !strings.Contains(m.View(), "(default 2.65)") {
		t.Fatalf("override marker missing:\n%s", m.View())
	}
	if m.scorer.SetScoreWorn(wornOf(m.gearRows())) <= baseScore {
		t.Fatal("score must increase with higher agility weight")
	}

	// the profile-level ranged DPS weight is editable like any stat
	for range 7 { // from Agility (1) to Ranged DPS (8)
		m = drive(t, m, tea.KeyMsg{Type: tea.KeyDown})
	}
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyRight})
	if got := m.scorer.Profile().RangedDPSWeight; got != 6.37 {
		t.Fatalf("live ranged DPS weight = %v, want 6.37", got)
	}
	m = keys(t, m, "r") // back to default before moving on
	if got := m.scorer.Profile().RangedDPSWeight; got != 6.32 {
		t.Fatalf("ranged DPS reset = %v, want 6.32", got)
	}

	// Hit is a unified row: edits must land on the melee side only. The
	// default is the profile's measured pre-cap slope, not a literal —
	// re-measuring a spec must not break an editor test.
	svHitDefault := score.Find(db.ClassHunter, "Survival").Weights[db.MeleeHit]
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyDown}) // Ranged DPS (8) to Hit (9)
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyRight})
	if w := m.scorer.Profile().Weights; w[db.MeleeHit] != svHitDefault+0.05 || w[db.SpellHit] != 0 {
		t.Fatalf("hit override sides wrong: melee=%v spell=%v", w[db.MeleeHit], w[db.SpellHit])
	}

	// r resets the hit row; the agility override stays
	m = keys(t, m, "r")
	if got := m.scorer.Profile().Weights[db.MeleeHit]; got != svHitDefault {
		t.Fatalf("hit reset = %v, want the profile default %v", got, svHitDefault)
	}

	// close saves; a fresh model restores the agility override
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	fresh, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Weights["wotlk/hunter/survival"]["agility"] != 3.15 {
		t.Fatalf("weights not persisted: %+v", fresh.Weights)
	}
	m2 := New(fresh)
	m2 = drive(t, m2, tea.WindowSizeMsg{Width: 140, Height: 50})
	m2 = drive(t, m2, dbLoadedMsg{slug: "wotlk", db: testDB()})
	if got := m2.scorer.Profile().Weights[db.Agility]; got != 3.15 {
		t.Fatalf("restored agility weight = %v, want 3.15", got)
	}

	// d restores all defaults
	m2 = keys(t, m2, "w")
	m2 = keys(t, m2, "d")
	if got := m2.scorer.Profile().Weights[db.Agility]; got != 2.65 {
		t.Fatalf("defaults reset = %v, want 2.65", got)
	}
}

func TestDraeneiRacialHit(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	draenei := `{"name":"Spacegoat","realm":"Icecrown","level":"80","class":"Hunter","race":"Draenei","faction":"Alliance",` + exportChunk2
	if err := cfg.UpsertCharacter([]byte(draenei), config.StoredCharacter{Game: "wotlk", Server: "warmane", Realm: "Icecrown"}, -1); err != nil {
		t.Fatal(err)
	}
	m := New(cfg)
	m = drive(t, m, tea.WindowSizeMsg{Width: 140, Height: 40})
	m = drive(t, m, dbLoadedMsg{slug: "wotlk", db: testDB()})
	// Heroic Presence: 8% - 1% = 7% → 230 rating, no talent input needed
	if !strings.Contains(m.View(), "Hit 40/230") {
		t.Fatalf("draenei racial not applied:\n%s", m.View())
	}
}

func TestTransparencyMask(t *testing.T) {
	const D, L = 0x000000, 0xffffff // dark, light
	// dark ring around light art with a dark pixel INSIDE the art
	px := [][]uint32{
		{D, D, D, D, D},
		{D, L, L, L, D},
		{D, L, D, L, D},
		{D, L, L, L, D},
		{D, D, D, D, D},
	}
	mask := transparencyMask(px)
	if !mask[0][0] || !mask[4][4] || !mask[0][2] {
		t.Error("border darkness must be transparent")
	}
	if mask[2][2] {
		t.Error("interior shadow must NOT be transparent")
	}
	if mask[1][1] {
		t.Error("light art must never be transparent")
	}
}

func TestVendorEmblemInference(t *testing.T) {
	m := Model{game: wotlk.New(testDB())}

	// item in the generated 3.3.5 price table → exact price
	noose := &db.Item{ID: 50995, ILvl: 264}
	sb := &db.SoldBySource{ZoneID: 4395}
	if got := m.vendorText(noose, sb); got != "Vendor: 60× Emblem of Frost" {
		t.Errorf("exact emblem price = %q", got)
	}

	// non-emblem item from a nameless vendor → plain "Vendor", no guessing
	it := &db.Item{ILvl: 232}
	if got := m.vendorText(it, sb); got != "Vendor" {
		t.Errorf("nameless vendor = %q, want plain Vendor", got)
	}
	named := &db.SoldBySource{NPCName: "Harold Winston", ZoneID: 4395}
	if got := m.vendorText(it, named); !strings.Contains(got, "Harold Winston") {
		t.Errorf("named vendor = %q", got)
	}
}

func TestEnrichedImportDetail(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	// head: meta gem present, red socket EMPTY, no enchant;
	// neck: enriched but nothing to warn about (no sockets, not enchantable)
	enriched := `{"name":"Testchar","realm":"Icecrown","level":"80","class":"Hunter","race":"Troll","faction":"Horde",` +
		`"equipment":[{"name":"X","item":"46143","ench":0,"gems":[41398,0]},` +
		`{"name":"Y","item":"45820","ench":0,"gems":[]}],` +
		`"talents":[{"tree":"Survival","points":[0,15,56]}]}`
	if err := cfg.UpsertCharacter([]byte(enriched), config.StoredCharacter{Game: "wotlk", Server: "warmane", Realm: "Icecrown"}, -1); err != nil {
		t.Fatal(err)
	}
	m := New(cfg)
	m = drive(t, m, tea.WindowSizeMsg{Width: 170, Height: 40})
	m = drive(t, m, dbLoadedMsg{slug: "wotlk", db: testDB()})

	view := m.View()
	for _, want := range []string{
		"Relentless Earthsiege Diamond", // actual gem, not an assumption
		"empty socket!",                 // the red socket is empty
		"no enchant!",                   // head is enchantable and bare
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("enriched detail missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "assume") {
		t.Fatalf("enriched item must not show assumptions:\n%s", view)
	}

	// scoring uses actuals: no red gem, no assumed +20 agi from it
	rows := m.gearRows()
	worn := m.scorer.WornStats(rows[0].worn())
	assumed := m.scorer.EffectiveStats(rows[0].item)
	if worn[db.Agility] >= assumed[db.Agility] {
		t.Fatalf("worn agility (%v) must be below assumed (%v) with an empty socket",
			worn[db.Agility], assumed[db.Agility])
	}
}

func TestImportSnippetCopyFeedback(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	m := New(cfg)
	m = drive(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = openImport(t, m)

	// without an embedded snippet (test build), y explains itself
	ArmorySnippet = ""
	m = keys(t, m, "y")
	if !strings.Contains(m.View(), "snippet unavailable") {
		t.Fatalf("missing unavailable feedback:\n%s", m.View())
	}

	// with a snippet and a fake clipboard note path, the confirmation
	// renders inside the modal (clipboard copy itself is exercised
	// manually — it shells out to the platform tool)
	m.errMsg = ""
	m.importNote = "✓ snippet copied — paste it into the browser console on your armory page"
	if !strings.Contains(m.View(), "✓ snippet copied") {
		t.Fatalf("confirmation not rendered in modal:\n%s", m.View())
	}
}

func TestCharacterSwitcher(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	m := New(cfg)
	m = drive(t, m, tea.WindowSizeMsg{Width: 140, Height: 40})
	m = drive(t, m, dbLoadedMsg{slug: "wotlk", db: testDB()})

	importChar := func(m Model, json string) Model {
		m = openImport(t, m)
		m = paste(t, m, json)
		return drive(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	}
	hunter := exportChunk1 + exportChunk2
	mage := `{"name":"Frosty","realm":"Icecrown","level":"80","class":"Mage","race":"Gnome","faction":"Alliance",` +
		`"equipment":[{"name":"X","item":"45820"}],"talents":[{"tree":"Frost","points":[0,0,71]}]}`

	m = importChar(m, hunter)
	m = importChar(m, mage)
	if got := len(m.cfg.Characters()); got != 2 {
		t.Fatalf("stored characters = %d, want 2", got)
	}
	if m.char.Name != "Frosty" {
		t.Fatalf("latest import should be active, got %s", m.char.Name)
	}

	// re-importing an existing character replaces, not duplicates
	m = importChar(m, hunter)
	if got := len(m.cfg.Characters()); got != 2 {
		t.Fatalf("re-import duplicated: %d chars", got)
	}
	if m.char.Name != "Testchar" {
		t.Fatalf("re-import should activate that character, got %s", m.char.Name)
	}

	// c opens the switcher listing both, with the active marked
	m = keys(t, m, "c")
	if m.mode != modeChars {
		t.Fatalf("mode = %v, want chars", m.mode)
	}
	view := m.View()
	for _, want := range []string{"Testchar @ Icecrown", "Frosty @ Icecrown", "Frost Mage", "●"} {
		if !strings.Contains(view, want) {
			t.Fatalf("switcher missing %q:\n%s", want, view)
		}
	}

	// switch to the mage
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyDown})
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != modeNormal || m.char.Name != "Frosty" {
		t.Fatalf("switch failed: %v %v", m.mode, m.char)
	}
	if !strings.Contains(m.View(), "Frost Mage") {
		t.Fatalf("header not updated:\n%s", m.View())
	}

	// persistence: fresh load restores the active character
	fresh, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	m2 := New(fresh)
	if m2.char == nil || m2.char.Name != "Frosty" {
		t.Fatalf("active character not persisted: %+v", m2.char)
	}

	// delete the mage: hunter becomes active
	m = keys(t, m, "c")
	m.cCursor = 1
	m = keys(t, m, "d")
	if got := len(m.cfg.Characters()); got != 1 {
		t.Fatalf("delete failed: %d chars", got)
	}
	if m.char == nil || m.char.Name != "Testchar" {
		t.Fatalf("active after delete = %+v", m.char)
	}
}

func TestQuickTipsPane(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	// enriched import: head with empty red socket and no enchant
	enriched := `{"name":"Testchar","realm":"Icecrown","level":"80","class":"Hunter","race":"Troll","faction":"Horde",` +
		`"equipment":[{"name":"X","item":"46143","ench":0,"gems":[41398,0]}],` +
		`"talents":[{"tree":"Survival","points":[0,15,56]}],` +
		`"professions":[{"name":"Leatherworking","skill":"450"}]}`
	if err := cfg.UpsertCharacter([]byte(enriched), config.StoredCharacter{Game: "wotlk", Server: "warmane", Realm: "Icecrown"}, -1); err != nil {
		t.Fatal(err)
	}
	m := New(cfg)
	m = drive(t, m, tea.WindowSizeMsg{Width: 170, Height: 50})
	m = drive(t, m, dbLoadedMsg{slug: "wotlk", db: testDB()})

	view := m.View()
	for _, want := range []string{
		"Quick tips",
		"gems on 1 item not optimal", // pointer to the optimization advisor
		"to optimize",
		"Raid Helm", // biggest upgrade hint
		"⬆",
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("tips pane missing %q:\n%s", want, view)
		}
	}

	// a gear hint carries the same route text the upgrade browser shows:
	// the pane says WHERE to farm the upgrade, not just that it exists
	var hint string
	for _, line := range strings.Split(ansi.Strip(view), "\n") {
		if strings.Contains(line, "⬆") && strings.Contains(line, "Raid Helm") {
			hint = line
		}
	}
	if hint == "" {
		t.Fatalf("no gear hint line:\n%s", view)
	}
	var raidHelm *db.Item
	for _, it := range m.game.DB.Items {
		if it.Name == "Raid Helm" {
			raidHelm = it
		}
	}
	if want := ansi.Strip(m.sourceText(raidHelm)); !strings.Contains(hint, want) {
		t.Errorf("hint %q missing the route %q", hint, want)
	}

	// on a short terminal the tips pane must shrink instead of pushing
	// the help line off screen — even with a status message showing
	// (it renders on the header separator row, adding no height)
	m.status = "switched to Testchar"
	for _, h := range []int{40, 30, 24} {
		m = drive(t, m, tea.WindowSizeMsg{Width: 170, Height: h})
		view = m.View()
		if got := lipgloss.Height(view); got > h {
			t.Fatalf("view is %d lines, terminal only %d:\n%s", got, h, view)
		}
		if !strings.Contains(view, "quit") {
			t.Fatalf("help line missing at height %d:\n%s", h, view)
		}
	}
}

func TestBeltBuckleTipWarningAndOptimizer(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	// enriched import: belt gemmed in its native socket, NO buckle (the
	// armory pads the gem list with zeros)
	enriched := `{"name":"Testchar","realm":"Icecrown","level":"80","class":"Hunter","race":"Troll","faction":"Horde",` +
		`"equipment":[{"name":"B","item":"107","ench":0,"gems":[40112,0,0]}],` +
		`"talents":[{"tree":"Survival","points":[0,15,56]}]}`
	if err := cfg.UpsertCharacter([]byte(enriched), config.StoredCharacter{Game: "wotlk", Server: "warmane", Realm: "Icecrown"}, -1); err != nil {
		t.Fatal(err)
	}
	m := New(cfg)
	m = drive(t, m, tea.WindowSizeMsg{Width: 170, Height: 50})
	m = drive(t, m, dbLoadedMsg{slug: "wotlk", db: testDB()})

	// main screen: detail-pane warning + quick tip to buy the buckle
	view := m.View()
	for _, want := range []string{
		"no Eternal Belt Buckle!",           // detail pane warning
		"apply Eternal Belt Buckle",         // quick tip verb
		"▣",                                 // buckle tip glyph
		"socket [ Delicate Cardinal Ruby ]", // and what to put in it
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("buckle advice missing %q:\n%s", want, view)
		}
	}

	// gem optimizer: the buckle socket is an explicit change
	m = keys(t, m, "g")
	view = m.View()
	for _, want := range []string{"Buckle", "apply Eternal Belt Buckle", "Delicate Cardinal Ruby"} {
		if !strings.Contains(view, want) {
			t.Fatalf("optimizer plan missing %q:\n%s", want, view)
		}
	}

	// with the buckle socketed, the tip and the warning disappear
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	buckled := `{"name":"Testchar","realm":"Icecrown","level":"80","class":"Hunter","race":"Troll","faction":"Horde",` +
		`"equipment":[{"name":"B","item":"107","ench":0,"gems":[40112,40112,0]}],` +
		`"talents":[{"tree":"Survival","points":[0,15,56]}]}`
	m = openImport(t, m)
	m = paste(t, m, buckled)
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	view = m.View()
	if strings.Contains(view, "Eternal Belt Buckle") {
		t.Fatalf("buckled belt must not warn or tip:\n%s", view)
	}
	if !strings.Contains(view, "◆ Buckle") {
		t.Fatalf("buckle gem line missing from detail pane:\n%s", view)
	}
}

func TestBlacksmithSocketTipAndWarning(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	// enriched import, blacksmith, socketless bracer with no extra gem
	smith := `{"name":"Testchar","realm":"Icecrown","level":"80","class":"Hunter","race":"Troll","faction":"Horde",` +
		`"equipment":[{"name":"W","item":"108","ench":0,"gems":[0,0,0]}],` +
		`"talents":[{"tree":"Survival","points":[0,15,56]}],` +
		`"professions":[{"name":"Blacksmithing","skill":"450"}]}`
	if err := cfg.UpsertCharacter([]byte(smith), config.StoredCharacter{Game: "wotlk", Server: "warmane", Realm: "Icecrown"}, -1); err != nil {
		t.Fatal(err)
	}
	m := New(cfg)
	m = drive(t, m, tea.WindowSizeMsg{Width: 170, Height: 50})
	m = drive(t, m, dbLoadedMsg{slug: "wotlk", db: testDB()})

	view := m.View()
	for _, want := range []string{
		"no Socket Bracer (Blacksmithing)!",   // detail pane warning
		"apply Socket Bracer (Blacksmithing)", // quick tip
	} {
		if !strings.Contains(view, want) {
			t.Fatalf("blacksmith socket advice missing %q:\n%s", want, view)
		}
	}

	// without the profession the socket is neither assumed nor advised
	plain := `{"name":"Nosmith","realm":"Icecrown","level":"80","class":"Hunter","race":"Troll","faction":"Horde",` +
		`"equipment":[{"name":"W","item":"108","ench":0,"gems":[0,0,0]}],` +
		`"talents":[{"tree":"Survival","points":[0,15,56]}]}`
	m = openImport(t, m)
	m = paste(t, m, plain)
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if view := m.View(); strings.Contains(view, "Socket Bracer") {
		t.Fatalf("non-blacksmith must not see socket advice:\n%s", view)
	}
}

func TestFilterEnterSelectsNotCloses(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	m := New(cfg)
	m = drive(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = keys(t, m, "f")

	// enter on the phase-2 row selects it and keeps the modal open
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyDown})
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != modeFilter {
		t.Fatal("enter must not close the filter modal")
	}
	if m.filter.ServerPhase != 2 {
		t.Fatalf("enter did not select phase: %d", m.filter.ServerPhase)
	}
	// esc closes and persists
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != modeNormal {
		t.Fatal("esc must close")
	}
	fresh, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if f, ok := fresh.Filters["wotlk"]; !ok || f.ServerPhase != 2 {
		t.Fatalf("phase not persisted per game: %+v", fresh.Filters)
	}
}

func TestRaidTokenGating(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.UpsertCharacter([]byte(exportChunk1+exportChunk2), config.StoredCharacter{Game: "wotlk", Server: "warmane", Realm: "Icecrown"}, -1); err != nil {
		t.Fatal(err)
	}
	m := New(cfg)
	m = drive(t, m, tea.WindowSizeMsg{Width: 140, Height: 40})

	d := testDB()
	// T9.75-style tier: "vendor" source, but the vendor takes raid tokens
	tokenHelm := &db.Item{ID: 200, Name: "Windrunner's Helm of Triumph", Slot: db.SlotHead,
		ILvl: 258, Quality: db.QualityEpic, Phase: 3, ArmorType: db.ArmorMail,
		Sources: []db.Source{{SoldBy: &db.SoldBySource{NPCID: 35575, NPCName: "Champion Isimode", ZoneID: 210}}}}
	tokenHelm.Stats[db.Agility] = 260
	// T10.5-style: sourceless "Sanctified" upgrade (ICC marks)
	sancHelm := &db.Item{ID: 201, Name: "Sanctified Test Headpiece", Slot: db.SlotHead,
		ILvl: 264, Quality: db.QualityEpic, Phase: 4, ArmorType: db.ArmorMail}
	sancHelm.Stats[db.Agility] = 270
	// raid-quest reward (Yogg-Saron kill quest)
	questHelm := &db.Item{ID: 202, Name: "Brann's Test Helm", Slot: db.SlotHead,
		ILvl: 252, Quality: db.QualityEpic, Phase: 2, ArmorType: db.ArmorMail,
		Sources: []db.Source{{Quest: &db.QuestSource{ID: 1, Name: "Heroic: All Is Well That Ends Well"}}}}
	questHelm.Stats[db.Agility] = 255
	d.Items[tokenHelm.ID] = tokenHelm
	d.Items[sancHelm.ID] = sancHelm
	d.Items[questHelm.ID] = questHelm
	m = drive(t, m, dbLoadedMsg{slug: "wotlk", db: d})

	countUps := func() int {
		rows := m.gearRows()
		return len(m.scorer.UpgradesWorn(wornOf(rows), 0, 50, m.allowItem))
	}
	baseline := countUps()

	// raids off: both token-gated items vanish along with real raid drops
	m.filter.Raids = false
	noRaids := countUps()
	rows := m.gearRows()
	for _, u := range m.scorer.UpgradesWorn(wornOf(rows), 0, 50, m.allowItem) {
		if u.Item.ID == tokenHelm.ID || u.Item.ID == sancHelm.ID || u.Item.ID == questHelm.ID {
			t.Fatalf("raid-gated item proposed without raids: %s", u.Item.Name)
		}
	}
	if noRaids >= baseline {
		t.Fatalf("raids-off should shrink the list: %d vs %d", noRaids, baseline)
	}

	// raids on: both appear (phase ceiling at 5 allows them)
	m.filter.Raids = true
	found := 0
	for _, u := range m.scorer.UpgradesWorn(wornOf(rows), 0, 50, m.allowItem) {
		if u.Item.ID == tokenHelm.ID || u.Item.ID == sancHelm.ID || u.Item.ID == questHelm.ID {
			found++
		}
	}
	if found != 3 {
		t.Fatalf("raid-gated items missing with raids on: %d", found)
	}
}

func TestLowILvlCandidatesFiltered(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.UpsertCharacter([]byte(exportChunk1+exportChunk2), config.StoredCharacter{Game: "wotlk", Server: "warmane", Realm: "Icecrown"}, -1); err != nil {
		t.Fatal(err)
	}
	m := New(cfg)
	m = drive(t, m, tea.WindowSizeMsg{Width: 140, Height: 40})

	d := testDB()
	// vanilla-era junk: outstats the low fixture neck but is useless at 80
	oldNeck := &db.Item{ID: 300, Name: "Vanilla Amulet", Slot: db.SlotNeck,
		ILvl: 88, Quality: db.QualityEpic, Phase: 1}
	oldNeck.Stats[db.Agility] = 500
	d.Items[oldNeck.ID] = oldNeck
	m = drive(t, m, dbLoadedMsg{slug: "wotlk", db: d})

	rows := m.gearRows()
	for i, r := range rows {
		if r.item.Slot != db.SlotNeck {
			continue
		}
		for _, u := range m.scorer.UpgradesWorn(wornOf(rows), i, 50, m.allowItem) {
			if u.Item.ID == oldNeck.ID {
				t.Fatal("sub-130 ilvl item must never be proposed")
			}
		}
	}
}

func TestUpgradeListShowsNearbyWorseItems(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.UpsertCharacter([]byte(exportChunk1+exportChunk2), config.StoredCharacter{Game: "wotlk", Server: "warmane", Realm: "Icecrown"}, -1); err != nil {
		t.Fatal(err)
	}
	m := New(cfg)
	m = drive(t, m, tea.WindowSizeMsg{Width: 150, Height: 50})

	d := testDB()
	lesser := &db.Item{ID: 400, Name: "Lesser Helm", Slot: db.SlotHead, ILvl: 200,
		Quality: db.QualityEpic, Phase: 1, ArmorType: db.ArmorMail}
	lesser.Stats[db.Agility] = 10
	d.Items[lesser.ID] = lesser
	m = drive(t, m, dbLoadedMsg{slug: "wotlk", db: d})

	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // upgrades for head
	view := m.View()
	if !strings.Contains(view, "▼") || !strings.Contains(view, "Lesser Helm") {
		t.Fatalf("worse item missing from list:\n%s", view)
	}
	// downgrades come after all upgrades
	if strings.Index(view, "Raid Helm") > strings.Index(view, "Lesser Helm") {
		t.Fatal("worse items must sort below upgrades")
	}
}

func TestUpgradeListDedupesFactionCopies(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.UpsertCharacter([]byte(exportChunk1+exportChunk2), config.StoredCharacter{Game: "wotlk", Server: "warmane", Realm: "Icecrown"}, -1); err != nil {
		t.Fatal(err)
	}
	m := New(cfg)
	m = drive(t, m, tea.WindowSizeMsg{Width: 140, Height: 40})

	d := testDB()
	// faction copies: two item IDs, same name / stats / quest name —
	// wowhead's usual shape for a quest that exists on both sides
	for _, id := range []int{500, 501} {
		copyItem := &db.Item{ID: id, Name: "Lifecrusher Test Amulet", Slot: db.SlotNeck,
			ILvl: 245, Quality: db.QualityRare, Phase: 3,
			Sources: []db.Source{{Quest: &db.QuestSource{ID: id, Name: "Coup de Grace"}}}}
		copyItem.Stats[db.Agility] = 400
		d.Items[id] = copyItem
	}
	m = drive(t, m, dbLoadedMsg{slug: "wotlk", db: d})

	rows := m.gearRows()
	for i, r := range rows {
		if r.item.Slot == db.SlotNeck {
			m.upSlot = i
			break
		}
	}
	m.recomputeUpgrades()
	count := 0
	for _, u := range m.upList {
		if u.Item.Name == "Lifecrusher Test Amulet" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("faction copies must collapse to one row, got %d", count)
	}
}

func TestHonorTrinketsGatedAsPvP(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.UpsertCharacter([]byte(exportChunk1+exportChunk2), config.StoredCharacter{Game: "wotlk", Server: "warmane", Realm: "Icecrown"}, -1); err != nil {
		t.Fatal(err)
	}
	m := New(cfg)
	m = drive(t, m, tea.WindowSizeMsg{Width: 140, Height: 40})
	d := testDB()
	// Battlemaster's Rage: honor-bought, sourceless, no resilience
	bm := &db.Item{ID: 42136, Name: "Battlemaster's Rage", Slot: db.SlotHead,
		ILvl: 245, Quality: db.QualityEpic, Phase: 3, ArmorType: db.ArmorMail}
	bm.Stats[db.Agility] = 500
	d.Items[bm.ID] = bm
	m = drive(t, m, dbLoadedMsg{slug: "wotlk", db: d})

	if m.filter.PvP {
		t.Fatal("PvP filter must default off")
	}
	if m.allowItem(bm) {
		t.Fatal("honor-vendor item must be gated by the PvP toggle")
	}
	m.filter.PvP = true
	if !m.allowItem(bm) {
		t.Fatal("PvP on must allow it")
	}
	if got := ansi.Strip(m.sourceText(bm)); got != "PvP vendor: honor / marks" {
		t.Fatalf("source = %q", got)
	}

	// arena relics: no resilience, sourceless, below the generated
	// table's ilvl horizon — caught by the name-family rule
	relic := &db.Item{ID: 42574, Name: "Deadly Gladiator's Idol of Tenacity",
		Slot: db.SlotRanged, ILvl: 213, Quality: db.QualityEpic, Phase: 1}
	m.filter.PvP = false
	if m.allowItem(relic) {
		t.Fatal("Gladiator relic must be gated by the PvP toggle")
	}
}

func TestOptimizationAdvisorScreen(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	// enriched import: head with an empty red socket AND no enchant —
	// the joint plan must propose filling the socket and enchanting
	enriched := `{"name":"Testchar","realm":"Icecrown","level":"80","class":"Hunter","race":"Troll","faction":"Horde",` +
		`"equipment":[{"name":"X","item":"46143","ench":0,"gems":[41398,0]}],` +
		`"talents":[{"tree":"Survival","points":[0,15,56]}]}`
	if err := cfg.UpsertCharacter([]byte(enriched), config.StoredCharacter{Game: "wotlk", Server: "warmane", Realm: "Icecrown"}, -1); err != nil {
		t.Fatal(err)
	}
	d := testDB()
	arcanum := &db.Enchant{EffectID: 3820, Name: "Arcanum of Torment", Slot: db.SlotHead}
	arcanum.Stats[db.Agility] = 50 // hunter fixture weights AP at zero; agility scores
	d.Enchants = append(d.Enchants, arcanum)

	m := New(cfg)
	m = drive(t, m, tea.WindowSizeMsg{Width: 170, Height: 50})
	m = drive(t, m, dbLoadedMsg{slug: "wotlk", db: d})

	// the quick-tips pane points every domain at the ONE advisor
	if view := m.View(); !strings.Contains(view, "gems on 1 item not optimal") ||
		!strings.Contains(view, "enchants on 1 item not optimal") ||
		!strings.Contains(view, "to optimize") {
		t.Fatalf("tips pane must point at the optimization advisor:\n%s", view)
	}

	m = keys(t, m, "o")
	if m.mode != modeOptimize {
		t.Fatalf("o must open the optimization advisor, mode=%v", m.mode)
	}
	view := m.View()
	for _, want := range []string{"Optimization advisor", "gems", "enchants",
		"empty", "Delicate Cardinal Ruby", "no enchant", "Arcanum of Torment",
		"set totals if applied:", "Agility", "total set-score gain"} {
		if !strings.Contains(view, want) {
			t.Fatalf("advisor screen missing %q:\n%s", want, view)
		}
	}
	// wotlk has no reforging: the section (and its scope word) is absent
	if strings.Contains(view, "reforge") {
		t.Fatalf("wotlk advisor must not mention reforges:\n%s", view)
	}

	// the c hotkey copies the plan as plain text; check the text itself
	// rather than pressing c, which would overwrite the real clipboard
	text := m.optPlanText()
	for _, want := range []string{"empty -> [ Delicate Cardinal Ruby ]",
		"no enchant -> [ Arcanum of Torment ]", "total set-score gain"} {
		if !strings.Contains(text, want) {
			t.Fatalf("plan text missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "\x1b[") {
		t.Fatalf("plan text must be style-free:\n%q", text)
	}

	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != modeNormal {
		t.Fatal("esc must close the advisor")
	}
	// the old per-domain keys stay as aliases
	for _, k := range []string{"g", "e", "R"} {
		m = keys(t, m, k)
		if m.mode != modeOptimize {
			t.Fatalf("%s must open the optimization advisor", k)
		}
		m = drive(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	}

	// plain (non-enriched) import: the screen explains why it can't work
	plain := `{"name":"Plain","realm":"R","level":"80","class":"Hunter","race":"Troll","faction":"Horde",` +
		`"equipment":[{"name":"X","item":"46143"}],"talents":[{"tree":"Survival","points":[0,15,56]}]}`
	if err := cfg.UpsertCharacter([]byte(plain), config.StoredCharacter{Game: "wotlk", Server: "warmane", Realm: "R"}, -1); err != nil {
		t.Fatal(err)
	}
	m2 := New(cfg)
	m2 = drive(t, m2, tea.WindowSizeMsg{Width: 170, Height: 50})
	m2 = drive(t, m2, dbLoadedMsg{slug: "wotlk", db: d})
	m2 = keys(t, m2, "o")
	if !strings.Contains(m2.View(), "needs an enriched import") {
		t.Fatalf("plain import must explain the limitation:\n%s", m2.View())
	}
}

func TestOffhandSlotAlwaysShown(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	d := testDB()
	sword := &db.Item{ID: 200, Name: "Test Greatsword", Slot: db.SlotWeapon, ILvl: 245,
		Quality: db.QualityEpic, HandType: db.HandTwoHand, WeaponType: db.WeaponSword}
	sword.Stats[db.Strength] = 100
	d.Items[sword.ID] = sword

	load := func(t *testing.T, tree string) Model {
		cfg, err := config.Load()
		if err != nil {
			t.Fatal(err)
		}
		char := `{"name":"Testchar","realm":"Icecrown","level":"80","class":"Warrior","race":"Orc","faction":"Horde",` +
			`"equipment":[{"name":"S","item":"200"}],"talents":[{"tree":"` + tree + `","points":[18,53,0]}]}`
		if err := cfg.UpsertCharacter([]byte(char), config.StoredCharacter{Game: "wotlk", Server: "warmane", Realm: "Icecrown"}, -1); err != nil {
			t.Fatal(err)
		}
		m := New(cfg)
		m = drive(t, m, tea.WindowSizeMsg{Width: 170, Height: 50})
		return drive(t, m, dbLoadedMsg{slug: "wotlk", db: d})
	}

	// fury Titan-Grips: the empty off hand is an actionable slot
	view := load(t, "Fury").View()
	if !strings.Contains(view, "Off hand") ||
		!strings.Contains(view, "Titan's Grip fits a second two-hander") {
		t.Fatalf("fury with one 2H must show the off-hand slot as fillable:\n%s", view)
	}

	// arms cannot: the slot still shows, dimmed as blocked
	view = load(t, "Arms").View()
	if !strings.Contains(view, "Off hand") ||
		!strings.Contains(view, "two-hander occupies both hands") {
		t.Fatalf("arms with a 2H must show the off-hand slot as blocked:\n%s", view)
	}
}

func TestSeasonalBossLabel(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.UpsertCharacter([]byte(exportChunk1+exportChunk2), config.StoredCharacter{Game: "wotlk", Server: "warmane", Realm: "Icecrown"}, -1); err != nil {
		t.Fatal(err)
	}
	m := New(cfg)
	m = drive(t, m, tea.WindowSizeMsg{Width: 140, Height: 40})
	d := testDB()
	d.NPCs[25740] = "Ahune"
	d.Zones[3717] = "The Slave Pens"
	scythe := &db.Item{ID: 54806, Name: "Frostscythe of Lord Ahune", Slot: db.SlotHead,
		ILvl: 232, Quality: db.QualityEpic, Phase: 3, ArmorType: db.ArmorMail,
		Sources: []db.Source{{Drop: &db.DropSource{NPCID: 25740, ZoneID: 3717, Difficulty: 1}}}}
	m = drive(t, m, dbLoadedMsg{slug: "wotlk", db: d})

	got := ansi.Strip(m.sourceText(scythe))
	if !strings.Contains(got, "Ahune") || !strings.Contains(got, "(Midsummer Fire Festival only)") {
		t.Fatalf("seasonal label missing: %q", got)
	}
	// year-round sources stay unlabeled
	plain := d.Items[100] // Raid Helm
	if strings.Contains(ansi.Strip(m.sourceText(plain)), "only)") {
		t.Fatal("non-seasonal source must not carry an event label")
	}
}

// TestCraftedRecipeSource: crafted sources name where the recipe comes
// from — vendor with price and standing, drop instance, or trainer.
func TestCraftedRecipeSource(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.UpsertCharacter([]byte(exportChunk1+exportChunk2), config.StoredCharacter{Game: "wotlk", Server: "warmane", Realm: "Icecrown"}, -1); err != nil {
		t.Fatal(err)
	}
	m := New(cfg)
	m = drive(t, m, tea.WindowSizeMsg{Width: 140, Height: 40})
	m = drive(t, m, dbLoadedMsg{slug: "wotlk", db: testDB()})

	crafted := func(spell int) *db.Item {
		return &db.Item{ID: 900000 + spell, Name: "Crafted", Slot: db.SlotFeet, ILvl: 264,
			Sources: []db.Source{{Crafted: &db.CraftedSource{Profession: 8, SpellID: spell}}}}
	}
	for spell, want := range map[int]string{
		70557: "Crafted: Leatherworking · recipe sold: Alchemist Finklestein (The Ashen Verdict — Honored)",
		63194: "Crafted: Leatherworking · recipe drop: Ulduar",
		60600: "Crafted: Leatherworking (trainer)",
		1:     "Crafted: Leatherworking", // unknown spell: plain profession line
	} {
		if got := ansi.Strip(m.sourceText(crafted(spell))); got != want {
			t.Errorf("spell %d: got %q, want %q", spell, got, want)
		}
	}
}

func TestImportRejectsNonMaxLevel(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	m := New(cfg)
	m = drive(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = openImport(t, m)

	lowbie := strings.Replace(exportChunk1, `"level":"80"`, `"level":"74"`, 1) + exportChunk2
	m = paste(t, m, lowbie)
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != modeImport {
		t.Fatal("non-80 import must be refused (modal stays open)")
	}
	if !strings.Contains(m.errMsg, "level 74") || !strings.Contains(m.errMsg, "level 80") {
		t.Fatalf("unexpected error message: %q", m.errMsg)
	}
	if len(m.cfg.Characters()) != 0 {
		t.Fatal("refused character must not be stored")
	}
}

func TestBiSScreen(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.UpsertCharacter([]byte(exportChunk1+exportChunk2), config.StoredCharacter{Game: "wotlk", Server: "warmane", Realm: "Icecrown"}, -1); err != nil {
		t.Fatal(err)
	}
	m := New(cfg)
	m = drive(t, m, tea.WindowSizeMsg{Width: 140, Height: 40})
	m = drive(t, m, dbLoadedMsg{slug: "wotlk", db: testDB()})

	// b opens the BiS view: the head slot upgrades to the raid helm, the
	// neck (no candidates in the fixture) stays owned, and the NEVER-WORN
	// wrist/waist slots are synthesized — the fixture belt and bracer
	// surface as rows to acquire
	m = keys(t, m, "b")
	if m.mode != modeBiS {
		t.Fatalf("mode = %v, want BiS", m.mode)
	}
	view := m.View()
	for _, want := range []string{"Best-in-slot set", "1/4 pieces owned",
		"Raid Helm", "✓", "Test Broach", "equipped",
		"Wrist", "Test Bracer", "Waist", "Test Belt",
		// bottom panes: acquisition card + target-set overview
		"to acquire: ", "target set totals:"} {
		if !strings.Contains(view, want) {
			t.Fatalf("BiS view missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "Gladiator Helm") {
		t.Fatalf("PvP gear must not enter the BiS set by default:\n%s", view)
	}
	// to-acquire rows carry no marker — only owned pieces get the ✓
	if strings.Contains(view, "▲") {
		t.Fatalf("BiS view must not mark to-acquire rows:\n%s", view)
	}
	// the raid helm is not emblem-buyable: no badge budget line
	if strings.Contains(view, "emblem budget") {
		t.Fatalf("unexpected emblem budget:\n%s", view)
	}

	// f re-filters from inside the view: capping the server at phase 3
	// drops the P4 raid helm — the emblem helm becomes BiS and the
	// shopping list prices it in badges
	m = keys(t, m, "f")
	if m.mode != modeFilter || !m.fromBis {
		t.Fatalf("filter not opened from BiS: mode=%v fromBis=%v", m.mode, m.fromBis)
	}
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyDown})
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyDown}) // row 2 = phase 3
	m = keys(t, m, " ")
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != modeBiS {
		t.Fatalf("filter close must return to BiS, got %v", m.mode)
	}
	view = m.View()
	for _, want := range []string{"Emblem Helm", "emblem budget:", "75× Emblem of Triumph"} {
		if !strings.Contains(view, want) {
			t.Fatalf("re-solved BiS view missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "Raid Helm") {
		t.Fatalf("phase-3 ceiling must hide the P4 raid helm:\n%s", view)
	}

	// esc returns to the main screen
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != modeNormal {
		t.Fatalf("esc must exit, got %v", m.mode)
	}
}

func TestTalentReachabilityClamp(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	m := New(cfg)
	m = drive(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = drive(t, m, dbLoadedMsg{slug: "wotlk", db: testDB()})

	// hunter with only 2 points in Marksmanship: Focused Aim (3 ranks,
	// row 1 → 0 pts required) can hold at most 2 points
	m = openImport(t, m)
	low := strings.Replace(exportChunk1+exportChunk2, `"points":[0,15,56]`, `"points":[0,2,56]`, 1)
	m = paste(t, m, low)
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.char == nil || m.mode != modeNormal {
		t.Fatalf("import failed: %q", m.errMsg)
	}
	m = keys(t, m, "t")
	for range 3 { // try to push past the reachable bound
		m = drive(t, m, tea.KeyMsg{Type: tea.KeyRight})
	}
	fa := score.WotLK.CapTalentsFor(m.char.Class)[0]
	if fa.Name != "Focused Aim" {
		t.Fatalf("unexpected first hunter cap talent %q", fa.Name)
	}
	if got := m.talentPoints(fa); got != 2 {
		t.Fatalf("Focused Aim points = %d, want 2 (editor must stop at reachable)", got)
	}
	// partial limits show through the locked · ranks only — no label
	if view := ansi.Strip(m.View()); strings.Contains(view, "unreachable") {
		t.Fatalf("partially reachable talent must not be labeled unreachable:\n%s", view)
	}
	m = keys(t, m, "t") // close & persist the 2 points

	// same class, zero Marksmanship points: the shared per-class config
	// value must clamp to 0 at read time for THIS character
	m = openImport(t, m)
	none := strings.Replace(exportChunk1+exportChunk2, `"points":[0,15,56]`, `"points":[0,0,71]`, 1)
	none = strings.Replace(none, `"name":"Testchar"`, `"name":"Otherchar"`, 1)
	m = paste(t, m, none)
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != modeNormal {
		t.Fatalf("second import failed: %q", m.errMsg)
	}
	if got := m.talentPoints(fa); got != 0 {
		t.Fatalf("Focused Aim for 0-MM character = %d, want 0 (read-time clamp)", got)
	}
	if view := ansi.Strip(keys(t, m, "t").View()); !strings.Contains(view, "unreachable") {
		t.Fatalf("talent modal must mark Focused Aim unreachable:\n%s", view)
	}
}

func TestCharImportAge(t *testing.T) {
	for _, tc := range []struct {
		age  time.Duration
		want string
	}{
		{30 * time.Second, "just now"},
		{30 * time.Minute, "30m"},
		{5 * time.Hour, "5h"},
		{72 * time.Hour, "3d"},
	} {
		if got := charAge(time.Now().Add(-tc.age)); got != tc.want {
			t.Errorf("charAge(-%v) = %q, want %q", tc.age, got, tc.want)
		}
	}
	if charAge(time.Time{}) != "" {
		t.Error("zero time (pre-field entries) must render no age")
	}

	t.Setenv("HOME", t.TempDir())
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	m := New(cfg)
	m = drive(t, m, tea.WindowSizeMsg{Width: 140, Height: 40})
	m = openImport(t, m)
	m = paste(t, m, exportChunk1+exportChunk2)
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if len(cfg.Chars) != 1 || cfg.Chars[0].ImportedAt.IsZero() {
		t.Fatalf("import must stamp ImportedAt: %+v", cfg.Chars)
	}
	// the character switcher shows the age
	m = keys(t, m, "c")
	if view := ansi.Strip(m.View()); !strings.Contains(view, "just now") {
		t.Fatalf("switcher missing import age:\n%s", view)
	}
}

// TestRaidDebuffToggle: the talent modal carries a raid-debuff row for
// spell-hit-capped builds; toggling it lowers the effective cap and the
// caps readout shows the soft cap with the hard cap in parentheses.
func TestRaidDebuffToggle(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	mage := `{"name":"Frosty","realm":"Icecrown","level":"80","class":"Mage","race":"Gnome","faction":"Alliance",` +
		`"equipment":[{"name":"X","item":"45820"}],"talents":[{"tree":"Frost","points":[0,0,71]}]}`
	if err := cfg.UpsertCharacter([]byte(mage), config.StoredCharacter{Game: "wotlk", Server: "warmane", Realm: "Icecrown"}, -1); err != nil {
		t.Fatal(err)
	}
	m := New(cfg)
	m = drive(t, m, tea.WindowSizeMsg{Width: 170, Height: 50})
	m = drive(t, m, dbLoadedMsg{slug: "wotlk", db: testDB()})

	// base: 17% spell hit vs bosses ≈ 446 rating, no raid debuff assumed
	if view := m.View(); !strings.Contains(view, "Hit 40/446") {
		t.Fatalf("base spell hit cap missing:\n%s", view)
	}

	// the modal lists the raid debuff below the mage's own cap talent
	m = keys(t, m, "t")
	view := m.View()
	for _, want := range []string{"Precision", "Misery / Imp. Faerie Fire", "(raid)", "0/1", "3% spell hit"} {
		if !strings.Contains(view, want) {
			t.Fatalf("talent modal missing %q:\n%s", want, view)
		}
	}
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyDown}) // onto the raid row
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyRight})
	if view := m.View(); !strings.Contains(view, "1/1") {
		t.Fatalf("toggle not reflected in modal:\n%s", view)
	}
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEsc})

	// soft cap with the debuff, hard cap without it — both visible
	if view := m.View(); !strings.Contains(view, "Hit 40/367 (446)") {
		t.Fatalf("soft/hard cap readout missing:\n%s", view)
	}
	if !m.cfg.RaidDebuff["wotlk"] {
		t.Fatal("raid debuff not persisted to config")
	}

	// the hunter never sees any of it: physical hit has no such debuff
	hunter := exportChunk1 + exportChunk2
	if err := m.cfg.UpsertCharacter([]byte(hunter), config.StoredCharacter{Game: "wotlk", Server: "warmane", Realm: "Icecrown"}, -1); err != nil {
		t.Fatal(err)
	}
	m = keys(t, m, "c")
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyDown})
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.char.Name != "Testchar" {
		t.Fatalf("switch failed, active = %s", m.char.Name)
	}
	m = keys(t, m, "t")
	if view := m.View(); strings.Contains(view, "Misery") {
		t.Fatalf("raid row offered to a physical-hit build:\n%s", view)
	}
}

// TestUnknownMetaTip: wearing a meta the rules table doesn't cover
// surfaces the curation prompt in the tips pane — no activation state
// is guessed (the old code silently assumed one-of-each).
func TestUnknownMetaTip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	enriched := `{"name":"Testchar","realm":"Icecrown","level":"80","class":"Hunter","race":"Troll","faction":"Horde",` +
		`"equipment":[{"name":"X","item":"46143","ench":0,"gems":[60999,40112]}],` +
		`"talents":[{"tree":"Survival","points":[0,15,56]}]}`
	if err := cfg.UpsertCharacter([]byte(enriched), config.StoredCharacter{Game: "wotlk", Server: "warmane", Realm: "Icecrown"}, -1); err != nil {
		t.Fatal(err)
	}
	m := New(cfg)
	m = drive(t, m, tea.WindowSizeMsg{Width: 170, Height: 50})
	d := testDB()
	d.Gems[60999] = &db.Gem{ID: 60999, Name: "Mystery Diamond", Color: db.GemMeta, Quality: db.QualityRare}
	m = drive(t, m, dbLoadedMsg{slug: "wotlk", db: d})

	view := ansi.Strip(m.View())
	for _, want := range []string{"Mystery Diamond", "activation rule unknown", "curate gem 60999"} {
		if !strings.Contains(view, want) {
			t.Fatalf("unknown-meta tip missing %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "likely inactive") {
		t.Fatalf("no rule known, yet an activation state was claimed:\n%s", view)
	}
}

// TestFilterPerRealm: the upgrade filter is REALM-local — free-server
// realms progress independently, so a phase ceiling set on one realm
// (or game) must not follow a character switch, and each realm's
// choice is restored on return.
func TestFilterPerRealm(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	hunter := exportChunk1 + exportChunk2
	onyx := `{"name":"Onyxchar","realm":"Onyxia","level":"80","class":"Hunter","race":"Troll","faction":"Horde",` +
		`"equipment":[{"name":"X","item":"46143"}],"talents":[{"tree":"Survival","points":[0,15,56]}]}`
	for _, c := range []struct {
		raw    string
		stored config.StoredCharacter
	}{
		{hunter, config.StoredCharacter{Game: "wotlk", Server: "warmane", Realm: "Icecrown"}},
		{onyx, config.StoredCharacter{Game: "wotlk", Server: "warmane", Realm: "Onyxia"}},
		{fetchedExport, config.StoredCharacter{Game: "cata", Server: "whitemane", Realm: "Gilneas"}},
	} {
		if err := cfg.UpsertCharacter([]byte(c.raw), c.stored, -1); err != nil {
			t.Fatal(err)
		}
	}
	m := New(cfg) // active = the cata character (latest import)
	m = drive(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	if m.game.Slug != "cata" || m.filter.ServerPhase != m.game.MaxPhase() {
		t.Fatalf("boot: game=%s phase=%d, want cata at max phase", m.game.Slug, m.filter.ServerPhase)
	}

	// helpers: pick a phase in the open filter modal / switch character
	setPhase := func(downs int) {
		m = keys(t, m, "f")
		for m.fCursor > 0 { // the modal cursor persists across opens
			m = drive(t, m, tea.KeyMsg{Type: tea.KeyUp})
		}
		for range downs {
			m = drive(t, m, tea.KeyMsg{Type: tea.KeyDown})
		}
		m = drive(t, m, tea.KeyMsg{Type: tea.KeyEnter})
		m = drive(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	}
	switchTo := func(idx int) {
		m = keys(t, m, "c")
		for m.cCursor > idx {
			m = drive(t, m, tea.KeyMsg{Type: tea.KeyUp})
		}
		for m.cCursor < idx {
			m = drive(t, m, tea.KeyMsg{Type: tea.KeyDown})
		}
		m = drive(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	}

	// cap the CATA realm at phase 2
	setPhase(1)
	if m.filter.ServerPhase != 2 {
		t.Fatalf("cata phase = %d, want 2", m.filter.ServerPhase)
	}

	// Icecrown: its own default, not the cata ceiling; cap it at 3
	switchTo(0)
	if m.game.Slug != "wotlk" || m.filter.ServerPhase != m.game.MaxPhase() {
		t.Fatalf("icecrown inherited a foreign filter: game=%s phase=%d", m.game.Slug, m.filter.ServerPhase)
	}
	setPhase(2)
	if m.filter.ServerPhase != 3 {
		t.Fatalf("icecrown phase = %d, want 3", m.filter.ServerPhase)
	}

	// Onyxia: SAME game, different realm — must not inherit Icecrown's
	// ceiling (realms progress independently on free servers)
	switchTo(1)
	if m.game.Slug != "wotlk" || m.filter.ServerPhase != m.game.MaxPhase() {
		t.Fatalf("onyxia inherited icecrown's filter: phase=%d", m.filter.ServerPhase)
	}
	setPhase(0)
	if m.filter.ServerPhase != 1 {
		t.Fatalf("onyxia phase = %d, want 1", m.filter.ServerPhase)
	}

	// every realm's choice is restored on return
	switchTo(0)
	if m.filter.ServerPhase != 3 {
		t.Fatalf("icecrown filter not restored: %d", m.filter.ServerPhase)
	}
	switchTo(2)
	if m.game.Slug != "cata" || m.filter.ServerPhase != 2 {
		t.Fatalf("cata filter not restored: game=%s phase=%d", m.game.Slug, m.filter.ServerPhase)
	}

	// and the config carries them under realm-scoped keys
	for key, want := range map[string]int{
		"wotlk/warmane/icecrown": 3,
		"wotlk/warmane/onyxia":   1,
		"cata/whitemane/gilneas": 2,
	} {
		if f, ok := m.cfg.Filters[key]; !ok || f.ServerPhase != want {
			t.Fatalf("filter key %q = %+v (ok=%v), want phase %d", key, f, ok, want)
		}
	}
}

// TestEmblemAdvisor: p opens the spending plan — emblem-buyable
// upgrade candidates grouped by currency and ranked by gain per token.
func TestEmblemAdvisor(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.UpsertCharacter([]byte(exportChunk1+exportChunk2), config.StoredCharacter{Game: "wotlk", Server: "warmane", Realm: "Icecrown"}, -1); err != nil {
		t.Fatal(err)
	}
	m := New(cfg)
	m = drive(t, m, tea.WindowSizeMsg{Width: 140, Height: 40})
	m = drive(t, m, dbLoadedMsg{slug: "wotlk", db: testDB()})
	if m.scorer == nil {
		t.Fatal("scorer must be built for Survival Hunter")
	}

	m = drive(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	if m.mode != modeEmblems {
		t.Fatalf("mode = %v, want modeEmblems", m.mode)
	}
	if len(m.embList) == 0 {
		t.Fatal("no emblem buys found — the fixture's Emblem Helm (75× Triumph) must qualify")
	}
	b := m.embList[0]
	if b.up.Item.ID != 40787 || b.price.Emblem != db.EmblemTriumph || b.price.Count != 75 {
		t.Fatalf("top buy = item %d for %d× %v, want 40787 for 75× Triumph",
			b.up.Item.ID, b.price.Count, b.price.Emblem)
	}
	view := m.View()
	for _, want := range []string{"Emblem advisor", "Emblem of Triumph", "Emblem Helm", "75×", "/token"} {
		if !strings.Contains(view, want) {
			t.Fatalf("emblem advisor view missing %q:\n%s", want, view)
		}
	}
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != modeNormal {
		t.Fatalf("esc must return to normal, got %v", m.mode)
	}

	// currency phase gate: the fixture helm is a phase-2 ITEM priced in
	// Triumph — a currency that only ships with ToC (phase 3). On a P2
	// server the item is content-legal but the price must not be
	// offered (the leak the item-phase filter alone cannot catch).
	m.filter.ServerPhase = 2
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	if m.mode != modeEmblems {
		t.Fatalf("mode = %v, want modeEmblems", m.mode)
	}
	for _, b := range m.embList {
		if b.price.Emblem == db.EmblemTriumph {
			t.Fatalf("Triumph-priced buy offered at phase 2: %+v", b)
		}
	}
	if !strings.Contains(m.View(), "no emblem-buyable upgrades") {
		t.Fatalf("P2 advisor should be empty:\n%s", m.View())
	}
}

// TestSortEmblemBuys: groups follow the game's currency order, rows
// inside a group rank by gain per token.
func TestSortEmblemBuys(t *testing.T) {
	buy := func(row int, e db.Emblem, count int, delta float64) emblemBuy {
		return emblemBuy{rowIdx: row, up: score.Upgrade{Delta: delta},
			price: db.EmblemPrice{Emblem: e, Count: count}}
	}
	list := []emblemBuy{
		buy(0, db.EmblemTriumph, 50, 100), // 2.0/token, later currency
		buy(1, db.EmblemFrost, 60, 60),    // 1.0/token
		buy(2, db.EmblemFrost, 95, 380),   // 4.0/token — Frost's best
	}
	sortEmblemBuys(list, []db.Emblem{db.EmblemFrost, db.EmblemTriumph})
	got := []int{list[0].rowIdx, list[1].rowIdx, list[2].rowIdx}
	if got[0] != 2 || got[1] != 1 || got[2] != 0 {
		t.Fatalf("order = %v, want [2 1 0] (Frost by ratio, then Triumph)", got)
	}
}

// TestDedupeEmblemBuys: one vendor ring suggested for both finger rows
// collapses to the single better trade (unique-equipped, bought once).
func TestDedupeEmblemBuys(t *testing.T) {
	ring := &db.Item{ID: 7001, Name: "Vendor Ring"}
	other := &db.Item{ID: 7002, Name: "Vendor Legs"}
	price := db.EmblemPrice{Emblem: db.EmblemFrost, Count: 60}
	list := []emblemBuy{
		{rowIdx: 4, up: score.Upgrade{Item: ring, Delta: 120}, price: price}, // weaker replacement
		{rowIdx: 5, up: score.Upgrade{Item: ring, Delta: 300}, price: price}, // better: weaker worn ring
		{rowIdx: 6, up: score.Upgrade{Item: other, Delta: 200}, price: price},
	}
	got := dedupeEmblemBuys(list)
	if len(got) != 2 {
		t.Fatalf("deduped to %d rows, want 2: %+v", len(got), got)
	}
	for _, b := range got {
		if b.up.Item.ID == 7001 && b.rowIdx != 5 {
			t.Fatalf("kept the weaker ring replacement (row %d), want row 5", b.rowIdx)
		}
	}
}

// TestStrongestPerSlot: per slot and currency only the strongest buy
// is shown — a cheaper weaker one (better gain per token) would have
// the player buy the slot twice.
func TestStrongestPerSlot(t *testing.T) {
	item := func(id int) *db.Item { return &db.Item{ID: id} }
	frag := func(n int) db.EmblemPrice { return db.EmblemPrice{Emblem: db.EmblemFissure, Count: n} }
	list := []emblemBuy{
		{rowIdx: 2, up: score.Upgrade{Item: item(1), Delta: 150}, price: frag(32)}, // weak shoulders, better ratio
		{rowIdx: 2, up: score.Upgrade{Item: item(2), Delta: 300}, price: frag(60)}, // strong shoulders
		{rowIdx: 2, up: score.Upgrade{Item: item(3), Delta: 120}, price: frag(70)},
		{rowIdx: 2, up: score.Upgrade{Item: item(4), Delta: 100}, price: db.EmblemPrice{Emblem: db.EmblemJustice, Count: 1650}},
		{rowIdx: 5, up: score.Upgrade{Item: item(5), Delta: 80}, price: frag(20)},
	}
	got := strongestPerSlot(list)
	ids := map[int]bool{}
	for _, b := range got {
		ids[b.up.Item.ID] = true
	}
	if len(got) != 3 || !ids[2] || !ids[4] || !ids[5] {
		t.Fatalf("kept %v, want the strong shoulders, the Justice shoulders and row 5", ids)
	}
}

// TestImportKeystrokePaste: Windows consoles deliver a paste as one
// keystroke per character (bubbletea's coninput reader never sets
// Paste). The export must still land in the buffer — even with the name
// field focused, and with letters that are shortcuts on this step (the
// "n" of "name" used to focus the name field and swallow the rest).
func TestImportKeystrokePaste(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	m := drive(t, New(cfg), tea.WindowSizeMsg{Width: 120, Height: 40})
	m = keys(t, m, "i")
	m.impCursor = 1                                 // Warmane — Icecrown
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // name field, focused
	m = keys(t, m, "Testchar")

	export := exportChunk1 + exportChunk2
	m = keys(t, m, export)
	if got := m.impName.Value(); got != "Testchar" {
		t.Fatalf("name field took the export: %q", got)
	}
	if string(m.pasted) != export {
		t.Fatalf("buffer = %q, want the export", m.pasted)
	}

	// esc drops a half-received export without leaving the step
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEsc})
	if len(m.pasted) != 0 || m.impStep != 1 {
		t.Fatalf("esc: buffer %q, step %d", m.pasted, m.impStep)
	}

	m = keys(t, m, export)
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != modeNormal || m.char == nil || m.char.Name != "Testchar" {
		t.Fatalf("keystroke paste did not import: mode %v err %q", m.mode, m.errMsg)
	}
}
