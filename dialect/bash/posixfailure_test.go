// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// What POSIX mode makes fatal that this shell's own name does not, each row
// measured on bash 5.3.20 on 2026-09-22 from a script file, under `set -o
// posix` and under the `sh` name alike, with `set +o posix` measured as the
// way back.
//
// The pairing matters and is why every row carries a line behind the failure:
// on one line, giving up the command and giving up the shell print the same
// nothing. #4166 is the row these came out of — `errors.tests` ran to
// completion under both shells, exited 0 under both, and printed eight extra
// lines because this shell carried on where bash had stopped.
func TestPosixModeEndsTheScriptWhereBashCarriesOn(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
	}{
		{
			// A bad *name* to a special builtin. Three builtins, because the
			// two axes behind them are two — `unset` is asked separately.
			"a bad name to readonly ends the script",
			"set -o posix\nreadonly non-ident\necho gone\n",
			"sh: line 2: readonly: `non-ident': not a valid identifier\n",
		},
		{
			"a bad name to export ends the script",
			"set -o posix\nexport non-ident\necho gone\n",
			"sh: line 2: export: `non-ident': not a valid identifier\n",
		},
		{
			"a bad name to unset -v ends the script",
			"set -o posix\nunset -v a-b\necho gone\n",
			"sh: line 2: unset: `a-b': not a valid identifier\n",
		},
		{
			// A subscripted operand is the same refusal one branch over, so
			// it moves with the same axis rather than needing one of its own.
			"a subscripted operand ends it too",
			"set -o posix\nreadonly AA[4]\necho gone\n",
			"sh: line 2: readonly: `AA[4]': not a valid identifier\n",
		},
		{
			"leaving POSIX mode puts the carrying-on back",
			"set -o posix\nset +o posix\nreadonly non-ident\necho after=$?\n" +
				"export non-ident\necho after=$?\n" +
				"unset -v a-b\necho after=$?\n",
			"sh: line 3: readonly: `non-ident': not a valid identifier\nafter=1\n" +
				"sh: line 5: export: `non-ident': not a valid identifier\nafter=1\n" +
				"sh: line 7: unset: `a-b': not a valid identifier\nafter=1\n",
		},
		{
			// `return` with nowhere to return from: the same special
			// builtin's failure, and bash is the only column that refuses it
			// at all rather than returning from the script.
			"a return with nothing to return from ends the script",
			"set -o posix\nreturn\necho gone\n",
			"sh: line 2: return: can only `return' from a function or sourced script\n",
		},
		{
			"and carries on again outside the mode",
			"set -o posix\nset +o posix\nreturn\necho after=$?\n",
			"sh: line 3: return: can only `return' from a function or sourced script\nafter=2\n",
		},
		{
			// The same rule one phase earlier: an expansion the shell cannot
			// perform ends it rather than costing the line.
			"a bad substitution ends the script",
			"set -o posix\necho pre\necho ${#+}\necho gone\n",
			"pre\nsh: line 3: ${#+}: bad substitution\n",
		},
		{
			"a division by zero ends the script",
			"set -o posix\necho pre\necho $((1/0))\necho gone\n",
			"pre\nsh: line 3: 1/0: division by 0 (error token is \"0\")\n",
		},
		{
			"and a failed expansion costs only the line outside the mode",
			"set -o posix\nset +o posix\necho pre\necho ${#+}\necho after\n",
			"pre\nsh: line 4: ${#+}: bad substitution\nafter\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := answersRun(t, tc.src)
			if out != tc.want {
				t.Errorf("got\n%s\nwant\n%s", out, tc.want)
			}
		})
	}
}

// TestPosixModeTurnsShiftVerboseOn is the one thing the mode moves that is
// not on the semantics vector: the *withholding* of `shift`'s complaint is a
// capability the runner holds, and `shopt shift_verbose` is this shell's
// spelling of it.
//
// Measured 2026-09-22 on bash 5.3.20: the listing reads `off` under this
// shell's own name, `on` after `set -o posix`, and `off` again after `set +o
// posix`; with it on, `shift` past the end names the count and still leaves 1.
func TestPosixModeTurnsShiftVerboseOn(t *testing.T) {
	out, _ := answersRun(t,
		"shopt shift_verbose\nset -o posix\nshopt shift_verbose\n"+
			"shift 12\necho a=$?\n"+
			"set +o posix\nshopt shift_verbose\nshift 12\necho b=$?\n")
	want := "shift_verbose       \toff\n" +
		"shift_verbose       \ton\n" +
		"sh: line 4: shift: 12: shift count out of range\na=1\n" +
		"shift_verbose       \toff\n" +
		"b=1\n"
	if out != want {
		t.Errorf("got\n%s\nwant\n%s", out, want)
	}
}

