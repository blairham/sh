// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// A PATH an assignment prefix supplied — `PATH=/nowhere ls` — and what it
// reaches. See interp/prefixedpath.go for the measurements; these tests name
// axes and never shells.
//
// The arrangement is two directories with the same command in each, the first
// one run so that it is in the command hash. Nothing less discriminates: with
// one copy every reading fails the prefixed run, and with no hashed entry
// every reading walks PATH again and finds the same thing.

// whereZzcIsRemembered names the directory the command hash holds `zzc` in,
// or reports the entry gone. A `case` over the path rather than a pipeline
// through `sed`, because a pipeline's status is the *last* command's: an
// empty table makes `hash -t` fail and leaves `sed` succeeding on nothing, so
// an `|| echo` on the end of one reports a table that is there.
const whereZzcIsRemembered = `at=$(hash -t zzc 2>/dev/null); ` +
	`case $at in */d1/*) echo d1;; */d2/*) echo d2;; *) echo "(empty)";; esac`

// TestAPrefixedPathIsTheOneSearched.
//
// The whole of #2626 in one line. The child was handed the prefix's PATH all
// along and this shell searched with its own, so the prefix could change what
// a command *saw* and never what was *found*: `PATH=/nowhere ls` ran `ls`
// where every column in the panel answers 127.
func TestAPrefixedPathIsTheOneSearched(t *testing.T) {
	out, st := withDirs(t, `zzc; PATH=$PWD/d2 zzc`, func(d1, d2 string) {
		hashable(t, d1, "zzc", "echo V1")
		hashable(t, d2, "zzc", "echo V2")
	}, nil)
	if st != 0 || out != "V1\nV2\n" {
		t.Errorf("out=%q st=%d, want the prefix's PATH to decide what runs", out, st)
	}
}

// And the failing half, which is the form the issue was filed as: a PATH with
// nothing on it finds nothing, whatever this shell's own PATH holds.
func TestAPrefixedPathThatHoldsNothingFindsNothing(t *testing.T) {
	out, st := withDirs(t, `PATH=/nowhere-zz zzc 2>/dev/null; echo "st=$?"`,
		func(d1, _ string) { hashable(t, d1, "zzc", "echo V1") }, nil)
	if out != "st=127\n" || st != 0 {
		t.Errorf("out=%q st=%d, want the command not found at 127", out, st)
	}
}

// The prefix is the command's and not the shell's: the name goes back to what
// it held, and the next search is made with it.
func TestAPrefixedPathIsGivenBack(t *testing.T) {
	out, st := withDirs(t, `p=$PATH; PATH=$PWD/d2 zzc; [ "$PATH" = "$p" ] && echo same; zzc`,
		func(d1, d2 string) {
			hashable(t, d1, "zzc", "echo V1")
			hashable(t, d2, "zzc", "echo V2")
		}, nil)
	if st != 0 || out != "V2\nsame\nV1\n" {
		t.Errorf("out=%q st=%d, want the shell's own PATH back afterward", out, st)
	}
}

// A name that was never set is not set afterward either, which is the other
// end of the same take-back: `unset PATH` leaves a shell with no PATH, and a
// prefix must not invent one.
func TestAPrefixedPathOverNoPathAtAllLeavesNoPath(t *testing.T) {
	out, st := withDirs(t, `unset PATH; PATH=$PWD/d2 zzc; echo "after=[${PATH-unset}]"`,
		func(_, d2 string) { hashable(t, d2, "zzc", "echo V2") }, nil)
	if st != 0 || out != "V2\nafter=[unset]\n" {
		t.Errorf("out=%q st=%d, want PATH unset again after the command", out, st)
	}
}

// The child is handed the prefix's PATH as well as the search being made with
// it. Unanimous, and it is the half that already worked — pinned so that
// applying the value to this shell cannot quietly become a substitute for
// putting it in the environment.
func TestAPrefixedPathReachesTheChildToo(t *testing.T) {
	out, st := run(t, `PATH=/zz-child /usr/bin/env | grep '^PATH=' || echo "(none)"`, nil)
	if st != 0 || out != "PATH=/zz-child\n" {
		t.Errorf("out=%q st=%d, want the child shown the prefix's PATH", out, st)
	}
}

// TestAPrefixedPathEmptiesTheCommandHashOrDoesNot is the axis, both ways.
//
// The table is either empty afterward — the prefix reached this shell's own
// PATH, which empties it, and putting the value back does not refill it — or
// exactly what it was, because the prefix never reached the shell at all. The
// run in the middle takes the *other* copy under both answers, which is what
// keeps this a test about the table rather than about the search.
func TestAPrefixedPathEmptiesTheCommandHashOrDoesNot(t *testing.T) {
	for _, tc := range []struct {
		name    string
		empties Answer
		want    string
	}{
		{"the prefix reaches the shell's PATH", Yes, "V1\nV2\n(empty)\n"},
		{"the prefix never reaches it", No, "V1\nV2\nd1\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := withDirs(t,
				`zzc; PATH=$PWD/d2 zzc; `+whereZzcIsRemembered,
				func(d1, d2 string) {
					hashable(t, d1, "zzc", "echo V1")
					hashable(t, d2, "zzc", "echo V2")
				},
				func(r *Runner) {
					s := testSemantics()
					s.APrefixedPathEmptiesTheCommandHash = tc.empties
					s.HashReportsThePath = Yes
					r.Semantics = &s
				})
			if st != 0 || out != tc.want {
				t.Errorf("out=%q st=%d, want %q", out, st, tc.want)
			}
		})
	}
}

