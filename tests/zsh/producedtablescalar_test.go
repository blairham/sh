// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"
)

// The sentence a **produced** table earns, which is not the stored table's.
const producedTableRefusal = "attempt to set slice of associative array"

// A scalar store over a name this shell *produces* — `aliases`, `functions`,
// `commands`, `galiases` — is refused in **both** option states, and with
// `ksharrays` off it is refused where a stored table is not refused at all
// (#4639).
//
// Measured 2026-09-26 from a script file under `zsh -f f.sh`, zsh 5.9.2
// (aarch64-apple-darwin25.4.0):
//
//	                          ksharrays off              ksharrays on
//	aliases=string            aliases: attempt to set    aliases: attempt to
//	                          slice of associative       set associative
//	                          array, status 1, leaves    array to scalar
//	typeset -A h=(one 1)
//	h=string                  typeset h=string, 0        the same sentence
//
// So the **producer** is what decides here and not the option alone: the
// option only picks which of the two sentences is written. A grid that asked
// the produced name only under the option would have found the two columns
// agreeing and reported nothing.
func TestAScalarStoredOverAProducedTableIsRefused(t *testing.T) {
	for _, name := range []string{"aliases", "functions", "commands", "galiases"} {
		t.Run(name, func(t *testing.T) {
			for _, state := range []struct {
				name, setopt, want string
			}{
				{"off", "", producedTableRefusal},
				{"on", "setopt ksharrays\n", scalarOverTableRefusal},
			} {
				t.Run(state.name, func(t *testing.T) {
					out, st, errs := runZshSplit(t, t.TempDir(),
						state.setopt+name+"=string\nprint reached\n")
					if !strings.Contains(errs, name+": "+state.want) {
						t.Errorf("stderr = %q, want %q with the name in front", errs, state.want)
					}
					if out != "" {
						t.Errorf("stdout = %q, want nothing — the shell leaves", out)
					}
					if st != 1 {
						t.Errorf("status = %d, want 1", st)
					}
				})
			}
		})
	}
}

// **The stored table is the control that says the producer decides**, and
// without it every row above reads as "a scalar over a table is refused",
// which is the thing #4617 measured as false with the option off.
func TestAStoredTableStillTakesAScalarWithTheOptionOff(t *testing.T) {
	out, st, errs := runZshSplit(t, t.TempDir(),
		"typeset -A h=(one 1)\nh=string\ntypeset -p h\nprint reached\n")
	if want := "typeset h=string\nreached\n"; out != want {
		t.Errorf("stdout = %q, want %q (stderr %q)", out, want, errs)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
	if errs != "" {
		t.Errorf("stderr = %q, want nothing", errs)
	}
}

// The reach, re-measured against the stored table's own. Every route that
// sets a name earns it, `||` does not catch it, an `eval` contains it, a
// function's name goes in front of the sentence, and an `unset` first makes
// the store an ordinary scalar.
//
// **The last three rows are the controls**, and they are what keep the noun
// from being "any write to a produced table": a subscripted store writes an
// alias, a keyed literal fills the table, and the produced *index* array
// beside it takes a scalar without a word.
func TestTheProducedTableRefusalReachesEveryScalarStore(t *testing.T) {
	for _, c := range []struct {
		name, src string
		// refused says the row earns the sentence and ends the shell; the
		// controls below are the rows that do not.
		refused bool
		// want is stdout where the row is not refused.
		want   string
		status int
	}{
		{"an append", "aliases+=string\nprint reached", true, "", 1},
		{"a declaration", "typeset aliases=string\nprint reached", true, "", 1},
		{"an empty value", "aliases=\nprint reached", true, "", 1},
		{"read", "print x | read aliases\nprint reached", true, "", 1},
		{"printf -v", "printf -v aliases x\nprint reached", true, "", 1},
		{"a for word list", "for aliases in x; do :; done\nprint reached", true, "", 1},
		{
			// `||` does not catch it: the shell is leaving rather than
			// reporting a failed command.
			"or-else does not catch it",
			"aliases=string || print caught\nprint reached", true, "", 1,
		},

		{
			// A subscripted store is not this question at all.
			"a subscripted store writes an alias",
			"aliases[k]=v\nprint -r -- $aliases[k]", false, "v\n", 0,
		},
		{
			// Nor is a keyed literal, which has somewhere for its elements
			// to go.
			"a keyed literal fills the table",
			"aliases=(k v)\nprint -r -- $aliases[k]", false, "v\n", 0,
		},
		{
			// And the produced *index* array beside it takes a scalar
			// without a word, which is what says this is about a table.
			"a produced index array takes a scalar",
			"path=string\nprint reached", false, "reached\n", 0,
		},
		{
			// An `unset` first and the name is an ordinary one again — the
			// same thing an `unset` does to a stored table.
			"unset first and nothing is refused",
			"unset aliases\naliases=string\nprint reached", false, "reached\n", 0,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st, errs := runZshSplit(t, t.TempDir(), c.src)
			if c.refused != strings.Contains(errs, "aliases: "+producedTableRefusal) {
				t.Errorf("stderr = %q, refused = %v", errs, c.refused)
			}
			if out != c.want {
				t.Errorf("stdout = %q, want %q (stderr %q)", out, c.want, errs)
			}
			if st != c.status {
				t.Errorf("status = %d, want %d", st, c.status)
			}
		})
	}
}

// An `eval` contains it: the sentence is written, the eval reports it, and the
// script carries on — which is the boundary every other fatal of this shell
// already draws, and the row that says the refusal is an **error** rather than
// a request to stop.
func TestAProducedTableRefusalInsideAnEvalDoesNotEndTheScript(t *testing.T) {
	out, st, errs := runZshSplit(t, t.TempDir(), "eval \"aliases=string\"\nprint reached\n")
	if !strings.Contains(errs, "aliases: "+producedTableRefusal) {
		t.Errorf("stderr = %q, want the refusal", errs)
	}
	if out != "reached\n" {
		t.Errorf("stdout = %q, want the script to carry on", out)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}

// And a function's name goes in front of the sentence where the file and line
// otherwise do, exactly as the stored table's refusal does.
func TestAProducedTableRefusalInsideAFunctionNamesTheFunction(t *testing.T) {
	_, st, errs := runZshSplit(t, t.TempDir(), "f() { aliases=string; }\nf\nprint reached\n")
	if !strings.HasPrefix(errs, "f: aliases: "+producedTableRefusal) {
		t.Errorf("stderr = %q, want it to open with the function's name", errs)
	}
	if st != 1 {
		t.Errorf("status = %d, want 1", st)
	}
}
