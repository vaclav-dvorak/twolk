package score

import (
	"testing"

	"github.com/vaclav-dvorak/twolk/internal/db"
)

// weapon builds a test weapon (stat stick unless dmg/speed set later).
func weapon(id int, wt db.WeaponType, ht db.HandType, agi float64) *db.Item {
	it := &db.Item{ID: id, Name: "weapon", Slot: db.SlotWeapon,
		WeaponType: wt, HandType: ht}
	it.Stats[db.Agility] = agi
	return it
}

// dwProfile is a hunter-flavored stat-stick profile that may play one
// two-hander or two one-handers.
func dwProfile() *Profile {
	p := testProfile()
	p.WeaponTypes = []db.WeaponType{db.WeaponAxe, db.WeaponDagger,
		db.WeaponFist, db.WeaponPolearm, db.WeaponStaff, db.WeaponSword}
	p.Layouts = []WeaponLayout{LayoutTwoHand, LayoutDualWield}
	return p
}

func TestBestSetSolvesWeaponLayout(t *testing.T) {
	d := testDatabase()
	staff := weapon(300, db.WeaponStaff, db.HandTwoHand, 100)
	sword := weapon(301, db.WeaponSword, db.HandOneHand, 60)
	d.Items[staff.ID] = staff
	d.Items[sword.ID] = sword
	s := New(d, dwProfile())

	// wearing the staff: each sword alone is worse (60 < 100 agi), but
	// the pair (120 agi) must win — the result carries TWO weapon rows,
	// both empty-handed in the baseline
	ws, set, sc := s.BestSet(wrapWorn([]*db.Item{staff}), nil)
	if len(set) != 2 || set[0].ID != sword.ID || set[1].ID != sword.ID {
		t.Fatalf("dual-wield pair not formed: %+v", set)
	}
	if ws[0].Item.ID != 0 || ws[1].Item.ID != 0 {
		t.Fatalf("baseline must hold empty hands, got %v/%v", ws[0].Item, ws[1].Item)
	}
	if want := 240.0; sc != want { // 2×60 agi × 2.0
		t.Fatalf("score = %v, want %v", sc, want)
	}

	// a big enough two-hander flips it back — one weapon row, worn kept
	big := weapon(302, db.WeaponStaff, db.HandTwoHand, 150)
	d.Items[big.ID] = big
	s = New(d, dwProfile())
	ws, set, _ = s.BestSet(wrapWorn([]*db.Item{staff}), nil)
	if len(set) != 1 || set[0].ID != big.ID {
		t.Fatalf("two-hand layout must win: %+v", set)
	}
	if ws[0].Item.ID != staff.ID {
		t.Fatalf("worn two-hander must stay the baseline, got %v", ws[0].Item)
	}
}

func TestBestSetKeepsWornWeaponInItsHand(t *testing.T) {
	d := testDatabase()
	owned := weapon(310, db.WeaponSword, db.HandOneHand, 60)
	better := weapon(311, db.WeaponAxe, db.HandOneHand, 70)
	d.Items[owned.ID] = owned
	d.Items[better.ID] = better
	s := New(d, dwProfile())

	// wearing one sword: dual-wield wins, the owned sword must stay in
	// its own row (not drift to the off hand as "to acquire")
	ws, set, _ := s.BestSet(wrapWorn([]*db.Item{owned}), nil)
	if len(set) != 2 {
		t.Fatalf("want 2 weapon rows, got %+v", set)
	}
	if ws[0].Item.ID != owned.ID {
		t.Fatalf("owned weapon must anchor the main hand, got %v", ws[0].Item.Name)
	}
	if set[0].ID != better.ID && set[0].ID != owned.ID {
		t.Fatalf("unexpected main-hand pick %v", set[0].Name)
	}
	// two copies of the better axe beat axe+sword: both rows may pick it
	if set[0].ID == set[1].ID && set[0].Unique {
		t.Fatal("unique weapon duplicated")
	}
}

