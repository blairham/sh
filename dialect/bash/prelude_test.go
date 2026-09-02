// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/bash"
)

// A shell that does not say which shell it is, is not that shell to the part
// of the world that checks: bats and brew both refuse to run on the strength
// of $BASH_VERSION alone, without finding out whether the shell could have
// run them.
func TestTheDialectNamesItself(t *testing.T) {
	p := bash.Prelude()
	for _, want := range []string{"BASH_VERSION=", "BASH_VERSINFO=", "BASH=$0"} {
		if !strings.Contains(p, want) {
			t.Errorf("prelude has no %s:\n%s", want, p)
		}
	}
}

// Two things are true of the version at once, and the tag is what makes them
// both true: a numeric test parses the digits and passes, and anything that
// prints the string sees at once that it is not upstream bash.
func TestTheVersionPassesANumericGateAndSaysWhoseItIs(t *testing.T) {
	m := regexp.MustCompile(`BASH_VERSION='(\d+)\.(\d+)\.(\d+)\((\d+)\)-(\w+)'`).FindStringSubmatch(bash.Prelude())
	if m == nil {
		t.Fatalf("no version in bash's shape:\n%s", bash.Prelude())
	}
	// The gates that turned up in the wild: bats wants >= 3.2 and brew's
	// relatives want >= 4.4. Asserting the major rather than the exact
	// number, because the number is meant to move.
	if m[1] < "4" {
		t.Errorf("major = %s, want one that clears the gates scripts actually write", m[1])
	}
	if m[5] == "release" {
		t.Error(`tag = "release", want this build named as itself rather than as upstream`)
	}
}

// The array and the string have to agree, or a script that reads one and
// tests the other gets two different answers from the same shell.
func TestTheVersionAndTheVersinfoAgree(t *testing.T) {
	p := bash.Prelude()
	ver := regexp.MustCompile(`BASH_VERSION='(\d+)\.(\d+)\.(\d+)\((\d+)\)-(\w+)'`).FindStringSubmatch(p)
	info := regexp.MustCompile(`BASH_VERSINFO=\((\d+) (\d+) (\d+) (\d+) (\w+) (\S+)\)`).FindStringSubmatch(p)
	if ver == nil || info == nil {
		t.Fatalf("could not read both:\n%s", p)
	}
	for i := 1; i <= 5; i++ {
		if ver[i] != info[i] {
			t.Errorf("field %d: version says %q and versinfo says %q", i, ver[i], info[i])
		}
	}
}
