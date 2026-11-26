package tui

import (
	"math"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/vaclav-dvorak/twolk/internal/config"
	"github.com/vaclav-dvorak/twolk/internal/db"
)

// foreverExport is what forever.FetchRaw produces for a level-32
// hunter (shape covered by that package's own tests); item and enchant
// ids are real so the embedded Forever database resolves them.
const foreverExport = `{"name":"Brannoc Ironvale","realm":"EU","level":32,"faction":"Alliance",
	"race":"Dwarf","class":"Hunter","origin":"blizzard forever",
	"equipment":[
		{"name":"Hillman's Shoulders","item":4251,"ench":0,"gems":[],"slot":"Shoulder"},
		{"name":"Toughened Leather Gloves","item":4253,"ench":8484,"gems":[],"slot":"Hands"},
		{"name":"Golemsight Long Gun","item":273029,"ench":0,"gems":[],"slot":"Ranged"}],
	"talents":[{"tree":"Beast Mastery","points":null}],
	"professions":[{"name":"Leatherworking","skill":152}],
	"talent_ranks":{"134422":5}}`

// TestForeverFetchImport walks the wizard onto the Blizzard EU row and
// feeds it a fetched export: a LEVEL-32 character is accepted (Forever
// supports leveling), stored under the forever slug with the region as
// its realm, and its leveling gear renders from the embedded database.
func TestForeverFetchImport(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	m := New(cfg)
	m = drive(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})

	m = keys(t, m, "i")
	eu := -1
	for i, r := range m.realmRows() {
		if r.server.Slug == "blizzard" && r.realm == "EU" {
			eu = i
		}
	}
	if eu < 0 {
		t.Fatalf("Blizzard EU not offered: %+v", m.realmRows())
	}
	m.impCursor = eu
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = keys(t, m, "Brannoc Ironvale")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // save → refresh → fetch
	m = next.(Model)
	if !m.impFetching {
		t.Fatal("adding a Forever character must start its fetch at once")
	}

	m = drive(t, m, importFetchedMsg{raw: []byte(foreverExport)})
	if m.mode != modeNormal {
		t.Fatalf("import did not complete: err=%q", m.errMsg)
	}
	if m.game.Slug != "forever" {
		t.Fatalf("active module = %s, want forever", m.game.Slug)
	}
	if len(cfg.Chars) != 1 || cfg.Chars[0].Game != "forever" ||
		cfg.Chars[0].Server != "blizzard" || cfg.Chars[0].Realm != "EU" {
		t.Fatalf("stored meta wrong: %+v", cfg.Chars)
	}

	d, err := db.LoadForever()
	if err != nil {
		t.Fatal(err)
	}
	m = drive(t, m, dbLoadedMsg{slug: "forever", db: d})
	view := m.View()
	for _, want := range []string{
		"Forever 1.60 (official)", "Brannoc Ironvale", "32", "Beast Mastery",
		"Hillman's Shoulders", "Toughened Leather Gloves", "Golemsight Long Gun",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("gear screen missing %q", want)
		}
	}

	// Forever has no emblem currencies: no advisor, no key, and the
	// filter's vendor row is plain vendor gear
	if strings.Contains(view, "emblems") {
		t.Error("help row offers the emblem advisor")
	}
	if m = keys(t, m, "p"); m.mode == modeEmblems {
		t.Error("p opened the emblem advisor")
	}
	m = keys(t, m, "f")
	if f := ansi.Strip(m.View()); strings.Contains(f, "Emblem") || !strings.Contains(f, "Vendor gear") {
		t.Error("filter modal still talks about emblems")
	}
}

