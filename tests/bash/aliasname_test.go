// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"bytes"
	"testing"

	"github.com/blairham/sh/driver"
)

// What this shell will and will not take as an alias **name**, measured on
// bash 5.3.15 on 2026-09-12 by defining `alias '<name>'=echo` for every
// printable ASCII character in turn (#2413).
//
// The set is a value rather than an axis because ksh93 checks too and refuses
// five characters more — see Semantics.AliasNameRefusedCharacters — so a row
// here is only about this column.

func runAlias(t *testing.T, src string) (string, string, int) {
	t.Helper()
	var out, errs bytes.Buffer
	code := driver.MainArgs(bashShell(&out, &errs), []string{"bash", "-c", src})
	return out.String(), errs.String(), code
}

func TestAnAliasNameHoldingARefusedCharacter(t *testing.T) {
	for _, c := range []struct {
		name, src, out, errs string
	}{
		// The complaint quotes the *name* and not the operand, and the
		// builtin answers 1 with nothing defined.
		{"a dollar", `alias 'a$b'=echo; echo "st=$?"; alias`, "st=1\n", "bash: line 1: alias: `a$b': invalid alias name\n"},
		{"a space", `alias 'a b'=echo; echo "st=$?"; alias`, "st=1\n", "bash: line 1: alias: `a b': invalid alias name\n"},
		{"a slash", `alias 'a/b'=echo; echo "st=$?"; alias`, "st=1\n", "bash: line 1: alias: `a/b': invalid alias name\n"},
		{"a pipe", `alias 'a|b'=echo; echo "st=$?"; alias`, "st=1\n", "bash: line 1: alias: `a|b': invalid alias name\n"},
		// The five ksh93 refuses beside those are taken here, and this is
		// the row that says the two sets are different.
		{"a star", `alias 'a*b'=echo; alias`, "alias a*b='echo'\n", ""},
		{"a question mark", `alias 'a?b'=echo; alias`, "alias a?b='echo'\n", ""},
		{"an open bracket", `alias 'a[b'=echo; alias`, "alias a[b='echo'\n", ""},
		{"a brace", `alias 'a{b'=echo; alias`, "alias a{b='echo'\n", ""},
		// And `=` is not refusable: the first one is the separator, so the
		// name that reaches the check is `a` and the value is `b=echo`.
		{"an equals", `alias 'a=b'=echo; alias`, "alias a='b=echo'\n", ""},
		// A refusal costs its own operand and nothing beside it.
		{"one refused among three", `alias 'a$b'=echo x=1 'a b'=echo; echo "st=$?"; alias`, "st=1\nalias x='1'\n", "bash: line 1: alias: `a$b': invalid alias name\nbash: line 1: alias: `a b': invalid alias name\n"},
		// A bare lookup is *not* checked here: the name goes on to be looked
		// up and answered the way any name the table does not hold is.
		{"a lookup is not checked", `alias 'a$b'; echo "st=$?"`, "st=1\n", "bash: line 1: alias: a$b: not found\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs, code := runAlias(t, c.src)
			if out != c.out || errs != c.errs || code != 0 {
				t.Errorf("ran %q: out %q errs %q status %d, want %q and %q",
					c.src, out, errs, code, c.out, c.errs)
			}
		})
	}
}

// And the script survives it. The builtin's own status is 1, so only the line
// after the refusal can tell this column from the one that ends the script.
func TestARefusedAliasNameDoesNotEndTheScript(t *testing.T) {
	out, errs, code := runAlias(t, `echo one; alias 'a$b'=echo; echo two`)
	if out != "one\ntwo\n" || code != 0 {
		t.Errorf("out %q status %d, want both lines and 0", out, code)
	}
	if errs != "bash: line 1: alias: `a$b': invalid alias name\n" {
		t.Errorf("errs %q, want the one complaint", errs)
	}
}

// And the *other* place an alias's text reaches a diagnostic: when a
// construct inside an alias body is refused, the line echoed back is the
// alias's own text and not the line the script wrote.
//
// Measured on bash 5.3.15, 2026-09-12. The three arrangements below are what
// says it is the borrowed text alone rather than the line reconstructed with
// the expansion in it: nothing the script wrote around the alias word appears
// in any of them, not the commands beside it and not the arguments after it.
// Ours echoed the script's line, which for a line holding nothing but the
// alias word looks like the alias word (#2413).
func TestARefusedConstructInAnAliasEchoesTheAliasText(t *testing.T) {
	const head = "shopt -s expand_aliases\nalias f=\"a= (x y)\"\n"
	for _, c := range []struct {
		name, last string
	}{
		{"the alias alone", "f\n"},
		{"commands beside it", "echo hi; f; echo bye\n"},
		{"arguments after it", "f arg1 arg2\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, errs, code := runAlias(t, head+c.last)
			want := "bash: -c: line 3: syntax error near unexpected token `('\n" +
				"bash: -c: line 3: `a= (x y)'\n"
			if errs != want || code != 2 {
				t.Errorf("errs %q status %d, want %q status 2", errs, code, want)
			}
		})
	}
}

// The control, and it is the row that says the change did not simply replace
// one text with another everywhere: a failure with no alias in it still
// echoes the line the script wrote.
func TestARefusedConstructWithNoAliasEchoesTheLine(t *testing.T) {
	_, errs, _ := runAlias(t, "echo one\necho two; a= (x y)\n")
	want := "bash: -c: line 2: syntax error near unexpected token `('\n" +
		"bash: -c: line 2: `echo two; a= (x y)'\n"
	if errs != want {
		t.Errorf("errs %q, want %q", errs, want)
	}
}
