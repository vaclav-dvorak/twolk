package tui

import (
	"github.com/vaclav-dvorak/twolk/internal/armory"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/vaclav-dvorak/twolk/internal/config"
	"github.com/vaclav-dvorak/twolk/internal/db"
)

// fetchedExport is what cata.FetchRaw produces for a level-85 hunter
// (shape covered by that package's own tests); item/gem/enchant ids are
// real so the embedded cata database resolves them.
const fetchedExport = `{"name":"Zulja","realm":"Gilneas","level":85,"faction":"Horde",
	"race":"Orc","class":"Hunter","origin":"whitemane",
	"equipment":[
		{"item":60303,"ench":4209,"gems":[68778,52209,0],"reforge":{"source":49,"target":31,"value":67}},
		{"item":62385,"ench":4105,"gems":[52212,0,0]}],
	"talents":[{"tree":"Survival","points":[]}],
	"professions":[{"name":"Alchemy","skill":525},{"name":"Engineering","skill":525}]}`

// TestCataFetchImport walks the wizard onto the Whitemane row, feeds it
// a fetched export, and checks the whole seam: the character is stored
// under the cata game slug, the cata module becomes active, and the
// gear screen renders real cata items without a scoring profile.
func TestCataFetchImport(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	m := New(cfg)
	m = drive(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})

	// wizard step 0: the whitemane row is offered after the wotlk realms
	m = keys(t, m, "i")
	rows := m.realmRows()
	whitemane := -1
	for i, r := range rows {
		if r.server.Slug == "whitemane" {
			whitemane = i
		}
	}
	if whitemane < 0 {
		t.Fatalf("whitemane not offered: %+v", rows)
	}
	m.impCursor = whitemane
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEnter}) // continue to the name

	// name is required: enter without one stores nothing
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.errMsg == "" || len(m.cfg.Chars) != 0 {
		t.Fatal("empty name must not add a character")
	}
	m = keys(t, m, "Zulja")
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // save → refresh → fetch
	m = next.(Model)
	if cmd == nil || !m.impFetching || !m.impRefresh {
		t.Fatal("adding a fetch-server character must start its fetch at once")
	}
	if !strings.Contains(m.View(), "fetch from the armory") {
		t.Fatal("fetch step not shown for an ImportFetch server")
	}

	// the fetch lands (network bypassed; cata.FetchRaw has its own tests)
	m = drive(t, m, importFetchedMsg{raw: []byte(fetchedExport)})
	if m.mode != modeNormal {
		t.Fatalf("import did not complete: err=%q", m.errMsg)
	}
	if m.game.Slug != "cata" {
		t.Fatalf("active module = %s, want cata", m.game.Slug)
	}
	if len(cfg.Chars) != 1 || cfg.Chars[0].Game != "cata" ||
		cfg.Chars[0].Server != "whitemane" || cfg.Chars[0].Realm != "Gilneas" {
		t.Fatalf("stored meta wrong: %+v", cfg.Chars)
	}

	// the real embedded cata database resolves the worn items
	d, err := db.LoadCata()
	if err != nil {
		t.Fatal(err)
	}
	m = drive(t, m, dbLoadedMsg{slug: "cata", db: d})
	view := m.View()
	for _, want := range []string{
		"Cataclysm 4.4.2 (classic)", "Zulja", "Survival",
		"Lightning-Charged Headguard", "359",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("gear screen missing %q", want)
		}
	}
	// survival hunter has a cata profile since session 19 — the char
	// scores, at level-85 caps (8% ranged hit = 961 rating)
	if m.scorer == nil {
		t.Fatal("no scorer for a cata survival hunter")
	}
	if strings.Contains(view, "no scoring profile") {
		t.Error("header still claims no profile")
	}
	caps := m.scorer.Profile().EffectiveCaps(nil)
	var hit float64
	for _, c := range caps {
		if c.Stat == db.MeleeHit {
			hit = c.Cap
		}
	}
	if hit != 961 {
		t.Errorf("hit cap = %v rating, want 961 (8%% at level 85)", hit)
	}

	// cata talents are exact (or absent, for hunters): the assumption
	// modal and its hotkey don't exist for this character
	if strings.Contains(view, "talents") {
		t.Error("help row offers the talents modal on a cata character")
	}
	m = keys(t, m, "t")
	if m.mode == modeTalents {
		t.Error("talent-assumption modal opened for a cata character")
	}
}

