package iconindex

import (
	"encoding/json"
	"testing"
)

func TestParseSets(t *testing.T) {
	raw := json.RawMessage(`{
		"lucide": {"name":"Lucide","total":1768,"category":"General","palette":false,"height":24,
			"author":{"name":"Lucide Contributors"},
			"license":{"title":"ISC","spdx":"ISC","url":"https://example.test/isc"}},
		"fluent-emoji-flat": {"name":"Fluent Emoji Flat","total":100,"palette":true,"height":32}
	}`)
	sets, err := ParseSets(raw)
	if err != nil {
		t.Fatalf("ParseSets: %v", err)
	}
	if len(sets) != 2 {
		t.Fatalf("want 2 sets, got %d", len(sets))
	}
	// Sorted by prefix, so fluent-emoji-flat comes first.
	if sets[0].Prefix != "fluent-emoji-flat" || !sets[0].Palette || sets[0].Height != 32 {
		t.Errorf("unexpected first set: %+v", sets[0])
	}
	if sets[1].Prefix != "lucide" || sets[1].SPDX != "ISC" || sets[1].Author != "Lucide Contributors" {
		t.Errorf("unexpected lucide row: %+v", sets[1])
	}
}

func TestCoerceHeight(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want int
	}{
		{"number", float64(24), 24},
		{"string", "32", 32},
		{"array takes first", []any{float64(16), float64(24)}, 16},
		{"nil", nil, 0},
		{"unparseable string", "tall", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := coerceHeight(tc.in); got != tc.want {
				t.Errorf("coerceHeight(%v) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

func TestParseCollection(t *testing.T) {
	raw := json.RawMessage(`{
		"prefix":"lucide","total":3,
		"uncategorized":["rocket"],
		"categories":{"Navigation":["house","compass"]},
		"hidden":["retired-thing"],
		"aliases":{"home":"house"}
	}`)
	rows, err := ParseCollection("lucide", raw)
	if err != nil {
		t.Fatalf("ParseCollection: %v", err)
	}
	byName := map[string]IconRow{}
	for _, r := range rows {
		byName[r.Name] = r
	}
	if got := byName["house"]; got.Category != "Navigation" || got.Hidden {
		t.Errorf("house: %+v", got)
	}
	if got := byName["rocket"]; got.Category != "" || got.Hidden {
		t.Errorf("rocket should be uncategorized and visible: %+v", got)
	}
	// Hidden icons are retained on purpose: audit needs to report a name that
	// was retired upstream rather than silently dropping it.
	if got, ok := byName["retired-thing"]; !ok || !got.Hidden {
		t.Errorf("retired-thing should be present and hidden: %+v", got)
	}
	if got := byName["home"]; got.AliasOf != "house" {
		t.Errorf("home should alias house, got %+v", got)
	}
	if got := byName["house"].FullName(); got != "lucide:house" {
		t.Errorf("FullName() = %q", got)
	}
}

func TestParseLastModified(t *testing.T) {
	got, err := ParseLastModified(json.RawMessage(`{"lastModified":{"mdi":1665726087,"lucide":1667373464}}`))
	if err != nil {
		t.Fatalf("ParseLastModified: %v", err)
	}
	if got["mdi"] != 1665726087 || got["lucide"] != 1667373464 {
		t.Errorf("unexpected map: %+v", got)
	}
	empty, err := ParseLastModified(json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("empty payload: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("want empty map, got %+v", empty)
	}
}

func TestFTSQuery(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"arrow right", "arrow* AND right*"},
		{"Arrow-Right", "arrow* AND right*"},
		{"rocket", "rocket*"},
		{`"quoted" OR junk`, "quoted* AND or* AND junk*"},
		{"   ", ""},
		{"", ""},
	}
	for _, tc := range cases {
		if got := FTSQuery(tc.in); got != tc.want {
			t.Errorf("FTSQuery(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestDiffNames(t *testing.T) {
	added, removed := DiffNames([]string{"a", "b", "c"}, []string{"b", "c", "d"})
	if len(added) != 1 || added[0] != "d" {
		t.Errorf("added = %v, want [d]", added)
	}
	if len(removed) != 1 || removed[0] != "a" {
		t.Errorf("removed = %v, want [a]", removed)
	}
	added, removed = DiffNames([]string{"a"}, []string{"a"})
	if len(added) != 0 || len(removed) != 0 {
		t.Errorf("identical lists should diff empty, got +%v -%v", added, removed)
	}
	// Empty inputs must produce empty slices, not nil, so JSON renders [] not null.
	added, removed = DiffNames(nil, nil)
	if added == nil || removed == nil {
		t.Errorf("DiffNames(nil,nil) must return empty slices, got %v %v", added, removed)
	}
}
