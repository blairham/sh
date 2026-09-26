// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// ReadArrayDefaultStyle is what the `-A` spelling of `read`'s array letter
// fills when the line names no parameter at all.
//
// The two shells with the letter disagree, so this cannot ride the letter the
// way the rest of the array rules do. Measured 2026-09-26, `-f` for zsh and a
// script file otherwise, each row read back with `typeset -p`:
//
//	read -A <<<'a b c'          zsh 5.9.2     reply=(a b c), REPLY not created
//	read -A <<<'a b c'          ksh93u+ 2012  REPLY='a b c', reply untouched
//	read -a  <<<'a b c'         bash 5.3.20   `-a: option requires an argument`
//
// The zsh reading is the neighbour of the bare-read rule and is deliberately
// *not* the same parameter: a bare `read` fills the scalar `REPLY` and
// `read -A` fills the array `reply`, and the shell is case-sensitive about
// which. Both were measured with the other pre-set, so neither row is reading
// back what it wrote a moment earlier:
//
//	reply=(x y); read -A <<<'hello world'   reply=(hello world)
//	reply=scalar; read <<<'plain'           reply=scalar, REPLY=plain
//
// The ksh93 reading is not a second default name. `read -A` there answers
// exactly as a bare `read` does — one scalar holding the line, stripped of
// leading and trailing `IFS` whitespace and split no further — so the letter
// contributes nothing once there is no name for it to apply to, and `reply`
// is a parameter ksh93 has never heard of.
//
// bash cannot be asked. Its spelling is the lowercase `-a`, whose name is the
// option's own argument, so a line with no name for the array does not reach
// this question: it is refused by the option parser first.
type ReadArrayDefaultStyle int

const (
	// ReadArrayDefaultUnspecified is no answer, and is refused like any
	// other.
	ReadArrayDefaultUnspecified ReadArrayDefaultStyle = iota
	// ReadArrayDefaultIsTheArrayReply fills the array `reply` — lowercase,
	// and a different parameter from the `REPLY` a bare `read` fills. The
	// line is split into elements exactly as it would be for a name the
	// script wrote. zsh.
	ReadArrayDefaultIsTheArrayReply
	// ReadArrayDefaultIsAPlainRead drops the letter: with no name to apply
	// it to, the builtin reads as a bare `read` does and leaves one scalar
	// holding the line. ksh93.
	ReadArrayDefaultIsAPlainRead
)

func (s ReadArrayDefaultStyle) String() string {
	switch s {
	case ReadArrayDefaultIsTheArrayReply:
		return "ReadArrayDefaultIsTheArrayReply"
	case ReadArrayDefaultIsAPlainRead:
		return "ReadArrayDefaultIsAPlainRead"
	}
	return "ReadArrayDefaultUnspecified"
}

// readArrayDefaultName is the array `read -A` fills when the line names no
// parameter, and whether the letter names an array at all.
//
// The second return is false for the reading that drops the letter, which is
// not the same as an empty name: an empty name is a name the script wrote and
// is judged as one.
func (r *Runner) readArrayDefaultName() (string, bool) {
	switch r.sem().ReadArrayDefault {
	case ReadArrayDefaultIsTheArrayReply:
		return "reply", true
	case ReadArrayDefaultIsAPlainRead:
		return "", false
	case ReadArrayDefaultUnspecified:
	}
	r.diagf("%s\n", r.unanswered("what `read -A` fills when the line names no parameter"))
	r.status = 2
	r.unspecified = true
	return "", false
}
