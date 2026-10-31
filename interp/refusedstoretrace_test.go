// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A store the freeze refuses under a table's key asks whether the key stays
// behind, by every writer: Yes leaves it, declared and holding nothing, and No
// leaves the table as it was. See Semantics.RefusedTableStoreKeepsTheKey
// (#6847).
func TestARefusedTableStoreAsksWhetherTheKeyStays(t *testing.T) {
	for _, tc := range []struct {
		name   string
		answer Answer
		want   string
	}{
		{"kept", Yes, "[1]"},
		{"left as it was", No, "[0]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, stmt := range []string{"let 'a[k]=5'", "read 'a[k]' <<< v", "a[k]=5"} {
				src := "typeset -A a\nreadonly a\n" + stmt + "\n" + `echo "[${#a[*]}]"` + "\n"
				out, _ := runGrammar(t, src, nil, func(r *Runner) {
					giveUpLineSemantics(r)
					// The statement's refusal waits for its store, which is
					// where its key is known.
					r.Semantics.FrozenElementIsRefusedBeforeItsSubscript = No
					r.Semantics.RefusedTableStoreKeepsTheKey = tc.answer
				})
				if !strings.HasSuffix(out, tc.want+"\n") {
					t.Errorf("%s: output %q, want it to end %s", stmt, out, tc.want)
				}
			}
		})
	}
}

// Both questions are put only once the freeze has refused, so a store nothing
// refuses asks neither and a vector that leaves them open still runs it.
func TestAStoreNothingRefusesAsksNothingAboutATrace(t *testing.T) {
	src := "typeset -A t\nt[k]=1\nlet 't[j]=2'\ntypeset -a a\n(( a[1]=3 ))\n" +
		`echo "${t[k]}${t[j]}${a[1]}"` + "\n"
	out, st := runGrammar(t, src, nil, func(r *Runner) {
		r.Semantics.RefusedTableStoreKeepsTheKey = Unspecified
		r.Semantics.ArithStoreRefusedByAFreezeSetsTheArray = Unspecified
	})
	if out != "123\n" || st != 0 {
		t.Errorf("output %q status %d, want the stores made and nothing asked", out, st)
	}
}
