// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package policy_test

import (
	"context"
	"strings"
	"sync"
	"syscall"
	"testing"

	"github.com/blairham/sh/internal/policy"
	"github.com/blairham/sh/interp"
)

// A denial that cannot be shown to actually deny is worthless, so these tests
// are written as questions about decisions rather than about the parser: each
// one builds a policy the way a person would write one and then asks it what
// it does with an action the interpreter would really produce.

func parse(t *testing.T, src string) *policy.Policy {
	t.Helper()
	p, err := policy.Parse(strings.NewReader(src))
	if err != nil {
		t.Fatalf("parsing:\n%s\nfailed: %v", src, err)
	}
	return p
}

func decide(t *testing.T, p *policy.Policy, a interp.Action) interp.Decision {
	t.Helper()
	return p.Allow(t.Context(), a)
}

func open(path string, write bool) interp.Action {
	return interp.Action{Kind: interp.ActionOpen, Path: path, Write: write}
}

func stat(path string) interp.Action {
	return interp.Action{Kind: interp.ActionStat, Path: path}
}

func list(path string) interp.Action {
	return interp.Action{Kind: interp.ActionReadDir, Path: path}
}

func exec(path string) interp.Action {
	return interp.Action{Kind: interp.ActionExec, Path: path, Args: []string{path}}
}

func want(t *testing.T, p *policy.Policy, d interp.Decision, as ...interp.Action) {
	t.Helper()
	for _, a := range as {
		if got := decide(t, p, a); got != d {
			t.Errorf("%s %s: got %s, want %s", a.Kind, a.Path, name(got), name(d))
		}
	}
}

func name(d interp.Decision) string {
	if d == interp.Allow {
		return "allow"
	}
	return "deny"
}

// TestSilenceIsRefusal is the default posture, and it is the single most
// important assertion here.
//
// A policy that says nothing about a path refuses it, for every kind. The
// per-kind question has a sharper answer than "a sandbox that defaults to
// allow is not one": if opens defaulted to deny while probes defaulted to
// allow, a script could learn that a file exists by asking the policy, and the
// ENOENT convention on ActionStat exists precisely to keep that difference
// invisible.
func TestSilenceIsRefusal(t *testing.T) {
	t.Parallel()
	p := parse(t, "version 1\n")
	want(t, p, interp.Deny,
		exec("/bin/echo"),
		open("/srv/x", false),
		open("/srv/x", true),
		stat("/srv/x"),
		list("/srv"),
		interp.Action{Kind: interp.ActionSignal, PID: 42, Signal: syscall.SIGTERM},
	)
}

// TestTheZeroPolicyRefuses guards a fail-open security type.
//
// interp.Allow is the zero value of interp.Decision, and correctly so — a
// Runner with no gate is unsandboxed. A Policy that stored its defaults as
// Decision would therefore allow everything before anyone filled it in, which
// is the worst possible default for this type and is exactly the kind of thing
// that is never noticed until it matters.
func TestTheZeroPolicyRefuses(t *testing.T) {
	t.Parallel()
	var zero policy.Policy
	want(t, &zero, interp.Deny, exec("/bin/echo"), open("/srv/x", false), stat("/srv/x"))
	var nilp *policy.Policy
	if got := nilp.Allow(t.Context(), exec("/bin/echo")); got != interp.Deny {
		t.Errorf("a nil policy answered %s", name(got))
	}
}

// TestDefaultAllowIsTheWatchingMode pins the other posture, which is what
// turns this into the -deny debug flag: allow everything, carve holes.
func TestDefaultAllowIsTheWatchingMode(t *testing.T) {
	t.Parallel()
	p := parse(t, "version 1\ndefault allow\ndeny read /etc/**\n")
	want(t, p, interp.Allow, exec("/bin/echo"), open("/srv/x", true), stat("/srv/x"))
	want(t, p, interp.Deny, open("/etc/passwd", false), stat("/etc/passwd"), list("/etc"))
	// A deny on `read` says nothing about writing, and the file says so
	// plainly. This is the case a selector that quietly meant "everything"
	// would get wrong in the dangerous direction.
	want(t, p, interp.Allow, open("/etc/passwd", true))
}

// TestReadCoversTheProbes is the selector grouping, and it exists because the
// ungrouped form is the mistake people actually make: `allow read /srv/**` and
// then `[ -f /srv/x ]` comes back false.
func TestReadCoversTheProbes(t *testing.T) {
	t.Parallel()
	p := parse(t, "version 1\nallow read /srv/**\n")
	want(t, p, interp.Allow, open("/srv/x", false), stat("/srv/x"), list("/srv/d"))
	want(t, p, interp.Deny, open("/srv/x", true), exec("/srv/x"))
}

