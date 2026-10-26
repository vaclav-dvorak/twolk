package whitemane

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// The fixtures mirror verbatim live responses recorded in whitemane.md
// (level-85 reference character, 2026-07-24, anonymized) — if the API
// drifts, update both. They deliberately cover the shapes only a
// max-level character exhibits: a populated reforge next to a null one,
// glyphs per spec, and the bare-array professions endpoint.
const (
	profileJSON = `{"guid":84512,"name":"Zulja","race":2,"class":3,"gender":0,"level":85,
		"playedTime":423062,"playedTimeLevel":338108,"faction":"Horde","chosenTitle":0,
		"achievementPoints":1605,"averageItemLevel":317}`
	equipmentJSON = `{"items":[
		{"slot":0,"itemId":60303,"transmogId":0,"enchantId":4209,"sockets":[68778,52209,0],
			"reforge":{"source":49,"target":31,"value":67}},
		{"slot":15,"itemId":19970,"transmogId":0,"enchantId":846,"sockets":[0,0,0],"reforge":null}]}`
	talentsJSON = `{"specializations":[{"index":0,"specializationId":809},{"index":1,"specializationId":0}],
		"talents":{"0":[
			{"talentId":9432,"spellId":52786,"rank":2},
			{"talentId":9450,"spellId":82834,"rank":1},
			{"talentId":9472,"spellId":53299,"rank":0}]},
		"glyphs":{"0":[
			{"glyphId":353,"spellId":56857},
			{"glyphId":357,"spellId":56850}]}}`
	professionsJSON = `[{"id":129,"skill":525},{"id":171,"skill":525},{"id":202,"skill":525}]`
)

func testServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	serve := func(path, body string) {
		mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			if ua := r.Header.Get("User-Agent"); ua != userAgent {
				t.Errorf("User-Agent = %q", ua)
			}
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.Write([]byte(body))
		})
	}
	// profile answers by NAME; sub-resources only by the resolved guid
	serve("/armory/gilneas/character/zulja", profileJSON)
	serve("/armory/gilneas/character/84512/equipment", equipmentJSON)
	serve("/armory/gilneas/character/84512/talents", talentsJSON)
	serve("/armory/gilneas/character/84512/professions", professionsJSON)
	return httptest.NewServer(mux)
}

func TestCharacterFetch(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()
	c := New()
	c.BaseURL = srv.URL

	// realm given in display capitalization — the client must slug it
	ch, err := c.Character(context.Background(), "Gilneas", "zulja")
	if err != nil {
		t.Fatal(err)
	}
	if ch.GUID != 84512 || ch.Name != "Zulja" || ch.Level != 85 || ch.Faction != "Horde" {
		t.Fatalf("profile wrong: %+v", ch.Profile)
	}
	if ch.Race != 2 || ch.Class != 3 {
		t.Fatalf("enums wrong: race=%d class=%d", ch.Race, ch.Class)
	}

	if len(ch.Items) != 2 || ch.Items[0].Slot != 0 || ch.Items[0].ItemID != 60303 {
		t.Fatalf("equipment wrong: %+v", ch.Items)
	}
	if r := ch.Items[0].Reforge; r == nil || r.Source != 49 || r.Target != 31 || r.Value != 67 {
		t.Fatalf("reforge wrong: %+v", ch.Items[0].Reforge)
	}
	if ch.Items[1].Reforge != nil {
		t.Fatalf("null reforge must stay nil: %+v", ch.Items[1].Reforge)
	}
	if len(ch.Items[0].Sockets) != 3 || ch.Items[0].Sockets[0] != 68778 {
		t.Fatalf("sockets wrong: %+v", ch.Items[0])
	}

	if len(ch.Specializations) != 2 || ch.Specializations[0].SpecializationID != 809 {
		t.Fatalf("specs wrong: %+v", ch.Specializations)
	}
	spec0 := ch.Talents["0"]
	if len(spec0) != 3 || spec0[0].Rank != 2 || spec0[2].Rank != 0 {
		t.Fatalf("talents wrong: %+v", spec0)
	}
	glyphs0 := ch.Glyphs["0"]
	if len(glyphs0) != 2 || glyphs0[0].GlyphID != 353 || glyphs0[1].SpellID != 56850 {
		t.Fatalf("glyphs wrong: %+v", glyphs0)
	}

	if len(ch.Professions) != 3 || ch.Professions[1].ID != 171 || ch.Professions[1].Skill != 525 {
		t.Fatalf("professions wrong: %+v", ch.Professions)
	}
}

func TestCharacterNotFound(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()
	c := New()
	c.BaseURL = srv.URL

	if _, err := c.Character(context.Background(), "gilneas", "nobody"); err == nil {
		t.Fatal("expected an error for an unknown character")
	}
}
