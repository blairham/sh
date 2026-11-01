// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"testing"

	"github.com/blairham/sh/internal/parsecache"
)

// TestEveryAliasChangeMovesTheTablesFingerprint is what lets the parse cache
// key a stored read on one number per table: every way an alias table can
// change moves that table's fingerprint — and only that table's — and a
// table put back as it was comes back to the number it had. The number is
// kept between changes, so a change that did not count itself would leave
// the old number standing; each row is that mutant's death.
func TestEveryAliasChangeMovesTheTablesFingerprint(t *testing.T) {
	t.Parallel()
	r := newTestRunner(t, &Runner{})
	r.aliasExpansion = true
	ask := parseAsker{r: r}
	sums := func() [3]uint64 {
		return [3]uint64{
			ask.AliasTable(parsecache.AliasPlain),
			ask.AliasTable(parsecache.AliasGlobal),
			ask.AliasTable(parsecache.AliasSuffix),
		}
	}
	empty := sums()
	steps := []struct {
		name  string
		do    func()
		moves []parsecache.AliasKind
	}{
		{
			"a plain alias",
			func() { r.DefineAlias("ll", "ls -l", AliasAnyKind) },
			[]parsecache.AliasKind{parsecache.AliasPlain},
		},
		{
			"a redefinition",
			func() { r.DefineAlias("ll", "ls -la", AliasAnyKind) },
			[]parsecache.AliasKind{parsecache.AliasPlain},
		},
		{
			"a global alias",
			func() { r.DefineAlias("G", "| grep", AliasGlobalKind) },
			[]parsecache.AliasKind{parsecache.AliasPlain, parsecache.AliasGlobal},
		},
		{
			"a suffix alias",
			func() { r.DefineAlias("txt", "cat", AliasSuffixKind) },
			[]parsecache.AliasKind{parsecache.AliasSuffix},
		},
		{"a removal", func() { r.RemoveAlias("ll") }, []parsecache.AliasKind{parsecache.AliasPlain}},
		// The plain table moves too: its fingerprint holds each entry's kind,
		// which answers nothing differently there and is cheaper kept than
		// argued about.
		{
			"a global made plain",
			func() { r.DefineAlias("G", "| grep", AliasAnyKind) },
			[]parsecache.AliasKind{parsecache.AliasPlain, parsecache.AliasGlobal},
		},
		// The switch is one fact about every table, whatever it holds.
		{
			"expansion off",
			func() { r.SetAliasExpansion(false) },
			[]parsecache.AliasKind{parsecache.AliasPlain, parsecache.AliasGlobal, parsecache.AliasSuffix},
		},
		{
			"expansion on",
			func() { r.SetAliasExpansion(true) },
			[]parsecache.AliasKind{parsecache.AliasPlain, parsecache.AliasGlobal, parsecache.AliasSuffix},
		},
	}
	for _, s := range steps {
		before := sums()
		s.do()
		after := sums()
		for k := range 3 {
			moved := before[k] != after[k]
			want := false
			for _, m := range s.moves {
				want = want || int(m) == k
			}
			if moved != want {
				t.Errorf("%s: table %d moved %v, want %v", s.name, k, moved, want)
			}
		}
	}
	r.RemoveAlias("G")
	r.removeAlias("txt", AliasSuffixKind)
	if got := sums(); got != empty {
		t.Errorf("the tables emptied again are %v, want the empty tables' %v", got, empty)
	}
}
