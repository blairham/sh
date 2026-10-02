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
		r := &interp.Runner{}
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
