// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// The command hash — what PATH resolved a name to, kept so the next run does
// not walk PATH again. See interp/commandhash.go for the measurements.

// hashable writes an executable script into a directory of the runner's own,
// making the directory if it is not there — withDirs puts it on PATH.
//
// A command of the test's own making rather than one on the machine, because
// what these tests are about is a *second* copy appearing and the first one
// going away, and neither is something to do to /bin.
func hashable(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	at := filepath.Join(dir, name)
	if err := os.WriteFile(at, []byte("#!/bin/sh\n"+body+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
}

// withDirs runs src with `d1` and `d2` under the runner's own directory on
// PATH in that order, and hands their paths to seed to fill in. Two of them,
// because the questions with an axis behind them need a second copy of one
// name for the answers to differ at all.
func withDirs(t *testing.T, src string, seed func(d1, d2 string), setup func(*Runner)) (string, int) {
	t.Helper()
	return run(t, `PATH=$PWD/d1:$PWD/d2:$PATH; `+src, func(r *Runner) {
		seed(filepath.Join(r.Dir, "d1"), filepath.Join(r.Dir, "d2"))
		if setup != nil {
			setup(r)
		}
	})
}

func TestARunCommandIsRemembered(t *testing.T) {
	out, st := withDirs(t, `zzc; hash -t zzc | sed "s|.*/||"`,
		func(d1, _ string) { hashable(t, d1, "zzc", "echo V1") },
		func(r *Runner) {
			s := testSemantics()
			s.HashReportsThePath = Yes
			r.Semantics = &s
		})
	if st != 0 || !strings.Contains(out, "V1\nzzc") {
		t.Errorf("out=%q st=%d, want the name in the table after it ran", out, st)
	}
}

// TestALookupRemembersThePathOrDoesNot is the half of "what goes in" that is
// **not** unanimous, and was written down here as though it were: a bash-only
// probe said `type` and `command -v` leave the table alone, and so they do —
// in bash. zsh, ksh93 and dash all hash what they were only asked about.
func TestALookupRemembersThePathOrDoesNot(t *testing.T) {
	for _, tc := range []struct {
		name      string
		remembers Answer
		want      string
	}{
		{"a lookup hashes", Yes, "zzc\n"},
		{"only a run hashes", No, "hash: hash table empty\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := withDirs(t, `type zzc >/dev/null; command -v zzc >/dev/null; hash | sed "s|.*/||"`,
				func(d1, _ string) { hashable(t, d1, "zzc", ":") },
				func(r *Runner) {
					s := testSemantics()
					s.ALookupRemembersThePath = tc.remembers
					r.Semantics = &s
					r.Diagnostics = &Diagnostics{HashEmptyTable: "hash: hash table empty"}
				})
			if st != 0 || out != tc.want {
				t.Errorf("out=%q st=%d, want %q", out, st, tc.want)
			}
		})
	}
}

// And the unanimous half beside it, which is what keeps the two apart: a
// command that *runs* is hashed whatever the axis above says.
func TestARunHashesWhateverALookupDoes(t *testing.T) {
	out, st := withDirs(t, `zzc; hash | sed "s|.*/||"`,
		func(d1, _ string) { hashable(t, d1, "zzc", ":") },
		func(r *Runner) {
			s := testSemantics()
			s.ALookupRemembersThePath = No
			r.Semantics = &s
		})
	if st != 0 || out != "zzc\n" {
		t.Errorf("out=%q st=%d, want a run to hash in every column", out, st)
	}
}

func TestTheHitCountIsOfLookupsAndNotOfRuns(t *testing.T) {
	// Two runs and a `type` are three lookups the table answered; the
	// listing itself walks nothing and does not count.
	out, st := withDirs(t, `zzc; zzc; type zzc >/dev/null; hash | sed -n 2p | cut -f1 | tr -d " "`,
		func(d1, _ string) { hashable(t, d1, "zzc", ":") },
		func(r *Runner) {
			r.Diagnostics = &Diagnostics{HashListing: HashListingHitsAndPath}
		})
	if st != 0 || strings.TrimSpace(out) != "3" {
		t.Errorf("out=%q st=%d, want three lookups counted", out, st)
	}
}

