package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/vaclav-dvorak/twolk/internal/config"
	"github.com/vaclav-dvorak/twolk/internal/db"
)

// TestConsumesAssumedAndNamed: consumables are assumed, never
// configured — the planner picks the best pair for the set it scores
// and names it next to the totals that carry it.
func TestConsumesAssumedAndNamed(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.UpsertCharacter([]byte(exportChunk1+exportChunk2),
		config.StoredCharacter{Game: "wotlk", Server: "warmane", Realm: "Icecrown"}, -1); err != nil {
		t.Fatal(err)
	}
	m := New(cfg)
	m = drive(t, m, tea.WindowSizeMsg{Width: 150, Height: 45})
	m = drive(t, m, dbLoadedMsg{slug: "wotlk", db: testDB()})

	if m.scorer.Baseline() == (db.Stats{}) {
		t.Fatal("no consumables assumed")
	}
	if m.consFood == nil || m.consFlask == nil {
		t.Fatalf("resolved buffs = %v/%v, want both", m.consFood, m.consFlask)
	}
	// the hunter fixture is far under the hit cap, so the meal that
	// closes it wins — the pick follows the gap, not the tooltip
	if m.consFood.Name != "Snapper Extreme" {
		t.Errorf("assumed food = %s, want the hit meal for an uncapped set", m.consFood.Name)
	}
	if m.scorer.Baseline()[db.MeleeHit] != 40 {
		t.Errorf("baseline hit = %g, want the meal's 40", m.scorer.Baseline()[db.MeleeHit])
	}

	// disclosure: the buffs are named, and the caps row says its totals
	// include them
	view := m.View()
	for _, want := range []string{"buffs: ", m.consFood.Name, m.consFlask.Name, "buffed"} {
		if !strings.Contains(view, want) {
			t.Errorf("gear screen does not disclose %q:\n%s", want, view)
		}
	}

	// the BiS screen re-picks against the SOLVED set and names that one
	m.openBis()
	if m.bisFood == nil {
		t.Fatal("BiS solved without an assumed meal")
	}
	if bis := m.View(); !strings.Contains(bis, "buffs: ") ||
		!strings.Contains(bis, m.bisFood.Name) {
		t.Errorf("BiS target pane does not name its assumed buffs:\n%s", bis)
	}
}
