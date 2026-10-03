// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// fakeLocales stands in for a machine's locale data: `here.UTF-8` and
// `latin.ISO8859-1` are installed, `ctype` exists only as a character type and
// so loads through LC_CTYPE and not as a whole locale, and every other name is
// missing.
func fakeLocales(asked *[]string) func(string, bool) (string, bool) {
	return func(name string, whole bool) (string, bool) {
		if asked != nil {
			*asked = append(*asked, name)
		}
		switch {
		case name == "here.UTF-8":
			return "UTF-8", true
		case name == "latin.ISO8859-1":
			return "ISO8859-1", true
		case name == "ctype" && !whole:
			return "UTF-8", true
		}
		return "", false
	}
}

// #5503: a name the machine has no data for leaves the shell in the locale it
// was in, and the encoding is the data's rather than the spelling's.
func TestALocaleTheMachineCannotLoadLeavesTheOneInForce(t *testing.T) {
	const x = `x=é; `
	for _, tc := range []struct {
		name, src, want string
	}{
		{"missing from the start is C", `LC_ALL=xx_XX.UTF-8; echo ${#x}`, "2"},
		{"an installed name is read", `LC_ALL=here.UTF-8; echo ${#x}`, "1"},
		{"missing after an installed one keeps it", `LC_ALL=here.UTF-8; echo ${#x}; LC_ALL=xx_XX.UTF-8; echo ${#x}`, "1\n1"},
		{"missing after C keeps C", `LC_ALL=here.UTF-8; echo ${#x}; LC_ALL=C; echo ${#x}; LC_ALL=xx_XX.UTF-8; echo ${#x}`, "1\n2\n2"},
		{"the data's codeset decides", `LC_ALL=latin.ISO8859-1; echo ${#x}`, "2"},
		{"a character type is not a locale", `LC_ALL=ctype; echo ${#x}`, "2"},
		{"but loads as a character type", `LC_CTYPE=ctype; echo ${#x}`, "1"},
		{"the same name through both", `LC_ALL=ctype; echo ${#x}; unset LC_ALL; LC_CTYPE=ctype; echo ${#x}`, "2\n1"},
		{"LANG is a whole locale too", `LANG=ctype; echo ${#x}; LANG=here.UTF-8; echo ${#x}`, "2\n1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, x+tc.src, func(r *Runner) {
				sem := *r.Semantics
				sem.MultibyteEncodingIsHonored = Yes
				r.Semantics = &sem
				r.LocaleCharset = fakeLocales(nil)
			})
			if got := strings.TrimSuffix(out, "\n"); got != tc.want {
				t.Errorf("%s: got %q, want %q", tc.src, got, tc.want)
			}
		})
	}
}

// Without the hook the spelling decides, which is the library's reading: a
// missing name is read as the codeset it names.
func TestWithNoLocaleHookTheNameIsReadAsWritten(t *testing.T) {
	out, _ := run(t, `x=é; LC_ALL=xx_XX.UTF-8; echo ${#x}`, func(r *Runner) {
		sem := *r.Semantics
		sem.MultibyteEncodingIsHonored = Yes
		r.Semantics = &sem
	})
	if got := strings.TrimSuffix(out, "\n"); got != "1" {
		t.Errorf("got %q, want 1", got)
	}
}

// The machine is asked once per name, not once per character.
func TestALocaleIsLookedUpOncePerName(t *testing.T) {
	var asked []string
	run(t, `x=éé; LC_ALL=here.UTF-8; echo ${#x} ${#x} ${#x}; LC_ALL=xx; echo ${#x} ${#x}`, func(r *Runner) {
		sem := *r.Semantics
		sem.MultibyteEncodingIsHonored = Yes
		r.Semantics = &sem
		r.LocaleCharset = fakeLocales(&asked)
	})
	if len(asked) != 2 || asked[0] != "here.UTF-8" || asked[1] != "xx" {
		t.Errorf("asked %q, want [here.UTF-8 xx]", asked)
	}
}