// TestImportAndRefresh: import (i) is always a NEW character — empty
// focused name on any realm, with the selected realm's own stored
// characters listed; refresh (r) is its own flow bound to the active
// character. The realm list tags every row with its game's flavor.
func TestImportAndRefresh(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.UpsertCharacter([]byte(fetchedExport),
		config.StoredCharacter{Game: "cata", Server: "whitemane", Realm: "Gilneas"}, -1); err != nil {
		t.Fatal(err)
	}
	m := New(cfg)
	m = drive(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})

	m = keys(t, m, "i")
	view := m.View()
	for _, want := range []string{"(wotlk legacy)", "(cata classic)"} {
		if !strings.Contains(view, want) {
			t.Errorf("realm list missing flavor tag %q", want)
		}
	}
	if sel := m.importSelection(); sel.server.Slug != "whitemane" {
		t.Fatalf("home realm not preselected: %+v", sel)
	}
	// adding starts with an empty, focused name even on the home realm;
	// the realm's stored characters are listed
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.impName.Value() != "" || !m.impName.Focused() {
		t.Fatalf("import: name %q focused=%v, want empty focused",
			m.impName.Value(), m.impName.Focused())
	}
	if v := m.View(); !strings.Contains(v, "on this realm:") || !strings.Contains(v, "Zulja") {
		t.Error("home realm step 1 missing the realm's imported characters")
	}
	// a realm with no stored characters mentions none of them
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEsc}) // back to realm list
	m.impCursor = 0                               // warmane/wotlk
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	if v := m.View(); strings.Contains(v, "on this realm:") || strings.Contains(v, "Zulja") {
		t.Error("foreign realm step 1 mentions another realm's character")
	}
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEsc}) // back to realm list
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEsc}) // leave the wizard

	// refresh: one keystroke, bound to the active character — the fetch
	// starts immediately with the stored realm and name
	m = keys(t, m, "r")
	if m.mode != modeImport || !m.impRefresh || !m.impFetching {
		t.Fatalf("r: mode=%v refresh=%v fetching=%v, want refresh fetch in flight",
			m.mode, m.impRefresh, m.impFetching)
	}
	if m.impName.Value() != "Zulja" || m.importSelection().server.Slug != "whitemane" {
		t.Fatalf("refresh targets %q @ %s", m.impName.Value(), m.importSelection().realm)
	}
	if v := m.View(); !strings.Contains(v, "Refresh character") {
		t.Error("refresh modal not titled Refresh character")
	}
	// the fetch lands: the stored entry is REPLACED, not duplicated
	m = drive(t, m, importFetchedMsg{raw: []byte(fetchedExport)})
	if m.mode != modeNormal || m.impRefresh {
		t.Fatalf("refresh did not complete: err=%q", m.errMsg)
	}
	if len(m.cfg.Chars) != 1 {
		t.Fatalf("refresh duplicated the character: %d entries", len(m.cfg.Chars))
	}
}

// TestCataWeightsEditor opens the weights editor on a cata hunter: the
// rows must be cata's stats (Mastery present; ArP/Defense/Block Value
// absent), and an override must persist under the game-prefixed key.
func TestCataWeightsEditor(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.UpsertCharacter([]byte(fetchedExport),
		config.StoredCharacter{Game: "cata", Server: "whitemane", Realm: "Gilneas"}, -1); err != nil {
		t.Fatal(err)
	}
	m := New(cfg)
	m = drive(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	d, err := db.LoadCata()
	if err != nil {
		t.Fatal(err)
	}
	m = drive(t, m, dbLoadedMsg{slug: "cata", db: d})

	m = keys(t, m, "w")
	if m.mode != modeWeights {
		t.Fatal("weights editor did not open for a cata hunter")
	}
	view := m.View()
	for _, want := range []string{"Mastery", "Survival Hunter"} {
		if !strings.Contains(view, want) {
			t.Errorf("editor missing %q", want)
		}
	}
	for _, gone := range []string{"Armor Penetration", "Defense", "Block Value", "MP5"} {
		if strings.Contains(view, gone) {
			t.Errorf("editor offers WotLK-only stat %q", gone)
		}
	}
	// the mastery row must show the sim default, not a dead 0.00
	if !strings.Contains(view, "0.88") {
		t.Error("mastery default (0.88) not shown")
	}

	// bump mastery: the override lands under the cata-prefixed key and
	// the scorer follows live
	rows := m.weightRows()
	for i, r := range rows {
		if r.key == "mastery" {
			m.wCursor = i
		}
	}
	before := m.scorer.SetScoreWorn(wornOf(m.gearRows()))
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyShiftRight})
	if got := cfg.Weights["cata/hunter/survival"]["mastery"]; got != 0.88+0.5 {
		t.Fatalf("mastery override = %v, want 1.38 under cata/hunter/survival", got)
	}
	if after := m.scorer.SetScoreWorn(wornOf(m.gearRows())); after <= before {
		t.Fatalf("set score did not react: %v -> %v", before, after)
	}
}

