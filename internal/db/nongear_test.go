package db

import "testing"

// The generated table is only as good as its anchors: the item a
// character most often logs out holding, and the jousting lances of
// both tournaments. Regenerate with tools/gen_nongear.py.
func TestNonGearAnchors(t *testing.T) {
	want := map[int]string{
		6256:  "Fishing Pole",
		45992: "Jeweled Fishing Pole",
		46069: "Alliance Lance",
		46070: "Horde Lance",
		46106: "Argent Lance",
		52716: "Twilight Firelance",
		2901:  "Mining Pick",
		46349: "Chef's Hat",         // head
		22279: "Lovely Black Dress", // chest
		33864: "Brown Brewfest Hat", // head
	}
	for id, name := range want {
		if got := NonGear[id]; got != name {
			t.Errorf("NonGear[%d] = %q, want %q", id, got, name)
		}
	}
	// a table that grew to swallow real gear would disable scoring for
	// everyone: these are tools and costumes, not a gear list
	if len(NonGear) > 400 {
		t.Errorf("NonGear holds %d items — too many to be tools and costumes", len(NonGear))
	}
	for _, real := range []int{65168, 58181, 41167, 6224} {
		if name, ok := NonGear[real]; ok {
			t.Errorf("real gear %d (%s) landed in the non-gear table", real, name)
		}
	}
}
