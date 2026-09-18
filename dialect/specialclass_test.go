// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package dialect_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// Which builtins are *special* is not one list (#3290).
//
// `interp.specialBuiltins` was POSIX's fourteen plus `source`, and it is read
// for four separate consequences: `type` calls the name special, an
// assignment prefixed to it persists, its failure is fatal to a
// non-interactive shell, and `set -x` traces the assignment before the
// command rather than after it. Two columns of the panel hold names we did
// not.
//
// Measured 2026-09-16, `env -i PATH=/usr/bin:/bin LC_ALL=C <shell> case.sh`
// with stdin from /dev/null, in a fresh directory. BusyBox v1.37.0 is the
// digest-pinned Alpine image internal/oracle reaches, under `--init`; dash
// was read twice, Apple's dash-16 here and 0.5.12 in `debian:stable-slim`,
// and the two agree line for line:
//
//	column                 special beyond POSIX's fourteen
//	bash 5.3.20            — (it draws no line at all)
//	that binary as `sh`    source
//	bash 3.2.57            — (it draws no line at all)
//	zsh 5.9.2              — (it draws no line at all)
//	ksh93u+ 2012-08-01     alias, unalias, typeset, newgrp
//	dash 0.5.12 / dash-16  local
//	BusyBox ash 1.37.0     source, local
//
// # Why these are Go rows and no suite tier can hold them
//
// A per-dialect tier runs under the one reference it claims to be, and
// `core/` holds only what every reference agrees on. `local` is special in
// two of the seven columns and `alias` in one, so neither has a home in
// `core/`, and each dialect tier can only ever pin *its own* side: with
// `dash/` and `bash/` both green the split itself is still unpinned, because
// a preset moved to the other side leaves every tier at 100% and only the
// column that moved is wrong. The roster is asserted here, where all six
// presets sit in one table with the controls beside the discriminators.
//
// The suite tiers get the reference-graded half on top — `dash/` and `ash/`
// for `local`, `ksh/` for the other three — which is the half that would
// catch a sentence this table has agreed to be wrong about.

// specialRosterCase is one name and what each preset should say about it.
type specialRosterCase struct {
	// name is the builtin asked about.
	name string
	// sentence is the presets whose `type` calls it special, and whose
	// assignment prefix therefore persists.
	special map[string]bool
}

// TestSpecialBuiltinMembershipIsPerDialect pins the roster itself: the
// sentence for every name over every preset, with two controls that no column
// moves.
//
// `echo` is the control that says a column drawing the distinction has not
// merely adopted a longer phrase, and `.` is the control that says a column
// draws it at all. Between them, a preset whose roster grew a name it should
// not have fails its own row and nothing else, and a shell that said
// `special` for everything fails every `echo`.
func TestSpecialBuiltinMembershipIsPerDialect(t *testing.T) {
	// `local` is absent from ksh93 and `typeset` from dash and BusyBox ash,
	// so a row is asked only of the presets that have the builtin: a name
	// that resolves to nothing has no membership to get wrong, and a row
	// written over it would be pinning the absence instead.
	cases := []specialRosterCase{
		{"local", map[string]bool{"dash": true, "ash": true}},
		{"alias", map[string]bool{"ksh": true}},
		{"unalias", map[string]bool{"ksh": true}},
	}
	for _, c := range cases {
		for _, preset := range []string{"bash", "zsh", "ksh", "dash", "ash", "posix"} {
			if c.name == "local" && preset == "ksh" {
				// ksh93 has no `local` at all — `whence local` is
				// `local: not found` there — so there is no sentence to
				// pin, only the claim that nothing calls it special. The
				// wording of the refusal belongs to the diagnostics tests.
				t.Run("local/ksh-has-none", func(t *testing.T) {
					out, _, err := presets["ksh"].Combined(t, dialecttest.Base{}, "type local")
					if err != nil {
						t.Fatal(err)
					}
					if !strings.Contains(out, "not found") || strings.Contains(out, "special") {
						t.Errorf("type local in ksh said %q, want a not-found and no `special`", out)
					}
				})
				continue
			}
			t.Run(c.name+"/"+preset, func(t *testing.T) {
				p := presets[preset]
				out, st, err := p.Combined(t, dialecttest.Base{}, "type "+c.name+"; type echo")
				if err != nil {
					t.Fatal(err)
				}
				want := c.name + " is a shell builtin\necho is a shell builtin\n"
				if preset == "zsh" && (c.name == "local" || c.name == "typeset") {
					// zsh's grammar reserves the declaration commands, so
					// the sentence never reaches a builtin there and the
					// membership is not what it reports (#3291). Not an
					// exemption: the row still says the word is not called
					// special, which is the claim this table makes.
					want = c.name + " is a reserved word\necho is a shell builtin\n"
				}
				if c.special[preset] {
					want = c.name + " is a special shell builtin\necho is a shell builtin\n"
				}
				if out != want || st != 0 {
					t.Errorf("type %s said %q status %d, want %q at 0", c.name, out, st, want)
				}
			})
		}
	}
	// `typeset` only where the builtin exists. ksh93 marks it special;
	// bash and zsh have it and draw no line at all.
	for _, preset := range []string{"bash", "zsh", "ksh", "posix"} {
		t.Run("typeset/"+preset, func(t *testing.T) {
			p := presets[preset]
			out, _, err := p.Combined(t, dialecttest.Base{}, "type typeset; type echo")
			if err != nil {
				t.Fatal(err)
			}
			want := "typeset is a shell builtin\necho is a shell builtin\n"
			if preset == "zsh" {
				// A reserved word there, not a builtin — see the note on
				// the loop above.
				want = "typeset is a reserved word\necho is a shell builtin\n"
			}
			if preset == "ksh" {
				want = "typeset is a special shell builtin\necho is a shell builtin\n"
			}
			if out != want {
				t.Errorf("type typeset said %q, want %q", out, want)
			}
		})
	}
}