// TestALengthPrefixOverABareOperatorIsRefused pins the reading of `${#+}`.
//
// `+` and `=` cannot begin a name, so behind a `${#` they are scanned as the
// name the length is over, no name comes out, and the whole expansion is
// refused — which is what bash 5.3.20, bash 3.2.57, dash and ksh93 all do.
// With an *operand* the `#` is the parameter `$#` and every column agrees, so
// the split is the empty word rather than the operator. See
// syntax.Dialect.ParamLengthBareOperatorIsTheParameter for the panel.
//
// The failure it came from is the interesting half: refusing nothing left
// `echo ${#+} second` printing `second` at status 0 where bash prints neither
// (#4166).
func TestALengthPrefixOverABareOperatorIsRefused(t *testing.T) {
	out, _ := answersRun(t,
		"set -- p q\n"+
			"echo \"[${#+}]\" second\necho a=$?\n"+
			"echo \"[${#=}]\" second\necho b=$?\n"+
			"echo \"[${#+w}]\"\necho \"[${#=w}]\"\n")
	want := "sh: line 2: [${#+}]: bad substitution\na=1\n" +
		"sh: line 4: [${#=}]: bad substitution\nb=1\n" +
		"[w]\n[2]\n"
	if out != want {
		t.Errorf("got\n%s\nwant\n%s", out, want)
	}
}

// TestADefinitionsRefusalIsLocatedAtItsEnd pins where a function definition's
// own complaints are numbered.
//
// This shell reads the whole definition before it runs any of it and reports
// where its reader had got to, which is the definition's last line and not
// the line its name stands on. Measured 2026-09-22 on bash 5.3.20 and 3.2.57
// alike; see interp.Diagnostics.FunctionDefinitionIsLocatedAtItsEnd for the
// five spellings.
func TestADefinitionsRefusalIsLocatedAtItsEnd(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
	}{
		{
			"a name that is not a name, over four lines",
			"$1 ()\n{\necho x\n}\necho after\n",
			"sh: line 4: `$1': not a valid identifier\nafter\n",
		},
		{
			"and on one line it is that line",
			"$1 () { echo x; }\necho after\n",
			"sh: line 1: `$1': not a valid identifier\nafter\n",
		},
		{
			"a redefinition of a frozen name, over four lines",
			"f() { :; }\nreadonly -f f\nf ()\n{\necho x\n}\necho after\n",
			"sh: line 6: f: readonly function\nafter\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := answersRun(t, tc.src)
			if out != tc.want {
				t.Errorf("got\n%s\nwant\n%s", out, tc.want)
			}
		})
	}
}

// TestASelectsRefusedNameIsLocatedAtTheReader pins the one construct that is
// numbered from where this shell's *reader* stands rather than from the
// clause that raised the complaint.
//
// The `for` spelling is the control: the same refusal in the same shell is
// located at its own clause there, which is what makes this a fact about
// `select` rather than about the sentence. See
// interp.Diagnostics.SelectNameIsLocatedAtTheReader for the table and for the
// one rule all of these rows are (#4166).
func TestASelectsRefusedNameIsLocatedAtTheReader(t *testing.T) {
	for _, tc := range []struct {
		name, src, want string
	}{
		{
			"inside a top-level group, the group's last line",
			"{\necho pre\nselect $1 in a; do :; done\n}\necho after\n",
			"pre\nsh: line 4: `$1': not a valid identifier\nafter\n",
		},
		{
			"inside a function, the body's first line",
			"f()\n{\necho pre\nselect $1 in a; do :; done\n}\nf\necho after\n",
			"pre\nsh: line 2: `$1': not a valid identifier\nafter\n",
		},
		{
			"a nested group inside that body does not move it",
			"f()\n{\n{\nselect $1 in a; do :; done\n}\n}\nf\necho after\n",
			"sh: line 2: `$1': not a valid identifier\nafter\n",
		},
		{
			"a for clause does move it, even inside a function",
			"f()\n{\nfor i in 1\ndo\nselect $1 in a; do :; done\ndone\n}\nf\necho after\n",
			"sh: line 3: `$1': not a valid identifier\nafter\n",
		},
		{
			"the for spelling is located at its own clause",
			"f()\n{\necho pre\nfor $1 in a; do :; done\n}\nf\necho after\n",
			"pre\nsh: line 4: `$1': not a valid identifier\nafter\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := answersRun(t, tc.src)
			if out != tc.want {
				t.Errorf("got\n%s\nwant\n%s", out, tc.want)
			}
		})
	}
}
