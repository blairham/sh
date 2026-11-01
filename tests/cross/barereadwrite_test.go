// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package cross_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
)

// **Which descriptor a `<>` with no number in front of it opens** (#7178):
// standard output in ksh93u+, standard input in the standard and in every
// other column. See interp.Semantics.BareReadWriteRedirectionIsStandardOutput.
//
// Measured 2026-10-31, each row verbatim as a script file with standard input
// on /dev/null, on ksh93u+ 2012-08-01, bash 5.3, zsh 5.9, dash and BusyBox
// 1.38.0. The third row is the control: a number written in front is honored
// everywhere, so it reads the file in ksh93 too.
func TestABareReadWriteRedirectionOpensTheDialectsDescriptor(t *testing.T) {
	const (
		echo  = `echo hi <>f; read -r l <f; echo "f=[$l]"`
		read  = `printf 'abcdef\n' >f; read -r l <>f; echo "st=$? l=[$l]"`
		zero  = `printf 'abcdef\n' >f; read -r l 0<>f; echo "st=$? l=[$l]"`
		exec_ = `: >f; exec 3>&1; exec <>f; echo hi; exec 1>&3; read -r l <f; echo "x=[$l]"`
	)
	output := map[string]string{
		echo:  "f=[hi]\n",
		read:  "st=1 l=[]\n",
		zero:  "st=0 l=[abcdef]\n",
		exec_: "x=[hi]\n",
	}
	input := map[string]string{
		echo:  "hi\nf=[]\n",
		read:  "st=0 l=[abcdef]\n",
		zero:  "st=0 l=[abcdef]\n",
		exec_: "hi\nx=[]\n",
	}
	for _, dialect := range []string{"ksh", "bash", "zsh", "dash", "ash", "posix"} {
		want := input
		if dialect == "ksh" {
			want = output
		}
		for _, src := range []string{echo, read, zero, exec_} {
			out, _, err := presets[dialect].Combined(t, dialecttest.Base{Dir: t.TempDir()}, src)
			if err != nil {
				t.Fatal(err)
			}
			if out != want[src] {
				t.Errorf("%s %q: got %q, want %q", dialect, src, out, want[src])
			}
		}
	}
}

// And the question is put only to a `<>` with no number, which is the only
// spelling the readings part over: with the axis left unanswered, a written
// number and a `{v}` open as ever, and the bare one is the refusal.
func TestABareReadWriteRedirectionIsAskedOnlyWithNoNumber(t *testing.T) {
	p := presets["bash"]
	answered := p.Semantics
	p.Semantics = func() interp.Semantics {
		s := answered()
		s.BareReadWriteRedirectionIsStandardOutput = interp.Unspecified
		return s
	}
	out, _, err := p.Combined(t, dialecttest.Base{Dir: t.TempDir()}, `: 0<>f 3<>f {v}<>f; echo ok`)
	if err != nil {
		t.Fatal(err)
	}
	if out != "ok\n" {
		t.Errorf("a numbered `<>`: got %q, want %q", out, "ok\n")
	}
	out, _, err = p.Combined(t, dialecttest.Base{Dir: t.TempDir()}, `: <>f; echo "st=$?"`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "a `<>` with no descriptor number opening standard output") {
		t.Errorf("a bare `<>`: got %q, want the axis named", out)
	}
}
