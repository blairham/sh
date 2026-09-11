// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// arraySemantics answers what an array *expansion* needs, which permissive() does not:
// the standard has no arrays, so `${arr[@]}` reads back empty under it however
// full the array is. A row asserting that a refusal left an array untouched
// cannot see the difference without this — the mutant that filled it anyway
// survived, because both answers rendered `arr=[]`.
func arraySemantics(s *Semantics) {
	s.ArraysAreSparse = Yes
	s.ArrayScalarIsTheWholeArray = No
	s.ArrayLiteralSubscriptIsAKey = No
	s.NegativeSubscriptPastTheStartInserts = No
	s.IndirectionYieldsName = No
	s.ArithNameValueRecurses = Yes
	s.ArithRecursedNameMustBeSet = No
}

// readRun is optRun with `read`'s own fatality turned off, so a test that is
// not about the fatality gets to see what happened after the refusal.
func readRun(t *testing.T, tweak func(*Semantics), dg Diagnostics, src string) (string, int) {
	t.Helper()
	return optRun(t, func(s *Semantics) {
		s.BadNameToReadFatal = No
		if tweak != nil {
			tweak(s)
		}
	}, dg, src)
}

// The bug: `read` took a word that is not an identifier as a variable name and
// said nothing about it (#1440).
//
// `read -r line -u $1` is invalid in every shell — options do not follow an
// operand — and every shell in the panel says so. Here the `-u` became a
// variable name, the read went to standard input instead of the descriptor,
// and the status was the plain 0 of a line that arrived. Status alone could
// not have told a caller either way: `read` answers 1 for the ordinary end of
// input, so a silent 1 reads as "the stream ended", which is the answer a
// caller is most likely to be checking for.
//
// Measured 2026-09-07 across dash, bash 5.3.15, bash as `sh`, bash 3.2.57,
// ksh93u+ and zsh 5.9.2. All six refuse, in four wordings and two statuses.
// The refusal is the core's; only the wording, the status, the fatality, how
// far the set of names reaches and whether the line is read first are the
// dialect's.
func TestAReadOperandThatIsNotANameIsRefused(t *testing.T) {
	for _, operand := range []string{"1x", "a-b", "a b", "a.b", "-x", "-u", ""} {
		src := `printf 'text\n' | { read -- "` + operand + `"; echo "st=$?"; }`
		out, _ := readRun(t, nil, Diagnostics{}, src)
		if !strings.Contains(out, "st=1") {
			t.Errorf("read %q: said %q, want status 1", operand, out)
		}
		if !strings.Contains(out, "read") {
			t.Errorf("read %q: said %q, want the builtin named", operand, out)
		}
	}
}

// Nothing is assigned to the word that was refused, and nothing is read into
// the name a caller would go on to use.
func TestARefusedReadNameIsNotAssignedTo(t *testing.T) {
	out, _ := readRun(t, nil, Diagnostics{},
		`v=keep; printf 'text\n' | { read -- "-u"; echo "st=$? v=[$v] u=[${u-unset}]"; }`)
	if !strings.Contains(out, "v=[keep]") || !strings.Contains(out, "u=[unset]") {
		t.Errorf("said %q, want the refusal to have assigned nothing", out)
	}
}

