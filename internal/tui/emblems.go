package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/vaclav-dvorak/twolk/internal/db"
	"github.com/vaclav-dvorak/twolk/internal/score"
)

// The emblem advisor (p): a spending plan for gear currencies, grouped
// by currency and ranked by set-score gain PER TOKEN — buy from the
// top. Each worn slot gets ONE row per currency: its TARGET, the
// strongest piece that currency buys for it. Weaker pieces for the
// same slot are not shown at all: a cheap one's gain per token always
// looks better, but buying it and then the target pays for the slot
// twice — the target's gain is measured from the CURRENT gear, so the
// tokens spent on the weaker piece are simply lost (user report,
// Gilneas hunter shoulders). Rows within one currency compete for the same tokens,
// so ratios rank priority; deltas do not add up.

// emblemBuy is one purchasable upgrade: the best gear row replacement
// a currency can buy for its slot.
type emblemBuy struct {
	rowIdx int
	up     score.Upgrade
	price  db.EmblemPrice
	label  string // protocol route name ("Protocol Inferno"), "" = vendor
}

// perToken is the row's ranking key: set-score gain per token spent.
func (b emblemBuy) perToken() float64 {
	return b.up.Delta / float64(b.price.Count)
}

func (m *Model) openEmblems() {
	m.embList = m.emblemBuys()
	m.embOff = 0
	m.mode = modeEmblems
}

// buyPrice is the currency route of an item AT THE FILTER PHASE: the
// badge/point vendor table first (phase-rotated for cata), else the
// protocol-fragment route when its vendor is open and the exact cost
// is known. A price in a currency the server phase has not shipped yet
// is no route at all — the item's own phase tag tracks the content,
// not the currency (a P3 wotlk server must see no Frost prices).
func (m Model) buyPrice(itemID int) (db.EmblemPrice, string, bool) {
	if p, ok := m.game.DB.PriceFor(itemID, m.filter.ServerPhase); ok {
		return p, "", p.Count > 0 && m.currencyOpen(p.Emblem)
	}
	if pp, ok := m.protocolFor(itemID); ok && pp.Count > 0 {
		return db.EmblemPrice{Emblem: pp.Emblem, Count: pp.Count}, pp.Label, true
	}
	return db.EmblemPrice{}, "", false
}

// emblemBuys collects every emblem-buyable upgrade candidate across
// all slots — same-slot alternatives (tier vs offset piece) each keep
// their own row so their cost/gain trade-offs stay visible; only the
// same ITEM proposed for paired slots collapses (dedupeEmblemBuys).
func (m Model) emblemBuys() []emblemBuy {
	rows := m.gearRows()
	ws := wornOf(rows)
	if m.scorer == nil || len(ws) == 0 {
		return nil
	}
	var list []emblemBuy
	for i := range ws {
		for _, up := range m.scorer.UpgradesWorn(ws, i, 200, m.allowItem) {
			if up.Delta <= 0 {
				continue
			}
			p, label, ok := m.buyPrice(up.Item.ID)
			if !ok {
				continue
			}
			list = append(list, emblemBuy{rowIdx: i, up: up, price: p, label: label})
		}
	}
	list = strongestPerSlot(dedupeEmblemBuys(list))
	sortEmblemBuys(list, m.game.Currencies)
	return list
}

// strongestPerSlot keeps one buy per (currency, worn slot): the one
// with the largest gain (ties: the cheaper). Inferior buys for the
// slot are dropped, not ranked.
func strongestPerSlot(list []emblemBuy) []emblemBuy {
	type key struct {
		e   db.Emblem
		row int
	}
	best := map[key]int{}
	for i, b := range list {
		k := key{b.price.Emblem, b.rowIdx}
		j, ok := best[k]
		if !ok || b.up.Delta > list[j].up.Delta ||
			(b.up.Delta == list[j].up.Delta && b.price.Count < list[j].price.Count) {
			best[k] = i
		}
	}
	var out []emblemBuy
	for i, b := range list {
		if best[key{b.price.Emblem, b.rowIdx}] == i {
			out = append(out, b)
		}
	}
	return out
}

// dedupeEmblemBuys collapses one vendor item suggested for several
// rows: paired slots (rings, trinkets) both resolve to the same best
// candidate, but it can only be bought once — and worn once, these
// pieces are unique-equipped. The surviving row is the better trade:
// it replaces the weaker worn piece.
func dedupeEmblemBuys(list []emblemBuy) []emblemBuy {
	keep := map[int]int{} // item id -> index of its surviving row
	for i, b := range list {
		j, ok := keep[b.up.Item.ID]
		if !ok || b.perToken() > list[j].perToken() ||
			(b.perToken() == list[j].perToken() && b.rowIdx < list[j].rowIdx) {
			keep[b.up.Item.ID] = i
		}
	}
	var out []emblemBuy
	for i, b := range list {
		if keep[b.up.Item.ID] == i {
			out = append(out, b)
		}
	}
	return out
}

