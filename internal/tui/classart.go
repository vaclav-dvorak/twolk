package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/vaclav-dvorak/twolk/internal/db"
)

// titleArt is the app name, shown on wide terminals where the plain
// one-liner would be underwhelming next to the class icon.
const titleArt = `|              |    |
|--- . . .,---.|    |__/
|    | | ||   ||    |  \
` + "`---'`-'-'`---'`---'`   `"

// classColors are the official WoW class colors (truecolor; lipgloss
// degrades them gracefully on 256-color terminals).
var classColors = map[db.Class]lipgloss.Color{
	db.ClassWarrior:     lipgloss.Color("#C79C6E"),
	db.ClassPaladin:     lipgloss.Color("#F58CBA"),
	db.ClassHunter:      lipgloss.Color("#ABD473"),
	db.ClassRogue:       lipgloss.Color("#FFF569"),
	db.ClassPriest:      lipgloss.Color("#FFFFFF"),
	db.ClassDeathKnight: lipgloss.Color("#C41F3B"),
	db.ClassShaman:      lipgloss.Color("#0070DE"),
	db.ClassMage:        lipgloss.Color("#69CCF0"),
	db.ClassWarlock:     lipgloss.Color("#9482C9"),
	db.ClassDruid:       lipgloss.Color("#FF7D0A"),
}

// classArt is a small emblem per class, 5 lines each.
var classArt = map[db.Class]string{
	db.ClassHunter: `   /^\
  |  |
 =|--+==>
  |  |
   \v/`,
	db.ClassWarrior: ` \\  ,  //
  \\ | //
   >-+-<
  // | \\
 //  '  \\`,
	db.ClassRogue: ` |\    /|
 ||    ||
 ||    ||
 \/    \/
  '    '`,
	db.ClassMage: `    /\
   /  \
  / ** \
 /______\
    ||`,
	db.ClassPriest: `    _
  _| |_
 |_   _|
   | |
   |_|`,
	db.ClassWarlock: ` (\    /)
  \\__//
  ( @@ )
   \~~/
    \/`,
	db.ClassDruid: `  ()  ()
 ()    ()
   ,--.
  ( :: )
   ` + "`--'",
	db.ClassShaman: `    __
   / /
  / /_
 /__ /
   /_/`,
	db.ClassPaladin: `  ______
 [______]
    ||
    ||
    ||`,
	db.ClassDeathKnight: `    A
   |||
   |||
  =====
   |*|`,
}

// classColor returns the class color, falling back to the default text
// color for unknown classes.
func classColor(c db.Class) lipgloss.Color {
	if col, ok := classColors[c]; ok {
		return col
	}
	return lipgloss.Color("255")
}

// classIconKeys maps classes to keys of the generated pixel table.
var classIconKeys = map[db.Class]string{
	db.ClassWarrior: "Warrior", db.ClassPaladin: "Paladin",
	db.ClassHunter: "Hunter", db.ClassRogue: "Rogue",
	db.ClassPriest: "Priest", db.ClassDeathKnight: "DeathKnight",
	db.ClassShaman: "Shaman", db.ClassMage: "Mage",
	db.ClassWarlock: "Warlock", db.ClassDruid: "Druid",
}

// iconCache holds rendered icons; building the styled string once per
// class is enough.
var iconCache = map[db.Class]string{}

// darkPixel reports whether a pixel is dark enough to count as icon
// background rather than art.
func darkPixel(p uint32) bool {
	r, g, b := p>>16&0xff, p>>8&0xff, p&0xff
	return r+g+b < 0x5a
}

// transparencyMask flood-fills from the image borders across dark pixels:
// dark areas connected to the outside are background and become
// transparent; dark pixels inside the art (shadows) are kept.
func transparencyMask(px [][]uint32) [][]bool {
	h, w := len(px), len(px[0])
	mask := make([][]bool, h)
	for y := range mask {
		mask[y] = make([]bool, w)
	}
	var stack [][2]int
	push := func(y, x int) {
		if y >= 0 && y < h && x >= 0 && x < w && !mask[y][x] && darkPixel(px[y][x]) {
			mask[y][x] = true
			stack = append(stack, [2]int{y, x})
		}
	}
	for x := 0; x < w; x++ {
		push(0, x)
		push(h-1, x)
	}
	for y := 0; y < h; y++ {
		push(y, 0)
		push(y, w-1)
	}
	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		push(p[0]-1, p[1])
		push(p[0]+1, p[1])
		push(p[0], p[1]-1)
		push(p[0], p[1]+1)
	}
	return mask
}

// renderClassIcon draws the class icon as truecolor half-block pixel art.
// Each cell carries two pixels (▀: foreground above, background below);
// transparent pixels leave the terminal background untouched (▀/▄/space).
func renderClassIcon(class db.Class) string {
	if s, ok := iconCache[class]; ok {
		return s
	}
	px, ok := classIconPixels[classIconKeys[class]]
	if !ok || len(px)%2 != 0 {
		return ""
	}
	mask := transparencyMask(px)
	hex := func(p uint32) lipgloss.Color { return lipgloss.Color(fmt.Sprintf("#%06x", p)) }
	var b strings.Builder
	for y := 0; y < len(px); y += 2 {
		for x := range px[y] {
			topT, botT := mask[y][x], mask[y+1][x]
			switch {
			case topT && botT:
				b.WriteByte(' ')
			case botT:
				b.WriteString(lipgloss.NewStyle().Foreground(hex(px[y][x])).Render("▀"))
			case topT:
				b.WriteString(lipgloss.NewStyle().Foreground(hex(px[y+1][x])).Render("▄"))
			default:
				b.WriteString(lipgloss.NewStyle().
					Foreground(hex(px[y][x])).Background(hex(px[y+1][x])).Render("▀"))
			}
		}
		if y+2 < len(px) {
			b.WriteByte('\n')
		}
	}
	iconCache[class] = b.String()
	return iconCache[class]
}

// classArtView renders the class emblem — the real class icon as pixel
// art, falling back to the hand-drawn ASCII art — or "" when the
// terminal is too narrow to spare the columns.
func (m Model) classArtView() string {
	if m.char == nil || m.width < 100 {
		return ""
	}
	if icon := renderClassIcon(m.char.Class); icon != "" {
		return icon
	}
	art, ok := classArt[m.char.Class]
	if !ok {
		return ""
	}
	return lipgloss.NewStyle().Foreground(classColor(m.char.Class)).Bold(true).Render(art)
}
