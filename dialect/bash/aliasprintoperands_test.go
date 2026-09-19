// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import "testing"

// `alias -p` is the whole listing and stops reading: every operand behind the
// letter is thrown away.
//
// Measured 2026-09-19 on bash 5.3.20 at /opt/homebrew/bin/bash, each probe a
// script file under `env -i PATH=/usr/bin:/bin LC_ALL=C` with standard input
// on /dev/null. The reference column and this one now agree on every row.
//
// **Two aliases are defined in the rows that matter**, and that is what makes
// them rows rather than coincidences: with one defined, `alias -p a` cannot
// be told from a lookup of `a` that was quiet about a miss. That is exactly
// how the narrower reading in #3701 — "`-p` suppresses the not-found report"
// — fit every probe it was measured on and was still the wrong rule.
func TestThePrintOptionIsTheWholeAliasListing(t *testing.T) {
	for _, c := range []struct{ name, src, out, errs string }{
		{
			"a name the table holds does not narrow the listing",
			`alias a=1 b=2; alias -p a; echo "st=$?"`,
			"alias a='1'\nalias b='2'\nst=0\n", "",
		},
		{
			"a name it has not got is neither reported nor counted",
			`alias a=1 b=2; alias -p nosuch; echo "st=$?"`,
			"alias a='1'\nalias b='2'\nst=0\n", "",
		},
		{
			"a definition behind the letter defines nothing",
			`alias -p z=1; echo "st=$?"; alias; echo end`,
			"st=0\nend\n", "",
		},
		{
			"and does not redefine a name already held",
			`alias z=1; alias -p z=2; alias`,
			"alias z='1'\nalias z='1'\n", "",
		},
		{
			"a name this shell would otherwise refuse is not checked",
			`alias -p 'a b'=echo; echo "st=$?"`,
			"st=0\n", "",
		},
		{
			"the letter written twice is still the letter",
			`alias -pp nosuch; echo "st=$?"`,
			"st=0\n", "",
		},
		// The controls. Without the letter the same operands filter,
		// report and define, and a separator is not this letter.
		{
			"the same name with nothing in front of it",
			`alias a=1 b=2; alias a; echo "st=$?"`,
			"alias a='1'\nst=0\n", "",
		},
		{
			"and behind a separator",
			`alias a=1 b=2; alias -- a; echo "st=$?"`,
			"alias a='1'\nst=0\n", "",
		},
		{
			"a missing name behind the separator reports and counts",
			`alias -- nosuch; echo "st=$?"`,
			"st=1\n", "bash: line 1: alias: nosuch: not found\n",
		},
		{
			"a missing name with no option at all",
			`alias nosuch; echo "st=$?"`,
			"st=1\n", "bash: line 1: alias: nosuch: not found\n",
		},
		{
			"the bare letter is that same whole listing",
			`alias a=1 b=2; alias -p; echo "st=$?"`,
			"alias a='1'\nalias b='2'\nst=0\n", "",
		},
		{
			"a name this shell refuses, with no letter, is still refused",
			`alias 'a b'=echo; echo "st=$?"`,
			"st=1\n", "bash: line 1: alias: `a b': invalid alias name\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, errs, _ := runAlias(t, c.src)
			if out != c.out || errs != c.errs {
				t.Errorf("%s =\nstdout %q stderr %q\nwant\nstdout %q stderr %q", c.src, out, errs, c.out, c.errs)
			}
		})
	}
}
