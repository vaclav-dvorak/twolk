// Package tui implements the twolk terminal user interface.
package tui

import (
	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"math"

	"github.com/vaclav-dvorak/twolk/internal/armory"
	"github.com/vaclav-dvorak/twolk/internal/config"
	"github.com/vaclav-dvorak/twolk/internal/db"
	"github.com/vaclav-dvorak/twolk/internal/game"
	"github.com/vaclav-dvorak/twolk/internal/score"
)

type mode int

const (
	modeNormal mode = iota
	modeImport
	modeUpgrades
	modeFilter
	modeTalents
	modeWeights
	modeChars
	modeOptimize
	modeBiS
	modeEmblems
)

var (
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("81"))
	dimStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("247"))
	// faintStyle marks source routes the active filter has closed —
	// still listed (the item exists there) but visibly not for you
	faintStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	// ok/err/warn come from the Dracula palette (green/red/orange) —
	// designed for dark backgrounds, softer than the raw ANSI colors
	okStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B"))
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555"))
	warnStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#FFB86C"))
	bisStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B")) // best-in-slot tick: green = done, unlike the orange ▲
	modalStyle  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("81")).Padding(1, 2)
	helpKey     = lipgloss.NewStyle().Foreground(lipgloss.Color("81"))
	helpDesc    = lipgloss.NewStyle().Foreground(lipgloss.Color("247"))
	statusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B"))
	// enchant names render in the game's tooltip-enchant green
	enchantStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("77"))
	// tier tags follow the game version's accent (re-tinted by
	// applyTheme) so "T11" reads the same in every screen
	tierStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("81"))
	detailStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("242")).Padding(0, 1)
	selBgColor  = lipgloss.Color("237") // selected-row background

	qualityStyles = map[db.Quality]lipgloss.Style{
		db.QualityPoor:      lipgloss.NewStyle().Foreground(lipgloss.Color("245")),
		db.QualityCommon:    lipgloss.NewStyle().Foreground(lipgloss.Color("255")),
		db.QualityUncommon:  lipgloss.NewStyle().Foreground(lipgloss.Color("77")),
		db.QualityRare:      lipgloss.NewStyle().Foreground(lipgloss.Color("69")),
		db.QualityEpic:      lipgloss.NewStyle().Foreground(lipgloss.Color("135")),
		db.QualityLegendary: lipgloss.NewStyle().Foreground(lipgloss.Color("214")),
	}
)

// applyTheme re-tints the accent-bearing styles with the active game
// version's signature color (WotLK frost blue, Cataclysm fire orange).
// The style vars stay package globals — threading the Model through
// every style usage isn't worth it for a single-goroutine TUI; the
// theme changes only in New and switchGame.
func applyTheme(accent string) {
	c := lipgloss.Color(accent)
	titleStyle = titleStyle.Foreground(c)
	tierStyle = tierStyle.Foreground(c)
	modalStyle = modalStyle.BorderForeground(c)
	helpKey = helpKey.Foreground(c)
	scrollThumb = scrollThumb.Foreground(c)
}

// dbLoadedMsg carries the result of one module's async item database
// load, tagged with the module it belongs to.
type dbLoadedMsg struct {
	slug string
	db   *db.Database
	err  error
}

// importFetchedMsg carries the result of an async armory-API import
// (ImportFetch servers).
type importFetchedMsg struct {
	raw []byte
	err error
}

