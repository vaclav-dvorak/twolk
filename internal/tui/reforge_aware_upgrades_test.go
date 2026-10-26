package tui

import (
	"math"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/vaclav-dvorak/twolk/internal/config"
	"github.com/vaclav-dvorak/twolk/internal/db"
)

// hitCappedExport is a cata hunter sitting ON the hit cap with the hit
// living on a trinket — the shape that makes a hit-less candidate look
// worthless until the set is allowed to re-reforge around it.
const hitCappedExport = `{"name":"Zulja","realm":"Gilneas","level":85,"faction":"Horde",
	"race":"Orc","class":"Hunter","origin":"whitemane",
	"equipment":[
		{"item":63459,"ench":4209,"gems":[68778,52209,0],"slot":"Head","reforge":{"source":49,"target":32,"value":80}},
		{"item":57931,"slot":"Neck"},
		{"item":58124,"slot":"Shoulder"},
		{"item":56564,"ench":4102,"gems":[52212,0,0],"slot":"Chest","reforge":{"source":36,"target":32,"value":99}},
		{"item":56539,"slot":"Waist"},
		{"item":69615,"slot":"Legs"},
		{"item":63446,"slot":"Feet"},
		{"item":56340,"slot":"Wrist"},
		{"item":58125,"slot":"Hands"},
		{"item":68812,"slot":"Finger"},
		{"item":62362,"slot":"Finger"},
		{"item":58181,"slot":"Trinket"},
		{"item":56328,"slot":"Trinket"},
		{"item":67134,"slot":"Back"},
		{"item":65168,"slot":"Weapon"},
		{"item":56366,"slot":"Ranged"}],
	"talents":[{"tree":"Survival","points":[]}],
	"professions":[{"name":"Leatherworking","skill":525},{"name":"Skinning","skill":525}]}`

// A trinket carrying no hit is scored far too low while the rest of the
// set's reforges are frozen: dropping the equipped piece puts the
// character under the cap with no way to recover. The ranking prices the
// swap with the whole set free to re-balance instead, which is what the
// reforge advisor would then actually do — and marks the row, because
// the value is only real after that pass.
func TestUpgradesRePlanReforges(t *testing.T) {
	m := cataModel(t, hitCappedExport, 3)
	rows := m.gearRows()
	slot := -1
	for i, r := range rows {
		if r.item != nil && r.item.Slot == db.SlotTrinket {
			slot = i
		}
	}
	if slot < 0 {
		t.Fatal("no trinket slot in the test export")
	}
	m.upSlot = slot
	m.mode = modeUpgrades
	m.recomputeUpgrades()

	// Ricket's Magnetic Fireball (Molten Front, no hit) is the case that
	// prompted this: negative frozen, positive once reforges re-plan.
	var found bool
	for _, u := range m.upList {
		if u.Item.ID != 70144 {
			continue
		}
		found = true
		// Until session 38 this was the SIGN-FLIP case: with cata hit
		// floored at 3.5, dropping a hit trinket at the cap sank the
		// frozen delta and only the re-plan rescued it. Measured, cata
		// hit is worth 1.73, so the loss no longer sinks anything — and
		// the re-plan now moves the delta DOWN, because re-planning
		// lifts the WORN side too and here it lifts it more. The
		// behavior under test is that the two bases are priced
		// differently and the row says so, not the direction of the day.
		if u.Delta == u.Frozen {
			t.Errorf("re-plan changed nothing: frozen %+.0f", u.Frozen)
		}
		if !u.ReforgeDependent() {
			t.Errorf("verdict flipped (%+.0f → %+.0f) but the row is unmarked",
				u.Frozen, u.Delta)
		}
	}
	if !found {
		t.Fatal("Ricket's Magnetic Fireball absent from the ranking")
	}

	// the pane must show both numbers, or the row's value reads as
	// unreachable
	pane := ansi.Strip(m.viewCompare(rows[slot].worn(), m.game.DB.Items[70144]))
	for _, want := range []string{"after a reforge pass", "as the set sits now", "cap"} {
		if !strings.Contains(pane, want) {
			t.Errorf("compare pane missing %q:\n%s", want, pane)
		}
	}
}

// WotLK has no reforging, so the re-plan is a no-op there and the two
// numbers must agree exactly — no marker, no behavior change.
func TestUpgradesNoReforgeVersion(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	// item 46143 is testDB's head piece, with head upgrades behind it
	char := `{"name":"Testchar","realm":"Icecrown","level":"80","class":"Hunter",` +
		`"race":"Orc","faction":"Horde","equipment":[{"name":"Test Headpiece","item":"46143"}],` +
		`"talents":[{"tree":"Survival","points":[0,0,51]}]}`
	if err := cfg.UpsertCharacter([]byte(char),
		config.StoredCharacter{Game: "wotlk", Server: "warmane", Realm: "Icecrown"}, -1); err != nil {
		t.Fatal(err)
	}
	m := New(cfg)
	m = drive(t, m, tea.WindowSizeMsg{Width: 160, Height: 45})
	m = drive(t, m, dbLoadedMsg{slug: "wotlk", db: testDB()})
	rows := m.gearRows()
	if len(rows) == 0 {
		t.Fatal("no gear rows: the export did not resolve against the test db")
	}
	m.upSlot = 0
	m.mode = modeUpgrades
	m.recomputeUpgrades()
	if len(m.upList) == 0 {
		t.Fatal("no candidates ranked: the assertion below would be vacuous")
	}
	for _, u := range m.upList {
		// same computation on both sides, so the numbers must agree —
		// to float summation order, not bit-for-bit (the two paths sum
		// the totals, consumable baseline included, in different order)
		if math.Abs(u.Delta-u.Frozen) > 1e-6 {
			t.Fatalf("%s: delta %+.4f != frozen %+.4f without reforging",
				u.Item.Name, u.Delta, u.Frozen)
		}
		if u.ReforgeDependent() {
			t.Fatalf("%s marked reforge-dependent without reforging", u.Item.Name)
		}
	}
}
