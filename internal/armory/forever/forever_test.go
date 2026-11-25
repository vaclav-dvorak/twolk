package forever

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// page mirrors the shape of a live armory page (2026-11-25, level-32
// reference character, anonymized and trimmed): the document is a
// script variable followed by more script text.
const page = `<html><script>var model = {};
var characterProfileInitialState = {"character":{"name":"Brannoc","surname":"Ironvale","slug":"brannoc-ironvale",
"region":"eu","level":32,"race":{"id":3,"name":"Dwarf"},"class":{"id":3,"name":"Hunter"},
"faction":{"id":0,"name":"Alliance"},
"gear":{"hand":{"id":4253,"name":"Toughened Leather Gloves","slot":{"type":"HANDS","name":"Hands"},
"enchantments":[{"enchantment_id":8484,"enchantment_slot":{"id":0,"type":"PERMANENT"}}]},
"weapon":{"id":281296,"name":"Knight's Lance","slot":{"type":"MAIN_HAND","name":"Main Hand"},
"enchantments":[{"enchantment_id":1,"enchantment_slot":{"id":1,"type":"TEMPORARY"}}]}},
"specs":[{"active":true,"id":"beast-mastery","name":"Beast Mastery","spentPoints":23,
"talents":[{"id":134422,"name":"Deadly Aspects","rank":5}]}],
"professions":{"primaries":[{"id":165,"name":"Leatherworking","tiers":[{"id":2945,"skillPoints":152}]}]}},
"summary":{}};
window.other = {"character":{"name":"wrong"}};</script></html>`

func TestParsePage(t *testing.T) {
	c, err := ParsePage([]byte(page))
	if err != nil {
		t.Fatal(err)
	}
	if c.FullName() != "Brannoc Ironvale" || c.Level != 32 || c.Class.Name != "Hunter" {
		t.Errorf("character = %q level %d class %q", c.FullName(), c.Level, c.Class.Name)
	}
	if got := c.Gear["hand"].PermanentEnchant(); got != 8484 {
		t.Errorf("gloves enchant = %d, want 8484", got)
	}
	// an oil is temporary, not the weapon's enchant
	if got := c.Gear["weapon"].PermanentEnchant(); got != 0 {
		t.Errorf("weapon enchant = %d, want 0", got)
	}
	if len(c.Professions.Primaries) != 1 || c.Professions.Primaries[0].Skill() != 152 {
		t.Errorf("professions = %+v", c.Professions.Primaries)
	}
}

func TestParsePageWithoutCharacter(t *testing.T) {
	if _, err := ParsePage([]byte("<html>no data</html>")); err == nil {
		t.Error("want an error for a page without character data")
	}
}

func TestSlug(t *testing.T) {
	for in, want := range map[string]string{
		"Brannoc Ironvale":    "brannoc-ironvale",
		"  brannoc  ironvale": "brannoc-ironvale",
		"Brannoc":             "brannoc",
	} {
		if got := Slug(in); got != want {
			t.Errorf("Slug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCharacterNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/en-us/forever/eu/armory/character/nobody-here" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusInternalServerError) // the armory's answer to unknown slugs
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	if _, err := c.Character(context.Background(), "EU", "Nobody Here"); !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}
