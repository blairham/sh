// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package cross_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// **`set -a` reaches a name a loop, `read` or `getopts` fills** —
// Semantics.AllexportMarksALoopOrGetoptsName and
// AllexportMarksGetoptsOwnParameters, and `read`'s operand, which no column
// leaves alone. Measured 2026-10-15 under `-c` with `env -i
// PATH=/usr/bin:/bin`, each name unset beforehand and read back from `env`
// after `set +a`: bash 5.3.20, dash 0.5.12 and BusyBox ash (the pinned alpine
// image) export all five; zsh 5.9.2 exports `f`, `o` and `r` and not OPTARG or
// OPTIND; ksh93u+ exports `r` alone. `x=9` is the control, exported in every
// column (#6462).
func TestAllexportReachesALoopReadAndGetopts(t *testing.T) {
	src := "PATH=/usr/bin:/bin\nset -a\nx=9\nfor f in 4; do :; done\nread r <<EOF\n5\nEOF\ngetopts x: o -x6\nset +a\nenv | grep -E '^(x|f|r|o|OPTARG|OPTIND)=' | sort\n"
	all := "OPTARG=6\nOPTIND=2\nf=4\no=x\nr=5\nx=9\n"
	for name, want := range map[string]string{
		"bash": all,
		"dash": all,
		"ash":  all,
		"zsh":  "f=4\no=x\nr=5\nx=9\n",
		"ksh":  "r=5\nx=9\n",
	} {
		out, _, err := presets[name].Combined(t, dialecttest.Base{Dir: t.TempDir()}, src)
		if err != nil {
			t.Fatal(err)
		}
		if out != want {
			t.Errorf("%s: got %q, want %q", name, out, want)
		}
	}
}

// The constructs dash and BusyBox ash do not have, measured the same day:
// `select`'s name and REPLY, `printf -v` and a `{fd}` redirection's name are
// exported by bash 5.3.20 and zsh 5.9.2, and ksh93u+ — which has no `printf
// -v` — exports `select`'s REPLY and neither its name nor the descriptor's.
// (zsh allocates from 11 where bash does from 10, as the real two do.)
func TestAllexportReachesSelectPrintfAndADescriptorsName(t *testing.T) {
	sel := "PATH=/usr/bin:/bin\nset -a\nx=9\nselect s in a; do break; done 2>/dev/null <<EOF\n1\nEOF\nexec {fd}>/dev/null\n"
	tail := "set +a\nenv | grep -E '^(x|s|REPLY|fd|p)=' | sort\n"
	withPrintf := sel + "printf -v p %s 3\n" + tail
	for _, tc := range []struct{ name, src, want string }{
		{"bash", withPrintf, "REPLY=1\nfd=10\np=3\ns=a\nx=9\n"},
		{"zsh", withPrintf, "REPLY=1\nfd=11\np=3\ns=a\nx=9\n"},
		{"ksh", sel + tail, "REPLY=1\nx=9\n"},
	} {
		out, _, err := presets[tc.name].Combined(t, dialecttest.Base{Dir: t.TempDir()}, tc.src)
		if err != nil {
			t.Fatal(err)
		}
		if out != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, out, tc.want)
		}
	}
}