// TestForeverEmptySlotsSelectable: a leveling character's open slots
// are real, selectable rows (stand-ins for "nothing"), so the upgrade
// browser can list what fits there; they don't drag the average down.
func TestForeverEmptySlotsSelectable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	m := New(cfg)
	m = drive(t, m, tea.WindowSizeMsg{Width: 120, Height: 60})
	m = keys(t, m, "i")
	for i, r := range m.realmRows() {
		if r.server.Slug == "blizzard" && r.realm == "EU" {
			m.impCursor = i
		}
	}
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = keys(t, m, "Brannoc Ironvale")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	m = drive(t, m, importFetchedMsg{raw: []byte(foreverExport)})
	d, err := db.LoadForever()
	if err != nil {
		t.Fatal(err)
	}
	m = drive(t, m, dbLoadedMsg{slug: "forever", db: d})

	rows := m.gearRows()
	head := -1
	for i, r := range rows {
		if r.item.Slot == db.SlotHead {
			head = i
		}
	}
	if head < 0 || !rows[head].empty {
		t.Fatalf("no empty head row: %+v", rows)
	}
	// 3 worn items + 12 open standard slots (15 in all: two rings, two trinkets)
	if len(rows) != 15 {
		t.Errorf("%d rows, want 15", len(rows))
	}
	// empty slots count as ilvl 0, as the armory's average does:
	// (26 + 27 + 33) / 15 rows
	if avg := ilvlStats(rows); math.Abs(avg-86.0/15) > 0.01 {
		t.Errorf("avg ilvl %.2f, want %.2f", avg, 86.0/15)
	}
	if !strings.Contains(ansi.Strip(m.View()), "▲ Head         0  unequipped") {
		t.Error("empty head not flagged as the weakest slot")
	}
	m.cursor = head
	if !strings.Contains(m.View(), "nothing equipped") {
		t.Error("detail pane of an empty slot")
	}

	// the empty head opens the upgrade browser, gated to level 32: a
	// leveling hunter wears leather, quest rewards wait for their item
	// level, and level-60 enchants (Warden's Signets) aren't assumed
	if m.scorer == nil {
		t.Fatal("no scorer for a Forever BM hunter")
	}
	// a dwarf: Artisan's Precision's +1% hit lowers the 8% cap to 7%
	if !strings.Contains(ansi.Strip(m.View()), "Hit 0%/7%") {
		t.Error("dwarf hit cap not lowered by the racial")
	}
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != modeUpgrades || len(m.upList) == 0 {
		t.Fatalf("no upgrade list for the empty head (mode %v)", m.mode)
	}
	names := map[string]bool{}
	for _, u := range m.upList {
		names[u.Item.Name] = true
		if lvl := equipLevel(u.Item); lvl > 32 {
			t.Errorf("%s needs level %d", u.Item.Name, lvl)
		}
		if u.Item.ArmorType != db.ArmorLeather {
			t.Errorf("%s is armor type %v, want leather below 40", u.Item.Name, u.Item.ArmorType)
		}
		if e := m.scorer.BestEnchant(u.Item); e != nil && e.ReqLevel == 0 {
			t.Errorf("%s assumes %s, an endgame enchant", u.Item.Name, e.Name)
		}
	}
	if !names["Tribal Worg Helm"] { // requires 29
		t.Error("Tribal Worg Helm not offered")
	}
	if names["Bloodsplattered Bandana"] { // ilvl 55 quest reward claiming level 1
		t.Error("ilvl-55 quest reward offered at 32")
	}
}

// TestForeverPercentStats: Forever gear states hit and crit as
// percentages; the data's fixed ratings must read the way the tooltip
// does ("+1% Crit", not "+14 Crit").
func TestForeverPercentStats(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	m := New(cfg)
	m.game = m.moduleBySlug("forever")
	var s db.Stats
	s[db.MeleeCrit], s[db.MeleeHit], s[db.Agility] = 7, 10, 5
	got := strings.Join(m.statLines(s, nil), ", ")
	for _, want := range []string{"+5 Agility", "+1% Hit", "+0.5% Crit"} {
		if !strings.Contains(got, want) {
			t.Errorf("stat lines %q missing %q", got, want)
		}
	}
}

