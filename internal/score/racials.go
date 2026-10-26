package score

import "github.com/vaclav-dvorak/twolk/internal/db"

// critRatingPerPct converts crit chance % to rating at level 80.
const critRatingPerPct = 45.91

// WeaponRacial is a race's bonus that applies only while wielding a
// specific weapon type, expressed as virtual gear stats so it flows
// through the normal cap-aware scoring (racial expertise counts toward
// the expertise cap, racial crit is plain crit). Stats carry the owning
// version's rating conversions.
type WeaponRacial struct {
	Race  string
	Note  string
	Match func(*db.Item) bool
	Stats db.Stats
}

// StatOf is a one-stat Stats value — the building block of racial
// tables in every version's game module.
func StatOf(st db.Stat, v float64) db.Stats {
	var s db.Stats
	s[st] = v
	return s
}

// Weapon-type matchers shared by every version's racial tables.
func MatchWeapon(types ...db.WeaponType) func(*db.Item) bool {
	return func(it *db.Item) bool {
		if it.Slot != db.SlotWeapon {
			return false
		}
		for _, t := range types {
			if it.WeaponType == t {
				return true
			}
		}
		return false
	}
}

func MatchRanged(t db.RangedWeaponType) func(*db.Item) bool {
	return func(it *db.Item) bool {
		return it.Slot == db.SlotRanged && it.RangedType == t
	}
}

// wotlkWeaponRacials are the 3.3.5 weapon racials at level-80 ratings.
var wotlkWeaponRacials = []WeaponRacial{
	{
		Race: "Orc", Note: "Orc racial: +5 expertise with axes",
		Match: MatchWeapon(db.WeaponAxe),
		Stats: StatOf(db.Expertise, 5*RatingPerExpertise),
	},
	{
		Race: "Human", Note: "Human racial: +3 expertise with swords and maces",
		Match: MatchWeapon(db.WeaponSword, db.WeaponMace),
		Stats: StatOf(db.Expertise, 3*RatingPerExpertise),
	},
	{
		Race: "Troll", Note: "Troll racial: +1% crit with bows",
		Match: MatchRanged(db.RangedBow),
		Stats: StatOf(db.MeleeCrit, critRatingPerPct),
	},
	{
		Race: "Dwarf", Note: "Dwarf racial: +1% crit with guns",
		Match: MatchRanged(db.RangedGun),
		Stats: StatOf(db.MeleeCrit, critRatingPerPct),
	},
}
