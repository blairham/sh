// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package oracle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestConformanceListsHaveAReason: every case a dialect's conformance list
// leaves short of exact has an entry in docs/spec/by-design.md that names it
// and that dialect, so a list cannot grow a case nobody has decided about.
// The README promises a written reason for each; this is what keeps that true.
func TestConformanceListsHaveAReason(t *testing.T) {
	doc, err := os.ReadFile(filepath.Join("..", "..", "docs", "spec", "by-design.md"))
	if err != nil {
		t.Fatal(err)
	}
	// An entry is "- `<case>` [<tag>]…", where the tag is `ash` or any run of
	// b, d, k and z for the other four (`[bzdk]` is one reason for four lists).
	reasons := map[string]map[string]bool{}
	letters := map[rune]string{'b': "bash", 'd': "dash", 'k': "ksh", 'z': "zsh"}
	for _, line := range strings.Split(string(doc), "\n") {
		rest, ok := strings.CutPrefix(line, "- `")
		if !ok {
			continue
		}
		id, rest, ok := strings.Cut(rest, "` [")
		if !ok {
			continue
		}
		tag, _, _ := strings.Cut(rest, "]")
		tag, _, _ = strings.Cut(tag, " ")
		if reasons[id] == nil {
			reasons[id] = map[string]bool{}
		}
		if tag == "ash" {
			reasons[id]["ash"] = true
			continue
		}
		for _, r := range tag {
			reasons[id][letters[r]] = true
		}
	}
	lists, err := filepath.Glob(filepath.Join("testdata", "conformance", "*.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if len(lists) != 5 {
		t.Fatalf("found %d conformance lists, want one per dialect", len(lists))
	}
	for _, list := range lists {
		dialect := strings.TrimSuffix(filepath.Base(list), ".txt")
		ids, err := LoadBaseline(list)
		if err != nil {
			t.Fatal(err)
		}
		if len(ids) == 0 {
			t.Errorf("%s: no cases read", list)
		}
		for _, id := range ids {
			if !reasons[id][dialect] {
				t.Errorf("%s lists %s with no %s entry in docs/spec/by-design.md", list, id, dialect)
			}
		}
	}
}
