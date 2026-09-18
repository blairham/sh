// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// LastBackgroundPidPolicy is what `$!` reads before a background command has
// been started.
//
// Two questions look like one here, and the panel answers them in three
// combinations. The first is what the parameter *reads* — nothing, or a
// number nothing ever had. The second is whether it is *set*, which is what
// `${!-word}`, `${!+word}` and `set -u` can each tell apart and a bare `$!`
// cannot.
//
// Measured 2026-09-18, script files under `env -i PATH=/usr/bin:/bin LC_ALL=C`
// with stdin from /dev/null, before any `&` has run:
//
//	                    ${!-unset}   ${!+set}   set -u; echo "[$!]"
//	bash 5.3.20         unset        (empty)    $!: unbound variable, 127
//	dash 0.5.12         unset        (empty)    !: parameter not set, 2
//	BusyBox ash 1.37.0  unset        (empty)    !: parameter not set, 2
//	ksh93u+ 2012-08-01  unset        (empty)    [] then st=0
//	zsh 5.9.2           0            set        [0] then st=0
//
// So ksh93 is the combination two flags could not hold: the parameter is
// unset to every operator that can see the difference, and `set -u` still has
// nothing to say about it. This was a pair of Answer fields —
// `LastBackgroundPidIsZeroBeforeAnyJob` and
// `LastBackgroundPidIsUnsetBeforeAnyJob` — and their four combinations spelled
// one state no shell has (set and empty, quiet) and left ksh93's with no
// spelling at all, which is why ksh93 was recorded as the empty-and-set column
// and `${!-unset}` answered `[]` there (#3011).
//
// ksh93's quiet is not `UnsetPositionalIsAllowed` reached by another route.
// That axis is ksh93 letting an argument it was not given be empty; zsh
// refuses `$1` and does not refuse `$!`, so the two split the panel
// differently and neither predicts the other.
//
// Read without asking, and Unspecified reads as a set, empty parameter. A
// dialect that has chosen nothing must still be able to run the `p=$!` of an
// ordinary script, including the read before its first job, where nothing is
// in dispute.
type LastBackgroundPidPolicy int

const (
	// LastBackgroundPidUnspecified is no answer, and reads as a parameter
	// that is set and holds nothing — which is what this shell did before
	// the question was asked, and what no column in the panel does.
	LastBackgroundPidUnspecified LastBackgroundPidPolicy = iota
	// LastBackgroundPidZero is zsh: a recorded `0`, set, and `set -u` has
	// nothing to say about it. Zero is not the same answer as nothing, and
	// the difference is reachable — a background builtin runs in this
	// process and its job carries no pid, so a shell really can hold a
	// recorded zero.
	LastBackgroundPidZero
	// LastBackgroundPidUnset is bash, dash and BusyBox ash: the parameter is
	// unset, and `set -u` is fatal about it in the same words and at the
	// same status an unset positional gets.
	LastBackgroundPidUnset
	// LastBackgroundPidUnsetButNotRefused is ksh93: unset to `${!-word}` and
	// `${!+word}`, and `set -u` still carries on at 0 with an empty
	// expansion. `set -u` there is about names the script could have
	// assigned, which `$!` is not.
	LastBackgroundPidUnsetButNotRefused
)

func (p LastBackgroundPidPolicy) String() string {
	switch p {
	case LastBackgroundPidZero:
		return "zero"
	case LastBackgroundPidUnset:
		return "unset"
	case LastBackgroundPidUnsetButNotRefused:
		return "unset but not refused"
	}
	return "unspecified"
}

// lastBackgroundPidIsUnset reports whether `$!` reads as an unset parameter
// because no background command has been started yet. The two answers that
// say unset part company only over `set -u`, which is the next question and
// not this one.
func (r *Runner) lastBackgroundPidIsUnset() bool {
	if r.lastJobPIDSet {
		return false
	}
	switch r.sem().LastBackgroundPid {
	case LastBackgroundPidUnset, LastBackgroundPidUnsetButNotRefused:
		return true
	}
	return false
}

// lastBackgroundPidRefusedUnderNounset reports whether `set -u` stops the
// script over that unset parameter. Asked only where the read above already
// said unset, so the one column that is quiet about it is the whole of what
// this decides.
func (r *Runner) lastBackgroundPidRefusedUnderNounset() bool {
	return r.sem().LastBackgroundPid == LastBackgroundPidUnset
}
