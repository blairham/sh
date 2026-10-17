// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package policy_test

import (
	"strings"
	"testing"

	"github.com/blairham/sh/internal/policy"
	"github.com/blairham/sh/interp"
)

// The three rules #6938 could not write, as one policy beside a grant of all
// three programs. Each row says what the shell built and what the policy must
// answer; the allowed rows are what keeps this from passing for a policy that
// refuses the program outright.

func run(path string, args ...string) interp.Action {
	return interp.Action{Kind: interp.ActionExec, Path: path, Args: append([]string{path}, args...)}
}

const argvPolicy = `version 1
default deny
allow exec-unconfined /usr/bin/make
allow exec-unconfined /usr/bin/git
allow exec-unconfined /usr/local/go/bin/go
deny exec-argv /**/make -- check
deny exec-argv /**/make -- oracle
deny exec-argv /**/git -- stash
deny exec-argv /**/git -- worktree remove
deny exec-argv /**/git -- commit --no-verify
deny exec-argv /**/go -- tool golangci-lint
`

func TestAnArgvRuleRefusesTheSubcommandAndNotTheProgram(t *testing.T) {
	t.Parallel()
	p := parse(t, argvPolicy)
	want(t, p, interp.Allow,
		run("/usr/bin/make", "build"),
		run("/usr/bin/make"),
		run("/usr/bin/git", "status"),
		run("/usr/bin/git", "worktree", "list"),
		run("/usr/bin/git", "commit", "-m", "x"),
		run("/usr/local/go/bin/go", "build", "./..."),
		run("/usr/local/go/bin/go", "vet", "./interp"),
		run("/usr/local/go/bin/go", "test", "-run", "TestX", "./interp"),
	)
	want(t, p, interp.Deny,
		run("/usr/bin/make", "check"),
		run("/usr/bin/make", "oracle"),
		run("/usr/bin/git", "stash"),
		run("/usr/bin/git", "stash", "pop"),
		run("/usr/bin/git", "worktree", "remove", "../x"),
		run("/usr/bin/git", "commit", "--no-verify", "-m", "x"),
		run("/usr/local/go/bin/go", "tool", "golangci-lint", "run"),
	)
}

// TestOptionsBeforeTheSubcommandDoNotHideIt is the issue's own counterexample
// to a prefix match. A rule that read the vector from the front would let all
// of these through.
func TestOptionsBeforeTheSubcommandDoNotHideIt(t *testing.T) {
	t.Parallel()
	p := parse(t, argvPolicy)
	want(t, p, interp.Deny,
		run("/usr/bin/git", "-C", "/srv/repo", "stash"),
		run("/usr/bin/git", "-c", "core.pager=cat", "worktree", "remove", "x"),
		run("/usr/bin/git", "commit", "-m", "x", "--no-verify"),
		run("/usr/bin/make", "-j8", "check"),
		run("/usr/bin/make", "-C", "/srv/repo", "build", "check"),
	)
}

// TestTheWordsAreInOrder: a rule about `worktree remove` is not a rule about
// any vector holding both words.
func TestTheWordsAreInOrder(t *testing.T) {
	t.Parallel()
	p := parse(t, argvPolicy)
	want(t, p, interp.Allow, run("/usr/bin/git", "remove", "worktree"))
}

// TestTheWordsAreWholeArguments: `check` is not `oracle-check`, and `stash` is
// not a substring of an argument that happens to contain it.
func TestTheWordsAreWholeArguments(t *testing.T) {
	t.Parallel()
	p := parse(t, argvPolicy)
	want(t, p, interp.Allow,
		run("/usr/bin/make", "oracle-check"),
		run("/usr/bin/make", "CHECK=1"),
		run("/usr/bin/git", "commit", "-m", "stash the thing"),
	)
}

// TestTheWideningIsOverRefusalAndIsMeant pins the cost the design doc names,
// so that narrowing the match to a prefix is a visible change rather than a
// quiet one: a word anywhere refuses, even where it is an option's value.
func TestTheWideningIsOverRefusalAndIsMeant(t *testing.T) {
	t.Parallel()
	p := parse(t, argvPolicy)
	want(t, p, interp.Deny, run("/usr/bin/git", "log", "--grep", "stash"))
}

// TestTheProgramIsTheResolvedPathNotTheNameItWasCalledBy, because argv[0] is
// whatever the script typed and the path is what the shell found. A shim and a
// system binary are one rule under `/**/git`; a program named `stash` is not
// an argument.
func TestTheProgramIsTheResolvedPathNotTheNameItWasCalledBy(t *testing.T) {
	t.Parallel()
	p := parse(t, argvPolicy+"allow exec-unconfined /home/u/.asdf/shims/git\nallow exec-unconfined /usr/bin/stash\n")
	want(t, p, interp.Deny, run("/home/u/.asdf/shims/git", "stash"))
	want(t, p, interp.Allow,
		interp.Action{Kind: interp.ActionExec, Path: "/usr/bin/git", Args: []string{"stash", "status"}},
		run("/usr/bin/stash"),
	)
}

// TestAnArgvRuleIsNotAnExecGrant: under default deny, a deny about some
// arguments must not be read as permission for the others.
func TestAnArgvRuleIsNotAnExecGrant(t *testing.T) {
	t.Parallel()
	p := parse(t, "version 1\ndefault deny\ndeny exec-argv /usr/bin/git -- stash\n")
	want(t, p, interp.Deny, run("/usr/bin/git", "status"), run("/usr/bin/git", "stash"))
}

