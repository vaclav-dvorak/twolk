package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/vaclav-dvorak/twolk/internal/db"
	"github.com/vaclav-dvorak/twolk/internal/game"
	"github.com/vaclav-dvorak/twolk/internal/game/cata"
	"github.com/vaclav-dvorak/twolk/internal/game/wotlk"
)

// allModules constructs one instance of every supported game module
// (databases nil until loaded on demand). The Model keeps these for the
// whole session: the import wizard offers every module's servers, and
// the active character's game slug picks which one m.game points at.
func allModules() []*game.Game {
	return []*game.Game{wotlk.New(nil), cata.New(nil)}
}

// loadModuleDB loads the embedded item database for a module slug.
func loadModuleDB(slug string) (*db.Database, error) {
	if slug == "cata" {
		return cata.LoadDB()
	}
	return wotlk.LoadDB()
}

// moduleBySlug finds the session's module instance for a game slug,
// nil for slugs no module claims.
func (m *Model) moduleBySlug(slug string) *game.Game {
	for _, g := range m.games {
		if g.Slug == slug {
			return g
		}
	}
	return nil
}

// switchGame makes g the active module, swapping in its version-local
// state (theme, saved upgrade filter), and returns the command that
// loads its database if needed.
func (m *Model) switchGame(g *game.Game) tea.Cmd {
	if m.game == g {
		return nil
	}
	m.game = g
	applyTheme(g.Accent)
	m.loadFilter()
	return m.ensureDB(g)
}

// ensureDB kicks off the async item-database load for a module whose
// database isn't resident yet — at most one load per module and session
// (a failed load may retry on the next switch).
func (m *Model) ensureDB(g *game.Game) tea.Cmd {
	if g.DB != nil || m.dbLoading[g.Slug] {
		return nil
	}
	m.dbLoading[g.Slug] = true
	slug := g.Slug
	return tea.Batch(m.spin.Tick, func() tea.Msg {
		d, err := loadModuleDB(slug)
		return dbLoadedMsg{slug: slug, db: d, err: err}
	})
}