// TestGameSwitchOnActivate stores one character per game version and
// checks that switching characters swaps the active module (and asks
// for the other module's database exactly once).
func TestGameSwitchOnActivate(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.UpsertCharacter([]byte(exportChunk1+exportChunk2),
		config.StoredCharacter{Game: "wotlk", Server: "warmane", Realm: "Icecrown"}, -1); err != nil {
		t.Fatal(err)
	}
	if err := cfg.UpsertCharacter([]byte(fetchedExport),
		config.StoredCharacter{Game: "cata", Server: "whitemane", Realm: "Gilneas"}, -1); err != nil {
		t.Fatal(err)
	}

	// tests run without a TTY: force the 256-color profile so the
	// accent assertions below see real escape codes
	lipgloss.SetColorProfile(termenv.ANSI256)

	// the active character (the cata one, index 1) selects the module
	cfg.SetActive(1)
	m := New(cfg)
	if m.game.Slug != "cata" {
		t.Fatalf("startup module = %s, want cata", m.game.Slug)
	}

	m = drive(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})

	// the expansion signature color themes the UI: cata renders fire
	// orange (256-color 208), wotlk frost blue (81)
	if view := m.View(); !strings.Contains(view, "38;5;208") {
		t.Error("cata accent (208) missing from the rendered view")
	}
	if cmd := m.activateChar(0); cmd == nil {
		t.Fatal("switching to the wotlk character must load its database")
	}
	if m.game.Slug != "wotlk" || m.char.Name != "Testchar" {
		t.Fatalf("switch wrong: game=%s char=%v", m.game.Slug, m.char)
	}
	if view := m.View(); !strings.Contains(view, "38;5;81") {
		t.Error("wotlk accent (81) missing after the switch")
	} else if i := strings.Index(view, "38;5;208"); i >= 0 {
		t.Errorf("cata accent (208) still present after the switch: %q", view[i:min(i+80, len(view))])
	}
	// a phase valid in both versions survives the switch untouched;
	// one past the target version's endgame is clamped down
	m.filter.ServerPhase = 5 // wotlk RS — no such phase in cata
	if cmd := m.activateChar(1); cmd == nil {
		t.Fatal("switching back must load the cata database")
	}
	if m.game.Slug != "cata" || m.filter.ServerPhase != 4 {
		t.Fatalf("switch back wrong: game=%s phase=%d (want clamped to 4)", m.game.Slug, m.filter.ServerPhase)
	}
}

// TestCataQuestSourceLine pins the source line of an item wowhead
// also credits to a phantom 4.4 vendor: the quest route wins.
func TestCataQuestSourceLine(t *testing.T) {
	m := cataModel(t, reforgedExport, 2)
	bow := m.game.DB.Items[63791]
	if bow == nil {
		t.Fatal("Big Bendy Blasting Bow not in the cata db")
	}
	line := ansi.Strip(m.sourceText(bow))
	if !strings.Contains(line, "Quest: The Crucible of Carnage: The Twilight Terror!") {
		t.Errorf("source line = %q", line)
	}
}