// The whole line, location included.
//
// A Contains over "not an identifier" cannot catch a wording that grew a
// prefix, and "not an identifier" is a substring of three of the four the
// panel writes. Each row here is one dialect's sentence as its binary printed
// it, with the shell's own name normalized to the runner's.
func TestTheReadRefusalIsTheDialectsWholeSentence(t *testing.T) {
	for _, c := range []struct {
		name string
		dg   Diagnostics
		src  string
		want string
	}{
		{
			"bash 5.3, which quotes the word back",
			Diagnostics{BuiltinBadName: map[string]string{"read": "%[1]s: `%[2]s': not a valid identifier"}},
			`read 1bad`,
			"testsh: read: `1bad': not a valid identifier",
		},
		{
			"dash, which names the word plainly",
			Diagnostics{
				BuiltinBadName:       map[string]string{"read": "%[1]s: %[2]s: bad variable name"},
				BuiltinBadNameStatus: 2,
			},
			`read 1bad`,
			"testsh: read: 1bad: bad variable name",
		},
		{
			"ksh93, which words it as readonly's",
			Diagnostics{BuiltinBadName: map[string]string{"read": "%[1]s: %[2]s: invalid variable name"}},
			`read 1bad`,
			"testsh: read: 1bad: invalid variable name",
		},
		{
			"zsh, whose location does not name the builtin",
			Diagnostics{
				BuiltinBadName:                map[string]string{"read": "not an identifier: %[2]s"},
				BadNameRefusalHidesTheBuiltin: map[string]bool{"read": true},
			},
			`read 1bad`,
			"testsh: not an identifier: 1bad",
		},
		{
			"and a dialect with no entry falls back rather than borrowing another builtin's",
			Diagnostics{BuiltinBadName: map[string]string{"export": "%[1]s: %[2]s: is not an identifier"}},
			`read 1bad`,
			"testsh: read: `1bad': not a valid identifier",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := readRun(t, nil, c.dg, c.src)
			if got := strings.SplitN(out, "\n", 2)[0]; got != c.want {
				t.Errorf("said %q, want %q", got, c.want)
			}
		})
	}
}

// dash reports it at 2 where the rest report 1, which is the number that tells
// this apart from the end of input.
func TestTheReadRefusalCarriesTheDialectsStatus(t *testing.T) {
	dg := Diagnostics{BuiltinBadNameStatus: 2}
	if _, st := readRun(t, nil, dg, `read 1bad`); st != 2 {
		t.Errorf("status = %d, want 2", st)
	}
	if _, st := readRun(t, nil, Diagnostics{}, `read 1bad`); st != 1 {
		t.Errorf("status = %d, want 1", st)
	}
}

// What may stand where `read` wants a name is a set of its own: zsh fills
// `$1` from `read 1` and refuses `export 1`.
func TestWhatMayStandWhereReadWantsAName(t *testing.T) {
	for _, c := range []struct {
		name    string
		takes   NameOperands
		operand string
		refused bool
	}{
		{"plain refuses a positional", PlainNamesOnly, "1", true},
		{"plain refuses a multi-digit positional", PlainNamesOnly, "12", true},
		{"positionals take one", NamesAndPositionals, "1", false},
		{"positionals take zero", NamesAndPositionals, "0", false},
		{"positionals still refuse a name that starts badly", NamesAndPositionals, "1x", true},
		{"positionals still refuse a special parameter", NamesAndPositionals, "?", true},
		{"a plain name is one under either set", PlainNamesOnly, "v", false},
	} {
		out, _ := readRun(t, func(s *Semantics) { s.ReadNameOperands = c.takes }, Diagnostics{},
			`printf 'text\n' | { read -- "`+c.operand+`"; echo "st=$?"; }`)
		if refused := !strings.Contains(out, "st=0"); refused != c.refused {
			t.Errorf("%s (%q): said %q, want refused=%v", c.name, c.operand, out, c.refused)
		}
	}
}

// zsh alone stops the script here, and it is a question about `read` rather
// than about special builtins — `read` is not one in any shell.
func TestAReadRefusalEndsTheScriptWhereTheDialectSaysSo(t *testing.T) {
	for _, c := range []struct {
		fatal Answer
		stops bool
	}{{Yes, true}, {No, false}} {
		out, _ := optRun(t, func(s *Semantics) { s.BadNameToReadFatal = c.fatal }, Diagnostics{},
			`read 1bad; echo after`)
		if stopped := !strings.Contains(out, "after"); stopped != c.stops {
			t.Errorf("fatal=%v: said %q, want stops=%v", c.fatal, out, c.stops)
		}
	}
}

