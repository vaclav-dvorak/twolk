// Package armory parses character exports from the armory of the free
// 3.3.5 server twolk targets (JSON API format).
package armory

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/vaclav-dvorak/twolk/internal/db"
)

// EquippedItem is one worn item. The plain armory API export carries no
// slot labels, gems, or enchants — only the base item; slots are resolved
// via the item DB. Exports enriched by tools/armory-export.js additionally
// carry the worn enchant (effect ID) and gems (item IDs, 0 = empty
// socket); HasDetail marks their presence. Cata API imports also carry
// the applied reforge (nil when the item is not reforged).
type EquippedItem struct {
	Name      string
	ItemID    int
	EnchantID int
	Gems      []int
	HasDetail bool
	Reforge   *db.Reforge
	// Slot is the export's own slot claim (fetch imports carry it) —
	// SlotUnknown on plain pastes, where slots resolve via the item db.
	// It names the row when the db can't: gear below the endgame pool.
	Slot db.Slot
}

// TalentSpec is one talent tree summary from the export.
type TalentSpec struct {
	Tree   string // e.g. "Survival"
	Points []int  // points per tree, e.g. [0, 15, 56]
}

// Profession is one learned profession.
type Profession struct {
	Name  string
	Skill int
}

// Character is a parsed armory export.
type Character struct {
	Name        string
	Realm       string
	Level       int
	Faction     string
	Race        string
	ClassName   string
	Class       db.Class
	Guild       string
	Origin      string // armory site origin, present in enriched exports
	Equipment   []EquippedItem
	Talents     []TalentSpec
	Professions []Profession
	// TalentRanks maps talent spell IDs to taken ranks for the ACTIVE
	// spec — exact data, present only in API-fetched imports (cata);
	// nil for paste exports, whose talents are per-tree sums only.
	TalentRanks map[int]int
}

// Spec returns the character's active spec tree name, if the export has one.
func (c *Character) Spec() string {
	if len(c.Talents) == 0 {
		return ""
	}
	return c.Talents[0].Tree
}

// flexInt tolerates the armory's habit of encoding numbers as strings.
type flexInt int

func (f *flexInt) UnmarshalJSON(data []byte) error {
	s := string(data)
	if len(s) >= 2 && s[0] == '"' {
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return fmt.Errorf("not a number: %s", data)
	}
	*f = flexInt(n)
	return nil
}

// Parse decodes an armory JSON export, or a name-only placeholder.
func Parse(raw []byte) (*Character, error) {
	var doc struct {
		Name      string `json:"name"`
		Realm     string `json:"realm"`
		Level     flexInt
		Faction   string `json:"faction"`
		Race      string `json:"race"`
		Class     string `json:"class"`
		Guild     string `json:"guild"`
		Origin    string `json:"origin"`
		Equipment []struct {
			Name    string     `json:"name"`
			Item    flexInt    `json:"item"`
			Ench    flexInt    `json:"ench"`
			Slot    string     `json:"slot"`
			Gems    *[]flexInt `json:"gems"` // pointer: presence marks enriched export
			Reforge *struct {
				Source flexInt `json:"source"` // ITEM_MOD_* ids
				Target flexInt `json:"target"`
				Value  flexInt `json:"value"`
			} `json:"reforge"`
		} `json:"equipment"`
		Talents []struct {
			Tree   string `json:"tree"`
			Points []int  `json:"points"`
		} `json:"talents"`
		Professions []struct {
			Name  string  `json:"name"`
			Skill flexInt `json:"skill"`
		} `json:"professions"`
		TalentRanks map[string]int `json:"talent_ranks"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parsing armory export: %w", err)
	}
	// a name alone is a placeholder: a character added but not yet
	// refreshed (the refresh rejects exports without equipment)
	if doc.Name == "" {
		return nil, fmt.Errorf("not an armory export (no character name)")
	}

	c := &Character{
		Name:      doc.Name,
		Realm:     doc.Realm,
		Level:     int(doc.Level),
		Faction:   doc.Faction,
		Race:      doc.Race,
		ClassName: doc.Class,
		Class:     db.ClassByName(doc.Class),
		Guild:     doc.Guild,
		Origin:    doc.Origin,
	}
	for _, e := range doc.Equipment {
		it := EquippedItem{Name: e.Name, ItemID: int(e.Item), EnchantID: int(e.Ench), Slot: db.SlotByName(e.Slot)}
		if e.Gems != nil {
			it.HasDetail = true
			for _, g := range *e.Gems {
				it.Gems = append(it.Gems, int(g))
			}
		}
		if r := e.Reforge; r != nil && r.Value > 0 {
			from, okF := db.StatByItemMod(int(r.Source))
			to, okT := db.StatByItemMod(int(r.Target))
			if okF && okT {
				it.Reforge = &db.Reforge{From: from, To: to, Amount: float64(r.Value)}
			}
		}
		c.Equipment = append(c.Equipment, it)
	}
	for _, t := range doc.Talents {
		c.Talents = append(c.Talents, TalentSpec{Tree: t.Tree, Points: t.Points})
	}
	for _, p := range doc.Professions {
		c.Professions = append(c.Professions, Profession{Name: p.Name, Skill: int(p.Skill)})
	}
	for id, rank := range doc.TalentRanks {
		spell, err := strconv.Atoi(id)
		if err != nil {
			continue
		}
		if c.TalentRanks == nil {
			c.TalentRanks = map[int]int{}
		}
		c.TalentRanks[spell] = rank
	}
	return c, nil
}