// TestMoltenFrontUnlock: closing the Molten Front unlock hides its
// vendor gear, crafts whose recipe Damek sells, and the Flintlocke's
// Woodchucker scope (schematic from Damek) — all through the filter
// modal, saved per realm.
func TestMoltenFrontUnlock(t *testing.T) {
	m := cataModel(t, reforgedExport, 3)
	m.scorer.SetServerPhase(3)
	d := m.game.DB
	treads := d.Items[70115]  // Fiery Treads, sold by Damek Bloombeard
	scalper := d.Items[70165] // crafted, plans sold by Damek
	gun := d.Items[65095]     // Dragonheart Piercer (a ranged weapon to scope)
	if treads == nil || scalper == nil || gun == nil {
		t.Fatal("Molten Front items missing from the cata db")
	}
	if !m.allowItem(treads) || !m.allowItem(scalper) {
		t.Fatal("open unlock must admit Molten Front routes")
	}
	if e := m.scorer.BestEnchant(gun); e == nil || e.Name != "Flintlocke's Woodchucker" {
		t.Fatalf("open unlock: best gun enchant = %v, want the Woodchucker", e)
	}

	// close it in the modal: the unlock row is the last one
	m = keys(t, m, "f")
	for range m.filterRows() - 1 {
		m = keys(t, m, "j")
	}
	m = keys(t, m, "xf") // toggle, close (saves)
	if m.unlockOpen("molten-front") {
		t.Fatal("toggle did not close the unlock")
	}
	if got := m.cfg.Filters[m.filterKey()].Locked; len(got) != 1 || got[0] != "molten-front" {
		t.Fatalf("saved filter Locked = %v", got)
	}
	if m.allowItem(treads) || m.allowItem(scalper) {
		t.Error("closed unlock must hide Molten Front vendor gear and its crafts")
	}
	if e := m.scorer.BestEnchant(gun); e != nil && e.Name == "Flintlocke's Woodchucker" {
		t.Error("closed unlock must hide the Woodchucker")
	}
	if s := m.filterSummary(); !strings.Contains(s, "no Molten Front") {
		t.Errorf("summary = %q", s)
	}
}

// TestWornSuffixRoll: a worn random-suffix item resolves to a roll — a
// reforge pins it exactly, otherwise the spec's best roll is assumed
// and flagged.
func TestWornSuffixRoll(t *testing.T) {
	m := cataModel(t, reforgedExport, 3)
	base := m.game.DB.Items[65367] // Mistral Circle: crit/haste/mastery/hit pairs
	// haste → mastery: only Windflurry (crit+haste) has haste and no mastery
	eq := armory.EquippedItem{ItemID: base.ID,
		Reforge: &db.Reforge{From: db.MeleeHaste, To: db.Mastery, Amount: 57}}
	it, guess := m.wornRoll(eq, base)
	if guess || it.Suffix != "of the Windflurry" {
		t.Errorf("reforge-pinned roll = %q (guess %v)", it.Suffix, guess)
	}
	it, guess = m.wornRoll(armory.EquippedItem{ItemID: base.ID}, base)
	if !guess || it.Suffix == "" {
		t.Errorf("unreforged roll = %q (guess %v), want an assumed roll", it.Suffix, guess)
	}
}

// TestItemLabelMatchesGearList: inline item mentions (quick tips,
// compare, cards) render like the gear list — name, " [H]", tier tag —
// never a parenthesized tier.
func TestItemLabelMatchesGearList(t *testing.T) {
	m := cataModel(t, reforgedExport, 3)
	var normal, heroic *db.Item
	for _, it := range m.game.DB.Items {
		if it.Name == "Lightning-Charged Tunic" && m.tierLabel(it) != "" {
			if it.Heroic {
				heroic = it
			} else {
				normal = it
			}
		}
	}
	if normal == nil || heroic == nil {
		t.Fatal("T11 tunics missing")
	}
	want := "Lightning-Charged Tunic " + m.tierLabel(normal)
	if got := ansi.Strip(m.itemLabel(normal, rowStyler(false))); got != want {
		t.Errorf("normal = %q, want %q", got, want)
	}
	want = "Lightning-Charged Tunic [H] " + m.tierLabel(heroic)
	if got := ansi.Strip(m.itemLabel(heroic, rowStyler(false))); got != want {
		t.Errorf("heroic = %q, want %q", got, want)
	}
}
