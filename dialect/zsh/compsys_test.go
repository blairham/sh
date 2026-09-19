// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/repl"
)

// The completion system's own half of #2776: a `zle -C` widget's *function*
// runs, and what it collects with `compadd` is what the key offers.
//
// Every expectation here was measured through a pseudo-terminal against zsh
// 5.9.2 on 2026-09-15, from inside a `zle -C probewid .complete-word _probe`
// bound to a key, with `_probe` printing its parameters and the status of each
// call. The measurements are written out in compsys.go, compadd.go and
// compset.go; what is here is them, asked of this shell.

// completionFor runs one completion the way a key bound to a completion widget
// runs it, and hands back what the widget offered.
//
// Through RunCompletion and not through the pieces, because the pieces cannot
// fail the way the whole can: the parameters, the builtins and the widget
// table are each testable on their own and were each working while a key
// bound to a completion widget still completed nothing, which is the state
// #2770 left and this issue found.
func completionFor(t *testing.T, src, line string) []string {
	t.Helper()
	return completionWords(completionCandidatesFor(t, src, line))
}

// completionWords is the replacement words of an answer: what goes into the
// line, which is what the rows below are about. What a listing *draws* for
// them is candidatesFor's, next to the measurement that says so.
func completionWords(candidates []repl.Candidate) []string {
	var out []string
	for _, c := range candidates {
		out = append(out, c.Word)
	}
	return out
}

// completionCandidatesFor is the same completion with the rows and the blocks
// still on it — see compadd.go for what each `compadd` letter puts there.
func completionCandidatesFor(t *testing.T, src, line string) []repl.Candidate {
	t.Helper()
	r := bindkeyRunner(t, src)
	start := strings.LastIndexAny(line, " \t") + 1
	return zsh.RunCompletion(r, t.Context(), "probewid", repl.Completion{
		Line:    line,
		Point:   len(line),
		Start:   start,
		Word:    line[start:],
		Command: start == 0,
		Dir:     r.Dir,
	})
}

// widgetOf wraps a completion function in the `zle -C` a real startup file
// writes, under the name completionFor asks for.
func widgetOf(body string) string {
	return "_probe() { " + body + " }\n" +
		"zle -C probewid .complete-word _probe\n"
}

// TestACompletionWidgetsFunctionSuppliesTheCandidates is the whole of what a
// person gets that they did not have: a completion written in a startup file
// decides what Tab offers.
//
// Measured on zsh 5.9.2 with `git che` typed and a widget whose function is
// `compadd checkout cherry cherry-pick commit`: the key offers the three that
// begin with what was typed, and `commit` is not among them.
func TestACompletionWidgetsFunctionSuppliesTheCandidates(t *testing.T) {
	got := completionFor(t,
		widgetOf("compadd checkout cherry cherry-pick commit"), "git che")
	want := []string{"checkout", "cherry", "cherry-pick"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("git che offered %q, want %q", got, want)
	}
}

// TestAWidgetThatOffersNothingLeavesTheEditorsOwnCompletionStanding is the
// rule that makes the change above safe to wire to a real startup file, and it
// is #2770 arrived at from the other side.
//
// Nothing here is a fallback the editor arranges: an empty answer *is* how a
// completer says it has no opinion, and repl's composition rule then asks the
// next one. So the four shapes below all have to answer with no matches rather
// than with something the editor would insert.
func TestAWidgetThatOffersNothingLeavesTheEditorsOwnCompletionStanding(t *testing.T) {
	for _, c := range []struct{ name, src string }{
		// A function that adds nothing at all.
		{"adds nothing", widgetOf(":")},
		// One whose candidates do not match what was typed.
		{"nothing matches", widgetOf("compadd zzz yyy")},
		// One that fails.
		{"function fails", widgetOf("return 1; compadd checkout")},
		// A widget whose function was never defined — measured, `zle -C w
		// complete-word nosuchfn` is status 0 in zsh, so this is a live
		// state and not a typo nobody reaches.
		{"function undefined", "zle -C probewid .complete-word nosuchfn\n"},
		// And a plain widget, which makes no claim about completion at all.
		{"not a completion widget", "_probe() { compadd checkout }\nzle -N probewid _probe\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := completionFor(t, c.src, "git che"); len(got) > 0 {
				t.Errorf("offered %q, want nothing — the editor completes its own way", got)
			}
		})
	}
}

