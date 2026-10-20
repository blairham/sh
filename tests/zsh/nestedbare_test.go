// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// **The empty elements of a nested list are bare, and so are the empty fields
// an `=` split makes, until a group unquotes them** (#5299). Measured
// 2026-10-01 against zsh 5.9.2 with this script as a file, which writes these
// lines byte for byte: a nested array's empty element is left bare by the
// single `q` and quoted by `qq`, and is gone unquoted before any flag; a
// nested value — an element, a scalar under `(@)` — is a value; an empty
// value splits into itself, so unquoted `${(q)=e}` is a quoted empty and
// `${=e}` is nothing; `(Q)` makes a split's bare fields values, which then go the way a
// letter split's do; and a nested `=` split keeps its bare fields unquoted
// until a `(Q)` there makes them values too.
func TestNestedAndSplitEmptiesAreBare(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `IFS=:; u=a::b:; b=(x '' y); e=; d=:
show() { printf '%s:' $#; for x in "$@"; do printf '<%s>' "$x"; done; print; }
show "${(@q)${b[@]}}"
show "${(@q)${(@s.:.)u}}"
a=(${(@q)${b[@]}}); show "${a[@]}"
show ${(@Q)=u}
show ${(@q)=e}
show ${(q)=e}
show "${(@q)b}"
show "${(@q)${b[2]}}"
s=''; show "${(@q)${(@)s}}"
show "${(@q)=e}"
show "${(@qq)${b[@]}}"
show ${(@qq)${b[@]}}
show "${(Q)=u}"
show ${(@Q)=d}
show ${=e}
show ${(@)${=u}}
show ${(@)${=d}}
show ${(q)${(s.:.)u}}
show ${(Q)${=u}}`)
	if want := "3:<x><><y>\n4:<a><><b><>\n2:<x><y>\n2:<a><b>\n1:<''>\n1:<''>\n3:<x><''><y>\n1:<''>\n1:<''>\n1:<''>\n3:<'x'><''><'y'>\n2:<'x'><'y'>\n3:<a><b><>\n0:\n0:\n4:<a><><b><>\n2:<><>\n2:<a><b>\n2:<a><b>\n"; out != want || st != 0 {
		t.Errorf("got %q (status %d), want %q", out, st, want)
	}
}
