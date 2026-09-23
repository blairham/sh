// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// ere answers the one axis these rows turn on the way the column that has the
// construct answers it, and nothing else: what a POSIX ERE is, is not a
// dialect's choice.
func ere() func(*Runner) {
	return func(r *Runner) {
		sem := permissive()
		sem.RegexQuotingMakesLiteral = Yes
		sem.EmptyRegexOperandIsAnError = Yes
		// The two the record raises, at the dense answers rematch() uses: they
		// are not what these rows are about, and an unanswered axis refuses.
		sem.RegexMatchSurvivesAFailedMatch = No
		sem.RegexMatchOmitsGroupsThatDidNotMatch = No
		r.Semantics = &sem
	}
}

// TestAPosixEREIsNotWhatTheEngineWouldCompile — `regexp` is RE2 and accepts a
// different language from POSIX ERE in both directions. Each row below answered
// the other way before #4173, and every one is measured against **bash 5.3.15
// in the digest-pinned image the suite is graded in** rather than only against
// the bash on the machine: the two C libraries disagree about three of these
// and the first attempt took the wrong one for two of them.
//
// The pattern is held in a parameter wherever a backslash is in play, so that
// the shell's own quote removal does not take it before the matcher sees it.
func TestAPosixEREIsNotWhatTheEngineWouldCompile(t *testing.T) {
	for _, tc := range []struct{ name, src, want string }{
		// A collating element and an equivalence class, which RE2 has not: in
		// the C locale every character is both of itself.
		{"a collating element is the character", `[[ a =~ [[.a.]] ]] && echo y || echo n`, "y"},
		{"and matches nothing else", `[[ b =~ [[.a.]] ]] && echo y || echo n`, "n"},
		{"an equivalence class is the character", `[[ a =~ [[=a=]] ]] && echo y || echo n`, "y"},
		{"one member of a larger set", `[[ x =~ [[.a.]x] ]] && echo y || echo n`, "y"},
		{"an end of a range", `[[ b =~ [[.a.]-[.c.]] ]] && echo y || echo n`, "y"},
		{"negated", `[[ b =~ [^[.a.]] ]] && echo y || echo n`, "y"},
		// A name longer than one character is the locale's table, which this
		// shell does not carry — and which the graded reference refuses too.
		{"a named element is refused", `[[ " " =~ [[.space.]] ]]; echo "st=$?"`, "st=2"},
		{"and so is an empty one", `[[ . =~ [[..]] ]]; echo "st=$?"`, "st=2"},
		// A backslash inside a bracket expression is an ordinary member.
		{"a backslash in a bracket is a member", `r='[\n]'; [[ '\' =~ $r ]] && echo y || echo n`, "y"},
		{"and so is the letter beside it", `r='[\n]'; [[ n =~ $r ]] && echo y || echo n`, "y"},
		{"no digit class in a bracket", `r='[\d]'; [[ 1 =~ $r ]] && echo y || echo n`, "n"},
		{"and the letter is one", `r='[\d]'; [[ d =~ $r ]] && echo y || echo n`, "y"},
		{"a backslash can end a range", `r='[a\-z]'; [[ b =~ $r ]] && echo y || echo n`, "y"},
		// A backslash before an ordinary character is that character. The six
		// GNU extension letters are the controls and are left to the engine.
		{"an escaped letter is the letter", `r='\d'; [[ d =~ $r ]] && echo y || echo n`, "y"},
		{"and is not a class", `r='\d'; [[ 7 =~ $r ]] && echo y || echo n`, "n"},
		{"a word class is still a word class", `r='\w'; [[ 7 =~ $r ]] && echo y || echo n`, "y"},
		{"and its negation", `r='\W'; [[ 7 =~ $r ]] && echo y || echo n`, "n"},
		{"the buffer anchors", `r='\` + "`" + `a'; [[ a =~ $r ]] && echo y || echo n`, "y"},
		// A `?` where an atom belongs, which is what `(?` is in an ERE.
		{"a non-capturing group is refused", `[[ abc =~ (?:a)bc ]]; echo "st=$?"`, "st=2"},
		{"and so is a flag group", `[[ abc =~ (?i)abc ]]; echo "st=$?"`, "st=2"},
		// An anchor is not an atom either.
		{"a repeated anchor is refused", `r='^*'; [[ abc =~ $r ]]; echo "st=$?"`, "st=2"},
		{"at the other end too", `r='a$*'; [[ abc =~ $r ]]; echo "st=$?"`, "st=2"},
		// An empty branch is **accepted**, which is the row the first reading
		// of this got wrong: BSD refuses it and the graded reference does not.
		{"an empty branch is taken", `r='(a|)'; [[ a =~ $r ]]; echo "st=$?"`, "st=0"},
		{"at the front as well", `r='(|a)'; [[ a =~ $r ]]; echo "st=$?"`, "st=0"},
		{"and a trailing bar", `r='a|'; [[ a =~ $r ]]; echo "st=$?"`, "st=0"},
		{"an empty group too", `[[ a =~ () ]]; echo "st=$?"`, "st=0"},
		// A repetition of a repetition is accepted, by rewriting.
		{"a repeat of a repeat", `r='a**'; [[ abc =~ $r ]]; echo "st=$?"`, "st=0"},
		{"and of a bound", `r='a{1}{2}'; [[ abc =~ $r ]]; echo "st=$?"`, "st=1"},
		{"the groups behind it keep their numbers", `r='(x)*(a)'; [[ a =~ $r ]]; echo "[${M[2]}]"`, "[a]"},
		// A bound with no lower half is one, and a brace that is not a bound
		// at all is refused.
		{"a bound with no lower half", `r='a{,2}'; [[ a =~ $r ]]; echo "st=$?"`, "st=0"},
		{"a brace that is not a bound", `r='a{x'; [[ 'a{x' =~ $r ]]; echo "st=$?"`, "st=2"},
		// A class may be neither end of a range, and a range neither end of
		// another — while a leading or trailing `-` is the character.
		{"a class is no range end", `r='[[:alpha:]-z]'; [[ a =~ $r ]]; echo "st=$?"`, "st=2"},
		{"nor is a range", `r='[a-b-c]'; [[ a =~ $r ]]; echo "st=$?"`, "st=2"},
		{"a leading dash is a member", `r='[--/]'; [[ . =~ $r ]] && echo y || echo n`, "y"},
		{"and a trailing one", `r='[a-]'; [[ - =~ $r ]] && echo y || echo n`, "y"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src, rematchERE())
			// The last line, because a refused pattern writes a diagnostic on
			// the way and what these rows are about is the answer.
			lines := strings.Split(strings.TrimSpace(out), "\n")
			if got := lines[len(lines)-1]; got != tc.want {
				t.Errorf("got %q, want %q (whole output %q)", got, tc.want, out)
			}
		})
	}
}

