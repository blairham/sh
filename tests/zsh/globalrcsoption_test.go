// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"bytes"
	"testing"

	"github.com/blairham/sh/driver"
)

// `globalrcs` is the option the narrower startup escape hatch writes, and the
// invocation's own word is what reaches it — #4733.
//
// The files really were suppressed all along; what was wrong is the answer a
// script gets when it asks, which is exactly the kind of thing a framework
// branches on. `-f` had the arrangement that gets this right one letter over.
//
// Measured 2026-09-26 on `/opt/homebrew/bin/zsh`, zsh 5.9.2
// (aarch64-apple-darwin25.4.0), `env -i` with an empty `HOME` and `ZDOTDIR`
// and `env -u FPATH`, so no startup file could speak for either side. `go
// version -m` says *not a Go executable* for the reference.
//
// The rows go through driver.MainArgs rather than through the option table,
// because the whole of what was missing is the *route*: the option moved
// correctly when a script wrote it, and a test that called the resolver would
// have passed throughout.
func TestTheSystemStartupHatchReachesTheOptionNamespace(t *testing.T) {
	for _, tc := range []struct {
		name string
		argv []string
		want string
	}{
		// The invocation's two spellings, and the control that says the name
		// is not simply off.
		{"the letter", []string{"zsh", "-d", "-c", askGlobalrcs}, "off\n"},
		{"the long spelling", []string{"zsh", "--no-globalrcs", "-c", askGlobalrcs}, "off\n"},
		{"nothing asked", []string{"zsh", "-c", askGlobalrcs}, "on\n"},
		// A script still moves it both ways, which is what makes it the
		// option's state rather than a record that the word was written.
		{"a script turns it back on", []string{"zsh", "-d", "-c", "setopt globalrcs; " + askGlobalrcs}, "on\n"},
		{"a script turns it off", []string{"zsh", "-c", "unsetopt globalrcs; " + askGlobalrcs}, "off\n"},
		// And the invocation decides the *base*, so an option word after it
		// is a move off that base: `zsh -d -o globalrcs` answers on.
		{"an option word after it", []string{"zsh", "-d", "-o", "globalrcs", "-c", askGlobalrcs}, "on\n"},
		// The listing is the second surface and the letter the third. `$-`
		// carries `d` for as long as the name is off, on a route that never
		// wrote the letter — `zsh -c 'unsetopt globalrcs; echo $-'` is
		// `569Xd` in the reference — so the two are one state.
		{"the bare listing", []string{"zsh", "-d", "-c", "setopt"}, "noglobalrcs\nnohashdirs\n"},
		{"the letter in $-", []string{"zsh", "-d", "-c", "echo $-"}, "569Xd\n"},
		{"and it goes when the name does", []string{"zsh", "-d", "-c", "setopt globalrcs; echo $-"}, "569X\n"},
		// The wider hatch is a *second* fact and not this one: `-f`
		// suppresses the machine's files as well and still answers
		// `globalrcs` on, because the option says what the invocation asked
		// for rather than which files were read. Both words is both names.
		{"the wider hatch alone", []string{"zsh", "-f", "-c", "setopt"}, "nohashdirs\nnorcs\n"},
		{"both words", []string{"zsh", "-f", "-d", "-c", "setopt"}, "noglobalrcs\nnohashdirs\nnorcs\n"},
		// And a strict emulation puts it back at the table's default, where
		// a plain one leaves it — `globalrcs` is in the 95 only `-R` resets,
		// as `rcs` is. Measured the same day.
		{"a strict emulation puts it back", []string{"zsh", "-d", "-c", "emulate -R zsh; " + askGlobalrcs}, "on\n"},
		{"a plain emulation leaves it", []string{"zsh", "-d", "-c", "emulate zsh; " + askGlobalrcs}, "off\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out, errs bytes.Buffer
			code := driver.MainArgs(zshWriting(&out, &errs), tc.argv)
			if out.String() != tc.want || errs.String() != "" || code != 0 {
				t.Errorf("%v wrote %q / said %q at %d, want %q and nothing said",
					tc.argv[1:], out.String(), errs.String(), code, tc.want)
			}
		})
	}
}

// askGlobalrcs asks the option namespace and writes the answer, which is the line a
// framework's startup file has.
const askGlobalrcs = "if [[ -o globalrcs ]]; then echo on; else echo off; fi"
