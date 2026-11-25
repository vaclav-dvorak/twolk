package forever

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/vaclav-dvorak/twolk/internal/armory/forever"
)

// FetchRaw imports a character from the official armory and returns it
// as armory-export JSON — the same bytes every other import stores, so
// config storage, armory.Parse and the gear screen are shared. The
// region stands in the realm's place; the name is both parts ("Brannoc
// Ironvale"), the way the armory addresses Forever characters.
func FetchRaw(ctx context.Context, region, name string) ([]byte, error) {
	ch, err := forever.New().Character(ctx, region, name)
	if err != nil {
		return nil, err
	}
	return convert(ch, region)
}

// export mirrors the armory JSON shape armory.Parse decodes.
type export struct {
	Name        string         `json:"name"`
	Realm       string         `json:"realm"`
	Level       int            `json:"level"`
	Faction     string         `json:"faction"`
	Race        string         `json:"race"`
	Class       string         `json:"class"`
	Origin      string         `json:"origin"`
	Equipment   []exportItem   `json:"equipment"`
	Talents     []exportTree   `json:"talents"`
	Professions []exportProf   `json:"professions"`
	TalentRanks map[string]int `json:"talent_ranks,omitempty"`
}

type exportItem struct {
	Name string `json:"name"`
	Item int    `json:"item"`
	Ench int    `json:"ench"`
	// Forever has no gems; the empty list still marks the entry as
	// carrying real detail (the worn enchant counts, nothing assumed)
	Gems []int  `json:"gems"`
	Slot string `json:"slot,omitempty"`
}

type exportTree struct {
	Tree   string `json:"tree"`
	Points []int  `json:"points"`
}

type exportProf struct {
	Name  string `json:"name"`
	Skill int    `json:"skill"`
}

// slotNames maps the armory's slot types to canonical slot names, in
// paper-doll order. Shirt and tabard map to "": worn, never a gear row.
var slotNames = []struct{ armory, canonical string }{
	{"HEAD", "Head"}, {"NECK", "Neck"}, {"SHOULDER", "Shoulder"},
	{"BACK", "Back"}, {"CHEST", "Chest"}, {"SHIRT", ""}, {"TABARD", ""},
	{"WRIST", "Wrist"}, {"HANDS", "Hands"}, {"WAIST", "Waist"},
	{"LEGS", "Legs"}, {"FEET", "Feet"},
	{"FINGER_1", "Finger"}, {"FINGER_2", "Finger"},
	{"TRINKET_1", "Trinket"}, {"TRINKET_2", "Trinket"},
	{"MAIN_HAND", "Weapon"}, {"OFF_HAND", "Weapon"}, {"RANGED", "Ranged"},
}

func convert(ch *forever.Character, region string) ([]byte, error) {
	e := export{
		Name:    ch.FullName(),
		Realm:   strings.ToUpper(region),
		Level:   ch.Level,
		Faction: ch.Faction.Name,
		Race:    ch.Race.Name,
		Class:   ch.Class.Name,
		Origin:  "blizzard forever",
	}
	bySlot := map[string]forever.Item{}
	for _, it := range ch.Gear {
		bySlot[it.Slot.Type] = it
	}
	for _, s := range slotNames {
		it, ok := bySlot[s.armory]
		if !ok || it.ID == 0 {
			continue
		}
		e.Equipment = append(e.Equipment, exportItem{
			Name: it.Name, Item: it.ID, Ench: it.PermanentEnchant(),
			Gems: []int{}, Slot: s.canonical,
		})
	}
	if len(e.Equipment) == 0 {
		return nil, fmt.Errorf("%s wears nothing", ch.FullName())
	}
	// the active tree first (armory.Character.Spec reads Talents[0]),
	// the rest by points spent; ranks of every tree are exact
	specs := append([]forever.Spec(nil), ch.Specs...)
	sort.SliceStable(specs, func(i, j int) bool {
		if specs[i].Active != specs[j].Active {
			return specs[i].Active
		}
		return specs[i].SpentPoints > specs[j].SpentPoints
	})
	for _, s := range specs {
		e.Talents = append(e.Talents, exportTree{Tree: s.Name})
		for _, t := range s.Talents {
			if t.Rank > 0 && t.ID != 0 {
				if e.TalentRanks == nil {
					e.TalentRanks = map[string]int{}
				}
				e.TalentRanks[strconv.Itoa(t.ID)] = t.Rank
			}
		}
	}
	for _, p := range ch.Professions.Primaries {
		e.Professions = append(e.Professions, exportProf{Name: p.Name, Skill: p.Skill()})
	}
	return json.Marshal(e)
}