// Model is the root bubbletea model for twolk.
type Model struct {
	cfg  *config.Config
	char *armory.Character
	// games holds one instance of every supported game module for the
	// session; game points at the active one (selected by the active
	// character's game slug) and is never nil. A module's rules are
	// usable immediately, its DB stays nil until loaded on demand.
	games      []*game.Game
	game       *game.Game
	dbLoading  map[string]bool // module slugs with a DB load in flight
	dbErr      error
	scorer     *score.Scorer // nil until db+char ready, or if build unsupported
	mode       mode
	cursor     int    // selected row in the gear list
	pasted     []byte // import modal paste buffer; not editable, only pasteable
	typedPaste bool   // pasted is arriving one keystroke per rune (Windows consoles)
	importNote string // confirmation line inside the import modal
	// import wizard: pick a realm from the game modules' server
	// registries, then (optionally) name the character for exact
	// armory links, then paste
	spin        spinner.Model   // shown while a DB load or armory fetch is in flight
	impStep     int             // 0 = realm select, 1 = get & paste (or fetch)
	impCursor   int             // selected realm row
	impName     textinput.Model // character name (required for fetch servers)
	impFetching bool            // an armory-API fetch is in flight
	// impRefresh: the modal is refreshing the ACTIVE character (r), not
	// importing a new one — realm and name are fixed, esc exits
	impRefresh bool
	width      int
	height     int
	status     string
	errMsg     string

	// gear the item database cannot resolve (gearcheck.go): non-empty
	// disables scoring entirely, gapWarn holds the modal open until a
	// key dismisses it, gapsSeen is the problem it was last armed for
	gaps     []gearGap
	gapWarn  bool
	gapsSeen string

	filter    config.UpgradeFilter
	fCursor   int  // selected row in the filter modal
	tCursor   int  // selected row in the talent modal
	wCursor   int  // selected row in the weights editor
	cCursor   int  // selected row in the character switcher
	fromUps   bool // filter modal was opened from the upgrades view
	fromBis   bool // filter modal was opened from the BiS view
	upSlot    int  // gear row the upgrades view is for
	upList    []score.Upgrade
	upCursor  int
	upPaneOff int                // compare-pane scroll offset (j/k; the list keeps its rows)
	tips      tipsData           // quick-tips pane content, see recomputeTips
	optPlan   score.OptimizePlan // joint gem/enchant/reforge plan, see openOptimize
	optNote   string             // confirmation line inside the optimization advisor
	optOff    int                // advisor body scroll offset (the plan can outgrow the screen)
	embList   []emblemBuy        // emblem advisor rows, see openEmblems
	embOff    int                // emblem advisor scroll offset

	bisSet     []*db.Item    // BiS picks, aligned with bisWorn, see openBis
	bisWorn    []score.Worn  // solver baseline: worn rows re-dealt onto the winning weapon layout
	bisFrom    []int         // worn item IDs at solve time (staleness check)
	bisScorer  *score.Scorer // gem-calibrated against bisSet, not the worn set
	bisScore   float64
	bisRefs    []*db.Reforge // the solved set's cap-aware reforge plan (score.AssumedPlan)
	bisGems    [][]*db.Gem   // the solved set's set-level gem plan (score.OptimizeGems)
	bisGemGain float64       // what that plan beats the per-socket assumption by
	// the consumables assumed for the worn set and for the solved BiS
	// set (they can differ — a set that caps expertise wants another
	// meal). Resolved when the scorer is built, not per render.
	consFood, consFlask *db.Consumable
	bisFood, bisFlask   *db.Consumable
	bisCursor           int
	bisPaneOff          int // info-pane scroll offset; the item list always keeps its rows
}

// gearRow is one navigable line of the gear list: an equipped item that
// resolved in the item DB (cosmetics like shirts/tabards are dropped).
type gearRow struct {
	equipped armory.EquippedItem
	item     *db.Item
	// rollGuess marks a random-suffix item whose roll the import does
	// not name: item is the planner's assumption (see wornRoll)
	rollGuess bool
}

// worn converts a row to a scoring instance.
func (r gearRow) worn() score.Worn {
	return score.Worn{
		Item:      r.item,
		Gems:      r.equipped.Gems,
		EnchantID: r.equipped.EnchantID,
		HasDetail: r.equipped.HasDetail,
		Reforge:   r.equipped.Reforge,
	}
}

// wornOf extracts the scoring instances from gear rows.
func wornOf(rows []gearRow) []score.Worn {
	ws := make([]score.Worn, len(rows))
	for i, r := range rows {
		ws[i] = r.worn()
	}
	return ws
}