// TestTheParametersACompletionWidgetsFunctionReads is the measurement in
// compsys.go's file comment, asked of this shell.
//
// The readings are printed by the function itself rather than asserted from
// outside, because what is being tested is what the *function* sees: a
// parameter this shell set on some other runner, or after the call, would pass
// an assertion made outside and still be invisible where it is needed.
func TestTheParametersACompletionWidgetsFunctionReads(t *testing.T) {
	for _, c := range []struct{ name, line, want string }{
		// Measured: `words` holds the words as typed, `CURRENT` is one-based,
		// `PREFIX` is the word and `SUFFIX` is empty.
		{
			"an argument", "git che",
			"words=git|che CURRENT=2 PREFIX=che SUFFIX= IPREFIX= ISUFFIX= " +
				"QIPREFIX= QISUFFIX= context=command",
		},
		// Measured: `context` is `command` in command position too, so it is
		// not the position of the word restated.
		{
			"the command word", "gi",
			"words=gi CURRENT=1 PREFIX=gi SUFFIX= IPREFIX= ISUFFIX= " +
				"QIPREFIX= QISUFFIX= context=command",
		},
		// Measured: a trailing blank is a word of its own — `git ` reports
		// two words, the second empty, and CURRENT 2.
		{
			"after a blank", "git ",
			"words=git| CURRENT=2 PREFIX= SUFFIX= IPREFIX= ISUFFIX= " +
				"QIPREFIX= QISUFFIX= context=command",
		},
		// Measured: the opening quote comes off PREFIX and is QIPREFIX, and
		// `words` keeps it — `echo "fo` is `words=(echo "fo)`, `PREFIX=fo`,
		// `QIPREFIX="`.
		{
			"inside double quotes", `echo "fo`,
			`words=echo|"fo CURRENT=2 PREFIX=fo SUFFIX= IPREFIX= ISUFFIX= ` +
				`QIPREFIX=" QISUFFIX= context=command`,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := completionFor(t, widgetOf(
				`compadd -U -- "words=${(j:|:)words} CURRENT=$CURRENT `+
					`PREFIX=$PREFIX SUFFIX=$SUFFIX IPREFIX=$IPREFIX `+
					`ISUFFIX=$ISUFFIX QIPREFIX=$QIPREFIX QISUFFIX=$QISUFFIX `+
					`context=$compstate[context]"`,
			), c.line)
			if len(got) != 1 {
				t.Fatalf("read back %q, want one line", got)
			}
			// `-U` was used so the reading is offered whatever it says, and
			// the quoting the editor would put on it is undone here rather
			// than written into every expectation — the backslashes, and the
			// opening quote a word typed inside one keeps. See
			// docs/spec/completion.md, where both are the editor's rule.
			unquoted := strings.TrimLeft(strings.ReplaceAll(got[0], `\`, ""), `"'`)
			if unquoted != c.want {
				t.Errorf("function saw\n %s\nwant\n %s", unquoted, c.want)
			}
		})
	}
}

// TestCompstateAndTheWordAreWritableByTheFunction is the other half of the
// parameters: a completion function does not only read them.
//
// `_arguments` rewrites `$words` and `$CURRENT`, `_normal` writes `$PREFIX`,
// and `$compstate` is how a widget asks for something other than an insertion.
// A parameter this shell produced but would not take an assignment to is one
// those functions stop on, and the failure is silent — the assignment goes
// somewhere and the next read finds the old value.
func TestCompstateAndTheWordAreWritableByTheFunction(t *testing.T) {
	got := completionFor(t, widgetOf(
		"PREFIX=ch; compstate[insert]=menu; words=(one two); CURRENT=2; "+
			`compadd -U -- "PREFIX=$PREFIX insert=$compstate[insert] `+
			`words=${(j:|:)words} CURRENT=$CURRENT"`,
	), "git che")
	want := "PREFIX=ch insert=menu words=one|two CURRENT=2"
	if len(got) != 1 || strings.ReplaceAll(got[0], `\`, "") != want {
		t.Errorf("read back %q, want %q", got, want)
	}
}

// TestTheCompletionParametersAreGoneAfterwards is the discipline
// runWidgetFunction keeps for `$BUFFER`, asked of these nine.
//
// A `$PREFIX` still standing at the next prompt is the kind of wrong nobody
// would connect to the Tab that left it there, and `$words` is a name a
// script is entitled to use for something else.
func TestTheCompletionParametersAreGoneAfterwards(t *testing.T) {
	r := bindkeyRunner(t, widgetOf("compadd checkout"))
	zsh.RunCompletion(r, t.Context(), "probewid", repl.Completion{
		Line: "git che", Point: 7, Start: 4, Word: "che", Dir: r.Dir,
	})
	for _, name := range []string{
		"PREFIX", "SUFFIX", "IPREFIX", "ISUFFIX",
		"QIPREFIX", "QISUFFIX", "CURRENT", "words", "compstate",
	} {
		if value, set := r.GetVar(name); set && value != "" {
			t.Errorf("$%s is %q after the completion, want nothing", name, value)
		}
	}
}

// TestTheseBuiltinsRefuseOutsideACompletion is the refusal `zle` outside a
// widget gives, one word to the left.
//
// Measured on zsh 5.9.2 with `zmodload zsh/complete` and `zmodload
// zsh/computil` first, so the builtins really are present: `compadd x`,
// `compadd` alone, `compadd -Z x` and `compset -p 1` are all `can only be
// called from completion function` at status 1. The bad option is among
// them, which says the context is checked before the letters.
//
// **`compset` alone is not.** It was in this list and it does not belong:
// re-measured 2026-09-16, a bare `compset` is `not enough arguments`,
// because zsh checks the word count in the dispatcher before the builtin
// runs and `compset` declares a minimum of one where `compadd` declares
// none. That half is TestTheCompletionBuiltinsCountTheirWordsFirst in
// computil_test.go; what is left here is the refusal about the *place*,
// which is only reached by a call that satisfies the count.
func TestTheseBuiltinsRefuseOutsideACompletion(t *testing.T) {
	for _, src := range []string{
		"compadd x", "compadd", "compadd -Z x", "compset -p 1",
	} {
		out, status := answersRun(t, src+`; echo "st=$?"`)
		if !strings.Contains(out, "can only be called from completion function") {
			t.Errorf("%s said %q", src, out)
		}
		if !strings.Contains(out, "st=1") {
			t.Errorf("%s left %q, status %d, want st=1", src, out, status)
		}
	}
}

// TestTheModuleGateMovedWithTheBuiltins is the property zmodload.go's comment
// promises: "the gate opens by itself. The day a module's parameters exist,
// `zmodload` starts succeeding for it with no change here."
//
// `zmodload -F zsh/complete b:compadd` is a script naming the one feature it
// wants, and it is the shape a real guard is written in — `zmodload -F
// zsh/stat b:zstat || return` is a line from a prompt theme. That question is
// answered 0 now because the builtin exists, and nothing in the module table
// was edited to make it so.
//
// The plain `zmodload zsh/complete` was still refused after this, and the
// refusal named what was left: the four conditions, which #3042 then
// answered. So the whole module loads now, and the narrowed forms still do —
// both halves are asserted, because a change that registered the module
// wholesale would pass the second and be a silent success about the builtins.
func TestTheModuleGateMovedWithTheBuiltins(t *testing.T) {
	out, _ := answersRun(t, `zmodload -F zsh/complete b:compadd; echo "named=$?"`+"\n"+
		`zmodload -F zsh/complete b:compset; echo "set=$?"`+"\n"+
		`zmodload zsh/complete; echo "whole=$?"`)
	for _, want := range []string{"named=0", "set=0", "whole=0"} {
		if !strings.Contains(out, want) {
			t.Errorf("want %s in %q", want, out)
		}
	}
	if strings.Contains(out, "not implemented yet") {
		t.Errorf("nothing of this module is missing now; it said %q", out)
	}
}
