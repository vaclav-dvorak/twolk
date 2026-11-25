// Package forever reads World of Warcraft: Forever characters from
// Blizzard's official armory. There is no public JSON endpoint for
// Forever characters; the armory page itself embeds the full character
// document (gear, talents with ranks, professions) as the JavaScript
// variable characterProfileInitialState, so one page fetch per refresh
// is the whole import.
//
// Forever characters are realmless: a region and the character's two
// names ("Brannoc Ironvale") address one, as the slug "brannoc-ironvale".
package forever

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// DefaultBaseURL is the official armory host.
const DefaultBaseURL = "https://worldofwarcraft.blizzard.com"

// userAgent identifies twolk politely.
const userAgent = "twolk gear planner (+https://github.com/vaclav-dvorak/twolk)"

// stateMarker introduces the embedded character document.
const stateMarker = "characterProfileInitialState"

// ErrNotFound reports a character the armory doesn't know. The armory
// answers an unknown slug with a 500, not a 404.
var ErrNotFound = errors.New("character not found on the armory")

// Client reads the official armory.
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

// New returns a client for the live armory.
func New() *Client {
	return &Client{
		BaseURL: DefaultBaseURL,
		HTTP:    &http.Client{Timeout: 15 * time.Second},
	}
}

// Slug turns a typed character name ("Brannoc Ironvale") into the
// armory's URL slug ("brannoc-ironvale").
func Slug(name string) string {
	return strings.ToLower(strings.Join(strings.Fields(name), "-"))
}

// PageURL is the armory page of a character in a region ("eu", "us").
func PageURL(region, name string) string {
	return fmt.Sprintf("%s/en-us/forever/%s/armory/character/%s",
		DefaultBaseURL, strings.ToLower(region), url.PathEscape(Slug(name)))
}

// Named is the {id, name} pair the armory uses for races, classes and
// professions.
type Named struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Character is the part of the embedded document the planner reads.
type Character struct {
	Name    string          `json:"name"`
	Surname string          `json:"surname"`
	Slug    string          `json:"slug"`
	Region  string          `json:"region"`
	Level   int             `json:"level"`
	Race    Named           `json:"race"`
	Class   Named           `json:"class"`
	Faction Named           `json:"faction"`
	Gear    map[string]Item `json:"gear"`
	Specs   []Spec          `json:"specs"`
	// Professions: primaries and secondaries, each with skill tiers
	// (Forever has one tier per profession).
	Professions struct {
		Primaries []Profession `json:"primaries"`
	} `json:"professions"`
	LastUpdated struct {
		ISO8601 string `json:"iso8601"`
	} `json:"lastUpdatedTimestamp"`
}

// FullName is the character's display name, both parts.
func (c *Character) FullName() string {
	return strings.TrimSpace(c.Name + " " + c.Surname)
}

// Item is one worn item.
type Item struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Slot struct {
		Type string `json:"type"` // HEAD, FINGER_1, MAIN_HAND, ...
	} `json:"slot"`
	Enchantments []struct {
		EnchantmentID int `json:"enchantment_id"`
		Slot          struct {
			Type string `json:"type"` // PERMANENT, TEMPORARY
		} `json:"enchantment_slot"`
	} `json:"enchantments"`
}

// PermanentEnchant is the item's permanent enchant effect id, 0 for
// none (temporary ones — oils, sharpening stones — don't count).
func (it Item) PermanentEnchant() int {
	for _, e := range it.Enchantments {
		if e.Slot.Type == "PERMANENT" {
			return e.EnchantmentID
		}
	}
	return 0
}

// Spec is one talent tree with points in it.
type Spec struct {
	ID          string   `json:"id"` // "beast-mastery"
	Name        string   `json:"name"`
	Active      bool     `json:"active"`
	SpentPoints int      `json:"spentPoints"`
	Talents     []Talent `json:"talents"`
}

// Talent is one taken talent; ID is the talent's spell id.
type Talent struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Rank int    `json:"rank"`
}

// Profession is one primary profession with its skill.
type Profession struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Tiers []struct {
		SkillPoints int `json:"skillPoints"`
	} `json:"tiers"`
}

// Skill is the profession's current skill (its last tier).
func (p Profession) Skill() int {
	if len(p.Tiers) == 0 {
		return 0
	}
	return p.Tiers[len(p.Tiers)-1].SkillPoints
}

// Character fetches one character's armory page and decodes the
// embedded document.
func (c *Client) Character(ctx context.Context, region, name string) (*Character, error) {
	if Slug(name) == "" {
		return nil, errors.New("empty character name")
	}
	u := strings.Replace(PageURL(region, name), DefaultBaseURL, c.BaseURL, 1)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound, http.StatusInternalServerError:
		return nil, fmt.Errorf("%s (%s): %w", name, strings.ToUpper(region), ErrNotFound)
	default:
		return nil, fmt.Errorf("GET %s: %s", u, resp.Status)
	}
	return ParsePage(body)
}

// ParsePage extracts the character document from an armory page.
func ParsePage(page []byte) (*Character, error) {
	s := string(page)
	i := strings.Index(s, stateMarker)
	if i < 0 {
		return nil, errors.New("armory page carries no character data")
	}
	s = s[i+len(stateMarker):]
	j := strings.IndexByte(s, '{')
	if j < 0 || strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s[:j]), "=")) != "" {
		return nil, errors.New("armory page: malformed character data")
	}
	var doc struct {
		Character *Character `json:"character"`
	}
	// Decode reads exactly one JSON value; the script text after it is
	// never looked at
	if err := json.NewDecoder(strings.NewReader(s[j:])).Decode(&doc); err != nil {
		return nil, fmt.Errorf("armory page: %w", err)
	}
	if doc.Character == nil || doc.Character.Name == "" {
		return nil, errors.New("armory page carries no character data")
	}
	return doc.Character, nil
}
