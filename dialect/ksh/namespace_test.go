// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/syntax"
)

// `namespace NAME { … }` is this shell's alone, and every row here was
// measured on ksh93u+ 2012-08-01, 2026-09-19, one script file per row under
// `env -i PATH=/usr/bin:/bin LC_ALL=C` with standard input on /dev/null.
//
// The construct was filed as blocked on "a named store this shell has
// nothing like" (#3309). It is not: a namespace is a **name-resolution
// region over a compound**, and a member is an ordinary name spelled with a
// leading dot — which is the store `c=(a=1)` already puts `c.a` in. The
// spelling that shows it is `${.ns.x}`; `${ns.x}` is unset, which is what
// the issue tried and what made the store look absent.
func TestANamespaceBodyResolvesThroughTheNamespace(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		// The body reads the outer name, and a write does not leave.
		{`x=OUTER; namespace ns { echo "[${x-unset}]"; }`, "[OUTER]\n"},
		{`x=OUTER; namespace ns { x=IN; }; echo "[$x]"`, "[OUTER]\n"},
		// Two blocks of one name share what the first left, which is what
		// makes the namespace outlive the block.
		{`namespace ns { x=1; }; namespace ns { echo "[${x-unset}]"; }`, "[1]\n"},
		{`namespace ns { x=1; }; namespace n2 { echo "[${x-unset}]"; }`, "[unset]\n"},
		// The dot is the spelling, and every ordinary thing that can be done
		// to a name can be done to a member.
		{`namespace ns { x=1; }; echo "[${ns.x-unset}]"`, "[unset]\n"},
		{`namespace ns { x=1; }; echo "[${.ns.x-unset}]"`, "[1]\n"},
		{`namespace ns { x=1; }; typeset -p .ns.x`, ".ns.x=1\n"},
		{`namespace ns { x=1; }; .ns.x=2; namespace ns { echo "[$x]"; }`, "[2]\n"},
		{`namespace ns { x=1; }; unset .ns.x; echo "[${.ns.x-unset}]"`, "[unset]\n"},
		{`namespace ns { x=1; }; unset .ns; echo "[${.ns.x-unset}]"`, "[unset]\n"},
		// It holds the globals, which is the same rule seen from outside: a
		// member the namespace does not have reads the plain name, live.
		{`gv=GLOBAL; namespace ns { y=1; }; echo "[${.ns.gv-unset}]"`, "[GLOBAL]\n"},
		{`gv=GLOBAL; namespace ns { y=1; }; echo "[${.ns.zzz-unset}]"`, "[unset]\n"},
		{`gv=A; namespace ns { y=1; }; gv=B; echo "[${.ns.gv}]"`, "[B]\n"},
		{`gv=G; namespace ns { gv=M; }; echo "[$gv][${.ns.gv}]"`, "[G][M]\n"},
		{`namespace ns { PATH=/zzz; }; echo "[${.ns.PATH}]"`, "[/zzz]\n"},
		// Functions go in it too, under the member name — which is what
		// `typeset +f` writes back there.
		{`namespace ns { f(){ echo hi; }; }; .ns.f`, "hi\n"},
		{`namespace ns { f(){ echo IN; }; f; }`, "IN\n"},
		{`namespace ns { f(){ :; }; }; typeset +f`, ".ns.f()\n"},
		// And an export does not reach the environment.
		{`namespace ns { typeset -x E=1; }; echo "[${.ns.E}]"`, "[1]\n"},
		// Namespaces are flat whatever the nesting.
		{`namespace a { namespace b { x=1; }; }; echo "[${.a.b.x-unset}][${.b.x-unset}]"`, "[unset][1]\n"},
		// The block's status is its last command's, and it takes
		// redirections the way a brace group does.
		{`namespace ns { true; false; }; echo "st=$?"`, "st=1\n"},
		{`false; namespace ns { :; }; echo "st=$?"`, "st=0\n"},
	} {
		if out, st := kshOut(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d\nwant %q at 0", c.src, out, st, c.want)
		}
	}
}

// It is lexical, which was the open question on #3309 and is now measured:
// the region a body resolves through belongs to where the code was *written*
// and not to where the call was made.
func TestANamespaceIsLexicalAndNotDynamic(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		// Defined outside, called from inside: the caller's scope.
		{
			`x=OUTER; g(){ echo "[${x-unset}]"; x=FROMG; }; namespace ns { g; }; echo "[$x]"`,
			"[OUTER]\n[FROMG]\n",
		},
		{`namespace ns { x=1; }; h(){ echo "[${x-unset}]"; }; namespace ns { h; }`, "[unset]\n"},
		// Defined inside, called from outside: still the namespace's.
		{`namespace ns { x=1; f(){ echo "[${x-unset}]"; }; }; .ns.f`, "[1]\n"},
		{`namespace ns { f(){ y=IN; }; }; .ns.f; echo "[${y-unset}][${.ns.y}]"`, "[unset][IN]\n"},
		// A bare call outside does not find it at all.
		{`namespace ns { f(){ echo hi; }; }; command -v f; echo "st=$?"`, "st=1\n"},
	} {
		if out, st := kshOut(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d\nwant %q at 0", c.src, out, st, c.want)
		}
	}
}