// TestReadingAndWritingAreDifferentQuestions is why a rule is keyed by
// something finer than the action kind.
//
// One ActionOpen is two questions: reading a script's input and destroying its
// output. A policy keyed by the kind alone could not tell them apart, and the
// direction the mistake falls in is the dangerous one.
func TestReadingAndWritingAreDifferentQuestions(t *testing.T) {
	t.Parallel()
	p := parse(t, "version 1\nallow read /srv/**\nallow write /srv/out/**\n")
	want(t, p, interp.Allow, open("/srv/in", false), open("/srv/out/x", true))
	want(t, p, interp.Deny, open("/srv/in", true))
	// And `open` is the both-directions selector, for a rule that means it.
	q := parse(t, "version 1\nallow open /srv/**\n")
	want(t, q, interp.Allow, open("/srv/x", false), open("/srv/x", true))
	want(t, q, interp.Deny, stat("/srv/x"))
}

// TestDenyOverridesWhateverTheOrder is the precedence rule, asserted from both
// directions because the whole point is that the directions agree.
//
// Order-independence is what makes concatenation safe: two policies joined
// have to allow only what both allow, or a composed policy means something
// that depends on which half was read first.
func TestDenyOverridesWhateverTheOrder(t *testing.T) {
	t.Parallel()
	after := parse(t, "version 1\nallow read /srv/**\ndeny read /srv/secret/**\n")
	before := parse(t, "version 1\ndeny read /srv/secret/**\nallow read /srv/**\n")
	for _, p := range []*policy.Policy{after, before} {
		want(t, p, interp.Allow, open("/srv/x", false))
		want(t, p, interp.Deny, open("/srv/secret/k", false), stat("/srv/secret"))
	}
}

// TestConcatenationIsIntersection is the property the precedence rule was
// chosen for, stated as the thing an operator would actually do.
func TestConcatenationIsIntersection(t *testing.T) {
	t.Parallel()
	base := "allow read /srv/**\n"
	extra := "deny read /srv/secret/**\nallow read /var/log/**\n"
	both := parse(t, "version 1\n"+base+extra)
	reversed := parse(t, "version 1\n"+extra+base)
	for _, p := range []*policy.Policy{both, reversed} {
		want(t, p, interp.Allow, open("/srv/x", false), open("/var/log/a", false))
		want(t, p, interp.Deny, open("/srv/secret/k", false))
	}
}

// TestASubtreePatternIncludesItsRoot is the `**`-matches-nothing case.
//
// If `/srv/**` did not match `/srv`, every rule about a subtree would need a
// second rule about its root and everybody would forget it — most visibly for
// a directory read, which is the action `cd /srv` and `echo /srv/*` both make
// of the root itself.
func TestASubtreePatternIncludesItsRoot(t *testing.T) {
	t.Parallel()
	p := parse(t, "version 1\nallow read /srv/**\n")
	want(t, p, interp.Allow, stat("/srv"), list("/srv"), stat("/srv/a/b/c"))
}

// TestPatternsMatchComponentsAndNotCharacters is the bug a string prefix has.
//
// `-deny /etc` as a character prefix refuses /etcetera because the name starts
// the same way. A rule that did that would refuse a neighboring directory
// nobody meant, and — worse under a deny-overrides policy — would do it
// silently.
func TestPatternsMatchComponentsAndNotCharacters(t *testing.T) {
	t.Parallel()
	p := parse(t, "version 1\ndefault allow\ndeny read /etc/**\n")
	want(t, p, interp.Deny, open("/etc/passwd", false), open("/etc", false))
	want(t, p, interp.Allow, open("/etcetera/x", false), open("/etc.bak/x", false))
}

// TestAStarDoesNotCrossASeparator keeps the glob honest about depth, which is
// the difference between "the files in this directory" and "the tree".
func TestAStarDoesNotCrossASeparator(t *testing.T) {
	t.Parallel()
	p := parse(t, "version 1\nallow read /srv/*\n")
	want(t, p, interp.Allow, open("/srv/a", false))
	want(t, p, interp.Deny, open("/srv/a/b", false))
}

// TestTheGlobHasTheUsualPowers pins the parts path.Match brings, so that a
// rule someone writes out of habit means what they expect.
func TestTheGlobHasTheUsualPowers(t *testing.T) {
	t.Parallel()
	p := parse(t, "version 1\nallow read /srv/log-?.txt\nallow read /srv/[abc]/**\nallow read /a/**/z\n")
	want(t, p, interp.Allow,
		open("/srv/log-1.txt", false),
		open("/srv/b/deep/file", false),
		open("/a/z", false),
		open("/a/one/two/z", false),
	)
	want(t, p, interp.Deny,
		open("/srv/log-10.txt", false),
		open("/srv/d/file", false),
		open("/a/one/two", false),
	)
}

