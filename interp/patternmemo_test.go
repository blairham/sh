// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import "testing"

// memoCases are patterns that ask enough questions to reach memoThreshold,
// paired with subjects on both sides of the answer.
//
// They have to be expensive or they prove nothing: the memo does not engage
// until a trial has asked memoThreshold questions, so a cheap pattern takes
// the same path with the memo present as without it and a test built on one
// would pass against any mistake in it at all. The `##` closure under
// alternation is what makes them expensive, which is the shape #1383 was.
//
// The subjects in each group are the *same length* on purpose. That is what
// makes a key collision possible: a key is (pattern offset, pattern length,
// subject offset, subject length, folding), so two different subjects of one
// length occupy exactly the same keys and a memo carried from one to the
// other is read as an answer about the other.
var memoCases = []struct {
	pattern  string
	subjects []string
}{
	{"(a|aa|aaa)##[a-z]", []string{
		"aaaaaaaaaaaaaaaaaaaax", // matches
		"aaaaaaaaaaaaaaaaaaaaZ", // does not: Z is outside the bracket
		"aaaaaaaaaaaaaaaaaaaa1", // does not
		"aaaaaaaaaaaaaaaaaaaab", // matches
	}},
	// The failing subjects come **first** here, and that ordering is the
	// whole value of the group rather than a detail of it. A memo carried
	// between subjects can only be caught by a subject that both reaches the
	// memo and *matches*, arriving after one that filled it — and a matching
	// subject usually asks very few questions, because a match stops as soon
	// as it is found. This one asks 2,702, so it consults what the two
	// failures before it left behind. Ordered the other way round, which is
	// how it was first written, `reset the counter but keep the entries`
	// survived: the match ran on an empty memo and the failures after it had
	// nothing to poison.
	{"(x|xx)##(y|yy)##z", []string{
		"xxxxxxxxxx_yyyyyyyyz", // does not match
		"xxxxxxxxxxyyyyyyyyy_", // does not match
		"xxxxxxxxxxyyyyyyyyyz", // matches, after 2,702 questions
	}},
	{`(#b)(([\\]|(%F))([\{]([^\}]##)[\}])|([\{]([^\}]##)[\}])([^\%\{\\]#))`, []string{
		"{error}Error{ehi}:{rst} Unknown{a}",
		"{error}Error{ehi}:{rst} Unknown{b}",
		"nothing here matches this pattern at.",
	}},
}

