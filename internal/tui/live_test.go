package tui

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/vaclav-dvorak/twolk/internal/config"
	"github.com/vaclav-dvorak/twolk/internal/db"
	"github.com/vaclav-dvorak/twolk/internal/game/cata"
)

// TestCataLiveRender is the opt-in end-to-end check for the Whitemane
// import: fetch a real character from the live armory API, run it
// through the import pipeline, and render the gear screen against the
// embedded cata database. Run with
//
//	TWOLK_WHITEMANE_LIVE="realm/name" go test -run CataLive ./internal/tui/
//
// Skipped otherwise; never part of the normal suite.
func TestCataLiveRender(t *testing.T) {
	target := os.Getenv("TWOLK_WHITEMANE_LIVE")
	if target == "" {
		t.Skip("set TWOLK_WHITEMANE_LIVE=realm/name to run")
	}
	realm, name, ok := strings.Cut(target, "/")
	if !ok {
		t.Fatalf("TWOLK_WHITEMANE_LIVE = %q, want realm/name", target)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	raw, err := cata.FetchRaw(ctx, realm, name)
	if err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", t.TempDir())
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.UpsertCharacter(raw,
		config.StoredCharacter{Game: "cata", Server: "whitemane", Realm: realm}, -1); err != nil {
		t.Fatal(err)
	}

	m := New(cfg)
	if m.game.Slug != "cata" {
		t.Fatalf("active module = %s, want cata", m.game.Slug)
	}
	m = drive(t, m, tea.WindowSizeMsg{Width: 120, Height: 45})
	d, err := db.LoadCata()
	if err != nil {
		t.Fatal(err)
	}
	m = drive(t, m, dbLoadedMsg{slug: "cata", db: d})

	rows := m.gearRows()
	if len(rows) < 10 {
		t.Fatalf("only %d worn items resolved in the cata db", len(rows))
	}
	view := m.View()
	for _, r := range rows {
		// long names render ellipsized — a stable prefix must appear
		prefix := []rune(r.item.Name)
		if len(prefix) > 20 {
			prefix = prefix[:20]
		}
		if !strings.Contains(view, string(prefix)) {
			t.Errorf("gear screen missing %s", r.item.Name)
		}
	}
	t.Logf("resolved %d/%d worn items", len(rows), len(m.char.Equipment))
	t.Logf("gear screen:\n%s", view)

	// scoring must be live for a hunter (cata profiles, session 19)
	if m.char.Class == db.ClassHunter {
		if m.scorer == nil {
			t.Fatal("no scorer for a live cata hunter")
		}
		// the server is progressing through phase 1 (T11) — with the
		// generated phase overlay the ceiling must actually bite
		m.filter.ServerPhase = 1
		// open the upgrade browser on the lowest-ilvl row
		low := 0
		for i, r := range rows {
			if r.item.ILvl < rows[low].item.ILvl {
				low = i
			}
		}
		m.cursor = low
		m = drive(t, m, tea.KeyMsg{Type: tea.KeyEnter})
		if len(m.upList) == 0 {
			t.Fatalf("no upgrade proposals for %s", rows[low].item.Name)
		}
		unsourced := 0
		for _, u := range m.upList {
			if u.Item.Phase > 1 {
				t.Errorf("phase-%d item proposed at P1: %s", u.Item.Phase, u.Item.Name)
			}
			if len(u.Item.Sources) == 0 {
				unsourced++
			}
		}
		t.Logf("upgrade browser for %s (%d proposals, %d unsourced) at P1:\n%s",
			rows[low].item.Name, len(m.upList), unsourced, m.View())
	}
}