// Whether the line is read before the operand is judged, which is observable
// only through the input: a refusal that comes first leaves the line for the
// next reader.
func TestWhenAReadJudgesItsFirstOperand(t *testing.T) {
	src := `printf 'AAA\nBBB\n' | { read 1bad; read next; echo "[$next]"; }`
	for _, c := range []struct {
		first Answer
		want  string
	}{
		{Yes, "[AAA]"},
		{No, "[BBB]"},
	} {
		out, _ := readRun(t, func(s *Semantics) { s.ReadRefusesABadNameBeforeReading = c.first },
			Diagnostics{}, src)
		if !strings.Contains(out, c.want) {
			t.Errorf("first=%v: said %q, want %q", c.first, out, c.want)
		}
	}
	// And it is the *first* operand alone. A bad name behind a good one is
	// reached after the line has been read whichever way the axis is
	// answered, because the good name in front of it was filled from that
	// line — which is what every shell in the panel does.
	behind := `printf 'AAA\nBBB\n' | { read good 1bad; read next; echo "[$good][$next]"; }`
	for _, first := range []Answer{Yes, No} {
		out, _ := readRun(t, func(s *Semantics) { s.ReadRefusesABadNameBeforeReading = first },
			Diagnostics{}, behind)
		if !strings.Contains(out, "[AAA][BBB]") {
			t.Errorf("first=%v: said %q, want [AAA][BBB]", first, out)
		}
	}
}

// The names in front of a bad one are filled and the ones behind it are left
// as they were, and the count the splitter works to is the one the script
// wrote rather than the truncated one.
func TestTheNamesInFrontOfABadReadNameAreStillFilled(t *testing.T) {
	out, _ := readRun(t, nil, Diagnostics{},
		`c=keep; printf 'X Y Z\n' | { read a 1bad c; echo "a=[$a] c=[$c]"; }`)
	// a=[X] and not a=[X Y Z]: the last name takes the remainder of the line
	// only when the line held more fields than there are names, and there are
	// three names here however far the filling got.
	if !strings.Contains(out, "a=[X] c=[keep]") {
		t.Errorf("said %q, want a=[X] c=[keep]", out)
	}
}

// The array name is judged too, and in the order it is filled: an array
// refused leaves it untouched, and an operand behind a good array name is
// refused only after the array is filled.
func TestAReadArrayNameIsJudgedAsAName(t *testing.T) {
	out, _ := readRun(t, func(s *Semantics) { s.ReadOptions = "ra:" }, Diagnostics{},
		`arr=keep; printf 'X Y\n' | { read -a 1bad; echo "st=$? arr=[$arr]"; }`)
	if !strings.Contains(out, "st=1") || !strings.Contains(out, "arr=[keep]") {
		t.Errorf("said %q, want a refusal that filled nothing", out)
	}
}

// `read "v?Name: "` is `read -p` in one word: two of the six shells split the
// first operand at a `?` and take the rest as a prompt. A name check that did
// not know the form would refuse the idiom in both of them.
func TestReadsFirstOperandMayCarryAPrompt(t *testing.T) {
	for _, c := range []struct {
		name  string
		style ReadPromptOperand
		src   string
		want  string
	}{
		{
			"the whole word is the name where there is no such form",
			ReadOperandIsAllName,
			`printf 'text\n' | { read "v?p"; echo "st=$? v=[$v]"; }`,
			"st=1 v=[]",
		},
		{
			"the name is the part in front of the mark",
			ReadPromptNeedsANameBeforeIt,
			`printf 'text\n' | { read "v?p"; echo "st=$? v=[$v]"; }`,
			"st=0 v=[text]",
		},
		{
			"and a mark with nothing in front of it is an empty name where the dialect wants one",
			ReadPromptNeedsANameBeforeIt,
			`printf 'text\n' | { read "?p"; echo "st=$? R=[$REPLY]"; }`,
			"st=1 R=[]",
		},
		{
			"where the other dialect reads into the default name instead",
			ReadPromptAloneNamesTheDefault,
			`printf 'text\n' | { read "?p"; echo "st=$? R=[$REPLY]"; }`,
			"st=0 R=[text]",
		},
		{
			"a bad name in front of the mark is refused as the name and not as the word",
			ReadPromptNeedsANameBeforeIt,
			`printf 'text\n' | { read "1bad?p"; echo "st=$?"; }`,
			"read: `1bad': not a valid identifier",
		},
		{
			"and the form is the first operand alone",
			ReadPromptAloneNamesTheDefault,
			`printf 'A B\n' | { read v "w?p"; echo "st=$?"; }`,
			"read: `w?p': not a valid identifier",
		},
		{
			"the array letter's operand takes the form too",
			ReadPromptAloneNamesTheDefault,
			`printf 'A B\n' | { read -A "arr?p"; echo "st=$? [${arr[0]}]"; }`,
			"st=0 [A]",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := readRun(t, func(s *Semantics) {
				s.ReadPromptOperand = c.style
				s.ReadOptions = "rA"
			}, Diagnostics{}, c.src)
			if !strings.Contains(out, c.want) {
				t.Errorf("said %q, want %q", out, c.want)
			}
		})
	}
}

