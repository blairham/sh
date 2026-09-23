// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// noHome runs src with no `HOME` at all, under one answer to
// [Semantics.TildeWithNoHome], and reports the names the password hook was
// asked for.
//
// The hook is wired in every case rather than only in the password one, so
// that a policy reaching for the database when it should not is a failure
// here rather than an unasked question: a hook nobody calls and a hook that
// does not exist look the same from the output.
func noHome(t *testing.T, policy TildeWithNoHomePolicy, src string) (out string, status int, asked []string) {
	t.Helper()
	out, status = run(t, src, func(r *Runner) {
		sem := CoreSemantics()
		sem.TildeWithNoHome = policy
		r.Semantics = &sem
		r.Vars = map[string]string{"PATH": "/usr/bin:/bin"}
		r.UserHomeDir = func(name string) (string, bool) {
			asked = append(asked, name)
			if name == "" {
				return "/from/the/database", true
			}
			return "", false
		}
	})
	return out, status, asked
}

// A written `~` with no home to become is three different words across the
// panel, and the axis is what keeps them apart.
//
// Measured 2026-09-23, `env -i PATH=/usr/bin:/bin LC_ALL=C <shell> -c`, the
// bare road as `printf "<%s>" ~ ~/x` and the colon road as `v=a:~:b`:
//
//	bash 5.3.20, bash 3.2.57   /Users/…      the password entry, $HOME empty
//	zsh 5.9.2                  (empty)       an unset HOME reads as an empty one
//	dash 0.5.12                ~             the word as written
//
// Both roads take the same answer in every column, which is why
// Runner.homeForAWrittenTilde is one helper and not two: a fallback written
// into the word road alone would make `PATH=~/bin` disagree with `echo ~/bin`
// in a shell no column has.
func TestATildeWithNoHomeIsTheDialectsAnswer(t *testing.T) {
	for _, tc := range []struct {
		name          string
		policy        TildeWithNoHomePolicy
		bare, slashed string
		colon         string
		asked         bool
	}{
		{
			name: "left as written", policy: TildeWithNoHomeStaysWritten,
			bare: "~", slashed: "~/x", colon: "a:~:b",
		},
		{
			name: "the empty string", policy: TildeWithNoHomeIsEmpty,
			bare: "", slashed: "/x", colon: "a::b",
		},
		{
			name: "the password entry", policy: TildeWithNoHomeReadsThePasswordEntry,
			bare: "/from/the/database", slashed: "/from/the/database/x",
			colon: "a:/from/the/database:b", asked: true,
		},
		{
			// The zero value is the standard's reading, and it is the one
			// a vector with no dialect falls back to.
			name: "unspecified", policy: TildeWithNoHomeUnspecified,
			bare: "~", slashed: "~/x", colon: "a:~:b",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := "printf '<%s>\\n' ~\nprintf '<%s>\\n' ~/x\nv=a:~:b\nprintf '<%s>\\n' \"$v\"\n"
			out, st, asked := noHome(t, tc.policy, src)
			want := "<" + tc.bare + ">\n<" + tc.slashed + ">\n<" + tc.colon + ">\n"
			if out != want || st != 0 {
				t.Errorf("= %q status %d, want %q at 0", out, st, want)
			}
			switch {
			case tc.asked && len(asked) == 0:
				t.Error("the password database was never asked")
			case tc.asked:
				for _, name := range asked {
					if name != "" {
						t.Errorf("asked the database for %q, want the empty name: no user is called the empty string, which is what makes it mean the current one", name)
					}
				}
			case len(asked) != 0:
				t.Errorf("asked the database for %q under %v, which does not read one", asked, tc.policy)
			}
		})
	}
}

// A Runner with no hook has no database to ask, and leaves the word as
// written rather than guessing — which is what a library embedded in a
// program that has no business reading a password file must do.
//
// The same reasoning Runner.UserHomeDir already carries for `~user`, and it
// has to hold for this road too: the password answer is the only one that
// needs anything outside the shell.
func TestTheDatabaseAnswerNeedsAHookAndSaysSoBySayingNothing(t *testing.T) {
	out, st := run(t, "printf '<%s>\\n' ~/x\n", func(r *Runner) {
		sem := CoreSemantics()
		sem.TildeWithNoHome = TildeWithNoHomeReadsThePasswordEntry
		r.Semantics = &sem
		r.Vars = map[string]string{"PATH": "/usr/bin:/bin"}
	})
	if out != "<~/x>\n" || st != 0 {
		t.Errorf("= %q status %d, want %q at 0", out, st, "<~/x>\n")
	}
}

// A `HOME=` that is **set** and empty is not this question, and the whole
// panel agrees it is simply empty — measured the same day on bash 5.3.20 and
// zsh 5.9.2, `env -i HOME= …`, where `${HOME-UNSET}` is empty rather than
// `UNSET` and `~` is nothing.
//
// The control that keeps the password answer from swallowing the case it
// must not reach: a shell that read the database for an empty home would
// answer `~` with a path on a line that deliberately cleared it.
func TestASetButEmptyHomeIsNotTheNoHomeQuestion(t *testing.T) {
	for _, policy := range []TildeWithNoHomePolicy{
		TildeWithNoHomeStaysWritten,
		TildeWithNoHomeIsEmpty,
		TildeWithNoHomeReadsThePasswordEntry,
	} {
		t.Run(policy.String(), func(t *testing.T) {
			out, st, asked := noHome(t, policy, "HOME=\nprintf '<%s>\\n' ~/x\n")
			if out != "</x>\n" || st != 0 {
				t.Errorf("= %q status %d, want %q at 0", out, st, "</x>\n")
			}
			if len(asked) != 0 {
				t.Errorf("asked the database for %q, and there was a home to read", asked)
			}
		})
	}
}

// `~user` is a different question and keeps its own answer under every value
// of this axis: the axis is about a home the shell has none of, and a name in
// the script is not that.
func TestANamedTildeIsUntouchedByTheNoHomeAnswer(t *testing.T) {
	for _, policy := range []TildeWithNoHomePolicy{
		TildeWithNoHomeStaysWritten,
		TildeWithNoHomeIsEmpty,
		TildeWithNoHomeReadsThePasswordEntry,
	} {
		t.Run(policy.String(), func(t *testing.T) {
			out, st, asked := noHome(t, policy, "printf '<%s>\\n' ~nosuchuser/x\n")
			if out != "<~nosuchuser/x>\n" || st != 0 {
				t.Errorf("= %q status %d, want the word as written at 0", out, st)
			}
			if len(asked) == 0 || asked[0] != "nosuchuser" {
				t.Errorf("the database was asked for %q, want nosuchuser", asked)
			}
		})
	}
}

// The core's own answer, read off the vector rather than set, so that a
// default flipped onto one column's reading fails here instead of quietly
// changing what every test above measures.
func TestTheCoreLeavesATildeWithNoHomeAsWritten(t *testing.T) {
	if got := CoreSemantics().TildeWithNoHome; got != TildeWithNoHomeStaysWritten {
		t.Errorf("CoreSemantics().TildeWithNoHome = %v, want stays written", got)
	}
	if got := PosixSemantics().TildeWithNoHome; got != TildeWithNoHomeStaysWritten {
		t.Errorf("PosixSemantics().TildeWithNoHome = %v, want stays written", got)
	}
	if got := TildeWithNoHomeUnspecified.String(); !strings.Contains(got, "unspecified") {
		t.Errorf("the zero value prints %q", got)
	}
}