func TestUpgradesRespectHandRoles(t *testing.T) {
	d := testDatabase()
	mh := weapon(320, db.WeaponSword, db.HandOneHand, 50)
	oh := weapon(321, db.WeaponSword, db.HandOneHand, 40)
	twoH := weapon(322, db.WeaponStaff, db.HandTwoHand, 500)
	mhOnly := weapon(323, db.WeaponDagger, db.HandMainHand, 60)
	ohOnly := weapon(324, db.WeaponFist, db.HandOffHand, 60)
	for _, it := range []*db.Item{mh, oh, twoH, mhOnly, ohOnly} {
		d.Items[it.ID] = it
	}
	s := New(d, dwProfile())

	// dual-wielding: neither hand may be offered the two-hander (the
	// other hand stays equipped), and the main-hand-only dagger must not
	// appear in the off hand — nor the off-hand-only fist in the main
	ws := wrapWorn([]*db.Item{mh, oh})
	for _, u := range s.UpgradesWorn(ws, 0, 50, nil) {
		if u.Item.ID == twoH.ID || u.Item.ID == ohOnly.ID {
			t.Fatalf("illegal main-hand proposal: %v", u.Item.Name)
		}
	}
	for _, u := range s.UpgradesWorn(ws, 1, 50, nil) {
		if u.Item.ID == twoH.ID || u.Item.ID == mhOnly.ID {
			t.Fatalf("illegal off-hand proposal: %v", u.Item.Name)
		}
	}

	// wearing the two-hander: one-handers never beat it one-for-one —
	// they are not offered at all
	ws = wrapWorn([]*db.Item{twoH})
	for _, u := range s.UpgradesWorn(ws, 0, 50, nil) {
		if u.Item.HandType != db.HandTwoHand {
			t.Fatalf("one-hander offered against a worn two-hander: %v", u.Item.Name)
		}
	}
}

func TestShieldRowOffersShieldsNotWeapons(t *testing.T) {
	d := testDatabase()
	mace := weapon(330, db.WeaponMace, db.HandOneHand, 40)
	shield := weapon(331, db.WeaponShield, db.HandOffHand, 30)
	bigShield := weapon(332, db.WeaponShield, db.HandOffHand, 80)
	frill := weapon(333, db.WeaponOffHand, db.HandOffHand, 50)
	sword := weapon(334, db.WeaponSword, db.HandOneHand, 90)
	for _, it := range []*db.Item{mace, shield, bigShield, frill, sword} {
		d.Items[it.ID] = it
	}
	prof := testProfile()
	// shields/frills are deliberately absent from the weapon-type list:
	// the off-hand role must admit them anyway
	prof.WeaponTypes = []db.WeaponType{db.WeaponMace, db.WeaponSword}
	prof.Layouts = []WeaponLayout{LayoutWeaponShield}
	s := New(d, prof)

	ws := wrapWorn([]*db.Item{mace, shield})
	var ids []int
	for _, u := range s.UpgradesWorn(ws, 1, 50, nil) {
		ids = append(ids, u.Item.ID)
	}
	if !contains(ids, bigShield.ID) || !contains(ids, frill.ID) {
		t.Fatalf("shield row must offer the better shield and the frill, got %v", ids)
	}
	if contains(ids, sword.ID) {
		t.Fatalf("shield row offered a weapon: %v", ids)
	}

	// and the solver fills the pair correctly
	_, set, _ := s.BestSet(ws, nil)
	if set[0].ID != sword.ID || set[1].ID != bigShield.ID {
		t.Fatalf("want sword + big shield, got %v + %v", set[0].Name, set[1].Name)
	}
}