func TestTheListingHasAShapePerDialect(t *testing.T) {
	for _, tc := range []struct {
		name string
		form HashListingForm
		want string
	}{
		{"bare path", HashListingPathOnly, "zzc\n"},
		{"name and path", HashListingNameEqualsPath, "zzc=zzc\n"},
		{"hits and path", HashListingHitsAndPath, "hits\tcommand\n   1\tzzc\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := withDirs(t, `zzc; hash | sed "s|$PWD/d1/||"`,
				func(d1, _ string) { hashable(t, d1, "zzc", ":") },
				func(r *Runner) { r.Diagnostics = &Diagnostics{HashListing: tc.form} })
			if st != 0 || out != tc.want {
				t.Errorf("out=%q st=%d, want %q", out, st, tc.want)
			}
		})
	}
}

func TestAssigningPathEmptiesTheTable(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"assigned", `zzc; PATH=$PATH; hash`},
		{"unset", `zzc; unset PATH; hash`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := withDirs(t, tc.src,
				func(d1, _ string) { hashable(t, d1, "zzc", ":") },
				func(r *Runner) {
					r.Diagnostics = &Diagnostics{HashEmptyTable: "hash: hash table empty"}
				})
			if st != 0 || !strings.Contains(out, "hash table empty") {
				t.Errorf("out=%q st=%d, want the table emptied", out, st)
			}
		})
	}
}

func TestAStaleEntryIsTrustedOrSearchedAgain(t *testing.T) {
	// Two copies on PATH, the first hashed and then removed. That is the
	// only arrangement that tells the readings apart: with one copy both
	// answers fail and only the wording moves.
	for _, tc := range []struct {
		name    string
		trusted Answer
		check   bool
		want    string
	}{
		{"trusted", Yes, false, "V1\nst=127\n"},
		{"searched again", No, false, "V1\nV2\nst=0\n"},
		{"trusted but checked", Yes, true, "V1\nV2\nst=0\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := withDirs(t, `zzc; rm d1/zzc; zzc 2>/dev/null; echo "st=$?"`,
				func(d1, d2 string) {
					hashable(t, d1, "zzc", "echo V1")
					hashable(t, d2, "zzc", "echo V2")
				},
				func(r *Runner) {
					s := testSemantics()
					s.CommandHashIsTrusted = tc.trusted
					r.Semantics = &s
					r.SetChecksHashedCommand(tc.check)
				})
			if st != 0 || out != tc.want {
				t.Errorf("out=%q st=%d, want %q", out, st, tc.want)
			}
		})
	}
}

func TestATrustedStaleEntryIsReportedAsItsPath(t *testing.T) {
	// bash names the *remembered path* rather than the name, which is the
	// sentence a command word with a slash in it gets — and 127 with it.
	out, st := withDirs(t, `zzc; rm d1/zzc; zzc; echo "st=$?"`,
		func(d1, d2 string) {
			hashable(t, d1, "zzc", "echo V1")
			hashable(t, d2, "zzc", "echo V2")
		},
		func(r *Runner) {
			s := testSemantics()
			s.CommandHashIsTrusted = Yes
			r.Semantics = &s
		})
	if st != 0 || !strings.Contains(out, "d1/zzc: ") || !strings.Contains(out, "st=127") {
		t.Errorf("out=%q st=%d, want the remembered path named at 127", out, st)
	}
	if strings.Contains(out, "zzc: command not found") {
		t.Errorf("out=%q named the word rather than the path it remembered", out)
	}
}

func TestASubshellOwnsWhatItHashes(t *testing.T) {
	out, st := withDirs(t, `( zzc ); hash`,
		func(d1, _ string) { hashable(t, d1, "zzc", ":") },
		func(r *Runner) {
			r.Diagnostics = &Diagnostics{HashEmptyTable: "hash: hash table empty"}
		})
	if st != 0 || !strings.Contains(out, "hash table empty") {
		t.Errorf("out=%q st=%d, want the subshell's entry to stay the subshell's", out, st)
	}
}