// newScorer builds a scorer for the current character — profile with
// talent-adjusted caps, user weight overrides, race and professions —
// without any gem calibration. Nil for builds without a profile.
func (m *Model) newScorer() *score.Scorer {
	if m.game.DB == nil || m.char == nil {
		return nil
	}
	p := m.game.FindProfile(m.char.Class, m.char.Spec(), m.char.TalentRanks)
	if p == nil {
		return nil
	}
	prof := *p
	prof.Caps = p.EffectiveCaps(m.capReductions())
	// user stat-weight overrides, resolved onto the side the profile
	// reads (twin-stat discipline preserved)
	if ov := m.cfg.Weights[p.Key()]; len(ov) > 0 {
		for _, row := range m.weightRows() {
			v, ok := ov[row.key]
			if !ok {
				continue
			}
			switch row.key {
			case "melee_dps":
				prof.WeaponDPSWeight = v
			case "ranged_dps":
				prof.RangedDPSWeight = v
			default:
				prof.Weights[resolveSide(p, row)] = v
			}
		}
	}
	s := score.New(m.game.DB, &prof)
	s.SetRace(m.char.Race)
	s.SetProfessions(m.professions())
	s.SetServerPhase(m.filter.ServerPhase)
	s.SetEnchantGate(m.enchantGate())
	// consumables are part of the character, not of its gear: the
	// baseline they set is what makes caps and curves see the buffed
	// totals (see consumes.go)
	m.applyConsumes(s, setOf(m.gearRows()))
	return s
}

// rebuildScorer refreshes the scorer whenever the database, character, or
// talent assumptions change; it stays nil for builds without a profile.
func (m *Model) rebuildScorer() {
	// a set with unscorable slots is scored by nobody: what the
	// planner cannot see would silently understate every total
	// (gearcheck.go)
	m.checkGear()
	m.scorer = nil
	if len(m.gaps) == 0 {
		m.scorer = m.newScorer()
	}
	if m.scorer == nil {
		m.consFood, m.consFlask = nil, nil
		m.recomputeTips() // clears the pane
		return
	}
	// adapt gem choices to the equipped set (e.g. stop socketing hit
	// once the gear itself saturates the hit cap)
	m.scorer.Calibrate(setOf(m.gearRows()))
	m.consFood, m.consFlask = m.applyWornConsumes(m.scorer, wornOf(m.gearRows()))
	m.recomputeTips()
}

func (m Model) gearRows() []gearRow {
	if m.game.DB == nil || m.char == nil {
		return nil
	}
	var rows []gearRow
	for _, eq := range m.char.Equipment {
		if it, ok := m.game.DB.Items[eq.ItemID]; ok {
			it, guess := m.wornRoll(eq, it)
			rows = append(rows, gearRow{equipped: eq, item: it, rollGuess: guess})
		}
	}
	return rows
}

// wornRoll resolves a worn random-suffix item to one of its rolls.
// Imports name the item, not the roll, but a reforge pins it: the
// source stat must be on the roll at exactly 40% → Amount, and the
// target stat absent. Otherwise the roll the spec values most is
// assumed (guess = true) — the same best-case stance the planner takes
// on a candidate's gems.
func (m Model) wornRoll(eq armory.EquippedItem, it *db.Item) (*db.Item, bool) {
	vs := m.game.DB.VariantsOf(it.ID)
	if len(vs) == 0 {
		return it, false
	}
	if r := eq.Reforge; r != nil {
		var fit []*db.Item
		for _, v := range vs {
			if v.Stats[r.From] > 0 && v.Stats[r.To] == 0 &&
				(r.Amount == 0 || math.Floor(v.Stats[r.From]*0.4) == r.Amount) {
				fit = append(fit, v)
			}
		}
		if len(fit) == 1 {
			return fit[0], false
		}
		if len(fit) > 1 {
			vs = fit
		}
	}
	best, bestV := vs[0], math.Inf(-1)
	if p := m.baseProfile(); p != nil {
		for _, v := range vs {
			var val float64
			for st, w := range p.Weights {
				val += w * v.Stats[st]
			}
			if val > bestV {
				best, bestV = v, val
			}
		}
	}
	return best, true
}

