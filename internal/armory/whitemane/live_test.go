package whitemane

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

// TestLive is an opt-in smoke test against the real API — the drift
// alarm for an undocumented upstream. Run with
//
//	TWOLK_WHITEMANE_LIVE="realm/name" go test -run Live ./internal/armory/whitemane/
//
// Skipped otherwise; never part of the normal suite.
func TestLive(t *testing.T) {
	target := os.Getenv("TWOLK_WHITEMANE_LIVE")
	if target == "" {
		t.Skip("set TWOLK_WHITEMANE_LIVE=realm/name to run")
	}
	realm, name, ok := strings.Cut(target, "/")
	if !ok {
		t.Fatalf("TWOLK_WHITEMANE_LIVE = %q, want realm/name", target)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	ch, err := New().Character(ctx, realm, name)
	if err != nil {
		t.Fatal(err)
	}

	if ch.GUID == 0 || ch.Level == 0 || ch.Class == 0 {
		t.Errorf("profile looks empty: %+v", ch.Profile)
	}
	if len(ch.Items) == 0 {
		t.Error("no equipment")
	}
	if len(ch.Specializations) == 0 {
		t.Error("no specializations")
	}
	if len(ch.Professions) == 0 {
		t.Error("no professions")
	}
	t.Logf("%s@%s: guid=%d level=%d class=%d ilvl=%d items=%d specs=%d glyph-sets=%d professions=%d",
		ch.Name, ch.Realm, ch.GUID, ch.Level, ch.Class, ch.AverageItemLevel,
		len(ch.Items), len(ch.Specializations), len(ch.Glyphs), len(ch.Professions))
	reforged := 0
	for _, it := range ch.Items {
		if it.Reforge != nil {
			reforged++
		}
	}
	t.Logf("reforged items: %d/%d", reforged, len(ch.Items))
}