func TestHashForgetsOneNameOrAllOfThem(t *testing.T) {
	out, st := withDirs(t, `zzc; hash -d zzc; echo "d=$?"; hash; hash -d zzc 2>/dev/null; echo "again=$?"`,
		func(d1, _ string) { hashable(t, d1, "zzc", ":") },
		func(r *Runner) {
			s := testSemantics()
			s.HashForgetsOneName = Yes
			r.Semantics = &s
			r.Diagnostics = &Diagnostics{HashEmptyTable: "hash: hash table empty"}
		})
	if st != 0 || !strings.Contains(out, "d=0\nhash: hash table empty\n") || !strings.Contains(out, "again=1") {
		t.Errorf("out=%q st=%d, want one name out and then a miss at 1", out, st)
	}
}

func TestHashPutsAPathThereByHandAndReadsItBack(t *testing.T) {
	out, st := run(t, `hash -p "$PWD/d1/zzp" zzq; echo "p=$?"; zzq; hash -t zzq | sed "s|.*/||"; hash -l | sed "s|/[^ ]*/||"`,
		func(r *Runner) {
			hashable(t, filepath.Join(r.Dir, "d1"), "zzp", "echo PUT")
			s := testSemantics()
			s.HashTakesAPathToRemember = Yes
			s.HashReportsThePath = Yes
			s.HashListsAsCommands = Yes
			r.Semantics = &s
		})
	if st != 0 || out != "p=0\nPUT\nzzp\nbuiltin hash -p zzp zzq\n" {
		t.Errorf("out=%q st=%d, want a name PATH would never find run and read back", out, st)
	}
}

func TestHashReportsThePathOfOneNameOrOfSeveral(t *testing.T) {
	out, st := withDirs(t, `zzc; zzd; hash -t zzc | sed "s|.*/||"; hash -t zzc zzd | sed "s|/.*/||"`,
		func(d1, _ string) {
			hashable(t, d1, "zzc", ":")
			hashable(t, d1, "zzd", ":")
		},
		func(r *Runner) {
			s := testSemantics()
			s.HashReportsThePath = Yes
			r.Semantics = &s
		})
	if st != 0 || out != "zzc\nzzc\tzzc\nzzd\tzzd\n" {
		t.Errorf("out=%q st=%d, want the path alone for one name and name-and-path for two", out, st)
	}
}

// TestHashAsksNothingWhereThePanelAgrees is the rule this tree keeps having
// to relearn: ask at the disagreement and nowhere else.
//
// A bare `hash`, a `hash -r` and a `hash name` are unanimous, so a shell that
// has chosen no dialect at all must run all three. It could not while the
// four letter axes were read on the way in — every one of those commands was
// refused at 2 to settle a letter none of them spelled.
func TestHashAsksNothingWhereThePanelAgrees(t *testing.T) {
	for _, src := range []string{`hash`, `hash -r`, `hash nosuchcmd-xyz`} {
		t.Run(src, func(t *testing.T) {
			out, _ := run(t, src, func(r *Runner) {
				sem := CoreSemantics()
				sem.HashReportsAMissingName = No
				sem.HashSearchesPathAlone = No
				r.Semantics = &sem
			})
			if strings.Contains(out, "no dialect was chosen") {
				t.Errorf("out=%q: %q consulted an axis it does not depend on", out, src)
			}
		})
	}
}

// And the other half of that pin: a call that *does* spell one of the letters
// is refused by a shell with no dialect, rather than being answered by a
// default nobody chose.
func TestHashAsksWhereTheLetterIsSpelled(t *testing.T) {
	out, st := run(t, `hash -t zzc`, func(r *Runner) {
		sem := CoreSemantics()
		r.Semantics = &sem
	})
	if st == 0 || !strings.Contains(out, "no dialect was chosen") {
		t.Errorf("out=%q st=%d, want the letter to be an unanswered axis here", out, st)
	}
}

