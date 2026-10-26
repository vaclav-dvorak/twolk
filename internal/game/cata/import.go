package cata

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/vaclav-dvorak/twolk/internal/armory/whitemane"
)

// The whitemane API speaks in numeric WoW client IDs; the planner's
// armory format speaks display strings. These tables translate. All
// values verified against client data (ChrClasses/ChrRaces/SkillLine;
// talent tabs from the cata-classic TalentTab table, 2026-07-24).

var classNames = map[int]string{
	1: "Warrior", 2: "Paladin", 3: "Hunter", 4: "Rogue", 5: "Priest",
	6: "Death Knight", 7: "Shaman", 8: "Mage", 9: "Warlock", 11: "Druid",
}

var raceNames = map[int]string{
	1: "Human", 2: "Orc", 3: "Dwarf", 4: "Night Elf", 5: "Undead",
	6: "Tauren", 7: "Gnome", 8: "Troll", 9: "Goblin", 10: "Blood Elf",
	11: "Draenei", 22: "Worgen",
}

// specTrees maps Cataclysm talent-tab IDs (whitemane's
// specializationId) to tree names — the same names cata profiles will
// key on.
var specTrees = map[int]string{
	746: "Arms", 815: "Fury", 845: "Protection", // warrior
	831: "Holy", 839: "Protection", 855: "Retribution", // paladin
	811: "Beast Mastery", 807: "Marksmanship", 809: "Survival", // hunter
	182: "Assassination", 181: "Combat", 183: "Subtlety", // rogue
	760: "Discipline", 813: "Holy", 795: "Shadow", // priest
	398: "Blood", 399: "Frost", 400: "Unholy", // death knight
	261: "Elemental", 263: "Enhancement", 262: "Restoration", // shaman
	799: "Arcane", 851: "Fire", 823: "Frost", // mage
	871: "Affliction", 867: "Demonology", 865: "Destruction", // warlock
	752: "Balance", 750: "Feral Combat", 748: "Restoration", // druid
}

// professionNames maps primary-profession SkillLine IDs; secondary
// skills (cooking, fishing, ...) are deliberately absent — the armory
// format lists the two primaries, and only those affect scoring.
var professionNames = map[int]string{
	164: "Blacksmithing", 165: "Leatherworking", 171: "Alchemy",
	182: "Herbalism", 186: "Mining", 197: "Tailoring", 202: "Engineering",
	333: "Enchanting", 393: "Skinning", 755: "Jewelcrafting",
	773: "Inscription",
}

// FetchRaw imports a character from the Whitemane armory API and
// returns it as armory-export JSON — the same bytes a paste-based
// import would have stored, so everything downstream (config storage,
// armory.Parse, the gear screen) is shared with the WotLK flow. Worn
// gems and enchants are real (HasDetail); each equipment entry also
// carries the raw reforge descriptor for the day the planner models
// reforging — armory.Parse ignores it today.
func FetchRaw(ctx context.Context, realm, name string) ([]byte, error) {
	ch, err := whitemane.New().Character(ctx, realm, name)
	if err != nil {
		return nil, err
	}
	return convert(ch)
}

// export mirrors the armory JSON shape armory.Parse decodes.
type export struct {
	Name        string       `json:"name"`
	Realm       string       `json:"realm"`
	Level       int          `json:"level"`
	Faction     string       `json:"faction"`
	Race        string       `json:"race"`
	Class       string       `json:"class"`
	Origin      string       `json:"origin"`
	Equipment   []exportItem `json:"equipment"`
	Talents     []exportTree `json:"talents"`
	Professions []exportProf `json:"professions"`
	// exact talent ranks of the active spec, keyed by spell id — the
	// API tells us what a paste export never could
	TalentRanks map[string]int `json:"talent_ranks,omitempty"`
}

type exportItem struct {
	Item int   `json:"item"`
	Ench int   `json:"ench"`
	Gems []int `json:"gems"`
	// canonical slot name ("Trinket") from the wire's equipment-slot
	// enum: it names the row even when the item db can't resolve the
	// item (leveling gear below the endgame pool)
	Slot    string             `json:"slot,omitempty"`
	Reforge *whitemane.Reforge `json:"reforge,omitempty"`
}

// equipSlotNames maps the client equipment-slot enum (whitemane's
// slot field) to canonical slot names. Cosmetic slots (shirt 3,
// tabard 18) map to "" — worn, but never a gear row. Both rings and
// both trinkets share one canonical slot, like the item db.
var equipSlotNames = map[int]string{
	0: "Head", 1: "Neck", 2: "Shoulder", 4: "Chest", 5: "Waist",
	6: "Legs", 7: "Feet", 8: "Wrist", 9: "Hands",
	10: "Finger", 11: "Finger", 12: "Trinket", 13: "Trinket",
	14: "Back", 15: "Weapon", 16: "Weapon", 17: "Ranged",
}

type exportTree struct {
	Tree   string `json:"tree"`
	Points []int  `json:"points"`
}

type exportProf struct {
	Name  string `json:"name"`
	Skill int    `json:"skill"`
}

func convert(ch *whitemane.Character) ([]byte, error) {
	class, ok := classNames[ch.Class]
	if !ok {
		return nil, fmt.Errorf("unknown class id %d", ch.Class)
	}
	e := export{
		Name:    ch.Name,
		Realm:   ch.Realm,
		Level:   ch.Level,
		Faction: ch.Faction,
		Race:    raceNames[ch.Race],
		Class:   class,
		Origin:  "whitemane",
	}
	for _, it := range ch.Items {
		if it.ItemID == 0 {
			continue
		}
		e.Equipment = append(e.Equipment, exportItem{
			Item: it.ItemID, Ench: it.EnchantID,
			Gems: it.Sockets, Slot: equipSlotNames[it.Slot],
			Reforge: it.Reforge,
		})
	}
	if len(e.Equipment) == 0 {
		return nil, fmt.Errorf("%s@%s wears nothing", ch.Name, ch.Realm)
	}
	// spec slots in index order; the wire doesn't mark the active one,
	// so slot 0 is assumed active (armory.Character.Spec() reads
	// Talents[0]). Unused slots (specializationId 0) miss the table.
	for _, spec := range ch.Specializations {
		if tree, ok := specTrees[spec.SpecializationID]; ok {
			e.Talents = append(e.Talents, exportTree{Tree: tree})
		}
	}
	for _, p := range ch.Professions {
		if name, ok := professionNames[p.ID]; ok {
			e.Professions = append(e.Professions, exportProf{Name: name, Skill: p.Skill})
		}
	}
	// spec slot 0 = assumed active (see the spec note above); only
	// taken ranks are stored — absence means "not taken", exactly
	for _, t := range ch.Talents["0"] {
		if t.Rank > 0 && t.SpellID != 0 {
			if e.TalentRanks == nil {
				e.TalentRanks = map[string]int{}
			}
			e.TalentRanks[strconv.Itoa(t.SpellID)] = t.Rank
		}
	}
	return json.Marshal(e)
}
