// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// **`(k)` and `(K)` read a table's keys as the patterns** and match the
// subscript against them (#5278). Measured against zsh 5.9.2: the key `a*`
// answers `abc`, `(K)` gives every match and `(k)` one, the subscript is read
// the way a key is — inside double quotes `\"` is a quote, and `\*` keeps its
// backslash —
// `(e)` and `(n:2:)` change nothing, `extendedglob` reaches the keys, a key
// that is not a well-formed pattern matches nothing and says nothing — not
// even the text it is spelled with — and a table with no pattern characters
// in its keys answers as the exact lookup it always looked like.
func TestKeyFlagsReadTheKeysAsPatterns(t *testing.T) {
	out, st, errs := runZshSplit(t, t.TempDir(), `typeset -A h; h=('a*' star 'b?' q x plain '\*' bs '[' br 'a(b' grp)
print -r -- "[${h[(k)abc]}] [${h[(K)b1]}] [${h[(k)a\*]}] [${h[(k)x]}] [${#h[(K)b1]}]"
print -r -- "[${h[(k)*]}] [${h[(k)\*]}] [${h[(ke)abc]}] [${h[(kn:2:)abc]}] [${(k)h[(k)b7]}]"
s1='['; s2='(b'; print -r -- "[${h[(k)$s1]}] [${h[(k)$s2]}]"
typeset -A m; m=(aa 1 bb 2 'a?' 3 'x"' dq 'y\' bsl)
print -r -- "[${#m[(K)aa]}] [${#m[(k)aa]}] [${m[(k)ab]}] [${m[(k)zz]}] [${(kv)m[(k)bb]}] [${m[(k)x\"]}] [${m[(k)y\\]}]"
print -r -- :${m[(k)x\"]}: :${m[(k)y\\]}:
setopt extendedglob; typeset -A g; g=('(#i)AB' ci)
print -r -- "[${g[(k)ab]}]"`)
	want := "[star] [q] [star] [plain] [1]\n[bs] [] [star] [star] [b?]\n[] []\n[2] [1] [3] [] [bb 2] [dq] [bsl]\n:: :bsl:\n[ci]\n"
	if out != want || st != 0 || errs != "" {
		t.Errorf("got %q (status %d, stderr %q), want %q", out, st, errs, want)
	}
}