// `command` is a precommand word, so a PATH in front of it is a PATH in front
// of what it runs — and the table answers it the same way. A fix that reached
// only the bare external route leaves this one recording the path the prefixed
// search found, which is an answer no column in the panel gives.
func TestAPrefixedPathThroughCommandAnswersTheTableTheSameWay(t *testing.T) {
	for _, tc := range []struct {
		name    string
		empties Answer
		want    string
	}{
		{"the prefix reaches the shell's PATH", Yes, "V1\nV2\n(empty)\n"},
		{"the prefix never reaches it", No, "V1\nV2\nd1\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := withDirs(t,
				`zzc; PATH=$PWD/d2 command zzc; `+whereZzcIsRemembered,
				func(d1, d2 string) {
					hashable(t, d1, "zzc", "echo V1")
					hashable(t, d2, "zzc", "echo V2")
				},
				func(r *Runner) {
					s := testSemantics()
					s.APrefixedPathEmptiesTheCommandHash = tc.empties
					s.HashReportsThePath = Yes
					r.Semantics = &s
				})
			if st != 0 || out != tc.want {
				t.Errorf("out=%q st=%d, want %q", out, st, tc.want)
			}
		})
	}
}

// A prefix to a *builtin* is a different question and is not this axis.
//
// It is visible to the builtin while it runs — that is what `IFS=: read x y`
// is — so it really is an assignment to the shell, and the table is emptied
// under either answer. Measured: `PATH=/nowhere true` leaves an empty table
// in every column of the panel but bash 3.2. Without this row the axis could
// be read as covering every prefix, and a rule that emptied the table only
// under one answer would pass every other test here.
func TestAPrefixedPathOnABuiltinEmptiesTheTableUnderEitherAnswer(t *testing.T) {
	for _, empties := range []Answer{Yes, No} {
		out, st := withDirs(t,
			`zzc; PATH=/nowhere-zz true; `+whereZzcIsRemembered,
			func(d1, _ string) { hashable(t, d1, "zzc", "echo V1") },
			func(r *Runner) {
				s := testSemantics()
				s.APrefixedPathEmptiesTheCommandHash = empties
				s.HashReportsThePath = Yes
				r.Semantics = &s
			})
		if st != 0 || out != "V1\n(empty)\n" {
			t.Errorf("%v: out=%q st=%d, want the table emptied by the assignment", empties, out, st)
		}
	}
}

// A prefix that names something other than PATH leaves the table alone, which
// is the control the axis needs: without it, a rule that emptied the table for
// every prefixed external command would pass the Yes side of every row above.
func TestAPrefixThatIsNotThePathLeavesTheTableAlone(t *testing.T) {
	out, st := withDirs(t, `zzc; v=1 zzc; `+whereZzcIsRemembered,
		func(d1, d2 string) {
			hashable(t, d1, "zzc", "echo V1")
			hashable(t, d2, "zzc", "echo V2")
		},
		func(r *Runner) {
			s := testSemantics()
			s.HashReportsThePath = Yes
			r.Semantics = &s
		})
	if st != 0 || out != "V1\nV1\nd1\n" {
		t.Errorf("out=%q st=%d, want the entry untouched by a prefix that is not PATH", out, st)
	}
}

// A frozen PATH is not a PATH the prefix supplies: the refusal leaves the name
// holding what it held, so the search is the shell's own and the child is shown
// the shell's own value too.
func TestAFrozenPathIsNotSuppliedByAPrefix(t *testing.T) {
	out, st := withDirs(t, `readonly PATH; PATH=$PWD/d2 zzc 2>/dev/null; echo "st=$?"`,
		func(d1, d2 string) {
			hashable(t, d1, "zzc", "echo V1")
			hashable(t, d2, "zzc", "echo V2")
		},
		func(r *Runner) {
			s := testSemantics()
			// How a refused prefix is reported, and whether it ends the
			// script, are questions of their own and not this one's. They
			// are answered here so that what is left to see is the search.
			s.PrefixToAFrozenNameIsCheckedFirst = FrozenPrefixCheckedWithTheCommand
			s.PrefixRefusalFatality = PrefixRefusalNeverFatal
			s.PrefixRefusalCostsTheCommand = No
			r.Semantics = &s
		})
	if st != 0 || !strings.HasPrefix(out, "V1\n") {
		t.Errorf("out=%q st=%d, want the refused prefix to leave the search alone", out, st)
	}
}
