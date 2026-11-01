// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"fmt"
	"math/rand"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"unsafe"

	"github.com/blairham/sh/internal/testenv"
	"github.com/blairham/sh/syntax"
)

// progCases are the constructs the compiler reads, each with subjects that
// match it, miss it, and match it more than one way — the last is what makes
// a `(#b)` report depend on the order the search tries things in, which is
// the half of the equivalence a yes/no could not see.
var progCases = []struct {
	pattern  string
	subjects []string
}{
	{"(a|ab)(c|bcd)(d*)", []string{"abcd", "abcdd", "acd", "abbcd"}},
	{"(#b)(a|ab)(c|bcd)(d*)", []string{"abcd", "abcdd"}},
	{"(#b)*(*)", []string{"abc93", ""}},
	{"(#b)x#(*)", []string{"xx93", "93"}},
	{"(#b)(a*)b", []string{"aabab", "b"}},
	{"(#b)(ab|a)*", []string{"abc", "a"}},
	{"(#m)a(b|bc)c", []string{"abcc", "abc"}},
	{"(#b)(a)((#B)(b))(c)", []string{"abc"}},
	{"(#b)((x)|a(b)c)", []string{"abc", "x"}},
	{"(#i)abc", []string{"ABC", "aBc", "abd"}},
	{"(#l)aBc", []string{"ABC", "aBc", "abc"}},
	{"(#i)a(#I)b", []string{"Ab", "AB", "ab"}},
	{"a(#s)b", []string{"ab"}},
	{"(a|(#s))b", []string{"ab", "b"}},
	{"*(#e)", []string{"ab"}},
	{"(#b)((#s)|x)y", []string{"y", "xy"}},
	{"a*~*b*", []string{"ac", "ab", "a"}},
	{"(a*~*b*|zz)", []string{"zz", "ab", "ac"}},
	{"^a*", []string{"a", "b", ""}},
	{"a^b", []string{"ab", "ac", "a"}},
	{"<1-10>0", []string{"100", "10", "007", "1000"}},
	{"<->x", []string{"12x", "x"}},
	{"<5->", []string{"99999999999999999999", "4"}},
	{"(#b)<1-3>(#c2)z", []string{"12z", "123z"}},
	{"a(#c2,3)", []string{"a", "aa", "aaa", "aaaa"}},
	{"(ab)(#c1,2)c", []string{"abc", "ababc", "abababc"}},
	{"[[:alpha:]]##", []string{"ab", "a1", ""}},
	{"[a[:digit:]]#x", []string{"a1x", "bx"}},
	{"[a[:nope:]b]", []string{"a", "b", "c"}},
	{"[!a[:nope:]b]", []string{"q", "a"}},
	{"[[:]", []string{":", "["}},
	{"[[:alpha:]", []string{"a", "[a"}},
	{"[a[.b.]c]", []string{"b", "."}},
	{"[a[=b=]c]", []string{"b", "="}},
	{"[a[.nosuch.]b]", []string{"a", "n"}},
	{"[a-]", []string{"-", "a", "b"}},
	{"[]a]", []string{"]", "a"}},
	{"[]", []string{"", "]"}},
	{"[!]", []string{"!", "a"}},
	{"[^ab]x", []string{"cx", "ax", "^x"}},
	{`[a\]b]`, []string{"]", `\`, "b"}},
	{`[\]~#]#`, []string{"]~#", "x"}},
	{`a[\)]`, []string{"a)", `a\`}},
	{"[a-é]", []string{"ç", "b", "é"}},
	{"[é]#", []string{"éé", "e"}},
	{"a[", []string{"a[", "a"}},
	{"@(ab|[)", []string{"ab", "["}},
	{"@(a[)]b)", []string{"a)b"}},
	{`@(a\[)`, []string{"a["}},
	{"[(]", []string{"("}},
	{"([(])", []string{"("}},
	{"([a|b])", []string{"a", "|"}},
	{"@(|a)b", []string{"b", "ab"}},
	{"(|a)b", []string{"b", "ab"}},
	{"+(a)b", []string{"aab", "b"}},
	{"*(ab)", []string{"abab", "aba", ""}},
	{"?(ab)c", []string{"abc", "c", "ababc"}},
	{"!(ab)", []string{"ab", "abc", ""}},
	{"a!(z)cd", []string{"abcd", "azcd"}},
	{"@(?|.?)", []string{".a", "x", ".."}},
	{"**(e|f)", []string{"abef", "ab"}},
	{"ab*+(e|f)", []string{"abcdef", "abcfef", "ab"}},
	{"{2}(a)", []string{"aa", "a", "aaa"}},
	{"{1,2}(a)b", []string{"ab", "aab", "b"}},
	{"{z,y}(a)", []string{"{z,y}(a)", "{z,y}a"}},
	{"{z,y}(a(b))", []string{"{z,y}(a(b))", "{z,y}(ab)"}},
	{"{z,y}(a@(b))", []string{"{z,y}(ab)"}},
	{"@(a)(b)", []string{"ab", "a(b)"}},
	{"a(b|c)d", []string{"abd", "a(b", "c)d"}},
	{"x(y)z", []string{"x(y)z", "xyz"}},
	{`a\`, []string{`a\`, "a"}},
	{`bet\a`, []string{"beta", `bet\a`}},
	{`\*#x`, []string{"**x", "x"}},
	{`(#i)\b`, []string{"B", "b"}},
	{"a?c", []string{"abc", "aéc", "ac"}},
	{"?é", []string{"aé", "éé"}},
	{"*é*", []string{"aéb", "é", "e"}},
	{"\xc3*", []string{"é", "\xc3a"}},
	{"é#", []string{"ééa", ""}},
	{".*", []string{".a", "a"}},
	{"*a", []string{".a", "ba"}},
	{"[.]a", []string{".a"}},
	{"a|b", []string{"a", "b", "a|b"}},
	{"a|", []string{"a", ""}},
	{"[a|b]", []string{"a", "|"}},
	{"(x|xx)##(y|yy)##z", []string{"xxxxxyyyyz", "xxxxxyyyy_"}},
	// A group's arm whose bracket, under the reading that skips to the next
	// `]` once a member matches, hands the rest of the arm to the walk: the
	// compiled last step is then not the step that ends the span, so the
	// arm's last byte says nothing about it (#6380).
	{`@([a\]x]y)z`, []string{"ayz", "a]xyz", "axyz", "x]yz", "a]yz"}},
	{`@([a\]y]|b)`, []string{"ay", "a]y", "y", "b", "a]"}},
	// A group's floor is in bytes and the rest's reach in units, so a rest
	// that takes one multi-byte character needs the floor's widening.
	{"(a|ab)?", []string{"abé", "aé"}},
	{"@(a|ab)?", []string{"abé", "aé"}},
	// A flag in front of a construct the walk is handed back: the fold has
	// to reach the text behind it.
	{"(#i)[[:alpha:]x", []string{"[aX", "[ax", "[[:alpha:]X"}},
	// A repetition's splits are between characters, and a split inside one
	// would hand `?` a lone byte it then takes for a character.
	{"?(#c3)", []string{"éa", "abc"}},
	// An exclusion fails *after* its left side wrote a group, which is the
	// one failure that leaves a span behind: what unwinds it is the attempt
	// around it, so these are what say the unwinding is there.
	{"(#b)((a)*~ab|ab)", []string{"ab", "ac"}},
	{"(#b)(a)*~ab|ab", []string{"ab", "ac"}},
	// A negation may not take a leading period.
	{"!(z)", []string{".a", "a"}},
	{"!(a)*", []string{".x", "x"}},
	// The class escapes, and the zero-width ones whose answer is about where
	// the piece sits — which moves between trials of one program.
	{`\S*`, []string{".a", "a", " "}},
	{`\d#`, []string{"12", "1a"}},
	{`\bab`, []string{"aab", "ab", "x ab"}},
	// A group's arm that is a closure of one plain unit has its spans read
	// off one scan of the item's bytes (#6380): its lower and upper bounds,
	// a run cut short by a byte it refuses, and characters wider than a byte,
	// over which the scan must not run.
	{"(#b)(x(#c2,3))y", []string{"xy", "xxy", "xxxxy", "y"}},
	{"(#b)([ab](#c2,3))*", []string{"a", "ab", "abab", "abxab"}},
	{"(#b)([^x]##)(x|)", []string{"ééx", "aéx", "x", "abxb"}},
	{"(#b)(?#)(é)", []string{"aéé", "é"}},
	{"(#b)(é##)(x|)", []string{"ééx", "éx", "x", "eéx"}},
	{`a\B`, []string{"ab", "a b", "a"}},
	{`ab\b`, []string{"abc", "ab c"}},
	// A bare group inside a group is a group again, however the group in
	// front of it was written.
	{"{1}(a(b))", []string{"ab", "a(b)"}},
	{"@(a|(b))", []string{"b", "(b)"}},
}

// programReadings are the option sets the program is compared under. Each is
// a dialect's reading named by the constructs it turns on, and between them
// they reach every field patternReading carries that a pattern can tell.
func programReadings() []struct {
	name string
	o    patternOpts
} {
	flags := patternOpts{
		caret: true, bracket: BracketLiteral, group: true, numericRange: true,
		extended: true, escapes: `-=!*?[]()|^~#<>\`,
	}
	flagsChars := flags
	flagsChars.chars = true
	textParens := flags
	textParens.group, textParens.quantified, textParens.bareParenIsText = false, true, true
	top := flags
	top.topGroup = true
	badBracket := flags
	badBracket.bracket = BracketBadPattern
	quantified := patternOpts{caret: true, bracket: BracketLiteral, quantified: true}
	quantifiedChars := quantified
	quantifiedChars.chars = true
	counted := patternOpts{
		caret:      true,
		bracket:    BracketLiteral,
		quantified: true,
		counted:    true,
		collating:  OneCharacterIsACollatingElement,
	}
	countedOperand := counted
	countedOperand.operandParens = true
	countedArm := counted
	countedArm.armParens = true
	classEscapes := counted
	classEscapes.classEscapes, classEscapes.tildeGlobRead = true, true
	classEscapesPeriod := classEscapes
	classEscapesPeriod.period = true
	fold := quantified
	fold.fold = true
	foldWide := fold
	foldWide.chars, foldWide.foldWide, foldWide.foldClass = true, true, true
	noMatch := patternOpts{
		bracket: BracketNoMatch, collating: CollatingElementsHoldNothing,
		unknownClass: UnknownClassEndsTheScan, unterminatedClass: UnterminatedClassEndsTheScan,
	}
	empties := noMatch
	empties.emptyBracket, empties.caret = true, true
	empties.unknownClass, empties.unterminatedClass = UnknownClassEmptiesTheBracket, UnterminatedClassEmptiesTheBracket
	swallow := quantified
	swallow.unterminatedClass = UnterminatedClassSwallowsTheClosingBracket
	swallow.collating = ACollatingElementMayBeNamed
	member := patternOpts{caret: true, bracket: BracketLiteral, bracketMember: true, matchSkipsToAnyBracket: true}
	skip := member
	skip.bracketMember = false
	skipGroups := skip
	skipGroups.quantified = true
	period := flags
	period.period = true
	periodQuantified := quantified
	periodQuantified.period = true
	whole := flags
	whole.whole = true
	return []struct {
		name string
		o    patternOpts
	}{
		{"flags over bytes", flags},
		{"flags over characters", flagsChars},
		{"parentheses as text", textParens},
		{"top-level bar", top},
		{"bad bracket", badBracket},
		{"quantified over bytes", quantified},
		{"quantified over characters", quantifiedChars},
		{"counted", counted},
		{"counted in an operand", countedOperand},
		{"counted in a case arm", countedArm},
		{"class escapes", classEscapes},
		{"class escapes and period", classEscapesPeriod},
		{"fold", fold},
		{"fold wide", foldWide},
		{"unterminated matches nothing", noMatch},
		{"empty brackets", empties},
		{"swallowing class", swallow},
		{"bracket member", member},
		{"bracket skip", skip},
		{"bracket skip in groups", skipGroups},
		{"period", period},
		{"period quantified", periodQuantified},
		{"whole", whole},
	}
}

// allProgramCases is every corpus the matcher's other equivalence tests use,
// and this file's own.
func allProgramCases() []struct {
	pattern  string
	subjects []string
} {
	out := slices.Clone(progCases)
	add := func(pattern string, subjects ...string) {
		out = append(out, struct {
			pattern  string
			subjects []string
		}{pattern, subjects})
	}
	for _, c := range endSetCases {
		add(c.pattern, c.subject)
	}
	for _, c := range spanCases {
		add(c.pattern, c.subject)
	}
	for _, c := range edgeCases {
		add(c.pattern, c.subject)
	}
	for _, c := range reachCases {
		add(c.pattern, c.subjects...)
	}
	for _, c := range memoCases {
		add(c.pattern, c.subjects...)
	}
	return out
}

// matchOutcome is everything a match leaves behind that a caller can read.
type matchOutcome struct {
	ok     bool
	report string
	bad    bool
	asked  int
}

// readingFor is a reading's options made ready for one pattern: a plan for
// its groups, a matchWhere, and a place for the bad-pattern verdict and the
// unterminated-bracket question to land.
func readingFor(o patternOpts, pattern string, bad *bool, asks *int) patternOpts {
	o.bad = bad
	policy := o.bracket
	o.askBracketAfterSub = func() BracketPolicy {
		*asks++
		return policy
	}
	o.where = &matchWhere{plan: planCapturesFor(pattern, o)}
	return o
}

// withCompiling runs f with the program on or off, compiling every pattern on
// its first match when it is on.
func withCompiling(on bool, f func()) {
	defer func(c bool, t int) { patternCompiles, compileThreshold = c, t }(patternCompiles, compileThreshold)
	patternCompiles, compileThreshold = on, 0
	f()
}

// sweep matches the pattern against every span of the subject, in the order a
// substitution asks — every start, every end from the far one down — through
// one matchWhere, so the program is compiled once and its memo is carried
// between trials exactly as a substitution carries it.
func sweep(o patternOpts, pattern, subject string, compiled bool) (out []matchOutcome, ran bool) {
	withCompiling(compiled, func() { out, ran = sweepAsSet(o, pattern, subject) })
	return out, ran
}

// sweepAsSet is sweep with the program left however withCompiling set it, so
// that one setting can serve many sweeps running at once.
func sweepAsSet(o patternOpts, pattern, subject string) (out []matchOutcome, ran bool) {
	var bad bool
	asks := 0
	o = readingFor(o, pattern, &bad, &asks)
	for i := 0; i <= len(subject); i++ {
		for j := len(subject); j >= i; j-- {
			bad = false
			ok, m := matchPatternIn(pattern, subject[i:j], subject, i, o)
			out = append(
				out,
				matchOutcome{ok: ok, report: fmt.Sprintf("%v %v %v", m.groups, m.whole, m.wantsAll), bad: bad, asked: asks},
			)
		}
	}
	return out, o.where.prog != nil
}

// eachInParallel calls f for every i in [0, n), spread over GOMAXPROCS
// goroutines, and returns once every call has.
//
// The equivalence tests below are the bulk of this package's running time:
// thousands of independent pattern-and-subject pairs, each a pure computation,
// and under -race on the Linux runner they took the package to its timeout
// (#6730, #6800) one core at a time. The pairs are independent by
// construction — every one gets its own matchWhere — and what they share is
// the process's program cache, which the shell itself reads from several
// goroutines at once. So running them side by side is both faster and the
// shape production has, and under -race it is a question the detector can
// now answer about that cache rather than none.
//
// The settings a test changes (withCompiling, memoThreshold and the rest) are
// package variables, so they are only ever changed between calls to this, never
// during one.
func eachInParallel(n int, f func(i int)) {
	var next atomic.Int64
	var wg sync.WaitGroup
	for range min(runtime.GOMAXPROCS(0), n) {
		wg.Go(func() {
			for {
				i := int(next.Add(1) - 1)
				if i >= n {
					return
				}
				f(i)
			}
		})
	}
	wg.Wait()
}

// casesForReading is the part of the corpus the k-th reading sweeps: all of it,
// except under the race detector, where each case is swept under one reading
// rather than all 23 — the readings take turns, so between them they still
// reach every case.
//
// The sweeps that read the corpus this way were 221s of this package's 508s on
// a hosted ubuntu runner under -race, and they already run their cases side by
// side on every core, so that was far more of the runner's CPU than its share
// (#7290). What the detector adds to them is whether cases running at once
// share state they must not — the program cache, the kept facts — and every
// reading still runs its cases side by side, so it is still asked that. Whether
// the program and the walk agree is the other question, and nothing about it is
// sampled: every run without -race — CI's macOS leg, `make test` — sweeps every
// case under every reading.
//
// Turns rather than a fraction of each reading's cases, because one case is
// most of the cost: `(a|aa|aaa)##[a-z]` was 950ms of reading 0's 1.03s under
// the detector, and a sample that kept it in several readings kept the cost.
func casesForReading[T any](cases []T, k int) []T {
	if !testenv.RaceDetector {
		return cases
	}
	turns := raceTurns()
	var out []T
	for i := k % turns; i < len(cases); i += turns {
		out = append(out, cases[i])
	}
	return out
}

// raceTurns is how many readings share the corpus out under the race detector
// — see casesForReading — and it is 1 without it.
func raceTurns() int {
	if !testenv.RaceDetector {
		return 1
	}
	return len(programReadings())
}

// programPair is one reading, pattern and subject of the equivalence corpus.
type programPair struct {
	reading          string
	o                patternOpts
	pattern, subject string
}

// programPairs flattens every reading over every case and subject, in the
// order the tests used to walk them, so that a failure names the same row.
func programPairs() []programPair {
	var out []programPair
	cases := allProgramCases()
	for k, rd := range programReadings() {
		for _, c := range casesForReading(cases, k) {
			for _, subject := range c.subjects {
				out = append(out, programPair{rd.name, rd.o, c.pattern, subject})
			}
		}
	}
	return out
}

// **The program answers every span exactly as the walk over the text does**,
// under every reading: whether it matched, what each `(#b)` group and the
// `(#m)` span reported, whether the pattern was found bad, and how many times
// the dialect was asked about an unterminated bracket.
//
// It also counts the pairs where a program actually ran, and that floor is
// half the assertion: a reading under which the compiler declined everything
// would compare the walk with itself and agree on every row.
func TestTheProgramAnswersWhatTheWalkAnswers(t *testing.T) {
	pairs := programPairs()
	for _, memo := range []int{memoThreshold, 0} {
		t.Run(fmt.Sprintf("memo after %d", memo), func(t *testing.T) {
			defer func(was int) { memoThreshold = was }(memoThreshold)
			memoThreshold = memo
			got := make([][]matchOutcome, len(pairs))
			ran := make([]bool, len(pairs))
			want := make([][]matchOutcome, len(pairs))
			withCompiling(true, func() {
				eachInParallel(len(pairs), func(i int) {
					p := pairs[i]
					got[i], ran[i] = sweepAsSet(p.o, p.pattern, p.subject)
				})
			})
			withCompiling(false, func() {
				eachInParallel(len(pairs), func(i int) {
					p := pairs[i]
					want[i], _ = sweepAsSet(p.o, p.pattern, p.subject)
				})
			})
			compiledPairs := 0
			for n, p := range pairs {
				if ran[n] {
					compiledPairs += len(got[n])
				}
				k := 0
				for i := 0; i <= len(p.subject); i++ {
					for j := len(p.subject); j >= i; j-- {
						if g, w := got[n][k], want[n][k]; g.ok != w.ok || g.report != w.report || g.bad != w.bad || g.asked != w.asked {
							t.Errorf("%s: %q on %q[%d:%d]: compiled %+v, walked %+v",
								p.reading, p.pattern, p.subject, i, j, g, w)
						}
						k++
					}
				}
			}
			if floor := 50000 / raceTurns(); compiledPairs < floor {
				t.Errorf(
					"a program ran for only %d spans, so the comparison above was mostly the walk against itself",
					compiledPairs,
				)
			}
		})
	}
}

// randomPattern builds a pattern from the constructs the compiler reads, so
// that the shapes nobody thought to write down are compared too.
func randomPattern(rng *rand.Rand, depth int) string {
	atoms := []string{
		"a", "b", "A", "é", ".", "?", "*", "[ab]", "[!a]", "[^a-c]", "[[:alpha:]]",
		"[[:nope:]]", "[a", `\*`, `\a`, "1", "<1-3>", "<->", "(#i)", "(#b)", "(#m)",
		"(#s)", "(#e)", "(#I)", "|", "~", "^", "#", "##", "(#c2)", "(#c1,2)", "{2}",
		"[]a]", `[\]]`, "[é-ê]", "\\",
	}
	var b strings.Builder
	for range 1 + rng.Intn(5) {
		if depth < 2 && rng.Intn(4) == 0 {
			b.WriteString([]string{"", "@", "?", "*", "+", "!"}[rng.Intn(6)])
			b.WriteByte('(')
			for k := range 1 + rng.Intn(3) {
				if k > 0 {
					b.WriteByte('|')
				}
				b.WriteString(randomPattern(rng, depth+1))
			}
			b.WriteByte(')')
			continue
		}
		b.WriteString(atoms[rng.Intn(len(atoms))])
	}
	return b.String()
}

// The same comparison over generated patterns and subjects, seeded so that a
// failure is a row that can be read back.
func TestTheProgramAgreesOnGeneratedPatterns(t *testing.T) {
	rng := rand.New(rand.NewSource(6380))
	readings := programReadings()
	compared := 0
	for range 1500 {
		pattern := randomPattern(rng, 0)
		var sb strings.Builder
		for range rng.Intn(7) {
			sb.WriteString([]string{"a", "b", "A", "é", ".", "1", "2", "]", "\\", "|", "~", "#", "["}[rng.Intn(13)])
		}
		subject := sb.String()
		rd := readings[rng.Intn(len(readings))]
		got, ran := sweep(rd.o, pattern, subject, true)
		want, _ := sweep(rd.o, pattern, subject, false)
		if ran {
			compared++
		}
		for k := range got {
			if got[k] != want[k] {
				t.Fatalf("%s: %q on %q, trial %d: compiled %+v, walked %+v", rd.name, pattern, subject, k, got[k], want[k])
			}
		}
	}
	if compared < 1000 {
		t.Errorf("a program ran for only %d of the generated patterns", compared)
	}
}

// And a whole substitution answers the same compiled as walked, down to what
// each replacement was handed — the text and every span — at every anchor.
func TestSubstitutionAnswersTheSameCompiledAsWalked(t *testing.T) {
	with := func(m matchReport, text string) string {
		return fmt.Sprintf("<%s|%v|%v>", text, m.groups, m.whole)
	}
	type row struct {
		programPair
		anchor byte
		all    bool
	}
	var rows []row
	for _, p := range programPairs() {
		for _, anchor := range []byte{0, '#', '%'} {
			for _, all := range []bool{false, true} {
				rows = append(rows, row{p, anchor, all})
			}
		}
	}
	// Every row compiled, then every row walked: the setting is the
	// package's, so it is changed between the two passes and not inside one.
	answers := func(compiled bool) []string {
		out := make([]string, len(rows))
		withCompiling(compiled, func() {
			eachInParallel(len(rows), func(i int) {
				r := rows[i]
				o := r.o
				o.where = &matchWhere{plan: planCapturesFor(r.pattern, o)}
				out[i] = replace(r.subject, r.pattern, replaceExprForTest(r.anchor, r.all), o, armOrder{},
					func() EmptyMatchDeclinedPolicy { return EmptyMatchDeclinedAtTheEnd }, with)
			})
		})
		return out
	}
	got, want := answers(true), answers(false)
	for i, r := range rows {
		if got[i] != want[i] {
			t.Errorf("%s: %q on %q anchor %q all=%v: compiled %q, walked %q",
				r.reading, r.pattern, r.subject, r.anchor, r.all, got[i], want[i])
		}
	}
}

// The substitution #6380 is measured on, counted in matcher questions rather
// than timed: a program is what ran, and it asked about a quarter of what the
// walk asks — 535 against 2,195 on the change that set this, 396 once a
// group stopped trying arms over spans their edge bytes rule out, and 98 once
// a closure arm's spans were read off one scan rather than asked one by one. Run with the
// program off, or with those edge tables off, the same count fails it, which
// is the check that this is counting the thing it claims to.
func TestTheThemeSubstitutionRunsAsAProgram(t *testing.T) {
	e := &syntax.ParamExpr{All: true}
	withCompiling(true, func() {
		o := benchOpts(t, benchPattern, benchSubject)
		got := replace(benchSubject, benchPattern, e, o, armOrder{}, benchDeclined,
			func(matchReport, string) string { return "X" })
		if got != "XXXXXXXXX" {
			t.Fatalf("the theme's substitution gave %q, want the nine X real zsh gives", got)
		}
		if o.where.prog == nil {
			t.Fatal("no program ran, so the count below is the walk's")
		}
		if n := o.where.asked; n > 120 {
			t.Errorf("the theme's substitution asked the program %d questions for nine matches", n)
		}
	})
}

// The patterns this exists for are ones the compiler reads whole, rather than
// ones it hands back to the walk a node in: an interpret node is exact and
// buys nothing.
func TestTheProgramReadsThePatternsItIsFor(t *testing.T) {
	t.Parallel()
	for _, p := range []string{benchPattern, "*.go", "(a|ab)c", "[^}]##", "x#(*)", "<1-3>#", "a*~*b*", "^a", "(#i)ab", "(#s)a(#e)"} {
		o := readingFor(programReadings()[0].o, p, new(bool), new(int))
		o.where.ready, o.where.pattern = true, p
		o.where.prepare(p, o.emptyBracket)
		prog, ok := compilePattern(p, o)
		if !ok {
			t.Errorf("%q: declined", p)
			continue
		}
		if n := countInterpretNodes(prog); n > 0 {
			t.Errorf("%q: %d pieces are handed back to the walk", p, n)
		}
	}
}

func countInterpretNodes(p *patternProgram) int {
	seen := map[*cpiece]bool{}
	n := 0
	var piece func(*cpiece)
	piece = func(c *cpiece) {
		if c == nil || seen[c] {
			return
		}
		seen[c] = true
		for x := c.first; x != nil; x = x.next {
			if x.op == opInterpret {
				n++
			}
			for _, s := range append(append([]*cpiece{x.rest, x.item, x.after, x.left}, x.arms...), x.rights...) {
				piece(s)
			}
		}
	}
	for _, t := range p.top {
		piece(t)
	}
	return n
}

// TestTheReadingNamesEveryOption keeps patternReading whole. Every field of
// patternOpts is either part of the reading a program is built for or state
// the executor takes from the match running, and a field in neither would be
// one a program could be reused across without anyone deciding it may be.
func TestTheReadingNamesEveryOption(t *testing.T) {
	t.Parallel()
	state := map[string]bool{
		"pieceBase": true, "pieceEnd": true, "period": true,
		"bad": true, "where": true, "askBracketAfterSub": true,
	}
	reading := map[string]bool{}
	rt := reflect.TypeOf(patternReading{})
	for i := range rt.NumField() {
		reading[rt.Field(i).Name] = true
	}
	ot := reflect.TypeOf(patternOpts{})
	for i := range ot.NumField() {
		name := ot.Field(i).Name
		switch {
		case reading[name] && state[name]:
			t.Errorf("patternOpts.%s is both read and carried", name)
		case !reading[name] && !state[name]:
			t.Errorf("patternOpts.%s is in neither patternReading nor the match state", name)
		}
		delete(reading, name)
	}
	for name := range reading {
		t.Errorf("patternReading.%s names no field of patternOpts", name)
	}
	// And readingOf copies each of them: a field listed and left at its zero
	// value would let two readings that differ there share a program.
	o := patternOpts{}
	ov := reflect.ValueOf(&o).Elem()
	for i := range ot.NumField() {
		if state[ot.Field(i).Name] {
			continue
		}
		before := readingOf(&o)
		raw := ov.Field(i)
		f := reflect.NewAt(raw.Type(), unsafe.Pointer(raw.UnsafeAddr())).Elem()
		switch f.Kind() {
		case reflect.Bool:
			f.SetBool(true)
		case reflect.Int:
			f.SetInt(7)
		case reflect.String:
			f.SetString("x")
		case reflect.Struct:
			inner := f.Field(0)
			reflect.NewAt(inner.Type(), unsafe.Pointer(inner.UnsafeAddr())).Elem().SetString("x")
		default:
			t.Fatalf("patternOpts.%s is a %s and this test cannot move it", ot.Field(i).Name, f.Kind())
		}
		if readingOf(&o) == before {
			t.Errorf("readingOf does not copy patternOpts.%s", ot.Field(i).Name)
		}
	}
}