// New builds the root model with previously saved state loaded from cfg.
func New(cfg *config.Config) Model {
	// the active character's game tag picks the module; characters from
	// before the tag (and no character at all) mean WotLK, the founding
	// version. Rules are available immediately — only the item database
	// loads async.
	m := Model{cfg: cfg, games: allModules(), dbLoading: map[string]bool{}, filter: defaultFilter()}
	// no WithStyle: the accent is applied at render time, so theme
	// switches (applyTheme) reach the spinner too
	m.spin = spinner.New(spinner.WithSpinner(spinner.Dot))
	m.impName = textinput.New()
	m.impName.Prompt = ""
	m.impName.CharLimit = 32
	m.impName.Width = 24
	m.game = m.games[0]
	if i := cfg.ActiveChar; i >= 0 && i < len(cfg.Chars) {
		if g := m.moduleBySlug(cfg.Chars[i].Game); g != nil {
			m.game = g
		}
	}
	applyTheme(m.game.Accent)
	m.loadFilter()
	if raw, err := cfg.Character(); err == nil && raw != nil {
		// A stored character that no longer parses is surfaced in the view
		// as "no character"; re-importing overwrites it.
		m.char, _ = armory.Parse(raw)
	}
	return m
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.ensureDB(m.game), m.spin.Tick)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case dbLoadedMsg:
		if g := m.moduleBySlug(msg.slug); g != nil {
			g.DB = msg.db
		}
		if msg.err != nil {
			delete(m.dbLoading, msg.slug) // allow a retry on the next switch
		}
		if msg.slug == m.game.Slug {
			m.dbErr = msg.err
			m.rebuildScorer()
		}
		return m, nil

	case importFetchedMsg:
		if m.mode != modeImport {
			return m, nil // wizard was left while the fetch ran
		}
		m.impFetching = false
		if msg.err != nil {
			m.errMsg = msg.err.Error()
			return m, nil
		}
		m.pasted = msg.raw
		return m.saveImport()

	case spinner.TickMsg:
		if !m.impFetching && (m.game.DB != nil || m.dbErr != nil) {
			return m, nil // nothing in flight: let the tick loop end
		}
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd

	case cursor.BlinkMsg:
		if m.mode == modeImport {
			var cmd tea.Cmd
			m.impName, cmd = m.impName.Update(msg)
			return m, cmd
		}
		return m, nil

	case tea.KeyMsg:
		if m.gapWarn && m.mode == modeNormal {
			m.gapWarn = false // any key dismisses the gear warning
			return m, nil
		}
		switch m.mode {
		case modeImport:
			return m.updateImport(msg)
		case modeUpgrades:
			return m.updateUpgrades(msg)
		case modeFilter:
			return m.updateFilter(msg)
		case modeTalents:
			return m.updateTalents(msg)
		case modeWeights:
			return m.updateWeights(msg)
		case modeChars:
			return m.updateChars(msg)
		case modeOptimize:
			return m.updateOptimize(msg)
		case modeBiS:
			return m.updateBis(msg)
		case modeEmblems:
			return m.updateEmblems(msg)
		}
		return m.updateNormal(msg)
	}
	return m, nil
}

func (m Model) View() string {
	if m.width == 0 {
		return "loading..."
	}
	if m.gapWarn && m.mode == modeNormal {
		return m.viewGearWarning()
	}
	switch m.mode {
	case modeImport:
		return m.viewImport()
	case modeUpgrades:
		return m.viewUpgrades()
	case modeFilter:
		return m.viewFilter()
	case modeTalents:
		return m.viewTalents()
	case modeWeights:
		return m.viewWeights()
	case modeChars:
		return m.viewChars()
	case modeOptimize:
		return m.viewOptimize()
	case modeBiS:
		return m.viewBis()
	case modeEmblems:
		return m.viewEmblems()
	}
	return m.viewNormal()
}

// tierLabel names the tier of a class-set piece ("T8.5") per the
// active game module's rules.
func (m Model) tierLabel(it *db.Item) string {
	return m.game.TierLabel(it)
}

// truncate cuts s to at most w runes, ellipsized.
// truncate shortens s to w visible cells, ANSI-aware: styled strings
// carry escape codes that must not count toward the width.
func truncate(s string, w int) string {
	if ansi.StringWidth(s) <= w {
		return s
	}
	return ansi.Truncate(s, w, "…")
}

// ilvlStats returns the mean item level of the gear.
func ilvlStats(rows []gearRow) float64 {
	if len(rows) == 0 {
		return 0
	}
	sum := 0
	for _, r := range rows {
		sum += r.item.ILvl
	}
	return float64(sum) / float64(len(rows))
}

// setOf extracts the plain item list from gear rows.
func setOf(rows []gearRow) []*db.Item {
	set := make([]*db.Item, len(rows))
	for i, r := range rows {
		set[i] = r.item
	}
	return set
}