// TestSpecialBuiltinPrefixPersistsByMembership is the second consequence, and
// the one that says this is a membership rather than a wording: a shell that
// had only changed what `type` prints would pass the table above and fail
// here.
//
// `cd` is the control on every row — an ordinary builtin whose prefix no
// column keeps — and the POSIX name `export` is the control that says the
// preset answers AssignmentPrefixPersistsOnSpecialBuiltin yes at all. Without
// it, a preset that had turned that axis off would look like one whose roster
// was empty.
//
// Measured 2026-09-16: `V=1 alias` leaves V at 1 in ksh93u+ and in zsh 5.9.2,
// and unset everywhere else that has the builtin; `LV=1 local x` inside a
// function leaves LV at 1 in dash and BusyBox ash, where `CV=1 command true`
// leaves CV unset in all seven.
//
// **zsh's `alias` row is not this roster's**, which is the trap this suite has
// to keep out of: that shell answers
// AssignmentPrefixPersistsOnSpecialBuiltin **no** — its `export` cell below is
// UNSET, and `:` and `shift 0` are unset there too — and keeps the prefix in
// front of `alias` and `hash` anyway, which is
// Semantics.BuiltinsKeepingAnAssignmentPrefix (#3313). So the cell is 1 for a
// reason the membership does not explain, and the `export` cell one column
// along is what says so.
func TestSpecialBuiltinPrefixPersistsByMembership(t *testing.T) {
	const src = `f() { LV=1 local x >/dev/null 2>&1; }
f
V1=1 alias >/dev/null 2>&1
V2=1 unalias -a >/dev/null 2>&1
V4=1 cd . >/dev/null 2>&1
V5=1 export >/dev/null 2>&1
printf 'local=[%s] alias=[%s] unalias=[%s] cd=[%s] export=[%s]\n' \
	"${LV-UNSET}" "${V1-UNSET}" "${V2-UNSET}" "${V4-UNSET}" "${V5-UNSET}"
`
	for _, c := range []struct {
		preset string
		want   string
	}{
		// bash keeps no prefix at all, POSIX name included.
		{"bash", "local=[UNSET] alias=[UNSET] unalias=[UNSET] cd=[UNSET] export=[UNSET]\n"},
		// zsh keeps none of them *as special builtins* — its `export` cell
		// is the one that says so — and keeps `alias`'s through the other
		// roster. See the note above.
		{"zsh", "local=[UNSET] alias=[1] unalias=[UNSET] cd=[UNSET] export=[UNSET]\n"},
		// ksh93's three, and not `local`, which that shell has no builtin for.
		{"ksh", "local=[UNSET] alias=[1] unalias=[1] cd=[UNSET] export=[1]\n"},
		// dash and BusyBox ash keep `local`'s and no other beyond POSIX's.
		{"dash", "local=[1] alias=[UNSET] unalias=[UNSET] cd=[UNSET] export=[1]\n"},
		{"ash", "local=[1] alias=[UNSET] unalias=[UNSET] cd=[UNSET] export=[1]\n"},
		// The POSIX preset is the fourteen and nothing else, which is what
		// dash and ash inherit where they set no value — so it is in the
		// table to say that their `local` is set here and not there.
		{"posix", "local=[UNSET] alias=[UNSET] unalias=[UNSET] cd=[UNSET] export=[1]\n"},
	} {
		t.Run(c.preset, func(t *testing.T) {
			out, st, err := presets[c.preset].Combined(t, dialecttest.Base{}, src)
			if err != nil {
				t.Fatal(err)
			}
			if out != c.want || st != 0 {
				t.Errorf("prefixes came back %q status %d, want %q at 0", out, st, c.want)
			}
		})
	}
}