func TestBestSetFillsMissingSlots(t *testing.T) {
	d := testDatabase()
	head := item(350, db.SlotHead, db.ArmorMail, db.Agility, 20)
	ring := item(351, db.SlotFinger, db.ArmorAny, db.Agility, 30)
	belt := item(352, db.SlotWaist, db.ArmorMail, db.Agility, 40)
	for _, it := range []*db.Item{head, ring, belt} {
		d.Items[it.ID] = it
	}
	s := New(d, testProfile())

	// wearing only a head: the naked waist and BOTH rings must be
	// synthesized and solved; slots with no candidates (neck, chest, …)
	// must not appear at all
	ws, set, _ := s.BestSet(wrapWorn([]*db.Item{head}), nil)
	if len(set) != 4 {
		t.Fatalf("want head+waist+2×finger, got %d rows: %v", len(set), names(set))
	}
	bySlot := map[db.Slot][]*db.Item{}
	for _, it := range set {
		bySlot[it.Slot] = append(bySlot[it.Slot], it)
	}
	if len(bySlot[db.SlotWaist]) != 1 || bySlot[db.SlotWaist][0].ID != belt.ID {
		t.Fatalf("missing waist not filled: %v", names(set))
	}
	if len(bySlot[db.SlotFinger]) != 2 {
		t.Fatalf("want both ring slots synthesized: %v", names(set))
	}
	// synthesized rows carry an empty baseline: everything but the worn
	// head is a piece to acquire
	owned := 0
	for i := range set {
		if ws[i].Item.ID == set[i].ID {
			owned++
		}
	}
	if owned != 1 {
		t.Fatalf("only the worn head is owned, got %d", owned)
	}
	// rows keep the character-pane order: head, waist, fingers
	wantOrder := []db.Slot{db.SlotHead, db.SlotWaist, db.SlotFinger, db.SlotFinger}
	for i, sl := range wantOrder {
		if set[i].Slot != sl {
			t.Fatalf("row %d = %v, want %v (%v)", i, set[i].Slot, sl, names(set))
		}
	}
}

func names(set []*db.Item) []string {
	var ns []string
	for _, it := range set {
		ns = append(ns, it.Slot.String()+":"+it.Name)
	}
	return ns
}

func TestTitanGripLayout(t *testing.T) {
	d := testDatabase()
	oneH := weapon(340, db.WeaponSword, db.HandOneHand, 50)
	axe2H := weapon(341, db.WeaponAxe, db.HandTwoHand, 90)
	polearm := weapon(342, db.WeaponPolearm, db.HandTwoHand, 300) // TG cannot hold it
	for _, it := range []*db.Item{oneH, axe2H, polearm} {
		d.Items[it.ID] = it
	}
	prof := testProfile()
	prof.WeaponTypes = []db.WeaponType{db.WeaponAxe, db.WeaponMace,
		db.WeaponSword, db.WeaponPolearm}
	prof.Layouts = []WeaponLayout{LayoutTitanGrip, LayoutDualWield}
	s := New(d, prof)

	_, set, _ := s.BestSet(wrapWorn([]*db.Item{oneH, oneH}), nil)
	if len(set) != 2 || set[0].ID != axe2H.ID || set[1].ID != axe2H.ID {
		t.Fatalf("Titan's Grip must dual-wield the 2H axes, got %v + %v",
			set[0].Name, set[1].Name)
	}

	// two copies of a non-unique 2H are legal (dual Edge of Ruin); a
	// unique-equipped one must be picked ONCE, the runner-up fills the
	// other hand (the Titansteel Destroyer trap)
	star := weapon(343, db.WeaponMace, db.HandTwoHand, 120)
	star.Unique = true
	d.Items[star.ID] = star
	s = New(d, prof) // fresh memoization
	_, set, _ = s.BestSet(wrapWorn([]*db.Item{oneH, oneH}), nil)
	got := []int{set[0].ID, set[1].ID}
	if !(got[0] == star.ID && got[1] == axe2H.ID ||
		got[0] == axe2H.ID && got[1] == star.ID) {
		t.Fatalf("unique 2H must appear once + runner-up, got %v", got)
	}
}