// TestCommandTrackingCanStopTheTable is command tracking turned off, and the
// two questions that used to be modeled as one: whether the *automatic*
// hashing stops, and whether the builtin itself closes with it.
func TestCommandTrackingCanStopTheTable(t *testing.T) {
	for _, tc := range []struct {
		name           string
		obeys, refuses Answer
		want           string
	}{
		// bash: the builtin says one sentence at 1 to everything, and with
		// tracking back on there is nothing behind it.
		{"a stop, and a closed builtin", Yes, Yes, "st=1\nhash: hash table empty\n"},
		// zsh: nothing is remembered automatically, and the builtin goes on
		// answering — so an explicit `hash` names the command it was given
		// and the run that happened while tracking was off is not there.
		{"a stop, with the builtin open", Yes, No, "st=0\nhash: hash table empty\n"},
		// ksh93: the option is a preference and the table fills anyway.
		{"a preference", No, No, "st=0\nzzc\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := withDirs(t, `set +h; zzc; hash >/dev/null 2>&1; echo "st=$?"; set -h; hash 2>/dev/null | sed "s|.*/||"`,
				func(d1, _ string) { hashable(t, d1, "zzc", ":") },
				func(r *Runner) {
					s := testSemantics()
					s.HashObeysCommandTracking = tc.obeys
					s.HashRefusesWhileTrackingIsOff = tc.refuses
					s.SetHLetterTracksCommands = Yes
					r.Semantics = &s
					r.Diagnostics = &Diagnostics{
						HashDisabled:   "hash: hashing disabled",
						HashEmptyTable: "hash: hash table empty",
					}
				})
			if st != 0 || out != tc.want {
				t.Errorf("out=%q st=%d, want %q", out, st, tc.want)
			}
		})
	}
}

// TestAnExplicitHashStillWorksWhileTrackingIsOff is zsh's reading of the
// option from the other side: it stops what a *run* puts in the table and
// leaves `hash name` doing exactly what it says.
func TestAnExplicitHashStillWorksWhileTrackingIsOff(t *testing.T) {
	out, st := withDirs(t, `set +h; zzc; hash zzc; echo "n=$?"; hash | sed "s|.*/||"`,
		func(d1, _ string) { hashable(t, d1, "zzc", ":") },
		func(r *Runner) {
			s := testSemantics()
			s.HashObeysCommandTracking = Yes
			s.HashRefusesWhileTrackingIsOff = No
			s.SetHLetterTracksCommands = Yes
			r.Semantics = &s
		})
	if st != 0 || out != "n=0\nzzc\n" {
		t.Errorf("out=%q st=%d, want the named command hashed anyway", out, st)
	}
}

// TestHashingIsAskedAboutOnlyWhenTheOptionMoved is the other half of "ask at
// the disagreement", and the half with teeth: hashCommandRun runs in front of
// **every external command**, so an axis read there unconditionally would
// refuse every command a Runner with no dialect tried to start.
func TestHashingIsAskedAboutOnlyWhenTheOptionMoved(t *testing.T) {
	out, st := withDirs(t, `zzc; echo "st=$?"`,
		func(d1, _ string) { hashable(t, d1, "zzc", "echo V1") },
		func(r *Runner) {
			sem := CoreSemantics()
			r.Semantics = &sem
		})
	if st != 0 || out != "V1\nst=0\n" {
		t.Errorf("out=%q st=%d, want a command to run in a shell that has chosen no dialect", out, st)
	}
}

// TestTheReusableFormIsAShapeAndNotASecondAction is `-l` beside `-t`.
//
// Measured: `hash -l -t ls` and `hash -t -l ls` both write the line that
// would put the entry back, in either order, while `hash -l ls` with no `-t`
// prints nothing at all — an operand without `-t` is a name to hash.
func TestTheReusableFormIsAShapeAndNotASecondAction(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"l then t", `zzc; hash -l -t zzc`, "builtin hash -p zzc zzc\n"},
		{"t then l", `zzc; hash -t -l zzc`, "builtin hash -p zzc zzc\n"},
		{"l with an operand alone", `zzc; hash -l zzc`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, st := withDirs(t, tc.src+` | sed "s|/[^ ]*/||"`,
				func(d1, _ string) { hashable(t, d1, "zzc", ":") },
				func(r *Runner) {
					s := testSemantics()
					s.HashReportsThePath = Yes
					s.HashListsAsCommands = Yes
					r.Semantics = &s
				})
			if st != 0 || out != tc.want {
				t.Errorf("out=%q st=%d, want %q", out, st, tc.want)
			}
		})
	}
}