// rematchERE is ere() with the capture record named, for the one row that reads
// a group back.
func rematchERE() func(*Runner) {
	return func(r *Runner) {
		ere()(r)
		r.SetRegexMatch("M")
	}
}

// TestACompatibilityLevelTakesBackTheQuotingRule — quoting a `=~` operand
// became literal in bash 3.2, so at compatibility level 31 the operand is an
// expression again, quotes and all.
//
// Measured 2026-09-23 against bash 5.3.15 in the digest-pinned image the suite
// is graded in. Every row here answered the modern way before: the option and
// the parameter were both accepted and neither did anything.
func TestACompatibilityLevelTakesBackTheQuotingRule(t *testing.T) {
	level := func(r *Runner) {
		sem := permissive()
		sem.RegexQuotingMakesLiteral = Yes
		sem.EmptyRegexOperandIsAnError = Yes
		sem.CompatibilityLevelVariable = "BASH_COMPAT"
		// The two the record raises, as ere() answers them.
		sem.RegexMatchSurvivesAFailedMatch = No
		sem.RegexMatchOmitsGroupsThatDidNotMatch = No
		r.Semantics = &sem
		r.SetRegexMatch("M")
	}
	for _, tc := range []struct{ name, level, src, want string }{
		{"the modern reading", "", `[[ axb =~ "a.b" ]] && echo y || echo n`, "n"},
		{"at the level the rule arrived in", "31", `[[ axb =~ "a.b" ]] && echo y || echo n`, "y"},
		{"the dot is legibility only", "3.1", `[[ axb =~ "a.b" ]] && echo y || echo n`, "y"},
		{"the release after it does not", "32", `[[ axb =~ "a.b" ]] && echo y || echo n`, "n"},
		{"a later one does not either", "51", `[[ axb =~ "a.b" ]] && echo y || echo n`, "n"},
		// Every spelling of quoting, and a partly quoted operand.
		{"single quotes too", "31", `[[ axb =~ 'a.b' ]] && echo y || echo n`, "y"},
		{"a quoted expansion", "31", `r='a.b'; [[ axb =~ "$r" ]] && echo y || echo n`, "y"},
		{"one quoted span of several", "31", `[[ axb =~ "a"'.'b ]] && echo y || echo n`, "y"},
		// And the captures are the expression's.
		{"a quoted group still captures", "31", `[[ abc =~ "(b)" ]]; echo "[${M[1]}]"`, "[b]"},
		// The control: a value that is not a release leaves the modern answer.
		{"a value that is no release", "abc", `[[ axb =~ "a.b" ]] && echo y || echo n`, "n"},
		{"and one below the floor", "30", `[[ axb =~ "a.b" ]] && echo y || echo n`, "n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := tc.src
			if tc.level != "" {
				// Assigned by the script, which is the one route a person
				// takes and is what makes the level mid-run rather than a
				// field settled at startup.
				src = "BASH_COMPAT=" + tc.level + "\n" + src
			}
			out, _ := run(t, src, level)
			lines := strings.Split(strings.TrimSpace(out), "\n")
			if got := lines[len(lines)-1]; got != tc.want {
				t.Errorf("got %q, want %q (whole output %q)", got, tc.want, out)
			}
		})
	}
}