// TestForeverCraftLabels: crafts the character can make (profession at
// the learn level) read "craftable", every other craft "buy
// on AH" — except crafts that bind on pickup, which only their crafter
// can get and so drop out of the proposals.
func TestForeverCraftLabels(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	m := New(cfg)
	m = drive(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = keys(t, m, "i")
	for i, r := range m.realmRows() {
		if r.server.Slug == "blizzard" && r.realm == "EU" {
			m.impCursor = i
		}
	}
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = keys(t, m, "Brannoc Ironvale")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	m = drive(t, m, importFetchedMsg{raw: []byte(foreverExport)}) // Leatherworking 152
	d, err := db.LoadForever()
	if err != nil {
		t.Fatal(err)
	}
	m = drive(t, m, dbLoadedMsg{slug: "forever", db: d})

	for _, c := range []struct {
		id      int
		label   string
		allowed bool
	}{
		{252520, "craftable", true}, // Skulker's Leather Belt: LW 150, BoP
		{277043, "buy on AH", true},             // Cloudy Gustwoven Spaulders: LW 175, BoE
		{250533, "buy on AH", true},             // Hard Gold Cuirass: blacksmithing, BoE
		{250532, "buy on AH", false},            // Crusader's Silvered Chain Helm: blacksmithing, BoP
	} {
		it := d.Items[c.id]
		if it == nil {
			t.Fatalf("item %d missing", c.id)
		}
		if src := ansi.Strip(m.sourceText(it)); !strings.Contains(src, c.label) {
			t.Errorf("%s: source %q, want %q", it.Name, src, c.label)
		}
		if got := m.allowItem(it); got != c.allowed {
			t.Errorf("%s: allowed = %v, want %v", it.Name, got, c.allowed)
		}
	}

	// "craftable" stands out in the version's accent:
	// Forever's #00D7D7 is 256-color 44 (no TTY in tests: force it)
	lipgloss.SetColorProfile(termenv.ANSI256)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })
	if src := m.sourceText(d.Items[252520]); !strings.Contains(src, "38;5;44") {
		t.Errorf("craftable-by-yourself tag not in the accent color: %q", src)
	}
	if src := m.sourceText(d.Items[277043]); strings.Contains(src, "38;5;44") {
		t.Errorf("buy-on-AH line uses the accent color: %q", src)
	}
	// ...and keeps it on the highlighted (cursor) row of a list
	if src := m.sourceTextSel(d.Items[252520], rowStyler(true)); !strings.Contains(src, "38;5;44") {
		t.Errorf("accent lost on the cursor row: %q", src)
	}
}

// TestForeverEnchantToggle: below max level candidates are scored bare
// (nobody enchants gear they outlevel in a few levels); e in the upgrade
// browser switches the best-enchant assumption on and back off.
func TestForeverEnchantToggle(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	m := New(cfg)
	m = drive(t, m, tea.WindowSizeMsg{Width: 120, Height: 60})
	m = keys(t, m, "i")
	for i, r := range m.realmRows() {
		if r.server.Slug == "blizzard" && r.realm == "EU" {
			m.impCursor = i
		}
	}
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = keys(t, m, "Brannoc Ironvale")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	m = drive(t, m, importFetchedMsg{raw: []byte(foreverExport)}) // level 32
	d, err := db.LoadForever()
	if err != nil {
		t.Fatal(err)
	}
	m = drive(t, m, dbLoadedMsg{slug: "forever", db: d})

	// the gloves slot: Heavy Armor Kit (Leatherworking, level 30) is a
	// real candidate enchant at 32
	for i, r := range m.gearRows() {
		if r.item.Slot == db.SlotHands {
			m.cursor = i
		}
	}
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.assumeEnchants() || !strings.Contains(ansi.Strip(m.View()), "no enchants") {
		t.Fatal("a level-32 character assumes enchants by default")
	}
	gloves := d.Items[4253]
	if e := m.scorer.BestEnchant(gloves); e != nil {
		t.Errorf("enchant %s assumed with the assumption off", e.Name)
	}
	bare := m.scorer.Value(m.scorer.EffectiveStats(gloves))

	m = keys(t, m, "e")
	if !m.assumeEnchants() || !strings.Contains(ansi.Strip(m.View()), "best enchants") {
		t.Fatal("e did not switch the enchant assumption on")
	}
	if e := m.scorer.BestEnchant(gloves); e == nil {
		t.Error("no enchant assumed with the assumption on")
	} else if m.scorer.Value(m.scorer.EffectiveStats(gloves)) <= bare {
		t.Errorf("enchanted gloves score %.1f, bare %.1f", m.scorer.Value(m.scorer.EffectiveStats(gloves)), bare)
	}

	m = keys(t, m, "e")
	if m.assumeEnchants() {
		t.Error("second e did not switch it back off")
	}
}

