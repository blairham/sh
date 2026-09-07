// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"
)

// Calling a function from outside the script.
//
// The call a front end needs and a script never does: a prompt loop that runs
// a hook has a name it read out of a variable and arguments it computed, and
// no shell text mentioning either. See repl's hooks.go for what asks.

// The arguments arrive as the positional parameters, untouched.
//
// Untouched is the claim that matters. The other way to reach a function from
// outside is to build the text of a call and parse it, and that puts every
// argument back through the grammar — a hook told `echo  a   b` would be told
// `echo a b`, or worse, would run the substitutions in what somebody typed.
func TestAFunctionCanBeCalledWithArgumentsFromOutside(t *testing.T) {
	var out, errs strings.Builder
	r := seamRunner(t, &out, &errs)
	if !r.DefineFunctionFromText("h", `printf '[%s]' "$#" "$1" "$2"`) {
		t.Fatal("defining h")
	}

	called, err := r.CallFunction(t.Context(), "h", `echo  a   b`, "$(uname)")
	if err != nil {
		t.Fatalf("calling h: %v", err)
	}
	if !called {
		t.Fatal("the function was not called")
	}

	const want = "[2][echo  a   b][$(uname)]"
	if got := out.String(); got != want {
		t.Errorf("h was told %q, want %q", got, want)
	}
}

// Only a function is one. A name with nothing behind it, and a name that is a
// builtin, are both answered `false` and neither runs.
//
// Measured on 2026-09-07, zsh 5.9.2 through a pseudo-terminal, with
// `precmd_functions=(precmd builtin_echo_zz print /bin/echo pcX pcX)`: the
// builtin and the file on the path printed nothing, the undefined name said
// nothing, and the two real functions ran — one of them twice.
func TestOnlyAFunctionIsCallableByName(t *testing.T) {
	for _, name := range []string{"nosuchfn_zz", "echo", "true"} {
		t.Run(name, func(t *testing.T) {
			var out, errs strings.Builder
			r := seamRunner(t, &out, &errs)

			called, err := r.CallFunction(t.Context(), name, "arg")
			if err != nil {
				t.Fatalf("calling %s: %v", name, err)
			}
			if called {
				t.Errorf("%s was reported called, want not", name)
			}
			if out.String() != "" || errs.String() != "" {
				t.Errorf("%s printed %q / %q, want nothing", name, out.String(), errs.String())
			}
		})
	}
}

// The status is the function's, left where a call in a script leaves it. A
// caller that must not disturb `$?` is the one that saves it.
func TestACalledFunctionLeavesItsStatusBehind(t *testing.T) {
	var out, errs strings.Builder
	r := seamRunner(t, &out, &errs)
	if !r.DefineFunctionFromText("h", `return 3`) {
		t.Fatal("defining h")
	}
	r.SetExitStatus(7)

	if _, err := r.CallFunction(t.Context(), "h"); err != nil {
		t.Fatalf("calling h: %v", err)
	}

	if got := r.ExitStatus(); got != 3 {
		t.Errorf("$? after the call is %d, want 3", got)
	}
}

// The call leaves the shell open. A `return` unwinds the function and nothing
// else, exactly as it does inside a script.
func TestACalledFunctionsReturnDoesNotEndTheShell(t *testing.T) {
	var out, errs strings.Builder
	r := seamRunner(t, &out, &errs)
	if !r.DefineFunctionFromText("h", `echo in; return 1; echo unreachable`) {
		t.Fatal("defining h")
	}

	if _, err := r.CallFunction(t.Context(), "h"); err != nil {
		t.Fatalf("calling h: %v", err)
	}
	if r.Exited() {
		t.Error("the shell exited on a return")
	}
	runSeam(t, r, "echo after")

	if got := out.String(); got != "in\nafter\n" {
		t.Errorf("the runner printed %q, want %q", got, "in\nafter\n")
	}
}

// And `exit` inside one does end it, which is what a hook that exits has to be
// able to do — measured, a zsh `precmd` calling `exit` ends the session and
// draws no further prompt.
func TestACalledFunctionCanExitTheShell(t *testing.T) {
	var out, errs strings.Builder
	r := seamRunner(t, &out, &errs)
	if !r.DefineFunctionFromText("h", `exit 3`) {
		t.Fatal("defining h")
	}

	if _, err := r.CallFunction(t.Context(), "h"); err != nil {
		t.Fatalf("calling h: %v", err)
	}
	if !r.Exited() {
		t.Error("the shell did not exit")
	}
	if got := r.ExitStatus(); got != 3 {
		t.Errorf("the exit status is %d, want 3", got)
	}
}

// Whether a name is a function, asked without calling it.
func TestAFunctionCanBeSeenWithoutCallingIt(t *testing.T) {
	var out, errs strings.Builder
	r := seamRunner(t, &out, &errs)
	if !r.DefineFunctionFromText("h", `echo ran`) {
		t.Fatal("defining h")
	}

	for name, want := range map[string]bool{"h": true, "echo": false, "nosuchfn_zz": false} {
		if got := r.HasFunction(name); got != want {
			t.Errorf("HasFunction(%q) = %v, want %v", name, got, want)
		}
	}
	if out.String() != "" {
		t.Errorf("asking ran something: %q", out.String())
	}

	r.RemoveFunction("h")
	if r.HasFunction("h") {
		t.Error("a removed function is still reported")
	}
}
