// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix

package bash_test

import (
	"strconv"
	"strings"
	"syscall"
	"testing"
)

// A definition inside the command a killed notice writes back is spelled as a
// nested one, `function f () `, the way `declare -f` writes a definition
// inside a body, and not `f () ` as the whole-script listing writes one at its
// top (#7419). Measured 2026-11-19 under `-c`, the shell in a process group of
// its own, against GNU bash 5.3.20(1)-release, which writes the keyword for
// both spellings of the definition.
func TestADefinitionInAKilledNoticeIsWrittenAsANestedOne(t *testing.T) {
	usr1 := strconv.Itoa(128 + int(syscall.SIGUSR1))
	for _, tc := range []struct{ name, src, command string }{
		{
			"the parentheses spelling",
			`( f() { :; }; kill -USR1 $BASHPID ); echo st=$?`,
			"( function f () \n{ \n    :\n}; kill -USR1 $BASHPID )\n",
		},
		{
			"the keyword spelling",
			`( function g { :; }; kill -USR1 $BASHPID ); echo st=$?`,
			"( function g () \n{ \n    :\n}; kill -USR1 $BASHPID )\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errs := runBashCWithin(t, tc.src)
			if want := "st=" + usr1 + "\n"; out != want {
				t.Errorf("stdout = %q, want %q", out, want)
			}
			if !strings.HasPrefix(errs, "bash: line 1: ") || !strings.HasSuffix(errs, "  "+tc.command) {
				t.Errorf("stderr = %q, want the located notice ending %q", errs, tc.command)
			}
		})
	}
	// And `declare -f`, whose nested definition is the spelling borrowed, is
	// unchanged by the borrowing.
	out, _ := runBashCWithin(t, `g() { f() { :; }; f; }; declare -f g`)
	if want := "g () \n{ \n    function f () \n    { \n        :\n    };\n    f\n}\n"; out != want {
		t.Errorf("declare -f = %q, want %q", out, want)
	}
}
