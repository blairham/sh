// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
	"github.com/blairham/sh/interp"
)

// TestTheEvalContextSaysWhatTheShellIsInside — `$ZSH_EVAL_CONTEXT` and
// `$zsh_eval_context`, which had no parameter at all here.
//
// Every row is byte-identical to `/opt/homebrew/bin/zsh` — `zsh 5.9.2
// (aarch64-apple-darwin25.4.0)`, which `go version -m` calls *not a Go
// executable* where it calls ours `github.com/blairham/sh/cmd/zsh` — measured
// 2026-09-27 in one run, script files under `env -i PATH=/usr/bin:/bin` with
// a scratch `HOME`.
//
// **The apparatus is in the answer**, so the rows are written with the shape
// they were read through rather than with it tidied away: a read that goes
// through a helper function carries that helper's `shfunc`, and a read
// wrapped in `$( … )` carries `cmdsubst`. A sweep that wrapped every cell to
// keep going past a name that might refuse would record `toplevel:cmdsubst`,
// which is a value this parameter never has at rest — and the first table
// filed against #4866 did exactly that.
func TestTheEvalContextSaysWhatTheShellIsInside(t *testing.T) {
	// One helper, used by the rows that need a function around the read, so
	// that its own `shfunc` is visible in every `want` rather than hidden.
	const p = "p() { print -r -- \"[$ZSH_EVAL_CONTEXT] (${(j:,:)zsh_eval_context})\"; }\n"
	for _, tc := range []struct{ name, src, want string }{
		{"a script's top level", `print -r -- "[$ZSH_EVAL_CONTEXT]"`, "[toplevel]\n"},
		{"through a function", p + "p", "[toplevel:shfunc] (toplevel,shfunc)\n"},
		{"a function inside a function", p + "f() { p; }\nf", "[toplevel:shfunc:shfunc] (toplevel,shfunc,shfunc)\n"},
		{"an anonymous function", "() { print -r -- \"[$ZSH_EVAL_CONTEXT]\" }", "[toplevel:shfunc]\n"},
		{"eval", p + `eval 'p'`, "[toplevel:eval:shfunc] (toplevel,eval,shfunc)\n"},
		{"a command substitution", `print -r -- "[$(print -rn -- $ZSH_EVAL_CONTEXT)]"`, "[toplevel:cmdsubst]\n"},
		{"the backquoted spelling", "print -r -- \"[`print -rn -- $ZSH_EVAL_CONTEXT`]\"", "[toplevel:cmdsubst]\n"},
		{"eval inside a substitution", `print -r -- "[$(eval 'print -rn -- $ZSH_EVAL_CONTEXT')]"`, "[toplevel:cmdsubst:eval]\n"},
		{"eval inside eval", `print -r -- "[$(eval 'eval "print -rn -- \$ZSH_EVAL_CONTEXT"')]"`, "[toplevel:cmdsubst:eval:eval]\n"},
		// The three substitution spellings, read through `read` rather than
		// through `$( … )` so that the word under test stands alone — and
		// note the pairing, which is the opposite of what the spellings
		// suggest: `<( … )` is `outsubst` and `>( … )` is `insubst`, because
		// the word names the direction the body's data travels.
		{
			"a file substitution",
			`read -r l < =(print -rn -- $ZSH_EVAL_CONTEXT); print -r -- "[$l]"`,
			"[toplevel:equalsubst]\n",
		},
		{
			"a read process substitution",
			`read -r l < <(print -rn -- $ZSH_EVAL_CONTEXT); print -r -- "[$l]"`,
			"[toplevel:outsubst]\n",
		},
		{
			"a write process substitution",
			`print -rn x > >(read -r _; print -r -- "[$ZSH_EVAL_CONTEXT]")`,
			"[toplevel:insubst]\n",
		},
		{"a trap action", p + "trap 'p' USR1\nkill -USR1 $$", "[toplevel:trap:shfunc] (toplevel,trap,shfunc)\n"},
		// The five shapes that push **nothing**, which is as much of the
		// answer as the ten that do: a subshell is a new shell and says so
		// nowhere, so a rule keyed on "a new shell" or "a new scope" would
		// have been wrong about four of these.
		{"a subshell pushes nothing", p + "( p )", "[toplevel:shfunc] (toplevel,shfunc)\n"},
		{"a brace group pushes nothing", p + "{ p; }", "[toplevel:shfunc] (toplevel,shfunc)\n"},
		{"a loop body pushes nothing", p + "for i in 1; do p; done", "[toplevel:shfunc] (toplevel,shfunc)\n"},
		{"an always block pushes nothing", `{ : } always { print -r -- "[$ZSH_EVAL_CONTEXT]" }`, "[toplevel]\n"},
		{"arithmetic pushes nothing", `: $(( 1 )); print -r -- "[$ZSH_EVAL_CONTEXT]"`, "[toplevel]\n"},
		// Both halves are frozen, and the refusal is the same sentence for
		// an assignment and for an `unset`.
		{"the scalar is frozen", `ZSH_EVAL_CONTEXT=x`, "zsh:1: read-only variable: ZSH_EVAL_CONTEXT\n"},
		{"the array is frozen", `zsh_eval_context=( x )`, "zsh:1: read-only variable: zsh_eval_context\n"},
		{"neither writes a typeset -p row", `typeset -p ZSH_EVAL_CONTEXT zsh_eval_context`, ""},
		{
			"the words a listing writes",
			`print -r -- "${(t)ZSH_EVAL_CONTEXT} ${(t)zsh_eval_context} ${+ZSH_EVAL_CONTEXT} ${+zsh_eval_context}"`,
			"scalar-readonly-tied-special array-readonly-tied-special 1 1\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := runZsh(t, t.TempDir(), tc.src)
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// TestASourcedFileIsItsOwnContextWord — the row that needs a file on disk,
// and the one that separates this stack from the call stack: `$funcstack`
// names a sourced file and a function alike, and this names them with
// different words.
func TestASourcedFileIsItsOwnContextWord(t *testing.T) {
	dir := t.TempDir()
	inner := filepath.Join(dir, "inc.zsh")
	const body = "print -r -- \"[$ZSH_EVAL_CONTEXT]\"\n" +
		"g() { print -r -- \"[$ZSH_EVAL_CONTEXT]\"; }\n" +
		"g\n"
	if err := os.WriteFile(inner, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	out, _ := runZsh(t, dir, ". "+inner)
	want := "[toplevel:file]\n[toplevel:file:shfunc]\n"
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

// TestTheCommandStringRouteNamesItself — `cmdarg` rather than `toplevel`, and
// it is the *route* that decides rather than anything the shell entered.
//
// The control for every row above: without it they would all pass in a shell
// that answered `toplevel` for the bottom of the stack whatever it was given.
func TestTheCommandStringRouteNamesItself(t *testing.T) {
	base := dialecttest.Base{Dir: t.TempDir(), Route: interp.RouteCommandString}
	out, _, err := preset.Combined(t, base, `print -r -- "[$ZSH_EVAL_CONTEXT]"`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if want := "[cmdarg]\n"; out != want {
		t.Errorf("got %q, want %q", out, want)
	}
}