// TestDotDotCannotWalkOutOfAPattern is the escape a name-based rule has to
// close, and the only one it can close honestly.
//
// Paths are cleaned lexically before matching, so /srv/../etc/passwd is
// matched as /etc/passwd. Symlinks are the escape that stays open — resolving
// them would mean the policy performing ungated filesystem reads of its own
// and would still be a time-of-check race — and that limit is written down in
// docs/design/sandboxing.md rather than papered over here.
func TestDotDotCannotWalkOutOfAPattern(t *testing.T) {
	t.Parallel()
	p := parse(t, "version 1\nallow read /srv/**\n")
	want(t, p, interp.Deny, open("/srv/../etc/passwd", false))
	q := parse(t, "version 1\ndefault allow\ndeny read /etc/**\n")
	want(t, q, interp.Deny, open("/srv/../etc/passwd", false))
}

// TestARelativePathIsNotAddressable pins the fail-closed reading.
//
// Patterns are absolute, because a relative one would mean "relative to a
// working directory" and the policy owns none. So a relative path matches no
// pattern and falls to the default, which under any policy worth the name is a
// refusal. In practice the interpreter resolves against the Runner's directory
// before it asks — a redirect joins r.Dir, a glob walks from r.workDir() — so
// one arriving here means something upstream did not.
func TestARelativePathIsNotAddressable(t *testing.T) {
	t.Parallel()
	p := parse(t, "version 1\nallow read /srv/**\n")
	want(t, p, interp.Deny, open("srv/x", false), open("x", false))
}

// TestAllowingAnExecPermitsFindingIt is the one implication in the engine.
//
// A PATH search stats each candidate, and `command -v` and `[ -x ]` ask the
// same question the search does. A policy that let a script run a program but
// not discover it would not mean what it says, and it leaks nothing: the stat
// can only succeed for a path the script was going to be allowed to execute.
func TestAllowingAnExecPermitsFindingIt(t *testing.T) {
	t.Parallel()
	p := parse(t, "version 1\nallow exec /usr/bin/git\n")
	want(t, p, interp.Allow, exec("/usr/bin/git"), stat("/usr/bin/git"))
	want(t, p, interp.Deny,
		stat("/usr/bin/curl"),
		open("/usr/bin/git", false),
		list("/usr/bin"),
	)
}

// TestTheExecImplicationNeverBeatsADeny is the bound on it. The implication
// rescues a stat that would otherwise fall to the default; an explicit deny
// has already decided the question and stays decided.
func TestTheExecImplicationNeverBeatsADeny(t *testing.T) {
	t.Parallel()
	p := parse(t, "version 1\nallow exec /usr/bin/git\ndeny stat /usr/bin/**\n")
	want(t, p, interp.Allow, exec("/usr/bin/git"))
	want(t, p, interp.Deny, stat("/usr/bin/git"))
	// And the implication reads the decision on the rule it consults, which is
	// the part that would be easy to drop: a rule *refusing* an exec is not a
	// reason to permit the stat that would find it.
	q := parse(t, "version 1\ndeny exec /usr/bin/curl\n")
	want(t, q, interp.Deny, exec("/usr/bin/curl"), stat("/usr/bin/curl"))
}

// TestSignalsAreNamedOnTheirOwn, because a signal names a process rather than
// a file and no pattern can select one.
func TestSignalsAreNamedOnTheirOwn(t *testing.T) {
	t.Parallel()
	kill := interp.Action{Kind: interp.ActionSignal, PID: 4321, Signal: syscall.SIGKILL}
	probe := interp.Action{Kind: interp.ActionSignal, PID: -900, Signal: 0}
	allowed := parse(t, "version 1\nallow signal\n")
	want(t, allowed, interp.Allow, kill, probe)
	// `path` is every kind that carries a path and deliberately not this one,
	// so a policy that meant to permit signals has to say so.
	broad := parse(t, "version 1\nallow path /**\n")
	want(t, broad, interp.Deny, kill)
	want(t, broad, interp.Allow, exec("/bin/echo"), open("/x", true), stat("/x"), list("/x"))
	denied := parse(t, "version 1\ndefault allow\ndeny signal\n")
	want(t, denied, interp.Deny, kill, probe)
	want(t, denied, interp.Allow, exec("/bin/echo"))
	// And the same exclusion has to hold for a *default*, which is the half a
	// rule's pattern would otherwise hide: a rule naming `path` can never
	// match a signal anyway, because a signal has no path for the pattern to
	// match, so only `default allow path` shows whether the selector really
	// leaves signals alone.
	byDefault := parse(t, "version 1\ndefault deny\ndefault allow path\n")
	want(t, byDefault, interp.Deny, kill, probe)
	want(t, byDefault, interp.Allow, exec("/bin/echo"), open("/x", true), list("/x"))
}