// An axis with no answer is refused by name rather than guessed at, and each
// of the three is asked only where it decides something — a `read v` asks
// none of them, which is what keeps an unanswered axis from being reported
// where a script could not act on it.
func TestAnUnansweredReadNameAxisIsRefusedByName(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
		names           NameOperands
	}{
		{
			name: "what a name is, for an operand that is not a plain one",
			src:  `read 1`,
			want: "what may stand where a name is wanted",
		},
		{
			// With the set answered, so that the axis this row is about is
			// the one the walk reaches.
			name:  "when the operand is judged, for one that is not a name",
			src:   `read 1bad`,
			want:  "`read` judging its first operand before it reads",
			names: PlainNamesOnly,
		},
		{
			name: "what a mark in the first operand means",
			src:  `read "v?p"`,
			want: "what a `?` in `read`'s first operand means",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := optRun(t, func(s *Semantics) {
				s.ReadNameOperands = c.names
				s.ReadRefusesABadNameBeforeReading = Unspecified
				s.ReadPromptOperand = ReadPromptOperandUnspecified
			}, Diagnostics{}, c.src)
			if !strings.Contains(out, c.want) || st != 2 {
				t.Errorf("said %q status %d, want %q at 2", out, st, c.want)
			}
		})
	}
	// And none of them is asked where it decides nothing.
	out, st := optRun(t, func(s *Semantics) {
		s.ReadNameOperands = NameOperandsUnspecified
		s.ReadRefusesABadNameBeforeReading = Unspecified
		s.ReadPromptOperand = ReadPromptOperandUnspecified
	}, Diagnostics{}, `printf 'text\n' | { read v; echo "st=$? v=[$v]"; }`)
	if !strings.Contains(out, "st=0 v=[text]") || st != 0 {
		t.Errorf("said %q status %d, want a plain name to ask nothing", out, st)
	}
}

// A subscripted operand is not judged as a name here. `read a[0]` fills the
// element in bash, bash 3.2 and ksh93, so refusing it as a bad name would
// answer three of the six wrongly — what this shell then does with it is a
// separate question and this leaves it exactly where it was.
func TestAReadOperandWithASubscriptIsNotJudgedAsAName(t *testing.T) {
	out, st := readRun(t, func(s *Semantics) { s.ReadNameOperands = PlainNamesOnly }, Diagnostics{},
		`printf 'text\n' | { read "a[0]"; echo "st=$?"; }`)
	if !strings.Contains(out, "st=0") || st != 0 {
		t.Errorf("said %q status %d, want a subscripted operand to reach no name check", out, st)
	}
	// And the base still has to be a name: `1bad[0]` is a bad name in every
	// column that has the word, quoted back whole.
	if out, _ := readRun(t, nil, Diagnostics{}, `printf 'text\n' | { read "1bad[0]"; echo "st=$?"; }`); !strings.Contains(out, "st=1") {
		t.Errorf("said %q, want a subscript on something that is not a name to be refused", out)
	}
}

