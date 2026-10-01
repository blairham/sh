// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import "testing"

// `disable -f` and `enable -f`, and the switched-off table they keep (#5267).
// Measured 2026-10-01 on zsh 5.9.2 (`-f`, a script file under `env -i
// PATH=/usr/bin:/bin LC_ALL=C`); each row is what that shell wrote.
func TestASwitchedOffFunctionIsNoFunctionUntilSwitchedBackOn(t *testing.T) {
	dir := t.TempDir()
	const def = "zq() { print zq; }\n"
	for _, c := range []struct {
		name, src, want string
	}{
		{"a call does not reach it", "disable -f zq\nzq 2>/dev/null\nprint -rn -- \"st=$?\"", "st=127"},
		{"whence calls it none", "disable -f zq\nwhence -w zq", "zq: none\n"},
		{"typeset -f finds nothing", "disable -f zq\ntypeset -f zq\nprint -rn -- \"st=$?\"", "st=1"},
		{"the two tables", "disable -f zq\nprint -rn -- \"${+functions[zq]} ${+dis_functions[zq]} [${dis_functions[zq]}]\"", "0 1 [\tprint zq]"},
		{"enable -f puts it back", "disable -f zq\nenable -f zq\nzq", "zq\n"},
		{"twice off is no change", "disable -f zq\ndisable -f zq\nprint -rn -- \"st=$?\"", "st=0"},
		{"on when already on", "enable -f zq\nprint -rn -- \"st=$?\"", "st=0"},
		{"a name that is no function", "disable -f nosuch 2>&1\nprint -rn -- \"st=$?\"", "zsh:disable:2: no such hash table element: nosuch\nst=1"},
		{"not a pattern", "disable -f 'z*' 2>/dev/null\nprint -rn -- \"st=$?\"", "st=1"},
		{"the listing", "zr() { print zr; }\ndisable -f zq\nprint A; disable -f; print B; enable -f", "A\nzq () {\n\tprint zq\n}\nB\nzr () {\n\tprint zr\n}\n"},
		{"a definition replaces it", "disable -f zq\nzq() { print new; }\nzq\nprint -rn -- \"${+dis_functions[zq]}\"", "new\n0"},
		{"unfunction removes it", "disable -f zq\nunfunction zq\nprint -rn -- \"$? ${+dis_functions[zq]} ${+functions[zq]}\"", "0 0 0"},
		{"a subshell's enable stays there", "disable -f zq\n(enable -f zq; zq)\nzq 2>/dev/null\nprint -rn -- \"st=$?\"", "zq\nst=127"},
		{"writing the table switches one off", "dis_functions[zz]='print zz'\nprint -rn -- \"$? ${+functions[zz]} ${+dis_functions[zz]}\"", "0 0 1"},
		// Read once first: an `unset` that is the table's *first* reference
		// meets zsh's not-yet-loaded module stub and is refused there for
		// `$functions` too — the state #1546 measured and chose not to model.
		{"unsetting the table removes it", "disable -f zq\n: ${#dis_functions}\nunset 'dis_functions[zq]'\nprint -rn -- \"$? ${+dis_functions[zq]} ${+functions[zq]}\"", "0 0 0"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if out, _ := runZsh(t, dir, def+c.src+"\n"); out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}