// memoTestOpts is the options a dialect with alternation and closures builds.
func memoTestOpts(pattern string) patternOpts {
	o := patternOpts{
		caret:        true,
		bracket:      BracketLiteral,
		group:        true,
		numericRange: true,
		extended:     true,
		escapes:      `-=!*?[]()|^~#<>\`,
	}
	o.where = &matchWhere{plan: planCapturesFor(pattern, o)}
	return o
}

// TestTheMemoChangesNoAnswer runs every case with the memo engaged and with it
// out of reach, and requires the two to agree on the match *and* on what a
// `(#b)` reported.
//
// The captures are half the assertion and not decoration. Which arm and which
// split won is what `$match` is made of, and a memo that remembered a
// *successful* trial would skip re-running it and so skip writing them — a
// match that still answers "yes" while reporting nothing, which is the
// plausible-wrong-answer shape this repository exists to avoid. Only dead
// ends are remembered, and this is what says so.
func TestTheMemoChangesNoAnswer(t *testing.T) {
	was := memoThreshold
	t.Cleanup(func() { memoThreshold = was })
	for _, c := range memoCases {
		engaged := false
		for _, subject := range c.subjects {
			// Out of reach: no trial here asks a billion questions.
			memoThreshold = 1 << 30
			o := memoTestOpts(c.pattern)
			wantOK, wantReport := matchPatternIn(c.pattern, subject, subject, 0, o)
			plain := o.where.asked

			memoThreshold = 512
			o = memoTestOpts(c.pattern)
			gotOK, gotReport := matchPatternIn(c.pattern, subject, subject, 0, o)
			memoized := o.where.asked

			if gotOK != wantOK {
				t.Errorf("%s vs %q: memoized says %v, unmemoized says %v", c.pattern, subject, gotOK, wantOK)
			}
			if gotReport.subject != wantReport.subject || gotReport.whole != wantReport.whole ||
				len(gotReport.groups) != len(wantReport.groups) {
				t.Errorf("%s vs %q: memoized reported %+v, unmemoized %+v", c.pattern, subject, gotReport, wantReport)
			}
			for i := range gotReport.groups {
				if gotReport.groups[i] != wantReport.groups[i] {
					t.Errorf("%s vs %q: group %d memoized %+v, unmemoized %+v",
						c.pattern, subject, i, gotReport.groups[i], wantReport.groups[i])
				}
			}
			// Whether this subject engaged the memo at all. Only a
			// *failing* trial can: a match that succeeds returns as soon as
			// it has found one, so it never revisits a dead end and there
			// is nothing for the memo to skip — which is itself the design,
			// since a success is deliberately not remembered.
			if !gotOK && plain > 512 && memoized < plain {
				engaged = true
			}
		}
		// Every pattern here has to have at least one subject the memo
		// actually worked on, or the agreement above is two runs of the same
		// code down the same path, and the case would pass against any
		// mistake in the memo whatsoever.
		if !engaged {
			t.Errorf("%s: no subject reached the memo, so this case proves nothing about it", c.pattern)
		}
	}
}

// memoPatterns and memoSubjects are crossed with each other, and they are
// deliberately small and varied rather than realistic: what is being tested
// is that the memo is invisible, and the way to test that is to ask it a lot
// of different questions rather than a few deep ones.
var memoPatterns = []string{
	"", "*", "?", "a", "abc", "a*", "*a", "a*b", "**", "*?*",
	"[abc]", "[^abc]", "[a-z]*", "[]a]", "[a", "a[b-]",
	"(a|b)", "(a|b)c", "(ab|a)*", "(|a)b", "a(b|c)d", "((a|b)|c)",
	"a#", "a##", "(a|b)#", "(a|b)##", "a#b", "(ab)#c", "((a)#b)#",
	"a~b", "*~b*", "a*~*b", "^a", "a^b", "^*",
	"(#i)abc", "(#i)a*", "a(#i)bc", "(#I)ABC", "(#l)abc",
	"(#b)(a)(b)", "(#b)(a*)b", "(#b)((x)|a(b)c)", "(#b)(a|ab)*",
	"(#s)a*", "a*(#e)", "(#s)abc(#e)",
	"<1-9>", "a<1-9>b", "<->",
	`a\*b`, `\[a`, `a\b`,
	"a*b*c*d", "*a*a*a*", "(a|aa)#b", "(a|aa|aaa)##[a-z]",
}

var memoSubjects = []string{
	"", "a", "b", "ab", "abc", "abcd", "ABC", "aab", "aaa", "aaaa",
	"a*b", "[a", `a`, "a5b", "5", "9", "x", "abcabc", "aabab",
	"aaaaaaaaaaaaaaaaaaaax", "aaaaaaaaaaaaaaaaaaaaZ",
	"{a}b", "%Fx", "a~b", "^a",
}

// TestTheMemoIsInvisibleAtEveryThreshold crosses every pattern above with
// every subject and requires the answer and the capture report to be the same
// with the memo recording every single question as with it recording none.
//
// The threshold is what makes this the strong version of the test below it.
// In use the memo does not engage until a trial has asked 512 questions, so
// an ordinary pattern never reaches it and a differential test built on
// ordinary patterns compares two runs of identical code — which is exactly
// how a first attempt at this file passed while `drop plen from the key`,
// `drop slen from the key`, `drop the folding from the key` and
// `remember successes too` all survived. Driving the threshold to zero makes
// every one of these 1,500 pairs exercise the memo, and all four die here.
//
// Soundness at zero is also the claim the threshold rests on: it is a
// performance tuning and nothing else, so if the memo were only correct
// because it usually does not run, this is the test that would say so.
func TestTheMemoIsInvisibleAtEveryThreshold(t *testing.T) {
	was := memoThreshold
	t.Cleanup(func() { memoThreshold = was })
	pairs, engaged := 0, 0
	for _, pattern := range memoPatterns {
		for _, subject := range memoSubjects {
			memoThreshold = 1 << 30
			off := memoTestOpts(pattern)
			wantOK, wantReport := matchPatternIn(pattern, subject, subject, 0, off)

			memoThreshold = 0
			on := memoTestOpts(pattern)
			gotOK, gotReport := matchPatternIn(pattern, subject, subject, 0, on)

			pairs++
			if len(on.where.dead) > 0 {
				engaged++
			}
			if gotOK != wantOK {
				t.Errorf("%q vs %q: %v with the memo, %v without", pattern, subject, gotOK, wantOK)
				continue
			}
			if !sameReport(gotReport, wantReport) {
				t.Errorf("%q vs %q: reported %+v with the memo, %+v without", pattern, subject, gotReport, wantReport)
			}
		}
	}
	// A cross product that never wrote a memo entry would agree with itself
	// for the dullest of reasons.
	if engaged*4 < pairs {
		t.Errorf("only %d of %d pairs recorded anything, so most of this proves nothing", engaged, pairs)
	}
	t.Logf("%d pairs compared, %d of them recorded a dead end", pairs, engaged)
}

// sameReport compares two match reports field by field, because a `(#b)` is
// the half of a match a wrong memo would corrupt while still answering yes.
func sameReport(a, b matchReport) bool {
	if a.subject != b.subject || a.whole != b.whole || a.wantsAll != b.wantsAll || len(a.groups) != len(b.groups) {
		return false
	}
	for i := range a.groups {
		if a.groups[i] != b.groups[i] {
			return false
		}
	}
	return true
}

// TestOneOptionsValueAnswersManySubjects is the invariant three callers rely
// on and none of them states.
//
// Pathname expansion builds its options once and matches every name in the
// directory with them (interp/glob.go), `unalias -m` and the element filters
// do the same over their own lists, and `${x#p}` reuses one value for every
// prefix of one subject — the whole reason matchWhere is a pointer. So a
// matchWhere carries state across subjects by design, and anything remembered
// in it has to be dropped when the subject changes or it becomes an answer
// about the wrong string.
//
// Measured: with the reset removed, `(a|aa|aaa)##[a-z]` over two names of
// equal length reported *no matches* in a directory where real zsh and this
// shell both list one. Nothing else in interp's tests, or any dialect's,
// noticed — which is why this is a test rather than a comment.
func TestOneOptionsValueAnswersManySubjects(t *testing.T) {
	for _, c := range memoCases {
		shared := memoTestOpts(c.pattern)
		for _, subject := range c.subjects {
			// The answer this subject gets from a value nothing else has
			// touched is the answer it has to get from the shared one.
			fresh := memoTestOpts(c.pattern)
			want, _ := matchPatternIn(c.pattern, subject, subject, 0, fresh)
			got, _ := matchPatternIn(c.pattern, subject, subject, 0, shared)
			if got != want {
				t.Errorf("%s vs %q: %v from a reused options value, %v from a fresh one",
					c.pattern, subject, got, want)
			}
		}
	}
}

// TestThePackedKeyIsInjective is the whole safety argument for packing five
// numbers into one, made by enumeration rather than by inspecting the
// arithmetic.
//
// A memo key has exactly one requirement: two different questions must not
// get the same number. A packed key that aliases answers a question nobody
// asked, and does it silently — the failure is a wrong match, not a panic.
// The first version of this packing was written with 16-bit shifts and then
// multiplied by four to make room for the folding, which pushed the top field
// past bit 63 and aliased every pattern offset above 16383. Nothing would
// have noticed on the patterns in this file, because they are short.
func TestThePackedKeyIsInjective(t *testing.T) {
	seen := map[uint64]matchKey{}
	// Values chosen around every boundary the packing has: zero, one, the
	// byte and 14-bit edges the broken version aliased at, and the top of
	// the field.
	vals := []int{0, 1, 2, 255, 256, 16383, 16384, 16385, 32766, packBase - 1}
	folds := []caseFolding{caseExact, caseEither, caseLowerEither}
	for _, pp := range vals {
		for _, plen := range vals {
			for _, at := range vals {
				for _, slen := range vals {
					for _, f := range folds {
						k := matchKey{pp: pp, plen: plen, at: at, slen: slen, fold: f}
						n := packKey(pp, plen, at, slen, f)
						if prev, ok := seen[n]; ok {
							t.Fatalf("packKey aliases %+v and %+v both to %d", prev, k, n)
						}
						seen[n] = k
					}
				}
			}
		}
	}
	t.Logf("%d distinct questions, %d distinct keys", len(seen), len(seen))
}

// TestTheWideMemoAnswersLikeThePackedOne exercises the fallback, which no
// ordinary test reaches because it only runs for a pattern or subject of
// 32,768 bytes or more.
//
// An unreached branch in a memo is the worst kind: it is correct-looking code
// that only ever runs on the inputs nobody tested, and the symptom of it being
// wrong is a wrong answer rather than a crash. So the same cross-product that
// checks the packed path is run again with packing forced off, and the two
// have to agree with each other and with no memo at all.
func TestTheWideMemoAnswersLikeThePackedOne(t *testing.T) {
	was := memoThreshold
	t.Cleanup(func() { memoThreshold = was })
	wide := 0
	for _, pattern := range memoPatterns {
		for _, subject := range memoSubjects {
			memoThreshold = 1 << 30
			off := memoTestOpts(pattern)
			wantOK, wantReport := matchPatternIn(pattern, subject, subject, 0, off)

			memoThreshold = 0
			packed := memoTestOpts(pattern)
			gotOK, gotReport := matchPatternIn(pattern, subject, subject, 0, packed)

			forced := memoTestOpts(pattern)
			// matchPatternIn sets packable from the lengths, so it is
			// overridden after the first question has established the rest.
			forced.where.pattern, forced.where.subject = pattern, subject
			forced.where.packable = false
			forced.where.total = len(subject)
			forced.where.caps = newCaptures(forced.where.plan)
			wideOK := matchHere(pattern, subject, 0, 0, forced)

			if !packed.where.packable {
				t.Errorf("%q vs %q was not packable, so the packed path was not the one compared", pattern, subject)
			}
			if len(forced.where.dead) != 0 {
				t.Errorf("%q vs %q wrote a packed entry with packing forced off", pattern, subject)
			}
			if len(forced.where.deadWide) > 0 {
				wide++
			}
			if gotOK != wantOK || !sameReport(gotReport, wantReport) {
				t.Errorf("%q vs %q: packed memo disagrees with no memo", pattern, subject)
			}
			if wideOK != wantOK {
				t.Errorf("%q vs %q: wide memo says %v, no memo says %v", pattern, subject, wideOK, wantOK)
			}
		}
	}
	if wide*4 < len(memoPatterns)*len(memoSubjects) {
		t.Errorf("only %d pairs used the wide memo, so most of this proves nothing", wide)
	}
	t.Logf("%d pairs went through the wide memo", wide)
}
