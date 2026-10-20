// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// **`(q)` leaves the empty fields a split made empty** (#5281), where it
// writes an empty value as a pair of quotes. Measured against zsh 5.9.2 with
// IFS=:: `${(@q)=u}` over `a::b:` is a, nothing, b, nothing, quoted or not;
// a split of a value that was already empty is still a quoted empty; `(qq)`,
// `(q-)` and a prompt-escape flag beside it each quote every empty; and an
// unquoted bare field is dropped where the split does not keep empty fields.
func TestQuotingLeavesASplitsEmptyFieldsEmpty(t *testing.T) {
	out, st := runZsh(t, t.TempDir(), `show() { for x in "$@"; do printf '<%s>' "$x"; done; print; }
IFS=:; u='a::b:'; d=':'; e=''
show ${(@q)=u}
show "${(@q)=u}"
show ${(q)=u}
show ${(@q)=d} "${(@q)=e}"
show ${(@qq)=u}
show ${(@q-)=u}
show ${(@qU)=u} ${(@q%)=u}
show ${(@q)u} ${(q)e}`)
	want := "<a><><b><>\n<a><><b><>\n<a><><b><>\n<><><''>\n<'a'><''><'b'><''>\n<a><''><b><''>\n<A><><B><><a><''><b><''>\n<a::b:><''>\n"
	if out != want || st != 0 {
		t.Errorf("got (status %d):\n%s\nwant:\n%s", st, out, want)
	}
}