// TestAFailedCallIsJudgedAtTheCall — the ERR trap that fires because a function
// *call* failed is located at the call and not at the body's last command.
//
// Measured 2026-09-23 with `trap 'echo "ERR:$LINENO"' ERR` over script files,
// against bash 5.3.20, bash 5.3.15 in the graded image, and ksh93u+ — the two
// columns that number a command trap's body from where it fired. Every row
// answered with the body's line before, and zsh is unmoved because it numbers
// such a body from the body itself.
func TestAFailedCallIsJudgedAtTheCall(t *testing.T) {
	trap := func(r *Runner) {
		sem := permissive()
		// The answers an ERR body needs before a line rule is reachable at
		// all, at the column's own: see commandtrapline_test.go.
		sem.TrapHasErrCondition = Yes
		sem.ErrTrapRunsInsideFunctions = No
		sem.ErrTrapRunsInSubshells = No
		sem.CommandTrapBodyLine = TrapBodyLineOffsetFromWhereItFired
		r.Semantics = &sem
	}
	for _, tc := range []struct{ name, src, want string }{
		{
			"the call and not the body",
			"trap 'echo \"ERR:$LINENO\"' ERR\ng() { false; }\ng\necho done\n",
			"ERR:3\ndone",
		},
		{
			"the outermost call of a chain",
			"trap 'echo \"ERR:$LINENO\"' ERR\ng() { false; }\nh() { g; }\nh\necho done\n",
			"ERR:4\ndone",
		},
		{
			"inside a loop, the call's own line",
			"trap 'echo \"ERR:$LINENO\"' ERR\ng() { false; }\nfor i in 1; do\ng\ndone\n",
			"ERR:4",
		},
		{
			"and the body's own failures are unmoved",
			"trap 'echo \"ERR:$LINENO\"' ERR\nfalse\necho done\n",
			"ERR:2\ndone",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, _ := run(t, tc.src, trap)
			if got := strings.TrimSpace(out); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
