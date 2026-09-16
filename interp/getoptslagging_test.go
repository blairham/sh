// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// The other end of the question TestGetoptsClusterOptindAxis asks, and what
// the run that finds no more options does to the name (#3275).
//
// One dialect counts a word at its first letter, four at its last, and one
// does not count it until the following call needs the next word. The third
// answer is visible on a *lone* option as much as on a cluster, which is what
// separates it from the first: `-a -c` is 1 then 2 in zsh where every other
// column is 2 then 3.

func laggingSem(a Answer) Semantics {
	s := getoptsSem()
	s.GetoptsCountsTheWordOnTheNextCall = a
	return s
}

// The issue's own case: a cluster, then a lone option, then an operand, with
// OPTIND read after every call and the operand count read at the end.
func TestGetoptsCountsTheWordOnTheNextCallIsAnAxis(t *testing.T) {
	const src = `set -- -ab -c rest
OPTIND=1
getopts abc o; printf '%s,%s ' "$o" "$OPTIND"
getopts abc o; printf '%s,%s ' "$o" "$OPTIND"
getopts abc o; printf '%s,%s ' "$o" "$OPTIND"
getopts abc o; printf '%s,%s ' "$o" "$OPTIND"
shift $((OPTIND-1)); echo "rest=$*"`
	for _, tc := range []struct {
		name   string
		answer Answer
		want   string
	}{
		// bash 5.3.20, bash 3.2.57, bash as `sh` and ksh93u+ 2012-08-01.
		{"counted at its last letter", No, "a,1 b,2 c,3 ?,3 rest=rest\n"},
		// zsh 5.9.2, measured 2026-09-16 over a script file under `env -i
		// PATH=/usr/bin:/bin LC_ALL=C` with stdin from /dev/null.
		{"counted on the next call", Yes, "a,1 b,1 c,2 ?,3 rest=rest\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := laggingSem(tc.answer)
			out, _ := run(t, src, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
	t.Run("unanswered", func(t *testing.T) {
		sem := laggingSem(Unspecified)
		out, _ := run(t, src, func(r *Runner) { r.Semantics = &sem })
		const axis = "OPTIND staying on a spent word until the next `getopts` call"
		if !strings.Contains(out, axis) {
			t.Fatalf("got %q, want it to carry %q", out, axis)
		}
	})
}

// A lone option is the half a cluster cannot show: `-a` is one letter, so the
// word is spent at the first, and the two counting axes then disagree with
// each other rather than agreeing on 2. An implementation that folded this
// answer into the cluster question would pass the case above and fail here.
func TestGetoptsCountsTheWordOnTheNextCallOverLoneOptions(t *testing.T) {
	const src = `set -- -a -c rest
OPTIND=1
while getopts 'abc' o; do printf '[%s:%s]' "$o" "$OPTIND"; done
printf ' end=%s\n' "$OPTIND"`
	for _, tc := range []struct {
		name   string
		answer Answer
		want   string
	}{
		{"counted at its last letter", No, "[a:2][c:3] end=3\n"},
		{"counted on the next call", Yes, "[a:1][c:2] end=3\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := laggingSem(tc.answer)
			out, _ := run(t, src, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// An option that took an argument is not asked, in either spelling: every
// column leaves OPTIND at the first word neither the option nor its argument
// occupies. So the two answers are one number here, and an implementation
// that lagged on these too would be wrong in six columns out of seven.
func TestGetoptsCountsTheWordOnTheNextCallSkipsAnArgument(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"attached", `set -- -Bval rest`, "[B:val:2] end=2\n"},
		{"separate", `set -- -B val rest`, "[B:val:3] end=3\n"},
		{"attached after a cluster", `set -- -aBval rest`, "[a:unset:1][B:val:2] end=2\n"},
		{"separate after a cluster", `set -- -aB val rest`, "[a:unset:1][B:val:3] end=3\n"},
	} {
		for _, answer := range []Answer{No, Yes} {
			t.Run(tc.name+", "+answer.String(), func(t *testing.T) {
				sem := laggingSem(answer)
				src := tc.src + "\n" + `OPTIND=1
while getopts 'abB:' o; do printf '[%s:%s:%s]' "$o" "${OPTARG-unset}" "$OPTIND"; done
printf ' end=%s\n' "$OPTIND"`
				out, _ := run(t, src, func(r *Runner) { r.Semantics = &sem })
				if out != tc.want {
					t.Errorf("got %q, want %q", out, tc.want)
				}
			})
		}
	}
}

// A missing argument is the same question by another door: nothing was
// consumed, so the word is spent and the count does not move in the column
// that lags. Only reading OPTIND *on that call* tells the two apart — a
// one-word list ends at the same number whichever way it got there.
func TestGetoptsCountsTheWordOnTheNextCallOverAMissingArgument(t *testing.T) {
	const src = `set -- -ab
OPTIND=1
getopts 'ab:c' o 2>/dev/null; printf '%s,%s ' "$o" "$OPTIND"
getopts 'ab:c' o 2>/dev/null; printf '%s,%s ' "$o" "$OPTIND"
getopts 'ab:c' o 2>/dev/null; printf '%s,%s\n' "$o" "$OPTIND"`
	for _, tc := range []struct {
		name   string
		answer Answer
		want   string
	}{
		{"counted at its last letter", No, "a,1 ?,2 ?,2\n"},
		{"counted on the next call", Yes, "a,1 ?,1 ?,2\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := laggingSem(tc.answer)
			out, _ := run(t, src, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// The word count every column agrees on is the one `shift` reads, and it is
// the same number under both answers however the scan got there. A lag that
// forgot to catch up would land here rather than in the rows above.
func TestGetoptsCountsTheWordOnTheNextCallCatchesUpAtTheEnd(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		{"nothing to scan", `set --`, "1\n"},
		{"an operand only", `set -- rest`, "1\n"},
		{"a bare dash is an operand", `set -- - rest`, "1\n"},
		{"a double dash alone", `set -- --`, "2\n"},
		{"a cluster then a double dash", `set -- -ab -- rest`, "3\n"},
		{"a lone option then an operand", `set -- -a rest`, "2\n"},
		{"a bad letter in a cluster", `set -- -aZb rest`, "2\n"},
	} {
		for _, answer := range []Answer{No, Yes} {
			t.Run(tc.name+", "+answer.String(), func(t *testing.T) {
				sem := laggingSem(answer)
				src := tc.src + "\n" + `OPTIND=1
while getopts 'ab' o 2>/dev/null; do :; done
echo "$OPTIND"`
				out, _ := run(t, src, func(r *Runner) { r.Semantics = &sem })
				if out != tc.want {
					t.Errorf("got %q, want %q", out, tc.want)
				}
			})
		}
	}
}

// And inside a call that has a scan position of its own, which is where the
// two numbers can come apart without a script seeing it. Two calls, because
// one is not enough: a call entered with OPTIND at 1 and nothing read yet is
// handed a cursor it cannot be told apart by, so the call's own record is
// never taken.
func TestGetoptsCountsTheWordOnTheNextCallInsideACallWithItsOwnCursor(t *testing.T) {
	const src = `g() { OPTIND=1; while getopts 'abc' o "$@" >/dev/null 2>&1; do printf '%s,%s ' "$o" "$OPTIND"; done; printf 'end=%s\n' "$OPTIND"; }
g -a rest
g -ab -c rest`
	for _, tc := range []struct {
		name   string
		answer Answer
		want   string
	}{
		{"counted at its last letter", No, "a,2 end=2\na,1 b,2 c,3 end=3\n"},
		{"counted on the next call", Yes, "a,1 end=2\na,1 b,1 c,2 end=3\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sem := laggingSem(tc.answer)
			sem.GetoptsFunctionPosition = GetoptsFunctionPositionIsTheCallsOwn
			sem.GetoptsLocalOptindRestoresTheCursor = No
			out, _ := run(t, src, func(r *Runner) { r.Semantics = &sem })
			if out != tc.want {
				t.Errorf("got %q, want %q", out, tc.want)
			}
		})
	}
}

// The name at the end of the options, which reads as "the last letter again"
// to anyone whose probe scanned first.
func TestGetoptsEndOfOptionsNamesItIsAnAxis(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"nothing to scan at all", `set --`},
		{"an operand", `set -- rest`},
		{"a double dash", `set -- --`},
	} {
		for _, names := range []Answer{Yes, No} {
			want := "PRESET 1\n"
			if names == Yes {
				want = "? 1\n"
			}
			t.Run(tc.name+", "+names.String(), func(t *testing.T) {
				sem := getoptsSem()
				sem.GetoptsEndOfOptionsNamesIt = names
				src := tc.src + "\n" + `OPTIND=1; o=PRESET; getopts ab o; printf '%s %s\n' "$o" "$?"`
				out, _ := run(t, src, func(r *Runner) { r.Semantics = &sem })
				if out != want {
					t.Errorf("got %q, want %q", out, want)
				}
			})
		}
	}
	t.Run("unanswered", func(t *testing.T) {
		sem := getoptsSem()
		sem.GetoptsEndOfOptionsNamesIt = Unspecified
		out, _ := run(t, `getopts ab o rest`, func(r *Runner) { r.Semantics = &sem })
		const axis = "`getopts` writing `?` into the name when it runs out of options"
		if !strings.Contains(out, axis) {
			t.Fatalf("got %q, want it to carry %q", out, axis)
		}
	})
}

// The letter a scan did read is written under both answers: this axis is
// about the run that reports "no more options" and about nothing else.
func TestGetoptsEndOfOptionsNamesItLeavesAFoundLetterAlone(t *testing.T) {
	const src = `set -- -a rest
OPTIND=1; o=PRESET; getopts ab o; printf '%s %s\n' "$o" "$?"`
	for _, names := range []Answer{Yes, No} {
		t.Run(names.String(), func(t *testing.T) {
			sem := getoptsSem()
			sem.GetoptsEndOfOptionsNamesIt = names
			out, _ := run(t, src, func(r *Runner) { r.Semantics = &sem })
			if out != "a 0\n" {
				t.Errorf("got %q, want the letter", out)
			}
		})
	}
}