// Only the first bad name is reported, and the walk stops there. A shell that
// reported the last would name a word the filling never reached.
//
// Both routes to the refusal, because they are different code: the first
// operand is judged before the read and the rest are judged as they are
// filled, so `read 1bad 2bad` never reaches the walk at all and
// `read good 1bad 2bad` is the row that does. Measured — every column that
// reports names `1bad`, and no column names `2bad`.
func TestOnlyTheFirstBadReadNameIsReported(t *testing.T) {
	for _, src := range []string{
		`printf 'X Y\n' | { read 1bad 2bad; echo "st=$?"; }`,
		`printf 'X Y Z\n' | { read good 1bad 2bad; echo "st=$? good=[$good]"; }`,
	} {
		out, _ := readRun(t, nil, Diagnostics{}, src)
		if !strings.Contains(out, "`1bad'") || strings.Contains(out, "2bad") {
			t.Errorf("%s: said %q, want the first bad name alone", src, out)
		}
	}
}

// A count changes who is judged past the first name: bash carries on and
// ksh93 stops. The filling stops at the bad name either way — only the
// complaint is withheld.
func TestAReadCountDecidesWhoIsJudgedAfterTheFirstName(t *testing.T) {
	src := `printf 'XYZW\n' | { b=keep; read -n 3 a 1bad b; echo "st=$? a=[$a] b=[$b]"; }`
	for _, c := range []struct {
		judges Answer
		want   string
	}{
		{Yes, "st=1 a=[XYZ] b=[keep]"},
		{No, "st=0 a=[XYZ] b=[keep]"},
	} {
		out, _ := readRun(t, func(s *Semantics) {
			s.ReadCountJudgesTheNamesAfterTheFirst = c.judges
			s.ReadOptions = "rn:N:"
		}, Diagnostics{}, src)
		if !strings.Contains(out, c.want) {
			t.Errorf("judges=%v: said %q, want %q", c.judges, out, c.want)
		}
	}
	// The exact spelling takes the same answer, and the first operand is
	// judged under a count in both: `read -N 3 1bad` is refused where
	// `read -N 3 a 1bad` is not.
	out, _ := readRun(t, func(s *Semantics) {
		s.ReadCountJudgesTheNamesAfterTheFirst = No
		s.ReadOptions = "rn:N:"
	}, Diagnostics{}, `printf 'XYZW\n' | { read -N 3 1bad; echo "st=$?"; }`)
	if !strings.Contains(out, "st=1") {
		t.Errorf("said %q, want the first operand judged under a count", out)
	}
	// And the axis is asked only where it decides something: a count with no
	// bad name past the first never reaches it.
	out, st := optRun(t, func(s *Semantics) {
		s.ReadCountJudgesTheNamesAfterTheFirst = Unspecified
		s.ReadOptions = "rn:N:"
	}, Diagnostics{}, `printf 'XYZW\n' | { read -n 3 a b; echo "st=$? a=[$a]"; }`)
	if !strings.Contains(out, "st=0 a=[XYZ]") || st != 0 {
		t.Errorf("said %q status %d, want a count with no bad name to ask nothing", out, st)
	}
}

// The operand's prompt is written for a terminal and for nothing else, which
// is the same rule `-p`'s prompt follows. Written unconditionally it would
// print into a pipe nobody is reading it from — and into the *data* a script
// is about to parse, since standard error and standard output are usually the
// same file.
func TestTheReadPromptOperandIsNotWrittenToAPipe(t *testing.T) {
	out, _ := readRun(t, func(s *Semantics) { s.ReadPromptOperand = ReadPromptAloneNamesTheDefault },
		Diagnostics{}, `printf 'text\n' | { read "v?PROMPT-42 "; echo "st=$? v=[$v]"; }`)
	if strings.Contains(out, "PROMPT-42") {
		t.Errorf("said %q, want no prompt where the stream is not a terminal", out)
	}
	if !strings.Contains(out, "st=0 v=[text]") {
		t.Errorf("said %q, want the read to have happened all the same", out)
	}
}