// TestSpecialBuiltinFatalityByMembership is the third consequence.
//
// `cd -Z` is the control: an ordinary builtin's bad option, which no column
// ends a script over, so a preset that had made *every* refusal fatal fails
// here rather than passing by accident. `local qq` outside a function is
// dash's and BusyBox's own shape, and it is the one that had already been
// right while the sentence beside it was wrong — the drift a single
// membership closes.
func TestSpecialBuiltinFatalityByMembership(t *testing.T) {
	for _, c := range []struct {
		preset string
		src    string
		// alive is whether the script reached the line after the refusal.
		alive bool
	}{
		{"ksh", "alias -Z", false},
		{"ksh", "unalias -Z", false},
		{"ksh", "cd -Z", true},
		{"dash", "alias -Z", true},
		{"dash", "cd -Z", true},
		{"ash", "alias -Z", true},
		{"bash", "alias -Z", true},
		{"zsh", "alias -Z", true},
	} {
		t.Run(c.preset+"/"+c.src, func(t *testing.T) {
			out, _, err := presets[c.preset].Combined(t, dialecttest.Base{},
				c.src+" 2>/dev/null\nprintf 'AFTER\\n'\n")
			if err != nil {
				t.Fatal(err)
			}
			if got := out == "AFTER\n"; got != c.alive {
				t.Errorf("%s in %s: alive=%v (output %q), want alive=%v", c.src, c.preset, got, out, c.alive)
			}
		})
	}
	// `local` outside a function, which ends a dash and a BusyBox script and
	// leaves the other three running.
	//
	// A regression guard rather than a discriminator, and the difference is
	// worth naming: emptying dash's roster does not move these rows. That
	// refusal has an axis of its own — LocalOutsideAFunctionIsFatal — and it
	// was already right while the sentence beside it was wrong, which is the
	// drift #3290 was filed about. See
	// TestLocalFatalityAndMembershipStillAgree for what holds the two
	// together now.
	for _, c := range []struct {
		preset string
		alive  bool
	}{
		{"dash", false},
		{"ash", false},
		{"bash", true},
		{"zsh", true},
		{"posix", true},
	} {
		t.Run("local-outside/"+c.preset, func(t *testing.T) {
			out, _, err := presets[c.preset].Combined(t, dialecttest.Base{},
				"local qq 2>/dev/null\nprintf 'AFTER\\n'\n")
			if err != nil {
				t.Fatal(err)
			}
			if got := out == "AFTER\n"; got != c.alive {
				t.Errorf("local outside a function in %s: alive=%v (output %q), want alive=%v", c.preset, got, out, c.alive)
			}
		})
	}
}

