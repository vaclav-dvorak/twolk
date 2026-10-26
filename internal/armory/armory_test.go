package armory

import (
	"os"
	"testing"

	"github.com/vaclav-dvorak/twolk/internal/db"
)

func TestParseRealExport(t *testing.T) {
	raw, err := os.ReadFile("testdata/character.json")
	if err != nil {
		t.Fatal(err)
	}
	c, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}

	if c.Name != "Testchar" || c.Realm != "Testrealm" {
		t.Errorf("identity = %s @ %s", c.Name, c.Realm)
	}
	if c.Level != 80 {
		t.Errorf("level = %d, want 80 (string-encoded in export)", c.Level)
	}
	if c.Class != db.ClassHunter {
		t.Errorf("class = %v, want Hunter", c.Class)
	}
	if len(c.Equipment) != 17 {
		t.Errorf("equipment count = %d, want 17", len(c.Equipment))
	}
	if c.Equipment[0].ItemID != 46143 || c.Equipment[0].Name != "Conqueror's Scourgestalker Headpiece" {
		t.Errorf("first item = %+v", c.Equipment[0])
	}
	if c.Spec() != "Survival" {
		t.Errorf("spec = %q, want Survival", c.Spec())
	}
	if len(c.Professions) != 2 || c.Professions[0].Skill != 442 {
		t.Errorf("professions = %+v", c.Professions)
	}
}

func TestParseRejectsGarbage(t *testing.T) {
	if _, err := Parse([]byte(`{"foo": 1}`)); err == nil {
		t.Error("expected error for non-armory JSON")
	}
}

func TestParseEnrichedExport(t *testing.T) {
	raw := []byte(`{"name":"X","realm":"R","level":"80","class":"Hunter",
		"origin":"https://armory.example.com",
		"equipment":[
			{"name":"A","item":"46143","ench":3817,"gems":[41398,40112]},
			{"name":"B","item":"45820"}
		]}`)
	c, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if c.Origin != "https://armory.example.com" {
		t.Fatalf("origin not parsed: %q", c.Origin)
	}
	a := c.Equipment[0]
	if !a.HasDetail || a.EnchantID != 3817 || len(a.Gems) != 2 || a.Gems[0] != 41398 {
		t.Fatalf("enriched item wrong: %+v", a)
	}
	b := c.Equipment[1]
	if b.HasDetail || b.EnchantID != 0 || b.Gems != nil {
		t.Fatalf("plain item must stay detail-less: %+v", b)
	}
}

func TestParseReforge(t *testing.T) {
	raw := []byte(`{"name":"X","realm":"R","level":85,"class":"Hunter",
		"equipment":[
			{"item":60303,"gems":[0],"reforge":{"source":49,"target":31,"value":67}},
			{"item":62378,"gems":[0]},
			{"item":56539,"gems":[0],"reforge":{"source":999,"target":31,"value":50}}
		]}`)
	c, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	r := c.Equipment[0].Reforge
	if r == nil || r.From != db.Mastery || r.To != db.MeleeHit || r.Amount != 67 {
		t.Fatalf("reforge = %+v, want 67 Mastery→Melee Hit (ITEM_MOD 49→31)", r)
	}
	if c.Equipment[1].Reforge != nil {
		t.Fatalf("unreforged item grew a reforge: %+v", c.Equipment[1].Reforge)
	}
	// unknown ITEM_MOD ids: drop the reforge, keep the item
	if c.Equipment[2].Reforge != nil {
		t.Fatalf("unknown stat id must drop the reforge: %+v", c.Equipment[2].Reforge)
	}
}
