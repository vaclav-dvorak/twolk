package score

import "github.com/vaclav-dvorak/twolk/internal/db"

// Weapon layouts and hand roles. All weapons share db.SlotWeapon, but a
// candidate is only legal for the position it would occupy: a two-hander
// cannot join a worn off-hand, a main-hand-only blade cannot sit in the
// off hand, a shield row takes shields. Each profile lists the layouts
// its build can play (Profile.Layouts); BestSet solves every one and
// keeps the best — two one-handers each score below a two-hander, yet
// the pair can win.

// WeaponLayout is one legal way a build fills its weapon slot(s).
type WeaponLayout int

const (
	LayoutTwoHand      WeaponLayout = iota // one two-handed weapon
	LayoutDualWield                        // two one-handed weapons
	LayoutTitanGrip                        // fury: 2H axes/maces/swords in both hands
	LayoutWeaponShield                     // one-hander + shield (or held frill)
	LayoutWeaponHeld                       // one-hander + held-in-off-hand frill
)

// weaponRole is what a single weapon row may hold.
type weaponRole int

const (
	roleNone      weaponRole = iota // not a weapon row
	roleTwoHand                     // the only weapon: two-handers
	roleMainHand                    // main hand of a pair
	roleOffWeapon                   // off hand of a dual-wield
	roleOffShield                   // off hand: shield or held frill
	roleOffHeld                     // off hand: held frill only
	roleTGMain                      // Titan's Grip main hand
	roleTGOff                       // Titan's Grip off hand
)

// layoutRoles is the row shape of a layout, main hand first.
func layoutRoles(l WeaponLayout) []weaponRole {
	switch l {
	case LayoutTwoHand:
		return []weaponRole{roleTwoHand}
	case LayoutDualWield:
		return []weaponRole{roleMainHand, roleOffWeapon}
	case LayoutTitanGrip:
		return []weaponRole{roleTGMain, roleTGOff}
	case LayoutWeaponShield:
		return []weaponRole{roleMainHand, roleOffShield}
	case LayoutWeaponHeld:
		return []weaponRole{roleMainHand, roleOffHeld}
	}
	return nil
}

// heldOrShield marks the off-hand items that are not weapons.
func heldOrShield(it *db.Item) bool {
	return it.WeaponType == db.WeaponShield || it.WeaponType == db.WeaponOffHand
}

// tgTwoHander is a two-hander Titan's Grip can hold: axe, mace or sword
// (polearms and staves stay two-hand-only).
func tgTwoHander(it *db.Item) bool {
	return it.HandType == db.HandTwoHand &&
		(it.WeaponType == db.WeaponAxe || it.WeaponType == db.WeaponMace ||
			it.WeaponType == db.WeaponSword)
}

// roleAllows reports whether an item may occupy a weapon row of the
// given role. Non-weapon rows (roleNone) accept anything — the caller
// gates those by db.Slot already.
func roleAllows(r weaponRole, it *db.Item) bool {
	switch r {
	case roleNone:
		return true
	case roleTwoHand:
		return it.HandType == db.HandTwoHand && !heldOrShield(it)
	case roleMainHand:
		return !heldOrShield(it) &&
			(it.HandType == db.HandMainHand || it.HandType == db.HandOneHand)
	case roleOffWeapon:
		return !heldOrShield(it) &&
			(it.HandType == db.HandOneHand || it.HandType == db.HandOffHand)
	case roleOffShield:
		return heldOrShield(it)
	case roleOffHeld:
		return it.WeaponType == db.WeaponOffHand
	case roleTGMain:
		return roleAllows(roleMainHand, it) || tgTwoHander(it)
	case roleTGOff:
		return roleAllows(roleOffWeapon, it) || tgTwoHander(it)
	}
	return false
}

// hasLayout reports whether the profile plays the given layout.
func (p *Profile) hasLayout(l WeaponLayout) bool {
	return contains(p.Layouts, l)
}

