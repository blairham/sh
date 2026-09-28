// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// typeWordUnder runs one `${(t)NAME}` in a shell handed that same name.
func typeWordUnder(t *testing.T, name string) string {
	t.Helper()
	base := dialecttest.Base{Dir: t.TempDir(), Env: []string{"PATH=/usr/bin:/bin", name + "=ZZ"}}
	out, _, err := preset.Combined(t, base, `print -r -- "${(t)`+name+`}"`)
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	return strings.TrimSpace(out)
}

// TestAnEntryThisShellDoesNotAdoptCarriesNoExport — the 24 names whose
// parameter here is not the environment's entry of that name.
//
// Every row is the reference's `${(t)NAME}` under `env -i PATH=/usr/bin:/bin
// NAME=ZZ`, measured 2026-09-27 against `/opt/homebrew/bin/zsh` — `zsh 5.9.2
// (aarch64-apple-darwin25.4.0)`, which `go version -m` calls *not a Go
// executable* — and every one of them carried an extra `export` here.
func TestAnEntryThisShellDoesNotAdoptCarriesNoExport(t *testing.T) {
	for _, tc := range []struct{ name, want string }{
		{"EGID", "integer-special"},
		{"EUID", "integer-special"},
		{"GID", "integer-special"},
		{"UID", "integer-special"},
		{"USERNAME", "scalar-special"},
		{"IFS", "scalar-special"},
		{"HISTCHARS", "scalar-special"},
		{"histchars", "scalar-special"},
		{"OPTIND", "integer-special"},
		{"KEYBOARD_HACK", "scalar-special"},
		{"TRY_BLOCK_ERROR", "integer-special"},
		{"TRY_BLOCK_INTERRUPT", "integer-special"},
		{"_", "scalar-special"},
		{"argv", "array-special"},
		{"cdpath", "array-tied-special"},
		{"fignore", "array-tied-special"},
		{"fpath", "array-tied-special"},
		{"mailpath", "array-tied-special"},
		{"manpath", "array-tied-special"},
		{"module_path", "array-tied-special"},
		{"MODULE_PATH", "scalar-tied-special"},
		{"path", "array-tied-special"},
		{"psvar", "array-tied-special"},
		{"signals", "array"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := typeWordUnder(t, tc.name); got != tc.want {
				t.Errorf("${(t)%s} under env %s=ZZ = %q, want %q", tc.name, tc.name, got, tc.want)
			}
		})
	}
}

// TestTheNamesThatKeepTheExportKeepIt — the control, and the reason the rule
// above is a table rather than the one #4940 proposed.
//
// That issue read the split as `special` deciding it, off four rows that are
// all `special`. Every name here is `special` too, and every one of them
// *keeps* the export the environment brought — measured in the same sweep, so
// a rule keyed on the word would have taken it off all seven.
//
// `$PATH` is on this list on purpose beside the `path` row above: a rule
// keyed on the tie would have been wrong in the other direction.
func TestTheNamesThatKeepTheExportKeepIt(t *testing.T) {
	for _, tc := range []struct{ name, want string }{
		{"PPID", "integer-readonly-export-special"},
		{"RANDOM", "integer-export-special"},
		{"SECONDS", "integer-export-special"},
		{"SHLVL", "integer-export-special"},
		{"LINENO", "integer-readonly-export-special"},
		{"status", "integer-readonly-export-special"},
		{"ARGC", "integer-readonly-export-special"},
		{"PATH", "scalar-tied-export-special"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := typeWordUnder(t, tc.name); got != tc.want {
				t.Errorf("${(t)%s} under env %s=ZZ = %q, want %q", tc.name, tc.name, got, tc.want)
			}
		})
	}
}

// TestTheEntryItselfStillReachesAChild — the half that separates this from
// taking the export off.
//
// `export -n` would take the name out of a child's environment altogether.
// Here the entry is untouched: it goes down as it arrived, an assignment does
// not reach it, `unset` does not reach it, and only an explicit `export` makes
// the shell's own value supersede it. Measured 2026-09-27 in one run with
// `/usr/bin/env` as the child.
func TestTheEntryItselfStillReachesAChild(t *testing.T) {
	const read = `/usr/bin/env | /usr/bin/grep -a '^IFS=' | /usr/bin/od -c | /usr/bin/head -1`
	for _, tc := range []struct{ name, src, want string }{
		{"as inherited", read, `0000000 I F S = Z Z \n`},
		{"an assignment does not reach it", "IFS=x; " + read, `0000000 I F S = Z Z \n`},
		{"unset does not reach it", "unset IFS; " + read, `0000000 I F S = Z Z \n`},
		// And `export` is the one thing that does, which is also the row
		// that says the NUL in this value has to be cut: the shell's own
		// `$IFS` is ` \t\n\0` in both shells, and handing four bytes to
		// `execve` made **every** command fail to start here.
		{"export supersedes it, cut at the NUL", "export IFS; " + read, `0000000 I F S = \t \n`},
		{"an assignment then export sends the assignment", "IFS=x; export IFS; " + read, `0000000 I F S = x \n`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := dialecttest.Base{Dir: t.TempDir(), Env: []string{"PATH=/usr/bin:/bin", "IFS=ZZ"}}
			out, _, err := preset.Combined(t, base, tc.src)
			if err != nil {
				t.Fatalf("run: %v", err)
			}
			// `od`'s own column widths differ between the BSD one on a
			// Mac and the GNU one on the Linux runner — one leading space
			// against two — so the run of spaces is not part of the
			// assertion. The bytes are.
			if got := spacedOnce(out); got != spacedOnce(tc.want) {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// spacedOnce collapses every run of whitespace to one space, so a row asserts
// the bytes `od` printed rather than the width it printed them at.
func spacedOnce(s string) string { return strings.Join(strings.Fields(s), " ") }
