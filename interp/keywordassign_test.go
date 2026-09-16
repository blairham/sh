// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// keywordRunner is a shell with `set -k` available and a probe function.
func keywordRunner(t *testing.T, out *strings.Builder) *Runner {
	t.Helper()
	sem := PosixSemantics()
	sem.KeywordAssignments = Yes
	// A promoted assignment *is* a prefix assignment, so the two axes a
	// prefix in front of a function already asks are asked for it too. They
	// are answered here rather than left unspecified because an unanswered
	// axis is a diagnostic and a refusal, which would hide what these cases
	// are about — and answering them is also the point: nothing in this file
	// is a rule of the option's own.
	sem.PrefixToAFunctionIsExported = No
	sem.AssignmentPrefixPersistsAfterAFunction = No
	return newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh",
		Stdout: out, Stderr: out,
	})
}

// Which words `set -k` takes and which it leaves where they stood.
//
// The **written** word decides, never the expanded one. Measured on bash
// 5.3.20 and ksh93u+ 2012-08-01 alike, from a script file: a word that
// *expands* to `A=9` stays a positional and a quoted `'A=10'` stays one,
// while `Q=$V` — written as an assignment with an expansion in its value — is
// promoted in both, and so are `Q="$V"`, `Q=x$V`, `Q=$(…)`, `Q=$((…))`, `Q=~`,
// `Q=` and `Q=a=b`. A name that is not a name is left alone.
func TestWhichWordsTheKeywordOptionTakes(t *testing.T) {
	for _, c := range []struct {
		word, want string
	}{
		{`Q=vv`, "1=[zz] Q=[vv] n=1"},
		{`Q=$V`, "1=[zz] Q=[vv] n=1"},
		{`Q="$V"`, "1=[zz] Q=[vv] n=1"},
		{`Q=x$V`, "1=[zz] Q=[xvv] n=1"},
		{`Q=$(printf cs)`, "1=[zz] Q=[cs] n=1"},
		{`Q=$((1+1))`, "1=[zz] Q=[2] n=1"},
		{`Q=`, "1=[zz] Q=[] n=1"},
		{`Q=a=b`, "1=[zz] Q=[a=b] n=1"},
		// The name must be written, unquoted, and a name.
		{`'Q=q'`, "1=[Q=q] Q=[unset] n=2"},
		{`"Q=q"`, "1=[Q=q] Q=[unset] n=2"},
		{`$W`, "1=[Q=w] Q=[unset] n=2"},
		{`1Q=b`, "1=[1Q=b] Q=[unset] n=2"},
		{`=v`, "1=[=v] Q=[unset] n=2"},
		{`Qv`, "1=[Qv] Q=[unset] n=2"},
		// A subscripted name is deliberately not promoted — see
		// keywordassign.go for the two answers the references give it.
		{`arr[0]=v`, "1=[arr[0]=v] Q=[unset] n=2"},
	} {
		t.Run(c.word, func(t *testing.T) {
			out := &strings.Builder{}
			r := keywordRunner(t, out)
			runCd(t, r, `f() { echo "1=[${1-.}] Q=[${Q-unset}] n=$#"; }`+"\n"+
				`V=vv; W='Q=w'`+"\n"+
				`set -k`+"\n"+
				`f `+c.word+` zz`)
			if got := strings.TrimSpace(out.String()); got != c.want {
				t.Errorf("`f %s zz` wrote %q, want %q", c.word, got, c.want)
			}
		})
	}
}

// Several on one command, a `--` that does not stop it, and the last of a
// repeated name winning — all measured on bash and ksh93 together.
func TestTheKeywordOptionTakesEveryAssignmentWordOnTheCommand(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"several, and the command word keeps its own arguments",
			`f A=1 xx B=2`,
			"1=[xx] A=[1] B=[2] n=1",
		},
		{
			"a double dash is not an end-of-options marker for this",
			`f -- C=3`,
			"1=[--] A=[unset] B=[unset] n=1",
		},
		{
			"the last of a repeated name wins",
			`f A=4 A=5 yy`,
			"1=[yy] A=[5] B=[unset] n=1",
		},
		{
			"a command with nothing left but assignments",
			`f A=6`,
			"1=[.] A=[6] B=[unset] n=0",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out := &strings.Builder{}
			r := keywordRunner(t, out)
			runCd(t, r, `f() { echo "1=[${1-.}] A=[${A-unset}] B=[${B-unset}] n=$#"; }`+"\n"+
				`set -k`+"\n"+c.src)
			if got := strings.TrimSpace(out.String()); got != c.want {
				t.Errorf("%s wrote %q, want %q", c.src, got, c.want)
			}
		})
	}
}

// The promotion does not outlive the command it was written on.
//
// The tree is the script and a loop body runs it again, so a promotion written
// back into the command would make the second pass through a `while` see
// assignments the source does not hold — and `set +k` inside the loop would
// then be unable to put them back. Measured against bash 5.3.20, where the
// second iteration is the positional and the first is not.
func TestTurningTheOptionOffPutsTheWordBack(t *testing.T) {
	out := &strings.Builder{}
	r := keywordRunner(t, out)
	runCd(t, r, `f() { echo "n=$# 1=[${1-.}]"; }`+"\n"+
		`i=0`+"\n"+
		`set -k`+"\n"+
		`while [ "$i" -lt 2 ]; do`+"\n"+
		`  f A=1 zz`+"\n"+
		`  set +k`+"\n"+
		`  i=$(( i + 1 ))`+"\n"+
		`done`)
	want := "n=1 1=[zz]\nn=2 1=[A=1]"
	if got := strings.TrimSpace(out.String()); got != want {
		t.Errorf("wrote\n%s\nwant\n%s", got, want)
	}
}

// A shell with no answer refuses the letter rather than quietly taking it.
//
// The zero vector belongs to a library embedder and to a test, and neither
// should get an option nobody said this shell has — the same rule every other
// letter behind an axis follows.
func TestAnUnansweredKeywordOptionIsRefused(t *testing.T) {
	out := &strings.Builder{}
	sem := PosixSemantics()
	sem.KeywordAssignments = No
	sem.PrefixToAFunctionIsExported = No
	sem.AssignmentPrefixPersistsAfterAFunction = No
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh",
		Stdout: out, Stderr: out,
	})
	runCd(t, r, `f() { echo "n=$#"; }`+"\n"+
		`set -k 2>/dev/null`+"\n"+
		`f A=1 zz`)
	if got := strings.TrimSpace(out.String()); !strings.HasSuffix(got, "n=2") {
		t.Errorf("wrote %q, want the word left where it stood", got)
	}
}