// The exact count hands the text to the first name and clears the rest, and
// the clearing stops at a bad name like every other filling: `b=keep;
// read -N 3 a 1bad b` leaves b as keep in bash and in ksh93 alike, which is
// the two answers of the count axis agreeing about the *list* while they
// disagree about the complaint.
func TestAnExactCountStopsFillingAtABadReadName(t *testing.T) {
	for _, judges := range []Answer{Yes, No} {
		out, _ := readRun(t, func(s *Semantics) {
			s.ReadCountJudgesTheNamesAfterTheFirst = judges
			s.ReadOptions = "rn:N:"
		}, Diagnostics{}, `printf 'XYZW\n' | { b=keep; read -N 3 a 1bad b; echo "a=[$a] b=[$b]"; }`)
		if !strings.Contains(out, "a=[XYZ] b=[keep]") {
			t.Errorf("judges=%v: said %q, want a=[XYZ] b=[keep]", judges, out)
		}
	}
}

// Which operands are judged at all rides the array letter, because the
// letters differ and no shell has both. Measured 2026-09-07:
//
//	bash    read -a arr good 1bad   0, array filled, not a word said
//	ksh93   read -A arr good 1bad   1bad refused, array filled behind it
//
// bash's `-a` takes the array's name in the option's argument and ignores
// the operands after it — `b=keep; read -a arr b` leaves b as keep — so only
// the first is judged, by the check in front of the read. ksh93's `-A` takes
// its name from among the operands and judges the rest, clearing the ones it
// filled and stopping at the bad one.
func TestWhetherAReadArrayIsFilledBehindABadNameRidesTheLetter(t *testing.T) {
	for _, c := range []struct{ name, letters, src, want string }{
		{
			"the option's argument names it",
			"ra:",
			`printf 'X Y Z\n' | { b=keep; read -a arr 1bad b; echo "st=$? arr=[${arr[@]}] b=[$b]"; }`,
			"st=1 arr=[] b=[keep]",
		},
		{
			"an operand names it",
			"rA",
			`printf 'X Y Z\n' | { b=keep; read -A arr 1bad b; echo "st=$? arr=[${arr[@]}] b=[$b]"; }`,
			"st=1 arr=[X Y Z] b=[keep]",
		},
		{
			// The row the walk got wrong: with the bad name behind a good
			// one the check in front of the read never sees it, so this is
			// the operands themselves being judged or not.
			"the option's argument ignores what follows it",
			"ra:",
			`printf 'X Y Z\n' | { read -a arr good 1bad; echo "st=$? arr=[${arr[@]}] good=[$good]"; }`,
			"st=0 arr=[X Y Z] good=[]",
		},
		{
			"an operand's array judges what follows it",
			"rA",
			`printf 'X Y Z\n' | { read -A arr good 1bad; echo "st=$? arr=[${arr[@]}] good=[$good]"; }`,
			"st=1 arr=[X Y Z] good=[]",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := readRun(t, func(s *Semantics) {
				s.ReadOptions = c.letters
				arraySemantics(s)
			}, Diagnostics{}, c.src)
			if !strings.Contains(out, c.want) {
				t.Errorf("said %q, want %q", out, c.want)
			}
		})
	}
	// An array with no bad operand behind it is filled either way, so the
	// rule above is about the refusal rather than about the letter alone.
	out, _ := readRun(t, func(s *Semantics) {
		s.ReadOptions = "ra:"
		arraySemantics(s)
	}, Diagnostics{},
		`printf 'X Y Z\n' | { read -a arr; echo "st=$? arr=[${arr[@]}]"; }`)
	if !strings.Contains(out, "st=0 arr=[X Y Z]") {
		t.Errorf("said %q, want a clean array read to fill it", out)
	}
	// And the exact spelling clears past its array the same way, stopping at
	// the bad name: `read -A -N 3 arr 1bad b` leaves b alone.
	out, _ = readRun(t, func(s *Semantics) {
		s.ReadOptions = "rAN:"
		s.ReadCountJudgesTheNamesAfterTheFirst = No
		arraySemantics(s)
	}, Diagnostics{}, `printf 'XYZW\n' | { b=keep; read -A -N 3 arr 1bad b; echo "st=$? arr=[${arr[@]}] b=[$b]"; }`)
	if !strings.Contains(out, "st=0 arr=[XYZ] b=[keep]") {
		t.Errorf("said %q, want st=0 arr=[XYZ] b=[keep]", out)
	}
}