// sortEmblemBuys orders the plan: currency groups in the game's
// display order (unknown currencies last), best gain-per-token first
// inside each group; the slot index breaks exact ties deterministically.
func sortEmblemBuys(list []emblemBuy, order []db.Emblem) {
	rank := map[db.Emblem]int{}
	for i, e := range order {
		rank[e] = i
	}
	group := func(e db.Emblem) int {
		if r, ok := rank[e]; ok {
			return r
		}
		return len(order) + int(e)
	}
	sort.SliceStable(list, func(a, b int) bool {
		ga, gb := group(list[a].price.Emblem), group(list[b].price.Emblem)
		if ga != gb {
			return ga < gb
		}
		if list[a].perToken() != list[b].perToken() {
			return list[a].perToken() > list[b].perToken()
		}
		return list[a].rowIdx < list[b].rowIdx
	})
}

func (m Model) updateEmblems(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "p", "q", "enter":
		m.mode = modeNormal
		return m, nil
	case "up", "k", "down", "j", "pgup", "pgdown":
		body := m.embBody()
		if body == "" {
			return m, nil
		}
		vis := m.optVisible(lipgloss.Height(m.viewHeader()))
		step := 1
		if k := msg.String(); k == "pgup" || k == "pgdown" {
			step = max(vis-1, 1)
		}
		if k := msg.String(); k == "up" || k == "k" || k == "pgup" {
			step = -step
		}
		m.embOff = clampOff(m.embOff+step, lipgloss.Height(body), vis)
		return m, nil
	case "ctrl+c":
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) viewEmblems() string {
	header := m.viewHeader()
	context := titleStyle.Render("▸ Emblem advisor") +
		dimStyle.Render("  (every buyable upgrade, ranked by gain per token — buy from the top)")
	help := helpLine("esc", "back")

	body := m.embBody()
	switch {
	case body == "":
		body = dimStyle.Render("no emblem-buyable upgrades under the current filter")
	default:
		vis := m.optVisible(lipgloss.Height(header))
		if lipgloss.Height(body) > vis {
			help = helpLine("↑/↓", "scroll") + "  " + help
			body = scrollRegion(body, m.embOff, vis, m.width)
		}
	}
	return m.pinFooter(lipgloss.JoinVertical(lipgloss.Left, header, "", context, "", body), help)
}

// embBody renders the grouped spending plan ("" when nothing is
// buyable): one section per currency, one ranked line per slot.
func (m Model) embBody() string {
	if len(m.embList) == 0 {
		return ""
	}
	rows := m.gearRows()
	nameMax := max(m.width-64, 20) // rank+slot+ilvl+tier+cost+delta+ratio columns

	var lines []string
	var cur db.Emblem
	n := 0
	for _, b := range m.embList {
		if b.rowIdx >= len(rows) {
			continue
		}
		if b.price.Emblem != cur || len(lines) == 0 {
			cur = b.price.Emblem
			n = 0
			if len(lines) > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, warnStyle.Render("◈ ")+titleStyle.Render(cur.String()))
		}
		n++
		it := b.up.Item
		name := it.Name
		if it.Heroic {
			name += " [H]"
		}
		if b.label != "" {
			name += " · " + b.label
		}
		line := dimStyle.Render(fmt.Sprintf(" %d. ", n)) +
			fmt.Sprintf("%-9s ", rows[b.rowIdx].item.Slot) +
			dimStyle.Render(fmt.Sprintf("%4d  ", it.ILvl)) +
			heroicCell(rowStyler(false), qualityStyles[it.Quality], name, nameMax) +
			tierStyle.Render(fmt.Sprintf(" %-7s", m.tierLabel(it))) +
			dimStyle.Render(fmt.Sprintf(" %5d× ", b.price.Count)) +
			okStyle.Render(fmt.Sprintf("+%-5.0f", b.up.Delta)) +
			dimStyle.Render(" = ") +
			okStyle.Render(fmt.Sprintf("%.2f", b.perToken())) +
			dimStyle.Render("/token")
		lines = append(lines, line)
	}
	// one full-length note, wrapped by the terminal width only when it
	// has to be (never pre-broken at an arbitrary point)
	lines = append(lines, "", dimStyle.Width(max(m.width-2, 40)).Render(
		"every gain is priced against your current gear — buy the top item first, "+
			"then reopen the advisor: it re-ranks against your new set"))
	return strings.Join(lines, "\n")
}