// OffhandUsable reports whether the build can hold anything in the
// off-hand slot alongside this main hand: some two-row layout must
// accept the worn weapon in its main hand. A two-hander blocks the off
// hand unless the profile Titan-Grips and the weapon qualifies.
func (p *Profile) OffhandUsable(mh *db.Item) bool {
	for _, l := range p.Layouts {
		if roles := layoutRoles(l); len(roles) == 2 && roleAllows(roles[0], mh) {
			return true
		}
	}
	return false
}

// wornWeaponRoles assigns a role to every row of a worn set from its
// current shape — the layout the character is actually playing — so
// single-swap proposals (upgrade browser, descent without layout
// exploration) stay legal within it. One weapon row: two-hander or lone
// main hand. Two rows: main hand first, the second read from what it
// holds (weapon, shield or frill). Titan's Grip profiles widen both
// hands of a pair to two-handers.
func (p *Profile) wornWeaponRoles(set []*db.Item) []weaponRole {
	roles := make([]weaponRole, len(set))
	var weap []int
	for i, it := range set {
		if it.Slot == db.SlotWeapon {
			weap = append(weap, i)
		}
	}
	switch len(weap) {
	case 1:
		i := weap[0]
		if set[i].HandType == db.HandTwoHand {
			roles[i] = roleTwoHand
		} else {
			roles[i] = roleMainHand
		}
	case 2:
		mh, oh := weap[0], weap[1]
		// worn order is not gospel (armory exports list items, not
		// hands): if only one of the two can be a main-hand weapon,
		// that one is the main hand
		if heldOrShield(set[mh]) || set[mh].HandType == db.HandOffHand {
			mh, oh = oh, mh
		}
		roles[mh] = roleMainHand
		switch {
		case set[oh].WeaponType == db.WeaponShield:
			roles[oh] = roleOffShield
		case set[oh].WeaponType == db.WeaponOffHand:
			roles[oh] = roleOffHeld
			if p.hasLayout(LayoutWeaponShield) {
				roles[oh] = roleOffShield // shield-capable: offer both
			}
		default:
			roles[oh] = roleOffWeapon
		}
		if p.hasLayout(LayoutTitanGrip) && roles[oh] == roleOffWeapon {
			roles[mh], roles[oh] = roleTGMain, roleTGOff
		}
	}
	return roles
}

// EmptyHand is the placeholder for a weapon row a layout adds but the
// character has nothing for (e.g. exploring dual-wield while wearing a
// two-hander). Zero stats, zero assumptions, zero score.
var EmptyHand = &db.Item{ID: 0, Name: "(empty)", Slot: db.SlotWeapon}

// reshapeWorn rebuilds the worn rows for a target layout: non-weapon
// rows unchanged, weapon rows re-dealt onto the layout's roles, in
// place of the worn weapon rows (or at the end when none are worn).
// Worn weapons keep their instance (gems/enchants) when they fit a
// role; rows nothing fits start empty. The returned roles align with
// the rows.
func reshapeWorn(ws []Worn, l WeaponLayout) ([]Worn, []weaponRole) {
	var weapons, head, tail []Worn
	for _, w := range ws {
		switch {
		case w.Item.Slot == db.SlotWeapon:
			weapons = append(weapons, w)
		case len(weapons) == 0:
			head = append(head, w)
		default:
			tail = append(tail, w)
		}
	}
	target := layoutRoles(l)
	used := make([]bool, len(weapons))
	out := append([]Worn(nil), head...)
	roles := make([]weaponRole, len(head), len(head)+len(target)+len(tail))
	for _, r := range target {
		row := Worn{Item: EmptyHand}
		for i, w := range weapons {
			if !used[i] && roleAllows(r, w.Item) {
				row, used[i] = w, true
				break
			}
		}
		out = append(out, row)
		roles = append(roles, r)
	}
	out = append(out, tail...)
	roles = append(roles, make([]weaponRole, len(tail))...)
	return out, roles
}