// TestAnArgvRuleSpeaksOnlyAboutExec: the selector is the exec slot and no
// other, so a file at the program's path is untouched by it.
func TestAnArgvRuleSpeaksOnlyAboutExec(t *testing.T) {
	t.Parallel()
	p := parse(t, "version 1\ndefault allow\ndeny exec-argv /usr/bin/git -- stash\n")
	want(t, p, interp.Allow, open("/usr/bin/git", false), stat("/usr/bin/git"))
}

// TestAProgramPatternKeepsItsSpaces is the reason for the delimiter. Split on
// whitespace, this rule would be the program `/Applications/My` and never
// fire.
func TestAProgramPatternKeepsItsSpaces(t *testing.T) {
	t.Parallel()
	p := parse(t, "version 1\ndefault allow\ndeny exec-argv /Applications/My App/bin/tool -- sync  now\n")
	want(t, p, interp.Deny, run("/Applications/My App/bin/tool", "sync", "now"))
	want(t, p, interp.Allow, run("/Applications/My App/bin/tool", "status"))
}

// TestOnlyTheFirstDelimiterSplits: after it, `--` is an ordinary word, which is
// what `git checkout -- .` needs.
func TestOnlyTheFirstDelimiterSplits(t *testing.T) {
	t.Parallel()
	p := parse(t, "version 1\ndefault allow\ndeny exec-argv /usr/bin/git -- checkout -- .\n")
	want(t, p, interp.Deny, run("/usr/bin/git", "checkout", "--", "."))
	want(t, p, interp.Allow, run("/usr/bin/git", "checkout", "main"))
}

func TestAnArgvRuleIsDenyOnlyAndSaysWhy(t *testing.T) {
	t.Parallel()
	_, err := policy.Parse(strings.NewReader("version 1\ndefault deny\nallow exec-argv /usr/bin/git -- status\n"))
	if err == nil {
		t.Fatal("allow exec-argv parsed; a grant must not rest on a best-effort match")
	}
	for _, w := range []string{"line 3", "deny only", "allow exec-unconfined"} {
		if !strings.Contains(err.Error(), w) {
			t.Errorf("%q does not say %q", err, w)
		}
	}
}

func TestABadArgvRuleIsRefusedAndNamed(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"no delimiter":               "deny exec-argv /usr/bin/git stash",
		"no words":                   "deny exec-argv /usr/bin/git --",
		"no program":                 "deny exec-argv -- stash",
		"a relative program":         "deny exec-argv git -- stash",
		"a malformed program glob":   "deny exec-argv /usr/[bin/git -- stash",
		"a glob in a word":           "deny exec-argv /usr/bin/make -- check*",
		"a class in a word":          "deny exec-argv /usr/bin/make -- [ab]",
		"a delimiter glued to words": "deny exec-argv /usr/bin/git --stash",
		"a default":                  "default deny exec-argv",
	}
	for what, line := range cases {
		_, err := policy.Parse(strings.NewReader("version 1\n" + line + "\n"))
		if err == nil {
			t.Errorf("%s: %q parsed", what, line)
			continue
		}
		if !strings.Contains(err.Error(), "line 2") {
			t.Errorf("%s: %q does not name the line", what, err)
		}
	}
}

// TestAnArgvRuleFollowsTheOtherDenyRules: the program pattern is a pattern
// like any other, so it gets the platform alias and, being a deny, the fold.
func TestAnArgvRuleFollowsTheOtherDenyRules(t *testing.T) {
	t.Parallel()
	p := parse(t, "version 1\ndefault allow\ndeny exec-argv /opt/Tools/git -- stash\n")
	want(t, p, interp.Deny, run("/opt/tools/GIT", "stash"))
	want(t, p, interp.Allow, run("/opt/tools/GIT", "status"))
}

// TestAnArgvRuleRendersBackAsWritten, delimiter included, so the line an
// operator is shown is a line the parser takes.
func TestAnArgvRuleRendersBackAsWritten(t *testing.T) {
	t.Parallel()
	r, err := policy.ParseRule(interp.Deny, "exec-argv /usr/bin/git  --  worktree   remove")
	if err != nil {
		t.Fatal(err)
	}
	const want = "deny exec-argv /usr/bin/git -- worktree remove"
	if got := r.String(); got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
	again, err := policy.ParseRule(interp.Deny, strings.TrimPrefix(r.String(), "deny "))
	if err != nil || again.String() != want {
		t.Errorf("the rendering does not parse back to itself: %q, %v", again.String(), err)
	}
}

// TestAnArgvDenyComposesLikeAnyOtherDeny: concatenating a file that grants git
// with one that carves `stash` out of it refuses `stash` in either order.
func TestAnArgvDenyComposesLikeAnyOtherDeny(t *testing.T) {
	t.Parallel()
	grant := "allow exec-unconfined /usr/bin/git\n"
	carve := "deny exec-argv /usr/bin/git -- stash\n"
	for _, body := range []string{grant + carve, carve + grant} {
		p := parse(t, "version 1\ndefault deny\n"+body)
		want(t, p, interp.Deny, run("/usr/bin/git", "stash"))
		want(t, p, interp.Allow, run("/usr/bin/git", "status"))
	}
}
