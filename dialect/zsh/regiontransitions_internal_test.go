// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/blairham/sh/interp"
)

// What `region_highlight` draws, byte for byte, against the 153 rows measured
// off zsh 5.9.2 on 2026-10-02 in testdata/regiontransitions.json: every pair
// of nine specs over 1–5 and 3–7, and sixty arrays of two and three regions
// drawn at random. Each row is a pseudo-terminal capture of the line
// `abcdefgh` drawn under the array, with the editor's own terminal operations
// blanked through `zle -T tc` so that what is left is the attributes.
func TestRegionHighlightDrawsTheMeasuredTransitions(t *testing.T) {
	data, err := os.ReadFile("testdata/regiontransitions.json")
	if err != nil {
		t.Fatal(err)
	}
	var rows []struct {
		Regions []string `json:"regions"`
		Drawn   string   `json:"drawn"`
	}
	if err := json.Unmarshal(data, &rows); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 153 {
		t.Fatalf("read %d rows, want 153", len(rows))
	}
	const line = "abcdefgh"
	for _, row := range rows {
		dialect := Dialect()
		r := &interp.Runner{Name: "zsh", Dialect: &dialect}
		r.SetArray(zleRegion, row.Regions)
		var b strings.Builder
		at := 0
		for _, h := range RegionHighlights(r, line) {
			if !h.Point {
				t.Fatalf("%q: a run where a point was expected: %+v", row.Regions, h)
			}
			b.WriteString(line[at:h.Start])
			b.WriteString(h.Style)
			at = h.Start
		}
		b.WriteString(line[at:])
		if got := b.String(); got != row.Drawn {
			t.Errorf("%q\n got %q\nwant %q", row.Regions, got, row.Drawn)
		}
	}
}

// `zle_highlight`'s code fields change how a color is written and how it is
// ended. Each row was measured on zsh 5.9.2 through a pseudo-terminal on
// 2026-10-02, the line `abcdefgh`; see regionCodes.
func TestZleHighlightCodesWriteTheColors(t *testing.T) {
	custom := []string{`fg_start_code:S|`, `fg_end_code:|E`, `bg_start_code:B|`, `bg_end_code:|F`}
	defaults := []string{`fg_default_code:D`, `bg_default_code:G`}
	for _, c := range []struct {
		codes   []string
		regions []string
		want    string
	}{
		{custom, []string{"2 4 fg=1"}, "abS|1|EcdS|9|Eefgh"},
		{custom, []string{"2 4 fg=196"}, "abS|196|EcdS|9|Eefgh"},
		{custom, []string{"2 4 bg=2"}, "abB|2|FcdB|9|Fefgh"},
		{custom, []string{"2 4 fg=#ff0000"}, "ab\x1b[38;2;255;0;0mcdS|9|Eefgh"},
		{custom, []string{"2 4 bold,fg=red"}, "ab\x1b[1mS|1|Ecd\x1b[0mS|9|Eefgh"},
		{custom, []string{"1 5 fg=1", "3 7 fg=2"}, "aS|1|EbcS|2|EdeS|9|ES|2|EfgS|9|Eh"},
		{defaults, []string{"2 4 fg=1"}, "ab\x1b[31mcd\x1b[3Dmefgh"},
		{defaults, []string{"2 4 bg=2"}, "ab\x1b[42mcd\x1b[4Gmefgh"},
		{nil, []string{"2 4 fg=196"}, "ab\x1b[38;5;196mcd\x1b[39mefgh"},
	} {
		dialect := Dialect()
		r := &interp.Runner{Name: "zsh", Dialect: &dialect}
		r.SetArray(zleRegion, c.regions)
		r.SetArray("zle_highlight", c.codes)
		const line = "abcdefgh"
		var b strings.Builder
		at := 0
		for _, h := range RegionHighlights(r, line) {
			b.WriteString(line[at:h.Start])
			b.WriteString(h.Style)
			at = h.Start
		}
		b.WriteString(line[at:])
		if got := b.String(); got != c.want {
			t.Errorf("%q under %q\n got %q\nwant %q", c.regions, c.codes, got, c.want)
		}
	}
}
