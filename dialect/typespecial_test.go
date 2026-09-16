// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// `type` on a builtin POSIX marks special (#3273).
//
// Four of the seven columns call it `a special shell builtin` and three call
// every builtin `a shell builtin`; ours said the short sentence in every
// dialect, because there was one wording and it took only the name.
//
// Measured 2026-09-16, `env -i PATH=/usr/bin:/bin LC_ALL=C <shell> case.sh`,
// BusyBox v1.37.0 in the digest-pinned Alpine image internal/oracle reaches,
// under `--init`:
//
//	                     type .            type eval       type echo
//	bash 5.3.20          shell builtin     shell builtin   shell builtin
//	that binary as `sh`  SPECIAL           SPECIAL         shell builtin
//	bash 3.2.57          shell builtin     shell builtin   shell builtin
//	zsh 5.9.2            shell builtin     shell builtin   shell builtin
//	ksh93u+ 2012-08-01   SPECIAL           SPECIAL         shell builtin
//	dash 0.5.12          SPECIAL           SPECIAL         shell builtin
//	BusyBox ash 1.37.0   SPECIAL           SPECIAL         shell builtin
//
// # Why this is a Go row and not a suite file
//
// A per-dialect suite tier runs under the one reference it claims to be, and
// `core/` holds only what every reference agrees on. A 4-3 split has no home
// in either: `bash/` can pin bash's short sentence and `dash/` can pin dash's
// long one, and with both green the *split* is still unpinned — a preset
// mutant that moved one dialect to the other side would leave every tier at
// 100% and only the missing column would be wrong. So the split is asserted
// where all six presets are in one table and the control is beside the
// discriminator.
//
// # What each row can fail
//
//   - A shell that never says `special` fails the four Yes rows — the state
//     before this change.
//   - A shell that says it for everything fails `type echo`, in all six.
//   - A preset moved to the other side of the split fails that preset's row
//     alone, and nothing else in the tree.
//   - `command -V` is the same sentence from another builtin, and ksh's
//     `whence -a` a third: each had its own literal at some point, which is
//     how one shell came to say `a special shell builtin` under `whence -v`
//     and `a shell builtin` under `whence -a`, one line apart.
func TestTypeSaysSpecialShellBuiltinWhereTheReferenceDoes(t *testing.T) {
	for _, c := range []struct {
		preset string
		// special is what this preset writes for `.` and for `eval`, whole.
		special string
	}{
		{"bash", "is a shell builtin"},
		{"zsh", "is a shell builtin"},
		{"ksh", "is a special shell builtin"},
		{"dash", "is a special shell builtin"},
		{"ash", "is a special shell builtin"},
		{"posix", "is a special shell builtin"},
	} {
		t.Run(c.preset, func(t *testing.T) {
			p := presets[c.preset]
			// The discriminator and the control in one run, so a shell that
			// agrees by using a longer phrase everywhere is visible as one
			// that also moved the control.
			out, st, err := p.Combined(t, dialecttest.Base{}, `type .; type eval; type echo; echo "st=$?"`)
			if err != nil {
				t.Fatal(err)
			}
			want := ". " + c.special + "\neval " + c.special + "\necho is a shell builtin\nst=0\n"
			if out != want || st != 0 {
				t.Errorf("type said %q status %d, want %q at 0", out, st, want)
			}
			// `command -V` is the same sentence written by another builtin.
			out, _, err = p.Combined(t, dialecttest.Base{}, `command -V .; command -V echo`)
			if err != nil {
				t.Fatal(err)
			}
			if want := ". " + c.special + "\necho is a shell builtin\n"; out != want {
				t.Errorf("command -V said %q, want %q", out, want)
			}
		})
	}
}

// ksh93's two spellings of the question agree with each other, which is the
// half that had drifted: `whence -v` took the shared sentence and `whence -a`
// wrote its own literal, so one builtin was special under one letter and
// ordinary under the other in the same shell.
//
// `whence -a echo` is the control. It is three lines there — the builtin and
// the PATH hits — and the builtin line is the short sentence in every column,
// so a shell that had simply lengthened the phrase fails here.
func TestKshWhenceAgreesWithItselfAboutASpecialBuiltin(t *testing.T) {
	p := presets["ksh"]
	for _, src := range []string{"whence -v .", "whence -a ."} {
		out, _, err := p.Combined(t, dialecttest.Base{}, src)
		if err != nil {
			t.Fatal(err)
		}
		if want := ". is a special shell builtin\n"; out != want {
			t.Errorf("%s said %q, want %q", src, out, want)
		}
	}
	out, _, err := p.Combined(t, dialecttest.Base{}, "whence -a echo")
	if err != nil {
		t.Fatal(err)
	}
	if first, _, _ := strings.Cut(out, "\n"); first != "echo is a shell builtin" {
		t.Errorf("whence -a echo began %q, want the plain builtin sentence", first)
	}
}

// The core answers what the panel agrees on and refuses what it does not, and
// this axis is the rare one where the same builtin call is on both sides of
// that line: `type echo` is unanimous and `type .` is a 4-3 split.
//
// So the axis is asked for a name that *is* special and for no other, and this
// is the row that says so. Asking it unconditionally would refuse `type echo`
// in a shell with no dialect, which is a question nobody disagrees about; not
// asking it at all would hand one side's answer to the other, which is what
// the core exists not to do.
func TestTheCoreRefusesOnlyTheNamesThePanelDisagreesAbout(t *testing.T) {
	core := dialecttest.Preset{
		Name: "sh", Dialect: syntax.Core,
		Semantics: interp.CoreSemantics, Diagnostics: interp.CoreDiagnostics,
		Apply: noApply,
	}
	out, st, err := core.Combined(t, dialecttest.Base{}, `type echo; echo "st=$?"`)
	if err != nil {
		t.Fatal(err)
	}
	if want := "echo is a shell builtin\nst=0\n"; out != want {
		t.Errorf("the core said %q for an ordinary builtin, want %q", out, want)
	}
	out, st, err = core.Combined(t, dialecttest.Base{}, `type .`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "no dialect was chosen") || st != 2 {
		t.Errorf("the core said %q status %d for a special builtin, want a refusal at 2", out, st)
	}
	if strings.Contains(out, "shell builtin\n") {
		t.Errorf("the core answered as well as refusing: %q", out)
	}
}