// The word is reserved where a command begins and nowhere else, which is what
// keeps it a name every dialect may still use — including this one.
func TestNamespaceIsAWordOnlyWhereACommandBegins(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`namespace=5; echo "[$namespace]"`, "[5]\n"},
		{`echo namespace`, "namespace\n"},
		{`echo a namespace b`, "a namespace b\n"},
		{`namespace ns { x=1; }; namespace=7; echo "[$namespace][${.ns.x}]"`, "[7][1]\n"},
		// And it is a keyword to a report about what is runnable.
		{`whence -v namespace`, "namespace is a keyword\n"},
		{`command -v namespace`, "namespace\n"},
	} {
		if out, st := kshOut(t, c.src); out != c.want || st != 0 {
			t.Errorf("%s\n got %q at %d\nwant %q at 0", c.src, out, st, c.want)
		}
	}
}

// No other dialect has the construct, and the word is an ordinary one in all
// of them — measured: the brace group after it is where each of the other
// four stops, every one of them naming the `}`.
func TestOnlyThisDialectReadsANamespace(t *testing.T) {
	for _, d := range []struct {
		name string
		d    syntax.Dialect
	}{
		{"bash", bash.Dialect()},
		{"zsh", zsh.Dialect()},
		{"dash", dash.Dialect()},
	} {
		if _, err := syntax.Parse("namespace ns { x=1; }", d.d); err == nil {
			t.Errorf("%s read a namespace block, want a syntax error", d.name)
		}
		if _, err := syntax.Parse("namespace=5; echo namespace", d.d); err != nil {
			t.Errorf("%s: the word is not an ordinary name: %v", d.name, err)
		}
	}
	if _, err := syntax.Parse("namespace ns { x=1; }", ksh.Dialect()); err != nil {
		t.Errorf("ksh: %v", err)
	}
}

// A word that is no name is refused when the clause runs, at 1, and the
// script stops there — where a *grammar* failure is 3 and the file never
// runs at all. Three sentences, picked by the word rather than by the
// dialect; see interp.Diagnostics.NamespaceNameNotAVariable.
func TestANamespaceNameIsRefusedWhenTheClauseRuns(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		{`namespace .ns { x=1; }; echo after`, "ksh: .ns: is not an identifier\n"},
		{`namespace a.b { x=1; }; echo after`, "ksh: a.b: is not an identifier\n"},
		{`namespace a-b { x=1; }; echo after`, "ksh: .a-b: invalid variable name\n"},
		{`namespace 1x { x=1; }; echo after`, "ksh: .1x: invalid variable name\n"},
		{`namespace a[1] { x=1; }; echo after`, "ksh: .a[1]: cannot be an array\n"},
		// The word is judged as written and never expanded.
		{`n=ns; namespace $n { x=1; }; echo after`, "ksh: .$n: invalid variable name\n"},
	} {
		out, st := kshOut(t, c.src)
		if out != c.want || st != 1 {
			t.Errorf("%s\n got %q at %d\nwant %q at 1", c.src, out, st, c.want)
		}
	}
	// And quoting may be on the name, which is what tells that row from a
	// quoted `namespace` — the second is a syntax error.
	if out, st := kshOut(t, `namespace "ns" { x=1; }; echo "[${.ns.x}]"`); out != "[1]\n" || st != 0 {
		t.Errorf("a quoted name: got %q at %d, want %q at 0", out, st, "[1]\n")
	}
	if _, err := syntax.Parse(`"namespace" ns { x=1; }`, ksh.Dialect()); err == nil {
		t.Error("a quoted keyword parsed, want a syntax error")
	}
}

// An unterminated block is named after the construct rather than after its
// brace, which is what the column with it reports.
func TestAnUnterminatedNamespaceNamesTheConstruct(t *testing.T) {
	_, err := syntax.Parse("namespace ns {", ksh.Dialect())
	if err == nil {
		t.Fatal("parsed, want a syntax error")
	}
	if got := err.Error(); !strings.Contains(got, "namespace") {
		t.Errorf("got %q, want the construct named", got)
	}
}
