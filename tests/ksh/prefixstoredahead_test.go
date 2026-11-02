// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// A prefix that persists — in front of a POSIX-form function or a special
// builtin — is the plain assignment it is, and is stored before the command's
// redirections are opened: a redirection that fails leaves it stored, and the
// redirection's own word reads it. Measured 2026-11-02 on ksh93u+ 2012-08-01
// (`/bin/ksh`) from script files under `env -i PATH=/usr/bin:/bin` with
// `f() { :; }` first, where this shell left every name as it was (#6986). The
// `:` rows read the name from an EXIT trap, because the failed redirection
// ends the script there. See interp/prefixstoredahead.go.
func TestAPersistingPrefixIsStoredAheadOfTheRedirections(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		// The issue's rows.
		{"a scalar", `x=0; x=5 f >/nope/x; echo "x=$x"`, "x=5\n"},
		{"a table element", `typeset -A a; a[k]=old; a[k]=new f >/nope/x; echo "${a[k]}"`, "new\n"},
		{"an indexed element", `b=(x y z); i=1; b[i++]=1 f >/nope/x; echo "$i ${b[@]}"`, "2 x 1 z\n"},
		// The same rows in front of a special builtin.
		{"a scalar at :", `trap 'echo "x=$x"' EXIT; x=0; x=5 : >/nope/x; echo after`, "x=5\n"},
		{
			"a table element at :",
			`typeset -A a; trap 'echo "${a[k]}"' EXIT; a[k]=old; a[k]=new : >/nope/x; echo after`, "new\n",
		},
		{
			"an indexed element at :",
			`b=(x y z); trap 'echo "$i ${b[@]}"' EXIT; i=1; b[i++]=1 : >/nope/x; echo after`, "2 x 1 z\n",
		},
		// A real store and not a held value: the export comes off, a typed
		// name becomes a plain scalar, and an append joins once.
		{"the export comes off", `export x=o; x=n f >/nope/x; typeset -p x`, "x=n\n"},
		{"a typed name is a plain scalar", `typeset -i v=1; v=2+2 f >/nope/x; typeset -p v`, "v=2+2\n"},
		{"an append joins once", `y=o; y+=a f >/nope/x; echo "$y"`, "oa\n"},
		// The traced `function`-form call's front pass is the shell's own
		// store too, and `command :` keeps its prefix here.
		{
			"a traced call's front pass",
			`function g { :; }; typeset -ft g; x=0; x=1 g >/nope/x; echo "x=$x"`, "x=1\n",
		},
		{"through command", `x=0; x=5 command : >/nope/x; echo "x=$x"`, "x=5\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			out, _ := runKsh(t, t.TempDir(), "f() { :; }\n"+c.src)
			// The redirection really failed, once, and nothing else was said.
			if rest, failed := withoutTheOpenFailure(out); failed != 1 || rest != c.want {
				t.Errorf("got %q (%d failed opens), want %q after one", out, failed, c.want)
			}
		})
	}
}

// The redirection's word is expanded behind the store, so it reads the value;
// and the controls: an external's prefix and an untraced `function`-form
// call's are not this shell's to keep, and are left as they were.
func TestARedirectionReadsThePersistingPrefix(t *testing.T) {
	t.Parallel()
	for _, c := range []struct{ name, src, want string }{
		{"a function", `x=0; x=5 f >out_$x; echo out_*`, "out_5\n"},
		{"a special builtin", `x=0; x=5 : >out_$x; echo out_*`, "out_5\n"},
		{"an external", `x=0; x=5 /usr/bin/true >out_$x; echo out_*`, "out_0\n"},
		{"a function-form call", `function g { :; }; x=0; x=5 g >out_$x; echo out_*`, "out_0\n"},
		{
			"a function-form call's failed open",
			`function g { :; }; x=0; x=5 g 2>/dev/null >/nope/x; echo "x=$x"`, "x=0\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if out, _ := runKsh(t, t.TempDir(), "f() { :; }\n"+c.src); out != c.want {
				t.Errorf("got %q, want %q", out, c.want)
			}
		})
	}
}

// withoutTheOpenFailure drops the lines complaining about `/nope/x` and says
// how many there were.
func withoutTheOpenFailure(out string) (string, int) {
	var kept []string
	n := 0
	for line := range strings.SplitSeq(out, "\n") {
		if strings.Contains(line, "/nope/x") {
			n++
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n"), n
}