// TestAPerKindDefaultOverridesTheBase is the escape hatch the design document
// records as discouraged: it is supported because refusing to represent it
// does not stop anyone writing `allow stat /**`, which is the same policy with
// less signal in it.
func TestAPerKindDefaultOverridesTheBase(t *testing.T) {
	t.Parallel()
	p := parse(t, "version 1\ndefault deny\ndefault allow stat\n")
	want(t, p, interp.Allow, stat("/anything"))
	want(t, p, interp.Deny, open("/anything", false), list("/anything"), exec("/anything"))
}

// TestTheBaseDefaultAppliesWhereverASelectorDidNot, and — the part that makes
// a file's meaning independent of its line order — it applies the same whether
// the base is written before or after the selector default.
func TestTheBaseDefaultAppliesWhereverASelectorDidNot(t *testing.T) {
	t.Parallel()
	first := parse(t, "version 1\ndefault allow\ndefault deny write\n")
	last := parse(t, "version 1\ndefault deny write\ndefault allow\n")
	for _, p := range []*policy.Policy{first, last} {
		want(t, p, interp.Allow, open("/x", false), stat("/x"), exec("/bin/echo"))
		want(t, p, interp.Deny, open("/x", true))
	}
}

// TestAPolicyIsConsultedFromEveryGoroutine is the contract interp.Gate states.
//
// A background job and each half of a pipeline run on their own, so a gate
// that keeps state has to guard it. This one keeps none after Parse returns,
// which is why it has no lock, and that is a claim -race can check.
func TestAPolicyIsConsultedFromEveryGoroutine(t *testing.T) {
	t.Parallel()
	p := parse(t, "version 1\nallow read /srv/**\ndeny read /srv/secret/**\n")
	var wg sync.WaitGroup
	for i := range 64 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a := open("/srv/x", false)
			expected := interp.Allow
			if i%2 == 0 {
				a, expected = open("/srv/secret/k", false), interp.Deny
			}
			if got := p.Allow(context.Background(), a); got != expected {
				t.Errorf("%s: got %s, want %s", a.Path, name(got), name(expected))
			}
		}()
	}
	wg.Wait()
}

// TestNewBuildsTheSamePolicyAsAFile keeps the embedder API and the file honest
// about each other. A rule written in Go means what the same rule written in a
// file means, or the document describes one of them and not the other.
func TestNewBuildsTheSamePolicyAsAFile(t *testing.T) {
	t.Parallel()
	built := policy.New(interp.Deny,
		policy.Rule{Decision: interp.Allow, Sel: policy.SelRead, Pattern: "/srv/**"},
		policy.Rule{Decision: interp.Deny, Sel: policy.SelRead, Pattern: "/srv/secret/**"},
	)
	fromFile := parse(t, "version 1\ndefault deny\nallow read /srv/**\ndeny read /srv/secret/**\n")
	probes := []interp.Action{
		open("/srv/x", false), open("/srv/secret/k", false),
		stat("/srv"), exec("/bin/echo"), open("/srv/x", true),
	}
	for _, a := range probes {
		if built.Allow(t.Context(), a) != fromFile.Allow(t.Context(), a) {
			t.Errorf("%s %s: New and Parse disagree", a.Kind, a.Path)
		}
	}
	if len(built.Rules()) != 2 {
		t.Errorf("Rules() returned %d rules, want 2", len(built.Rules()))
	}
}

// TestAnInterpreterInTheAllowlistEndsThePolicy is the sharpest footgun in the
// format, asserted so that it is documented by something that runs.
//
// The gate refuses what the *shell* does. A child process makes its own
// accesses and nothing here has any say over them, so allowlisting a program
// that runs programs is allowlisting everything. The policy engine cannot
// detect it — an allowlisted binary is a name, and what a name can do is not
// knowable from here — so all that is left is to say so.
func TestAnInterpreterInTheAllowlistEndsThePolicy(t *testing.T) {
	t.Parallel()
	p := parse(t, "version 1\nallow exec /bin/sh\ndeny read /etc/**\n")
	// The policy refuses the shell's own read, and permits starting the
	// process that will make the same read outside the boundary.
	want(t, p, interp.Deny, open("/etc/passwd", false))
	want(t, p, interp.Allow, exec("/bin/sh"))
}
