package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/vaclav-dvorak/twolk/internal/config"
	"github.com/vaclav-dvorak/twolk/internal/db"
)

// TestCataProtocolRoutes covers the Elemental Rune Dungeon economy in
// the filter: raid loot with a protocol route must stay reachable with
// raid drops OFF once the protocol's phase opens — that is the whole
// point of the system (raid gear for 5-man players).
func TestCataProtocolRoutes(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	m := New(cfg)
	m = drive(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	m = keys(t, m, "i")
	for i, r := range m.realmRows() {
		if r.server.Slug == "whitemane" {
			m.impCursor = i
		}
	}
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	m = keys(t, m, "Zulja")
	m = drive(t, m, tea.KeyMsg{Type: tea.KeyEnter})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	m = drive(t, m, importFetchedMsg{raw: []byte(fetchedExport)})
	d, err := db.LoadCata()
	if err != nil {
		t.Fatal(err)
	}
	m = drive(t, m, dbLoadedMsg{slug: "cata", db: d})
	if m.scorer == nil {
		t.Fatal("no scorer")
	}

	// a raids-off, dungeons-on filter — the user who runs heroics only
	m.filter = defaultFilter()
	m.filter.Raids, m.filter.RaidBoE = false, false

	shoulders := d.Items[60306] // Lightning-Charged Spaulders — T11 token tier
	bell := d.Items[65053]      // Bell of Enraging Resonance [H] — BWD boss drop
	lfr := d.Items[78793]       // Wyrmstalker's Headguard 384 — DS "Raid Finder"

	m.filter.ServerPhase = 1 // no protocols yet: raid loot stays raid loot
	for _, it := range []*db.Item{shoulders, bell, lfr} {
		if m.allowItem(it) {
			t.Errorf("P1 raids-off: %s allowed", it.Name)
		}
	}
	if txt := ansi.Strip(m.sourceText(shoulders)); strings.Contains(txt, "Protocol") {
		t.Errorf("P1 source text names a protocol: %q", txt)
	}

	m.filter.ServerPhase = 3 // Protocol Inferno: T11 gear farmable in 5-mans
	for _, it := range []*db.Item{shoulders, bell} {
		if !m.allowItem(it) {
			t.Errorf("P3 raids-off: %s hidden despite the Inferno route", it.Name)
		}
	}
	if m.allowItem(lfr) {
		t.Error("P3: Twilight-phase LFR piece allowed early")
	}
	if txt := ansi.Strip(m.sourceText(shoulders)); !strings.Contains(txt, "Protocol Inferno: 32× Fissure Stone Fragment") {
		t.Errorf("P3 source text misses the fragment price: %q", txt)
	}

	m.filter.ServerPhase = 4 // Protocol Twilight: LFR gear from final bosses
	if !m.allowItem(lfr) {
		t.Error("P4 raids-off: LFR piece hidden")
	}
	// display: with raids off, the reachable route (protocol vendor)
	// sorts before the filtered-off raid drop
	fandral := d.Items[69897] // Fandral's Flamescythe — Majordomo, FL normal
	txt := ansi.Strip(m.sourceText(fandral))
	proto := strings.Index(txt, "Protocol Twilight")
	raid := strings.Index(txt, "Majordomo")
	if proto < 0 || raid < 0 || proto > raid {
		t.Errorf("raids-off source order wrong: %q", txt)
	}
	m.filter.Raids = true // raids back on: the raid drop leads again
	txt = ansi.Strip(m.sourceText(fandral))
	if strings.Index(txt, "Majordomo") > strings.Index(txt, "Protocol Twilight") {
		t.Errorf("raids-on source order wrong: %q", txt)
	}
	m.filter.Raids = false
	m.filter.Dungeon = false // no dungeons at all: no protocol either
	if m.allowItem(lfr) || m.allowItem(shoulders) {
		t.Error("dungeons-off filter lets protocol loot through")
	}
	m.filter.Dungeon = true

	// the user's original complaint, end to end: at P4 with no raid
	// group the upgrade list for the T11 head must offer the LFR head
	head := -1
	for i, r := range m.gearRows() {
		if r.item.Slot == db.SlotHead {
			head = i
		}
	}
	m.upSlot = head
	m.recomputeUpgrades()
	found := false
	for _, u := range m.upList {
		if u.Item.ID == 78793 {
			found = true
		}
	}
	if !found {
		t.Error("LFR T13 head missing from raids-off P4 upgrade list")
	}
}

// TestFirestoneUpgrades: heroic Firelands items traded up at Lurah
// Wrathvine (normal item + Crystallized Firestone) exist from the
// Firelands phase; the Firestone comes from heroic Firelands (raids)
// or, from Dragon Soul on, Protocol Twilight's Obsidian vendor.
func TestFirestoneUpgrades(t *testing.T) {
	m := cataModel(t, reforgedExport, 3)
	d := m.game.DB
	xbow := d.Items[71558] // Lava Bolt Crossbow (heroic)
	if xbow == nil || xbow.Phase != 3 {
		t.Fatalf("heroic Lava Bolt Crossbow missing or mis-phased: %+v", xbow)
	}
	if base := d.UpgradeBase(xbow.ID); base == nil || base.ID != 71366 {
		t.Fatalf("upgrade base = %v, want normal Lava Bolt Crossbow", base)
	}
	if got := ansi.Strip(m.sourceText(xbow)); !strings.Contains(got, "Upgrade: Lava Bolt Crossbow + Crystallized Firestone") {
		t.Errorf("source line = %q", got)
	}

	m.filter = defaultFilter()
	m.filter.ServerPhase = 3
	if !m.allowItem(xbow) {
		t.Error("P3 raids on: upgrade hidden")
	}
	m.filter.Raids = false // BoE base from the AH, but no Firestone yet
	if m.allowItem(xbow) {
		t.Error("P3 raids off: upgrade allowed without a Firestone source")
	}
	m.filter.ServerPhase = 4 // Protocol Twilight sells the Firestone
	if !m.allowItem(xbow) {
		t.Error("P4 raids off: BoE base + Obsidian Firestone route hidden")
	}
	m.filter.RaidBoE = false // and without the AH, no base item
	if m.allowItem(xbow) {
		t.Error("P4 without BoE raid drops: upgrade allowed with no base")
	}
}