// TestSpecialBuiltinTraceOrderFollowsMembership is the fourth consequence and
// the sibling this was nearly shipped without.
//
// ksh93 is the one column that traces a prefix on its own line *after* the
// command, and it does that for an ordinary builtin only: a special builtin's
// prefix is traced before it, the way a bare assignment is. So the same
// roster that moves the sentence moves the order, and `cd` is the control
// that stays behind its prefix.
//
// Measured 2026-09-16 on ksh93u+ 2012-08-01: `set -x; V=1 alias` writes
// `+ V=1` then `+ alias`, and `W=1 cd .` writes `+ cd .` then `+ W=1`.
func TestSpecialBuiltinTraceOrderFollowsMembership(t *testing.T) {
	out, _, err := presets["ksh"].Combined(t, dialecttest.Base{},
		"set -x\nV=1 unalias -a\nW=1 cd .\n")
	if err != nil {
		t.Fatal(err)
	}
	const want = "+ V=1\n+ unalias -a\n+ cd .\n+ W=1\n"
	if out != want {
		t.Errorf("trace was %q, want %q", out, want)
	}
}

// TestLocalFatalityAndMembershipStillAgree is the guard on the drift #3290
// named: two fields hold one fact about `local` in every column measured, and
// nothing until now would have noticed them parting.
//
// They are two rather than one on purpose. `local` outside a function is a
// *usage* error with its own wording, not a bad option, and bash reaches that
// refusal — it says `local: can only be used in a function` at 1 — while
// marking no builtin special at all. So the columns cannot tell "the refusal
// is fatal because `local` is special here" from "the refusal is fatal here",
// and collapsing the axis into the roster would be asserting a rule the panel
// cannot distinguish from a coincidence.
//
// What the panel *can* say is that the two agree, column for column, and that
// is what this pins: a preset whose roster holds `local` ends the script over
// it, and one whose roster does not, does not. Measured 2026-09-16: dash
// 0.5.12 and BusyBox ash 1.37.0 mark `local` special and end the script at 2;
// the three bash builds mark nothing special and carry on at 1; zsh sets a
// global at 0; ksh93 has no `local`.
func TestLocalFatalityAndMembershipStillAgree(t *testing.T) {
	for _, preset := range []string{"bash", "zsh", "dash", "ash", "posix"} {
		t.Run(preset, func(t *testing.T) {
			p := presets[preset]
			// The membership, read through the sentence rather than through
			// the field, so that this asks the same question the shell does.
			out, _, err := p.Combined(t, dialecttest.Base{}, "type local")
			if err != nil {
				t.Fatal(err)
			}
			special := strings.Contains(out, "special shell builtin")
			// And the fatality, read the same way.
			out, _, err = p.Combined(t, dialecttest.Base{},
				"local qq 2>/dev/null\nprintf 'AFTER\\n'\n")
			if err != nil {
				t.Fatal(err)
			}
			fatal := out != "AFTER\n"
			if special != fatal {
				t.Errorf("%s calls local special=%v but ends the script over it=%v; the two must move together",
					preset, special, fatal)
			}
		})
	}
}

// TestRefusedPrefixOnASpecialBuiltinFollowsMembership is the fifth
// consequence, and the one a first pass at #3290 changed without a row that
// could see it: reverting the membership read there left every test green.
//
// ksh93 is the column with PrefixRefusalFatalOnASpecialBuiltinOrFunction —
// a prefix the shell refuses ends the script in front of a special builtin
// and costs nothing at all in front of an ordinary one. So the same roster
// that moves the sentence moves this, and `cd` is the control.
//
// Measured 2026-09-16 on ksh93u+ 2012-08-01, `readonly V=0` first:
//
//	V=1 alias    `V: is read only`, and the script ends at 1
//	V=1 export   the same, which is the POSIX-name control
//	W=1 cd .     `after cd st=0`, and the script runs on
//
// Before this change ours answered `after alias st=0` and carried on, which
// is the ordinary-builtin answer given to a name ksh93 marks special.
func TestRefusedPrefixOnASpecialBuiltinFollowsMembership(t *testing.T) {
	for _, c := range []struct {
		name string
		// alive is whether the script reached the line after the refusal.
		alive bool
	}{
		{"alias", false},
		{"unalias -a", false},
		// The POSIX name, which says the preset answers the fatality at all.
		{"export", false},
		// And the ordinary builtin, which says it is not answering it for
		// everything.
		{"cd .", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _, err := presets["ksh"].Combined(t, dialecttest.Base{},
				"readonly V=0\nV=1 "+c.name+" >/dev/null\nprintf 'AFTER\\n'\n")
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Contains(out, "AFTER"); got != c.alive {
				t.Errorf("a refused prefix on %s: alive=%v (output %q), want alive=%v", c.name, got, out, c.alive)
			}
		})
	}
}
