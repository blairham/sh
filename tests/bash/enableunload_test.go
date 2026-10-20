// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"strings"
	"testing"
)

// `enable -d` is read here, and what it answers is about the **operand**.
//
// Every expected string is a transcript, measured 2026-09-18 on bash 5.3.20
// and again on bash 3.2.57, `env -i PATH=/usr/bin:/bin LC_ALL=C` with a
// scratch HOME. It was `enable: -d: invalid option` and a usage block at 2
// here, which is two whole lines per call apart from the real shell in a
// script that probes for the feature (#3057).
//
// Nothing is loadable in this shell and nothing has to become loadable for the
// letter to parse: every name reaches one of the two sentences below, and
// neither is ever wrong for the shell it is asked of.
func TestEnableDeleteAnswersAboutTheOperand(t *testing.T) {
	for _, c := range []struct{ src, want string }{
		// A name that is not a builtin at all.
		{"enable -d notbuiltin", "enable: notbuiltin: not a shell builtin"},
		// A builtin that came from nowhere, which is every builtin here.
		{"enable -d echo", "enable: echo: not dynamically loaded"},
		{"enable -d cd", "enable: cd: not dynamically loaded"},
		// The letter wins over `-n`, which would otherwise switch the name
		// off in silence.
		{"enable -dn echo", "enable: echo: not dynamically loaded"},
	} {
		t.Run(c.src, func(t *testing.T) {
			out, errs := runOptionScript(t, c.src+"\n")
			if out != "" {
				t.Errorf("wrote %q to standard output, want nothing", out)
			}
			if !strings.Contains(errs, c.want) {
				t.Errorf("said %q, want it to carry %q", errs, c.want)
			}
			if strings.Contains(errs, "invalid option") || strings.Contains(errs, "usage:") {
				t.Errorf("said %q, want no complaint about the letter and no usage block", errs)
			}
		})
	}
	// Every operand is answered and the status is the builtin's own 1, not
	// the 2 a refused option carries.
	out, errs := runOptionScript(t, "enable -d echo cd nope\necho status=$?\n")
	for _, want := range []string{
		"enable: echo: not dynamically loaded",
		"enable: cd: not dynamically loaded",
		"enable: nope: not a shell builtin",
	} {
		if !strings.Contains(errs, want) {
			t.Errorf("said %q, want it to carry %q", errs, want)
		}
	}
	if out != "status=1\n" {
		t.Errorf("status %q, want 1 — the builtin's own, not an option's 2", out)
	}
}

// The letter with no operand adds nothing, which is what says it is read and
// then falls through rather than becoming a listing of its own.
func TestEnableDeleteWithNoOperandIsTheOrdinaryListing(t *testing.T) {
	plain, errs := runOptionScript(t, "enable\n")
	if errs != "" {
		t.Fatalf("a plain listing complained: %q", errs)
	}
	deleted, errs := runOptionScript(t, "enable -d\n")
	if errs != "" {
		t.Fatalf("the `-d` listing complained: %q", errs)
	}
	if plain != deleted {
		t.Errorf("`enable -d` wrote a different listing:\n plain %q\n  -d   %q", plain, deleted)
	}
	if plain == "" {
		t.Error("the listing is empty, so the comparison above proves nothing")
	}
}
