// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package cross_test

import (
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// **`getopts` over a name operand spelled OPTARG** —
// Semantics.GetoptsOptargOrder. Measured 2026-10-15 under `-c` against
// bash 5.3.20, ksh93u+ 2012-08-01, dash 0.5.12, BusyBox ash 1.37.0 (the
// pinned alpine image) and zsh 5.9.2; each line is followed by `echo
// "[$?] [${OPTARG-unset}] [$OPTIND]"`, and the complaints a fault writes are
// sent to /dev/null since their wording is not the question. zsh dies of a
// trap signal on the four fault rows, so only its first three are pinned; its
// end of the options leaves OPTARG empty, as it does over any operand (#6498).
func TestGetoptsOptargOrderOverAnOperandSpelledOPTARG(t *testing.T) {
	lines := []string{
		"getopts x: OPTARG -x6",
		"getopts x OPTARG -x",
		"getopts x OPTARG a",
		"getopts :x OPTARG -y",
		"getopts x OPTARG -y",
		"getopts :x: OPTARG -x",
		"getopts x: OPTARG -x",
	}
	want := map[string][]string{
		"bash": {"[0] [x] [2]", "[0] [x] [2]", "[1] [?] [1]", "[0] [y] [2]", "[0] [unset] [2]", "[0] [x] [2]", "[0] [unset] [2]"},
		"ksh":  {"[0] [6] [2]", "[0] [unset] [2]", "[1] [unset] [1]", "[0] [y] [2]", "[0] [unset] [2]", "[0] [x] [2]", "[0] [unset] [2]"},
		"dash": {"[0] [x] [2]", "[0] [x] [2]", "[1] [?] [1]", "[0] [?] [2]", "[0] [?] [2]", "[0] [:] [2]", "[0] [?] [2]"},
		"ash":  {"[0] [x] [2]", "[0] [x] [2]", "[1] [?] [1]", "[0] [?] [2]", "[0] [?] [2]", "[0] [:] [2]", "[0] [?] [2]"},
		"zsh":  {"[0] [x] [2]", "[0] [x] [1]", "[1] [] [1]"},
	}
	for name, rows := range want {
		for i, w := range rows {
			src := lines[i] + ` 2>/dev/null; echo "[$?] [${OPTARG-unset}] [$OPTIND]"` + "\n"
			out, _, err := presets[name].Combined(t, dialecttest.Base{Dir: t.TempDir()}, src)
			if err != nil {
				t.Fatal(err)
			}
			if out != w+"\n" {
				t.Errorf("%s: %s: got %q, want %q", name, lines[i], out, w)
			}
		}
	}
}

// **OPTIND is an integer in ksh93, as it is in bash.** Measured 2026-10-15
// on ksh93u+ 2012-08-01 under `-c`: `typeset -p OPTIND` is `typeset -i
// OPTIND=1`, `OPTIND=2+3` reads back 5, and `getopts x OPTIND -x` — the
// operand's letter landing last, as in every column — leaves 0, the value of
// the unset name `x`. `unset OPTIND` takes the attribute with the name. dash
// refuses the letter outright, which is #6537 (#6498).
func TestKshOptindIsAnInteger(t *testing.T) {
	src := "typeset -p OPTIND\nOPTIND=2+3; echo $OPTIND\nOPTIND=1; getopts x OPTIND -x; echo $OPTIND\nunset OPTIND; OPTIND=x; echo $OPTIND\n"
	out, _, err := presets["ksh"].Combined(t, dialecttest.Base{Dir: t.TempDir()}, src)
	if err != nil {
		t.Fatal(err)
	}
	if want := "typeset -i OPTIND=1\n5\n0\nx\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// A freeze on an operand spelled OPTARG is the operand's: measured
// 2026-10-15, `readonly OPTARG; getopts x: OPTARG -x6` is refused in every
// column — ksh93u+ reports it and returns 2 though it writes its *own* OPTARG
// through a freeze, and zsh 5.9.2 ends the script on it as it does for any
// frozen operand — where before this both wrote through it (#6498).
func TestAFrozenOperandSpelledOPTARGIsTheOperands(t *testing.T) {
	src := "readonly OPTARG\ngetopts x: OPTARG -x6 2>/dev/null\necho \"st=$?\"\n"
	for name, want := range map[string]string{
		"ksh":  "st=2\n",
		"bash": "st=2\n",
		"dash": "st=2\n",
		"zsh":  "",
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

// The same order through a freeze on an ordinary operand: measured
// 2026-10-15, `OPTARG=pre; readonly o; getopts x: o -x6` returns 2 in all
// four columns and leaves `pre` in ksh93u+ alone, whose refused name ends the
// builtin before its OPTARG store; bash 5.3.20, dash 0.5.12 and BusyBox ash
// 1.37.0 leave `6` (#6498).
func TestARefusedNameAndTheOptargStore(t *testing.T) {
	src := "OPTARG=pre; readonly o\ngetopts x: o -x6 2>/dev/null\necho \"$? ${OPTARG-unset}\"\n"
	for name, want := range map[string]string{
		"ksh":  "2 pre\n",
		"bash": "2 6\n",
		"dash": "2 6\n",
		"ash":  "2 6\n",
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