// TestForeverFactionSides: an Alliance character sees only Alliance and
// neutral routes, items reachable only from the Horde side are never
// proposed, and same-named routes render once.
func TestForeverFactionSides(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	m := New(cfg)
	m = drive(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = keys(t, m, "i")
	for i, r := range m.realmRows() {
		if r.server.Slug == "blizzard" && r.realm == "EU" {
			m.impCursor = i
		}
	}
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = keys(t, m, "Brannoc Ironvale")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	m = drive(t, m, importFetchedMsg{raw: []byte(foreverExport)}) // Alliance
	d, err := db.LoadForever()
	if err != nil {
		t.Fatal(err)
	}
	m = drive(t, m, dbLoadedMsg{slug: "forever", db: d})

	// Triprunner Dungarees: "Rig Wars" is the Horde quest, "The Grand
	// Betrayal" the Alliance one
	src := ansi.Strip(m.sourceText(d.Items[9624]))
	if !strings.Contains(src, "The Grand Betrayal") || strings.Contains(src, "Rig Wars") {
		t.Errorf("Triprunner Dungarees for Alliance: %q", src)
	}
	// Argent Dawn Commission: three neutral quests of the same name
	if src := ansi.Strip(m.sourceText(d.Items[12846])); strings.Count(src, "Argent Dawn Commission") != 1 {
		t.Errorf("Argent Dawn Commission: %q, want the quest once", src)
	}
	// Advisor's Ring: only the Horde Warsong Gulch quartermaster sells
	// it — never proposed to Alliance, every route switched on or not
	m.filter.PvP, m.filter.Vendor, m.filter.Other = true, true, true
	if m.allowItem(d.Items[19521]) {
		t.Error("a Horde-only vendor item proposed to an Alliance character")
	}
}

// TestForeverPvPFilter: Forever's PvP gear follows the filter's PvP
// toggle (off by default), and the row names honor and battlegrounds
// rather than resilience, which Forever doesn't have.
func TestForeverPvPFilter(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	m := New(cfg)
	m = drive(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = keys(t, m, "i")
	for i, r := range m.realmRows() {
		if r.server.Slug == "blizzard" && r.realm == "EU" {
			m.impCursor = i
		}
	}
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = keys(t, m, "Brannoc Ironvale")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	m = drive(t, m, importFetchedMsg{raw: []byte(foreverExport)}) // Alliance
	d, err := db.LoadForever()
	if err != nil {
		t.Fatal(err)
	}
	m = drive(t, m, dbLoadedMsg{slug: "forever", db: d})

	rune := d.Items[21566] // Rune of Perfection (req 20): Silverwing Sentinels (Alliance WSG)
	if m.allowItem(rune) {
		t.Error("PvP gear proposed with the PvP toggle off (the default)")
	}
	m.filter.PvP = true
	if !m.allowItem(rune) {
		t.Error("PvP gear hidden with the PvP toggle on")
	}
	if m.allowItem(d.Items[274444]) { // Greater Insignia of the Horde
		t.Error("the Horde insignia passes for an Alliance character")
	}
	m = keys(t, m, "f")
	if f := ansi.Strip(m.View()); !strings.Contains(f, "PvP gear (honor & battlegrounds)") {
		t.Error("filter row still talks about resilience")
	}
}

// TestForeverUnpricedEffects: an on-use with no stats (a summon) is
// named and flagged unscored, so a 0 score explains itself.
func TestForeverUnpricedEffects(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	m := New(cfg)
	m.game = m.moduleBySlug("forever")
	d, err := db.LoadForever()
	if err != nil {
		t.Fatal(err)
	}
	m.game.DB = d
	es := d.Effects[3456] // Dog Whistle
	if len(es) == 0 {
		t.Fatal("Dog Whistle's on-use not loaded")
	}
	got := ansi.Strip(strings.Join(m.effectLines(es[0]), " "))
	for _, want := range []string{"Use: Summon Tracking Hound", "10m", "not scored"} {
		if !strings.Contains(got, want) {
			t.Errorf("effect line %q missing %q", got, want)
		}
	}
	if es[0].Bonus != (db.Stats{}) {
		t.Errorf("a summon carries stats: %v", es[0].Bonus)
	}
}

// TestForeverOptimizeEnchants: below max level the candidate enchant
// assumption is off, yet the advisor still proposes enchants of the
// character's level for the worn gear, and no gem text shows anywhere
// (Forever has no gems).
func TestForeverOptimizeEnchants(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	m := New(cfg)
	m = drive(t, m, tea.WindowSizeMsg{Width: 160, Height: 60})
	m = keys(t, m, "i")
	for i, r := range m.realmRows() {
		if r.server.Slug == "blizzard" && r.realm == "EU" {
			m.impCursor = i
		}
	}
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = keys(t, m, "Brannoc Ironvale")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	m = drive(t, m, importFetchedMsg{raw: []byte(foreverExport)}) // level 32
	d, err := db.LoadForever()
	if err != nil {
		t.Fatal(err)
	}
	m = drive(t, m, dbLoadedMsg{slug: "forever", db: d})
	if m.assumeEnchants() {
		t.Fatal("a level-32 character assumes candidate enchants")
	}
	if strings.Contains(strings.ToLower(m.status), "gem") {
		t.Errorf("import status mentions gems: %q", m.status)
	}
	m = keys(t, m, "o")
	plan := m.optPlan
	if len(plan.Enchants) == 0 {
		t.Fatal("the advisor proposes no enchant for a level-32 character")
	}
	rows := m.gearRows()
	for _, tip := range plan.Enchants {
		if rows[tip.ItemIdx].empty {
			t.Errorf("enchant %s proposed for the empty %s slot", tip.To.Name, rows[tip.ItemIdx].item.Slot)
		}
		need := tip.To.ReqLevel
		if need == 0 {
			need = m.game.PlayerLevel
		}
		if need > 32 {
			t.Errorf("%s needs level %d", tip.To.Name, need)
		}
	}
	view := strings.ToLower(ansi.Strip(m.View()))
	if strings.Contains(view, "gem") || strings.Contains(view, "socket") {
		t.Errorf("the Forever advisor mentions gems:\n%s", view)
	}
	// the candidate assumption is untouched by the advisor
	if e := m.scorer.BestEnchant(d.Items[4253]); e != nil {
		t.Errorf("advisor left enchant %s assumed on candidates", e.Name)
	}
}

// importForever drives a fresh model through the Forever import of the
// given export JSON and loads the Forever database.
func importForever(t *testing.T, export string) (Model, *db.Database) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	m := New(cfg)
	m = drive(t, m, tea.WindowSizeMsg{Width: 160, Height: 40})
	m = keys(t, m, "i")
	for i, r := range m.realmRows() {
		if r.server.Slug == "blizzard" && r.realm == "EU" {
			m.impCursor = i
		}
	}
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = keys(t, m, "Brannoc Ironvale")
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	m = drive(t, m, importFetchedMsg{raw: []byte(export)})
	d, err := db.LoadForever()
	if err != nil {
		t.Fatal(err)
	}
	return drive(t, m, dbLoadedMsg{slug: "forever", db: d}), d
}

// TestForeverRecipeRoutes: crafted gear names where its recipe comes
// from — the profession camp of the character's own side, reputation
// vendors with the standing they need — after the craftable label.
func TestForeverRecipeRoutes(t *testing.T) {
	// Totemic Leather Gloves: Leatherworking 75, sold at the camps
	m, d := importForever(t, foreverExport) // Alliance, Leatherworking 152
	src := ansi.Strip(m.sourceText(d.Items[252497]))
	want := "craftable · ACA · 30 Favor"
	if !strings.Contains(src, want) {
		t.Errorf("Alliance: %q, want %q", src, want)
	}
	// the camp label is gold on the line
	if raw := m.sourceText(d.Items[252497]); !strings.Contains(raw, goldStyle.Render("ACA")) {
		t.Errorf("ACA not gold: %q", raw)
	}
	// the footer spells the abbreviations out on a screen showing them
	foot := ansi.Strip(m.pinFooter(m.sourceText(d.Items[252497]), "esc back"))
	if !strings.Contains(foot, "ACA Azeroth Commerce Authority") || !strings.Contains(foot, "Favor Merchant's Favor") ||
		strings.Contains(foot, "DSL") {
		t.Errorf("legend: %q", foot)
	}
	if foot := ansi.Strip(m.pinFooter("Crafted: Leatherworking", "esc back")); strings.Contains(foot, "Azeroth") {
		t.Errorf("legend without an abbreviation on screen: %q", foot)
	}
	horde := strings.Replace(foreverExport, `"faction":"Alliance"`, `"faction":"Horde"`, 1)
	m, d = importForever(t, horde)
	src = ansi.Strip(m.sourceText(d.Items[252497]))
	if !strings.Contains(src, "DSL · 30 Favor") || strings.Contains(src, "ACA") {
		t.Errorf("Horde: %q, want the Durotar camp", src)
	}
	// Forever's own Kirin Tor is Alliance-only and its Earthen Ring
	// Horde-only (client race masks): the other side can't learn the
	// recipe, whatever its skill
	const azure, cloudy = 1301537, 1301529 // Gustwoven Spaulders crafts
	for _, c := range []struct {
		spell         int
		faction, want string
	}{
		{azure, "Alliance", "quartermaster (Kirin Tor — Friendly)"},
		{cloudy, "Alliance", " (Earthen Ring — Friendly)"}, // no route text for the other side
		{azure, "Horde", " (Kirin Tor — Friendly)"},
		{cloudy, "Horde", "quartermaster (Earthen Ring — Friendly)"},
	} {
		r, ok := d.RecipeFor(c.spell, c.faction)
		if !ok || r.Where+" ("+r.Rep+")" != c.want {
			t.Errorf("%d for %s: %+v, want %q", c.spell, c.faction, r, c.want)
		}
		learnable := strings.HasPrefix(c.want, "quartermaster")
		if d.RecipeLearnable(c.spell, c.faction) != learnable {
			t.Errorf("%d learnable by %s = %v, want %v", c.spell, c.faction, !learnable, learnable)
		}
	}
	// on the line, the other side's recipe says nothing at all
	for _, it := range d.Items {
		for _, src := range it.Sources {
			if src.Crafted != nil && src.Crafted.SpellID == azure { // Alliance-only; m is Horde here
				if line := ansi.Strip(m.sourceText(it)); strings.Contains(line, "only") || strings.Contains(line, "Kirin") {
					t.Errorf("Alliance-only recipe shown to a Horde character: %q", line)
				}
			}
		}
	}
	// a craft whose recipe no source places says so
	if line := ansi.Strip(m.sourceText(d.Items[250498])); !strings.Contains(line, "recipe unknown") { // Veteran's Chain Helm
		t.Errorf("unplaced recipe: %q", line)
	}
	// a reputation recipe: the vendor and the standing (client data)
	r, ok := d.RecipeFor(20848, "Alliance") // Flarecore Mantle
	if !ok || r.Where != "Lokhtos Darkbargainer" || r.Rep != "Thorium Brotherhood — Honored" {
		t.Errorf("Flarecore Mantle recipe: %+v", r)
	}
}

// TestForeverHitTalentsDetected: a Forever character's hit cap follows
// its imported talent ranks — a mage with Precision 3/3 (+6% spell hit)
// caps gear at 11%, one without at the raw 17%.
func TestForeverHitTalentsDetected(t *testing.T) {
	// a Dwarf's racial is named too
	dwarf, _ := importForever(t, foreverExport)
	if v := ansi.Strip(dwarf.View()); !strings.Contains(v, "Hit 0%/7% (1% racial)") {
		t.Errorf("Dwarf header lacks the racial note")
	}
	mage := strings.NewReplacer(`"class":"Hunter"`, `"class":"Mage"`, `"race":"Dwarf"`, `"race":"Gnome"`,
		`"Beast Mastery"`, `"Frost"`).Replace(foreverExport)
	for _, c := range []struct {
		ranks, want string
	}{
		{`{"135308":3}`, "Hit 0%/11% (6% talents)"},
		{`{"134422":5}`, "Hit 0%/17%"},
	} {
		m, _ := importForever(t, strings.Replace(mage, `{"134422":5}`, c.ranks, 1))
		if m.scorer == nil {
			t.Fatal("no Frost Mage profile")
		}
		v := ansi.Strip(m.View())
		if !strings.Contains(v, c.want) {
			t.Errorf("ranks %s: header lacks %q", c.ranks, c.want)
		}
		if c.ranks == `{"134422":5}` && strings.Contains(v, "talents)") {
			t.Errorf("no hit talent, yet the header names one")
		}
	}
}

// TestCloaksForEveryArmorClass: the Forever (and Cata) data label
// cloaks cloth; a leather-wearing hunter still gets cloak upgrades.
func TestCloaksForEveryArmorClass(t *testing.T) {
	m, _ := importForever(t, foreverExport)
	ws := wornOf(m.gearRows())
	for i, r := range m.gearRows() {
		if r.item.Slot == db.SlotBack {
			if ups := m.scorer.UpgradesWorn(ws, i, 5, m.allowItem); len(ups) == 0 {
				t.Error("no cloak upgrade for an empty back slot")
			}
			return
		}
	}
	t.Fatal("no back row")
}
