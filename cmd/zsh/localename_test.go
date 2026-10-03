// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// runZshInLocale runs a command string through the front end with the
// environment naming a locale, which is what decides whether a name may hold
// a character past ASCII — through the parse the front end makes before it
// has a runner as well as the one that runs.
func runZshInLocale(t *testing.T, locale, src string) string {
	t.Helper()
	var out, errs strings.Builder
	sh := scratchShell(t)
	sh.Stdout, sh.Stderr = &out, &errs
	sh.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + t.TempDir(), "LC_ALL=" + locale}
	driver.MainArgs(sh, []string{"zsh", "-fc", src})
	return out.String()
}

// A name may hold the locale's letters and digits past ASCII, until
// `posix_identifiers` says otherwise. See
// interp.Semantics.NamesTakeTheLocalesLetters.
//
// Measured 2026-10-02 on zsh 5.9.2, `env -i PATH=/usr/bin:/bin
// LC_ALL=en_US.UTF-8 zsh -fc`, standard output only (#5153).
func TestANameHoldsTheLocalesLetters(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"an assignment and both expansions", "hähä=3; print $hähä ${hähä} $#hähä", "3 3 1\n"},
		{"a digit after the first character, and a digit past ASCII first", "ä1=4; print $ä1; ١=5; print $١", "4\n5\n"},
		{"a name ends at a character that is not a letter", "x=ä; print $xä; a€=1; print $a€", "\n€\n"},
		{"typeset, arithmetic, for and read", "typeset ñ=2; (( ü = 4 )); for ö in 1 2; do print $ö; done; read é <<< hi; print $ñ $ü $é", "1\n2\n2 4 hi\n"},
		{"posix_identifiers takes it away", "setopt posix_identifiers; eval 'hähä=3' || print refused", "refused\n"},
		{"IDENT follows the rule", "[[ é = [[:IDENT:]] ]] && print in; setopt posix_identifiers; [[ é = [[:IDENT:]] ]] || print out", "in\nout\n"},
		{"a declaration's value takes its tildes", "HOME=/hh; export ö=~/z; f() { local ü=~/w; print $ü }; f; typeset é=~/y; print $ö $é", "/hh/w\n/hh/z /hh/y\n"},
		{"an element, a substitution's file and a brace", "hä[2]=b; print $hä; ö==(print hi); [[ $ö == /* ]] && print path; ü=x}; print $ü", "b\npath\nx}\n"},
		{"an array literal and text eval reads", "hä=(1 2); print $hä[2]; eval 'ö=1; print $ö'", "2\n1\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := runZshInLocale(t, "en_US.UTF-8", c.src); got != c.want {
				t.Errorf("%s\n got %q\nwant %q", c.src, got, c.want)
			}
		})
	}
}

// Under the C locale the bytes of `ä` are no letter, so the word is a
// command and `$hä` is `$h` followed by text: measured, `hähä=3; print
// $hähä` writes `ähä` there.
//
// And with no locale named at all, which this shell reads as C: measured, the
// same line under `env -i PATH=/usr/bin:/bin` alone writes the same.
func TestANameIsASCIIUnderTheCLocale(t *testing.T) {
	if got := runZshInLocale(t, "C", "hähä=3; print $hähä"); got != "ähä\n" {
		t.Errorf("got %q, want %q", got, "ähä\n")
	}
	var out, errs strings.Builder
	sh := scratchShell(t)
	sh.Stdout, sh.Stderr = &out, &errs
	sh.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + t.TempDir()}
	driver.MainArgs(sh, []string{"zsh", "-fc", "hähä=3; print $hähä"})
	if got := out.String(); got != "ähä\n" {
		t.Errorf("with no locale named: got %q, want %q", got, "ähä\n")
	}
}

// An exported name past ASCII is exported in the shell and reaches no child.
// See interp.Semantics.NameBeyondASCIIStaysOutOfTheEnvironment.
//
// Measured 2026-10-02 on zsh 5.9.2 under the same environment as above: the
// child sees `a=2` alone, and the shell lists `export ñ=1` (#5364).
func TestAnExportedNamePastASCIIReachesNoChild(t *testing.T) {
	got := runZshInLocale(t, "en_US.UTF-8",
		"export ñ=1 a=2; /usr/bin/env | /usr/bin/grep -c '=' >/dev/null; /usr/bin/env | while read -r l; do [[ $l == (ñ|a)=* ]] && print -r -- $l; done; typeset -p ñ")
	if want := "a=2\nexport ñ=1\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// A locale the machine has no data for is not in force: the shell stays in
// the one it was in, which at startup is C. Measured 2026-10-02 on zsh 5.9.2
// under `env -i PATH=/usr/bin:/bin`, on macOS and on glibc (#5503). The name
// is one no machine has, so this holds wherever the front end can ask; it
// would not on musl, which loads any name, and CI has no musl runner.
func TestALocaleTheMachineLacksIsNotInForce(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"at startup", "x=é; print ${#x}; hähä=3 2>/dev/null; print $hähä", "2\nähä\n"},
		{"after an installed locale", "x=é; LC_ALL=en_US.UTF-8; print ${#x}; LC_ALL=xx_XX.UTF-8; print ${#x}", "1\n1\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := runZshInLocale(t, "xx_XX.UTF-8", c.src); got != c.want {
				t.Errorf("%s\n got %q\nwant %q", c.src, got, c.want)
			}
		})
	}
}
