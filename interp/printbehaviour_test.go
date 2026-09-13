// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"os"
	"os/signal"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"

	"github.com/blairham/sh/internal/oracle"
	"github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// The corpus, printed back and then *run*, against what the original does.
//
// Re-parsing is the weaker half of the promise and it is the half that looks
// convincing. A dropped here-document body re-parses perfectly and reads from
// the terminal instead of from the body; a lost `&` re-parses and waits. Only
// running both says whether the printed source means what it said.
//
// Under a wider grammar than the core, because the core refuses whatever the
// shells disagree about and most of the corpus is about exactly that — the
// question here is the printer, not the semantics.
func TestPrintedSourceStillMeansTheSameThing(t *testing.T) {
	unread := 0
	for _, c := range oracle.Corpus {
		if c.SyntaxError {
			continue
		}
		if c.LayoutSensitive {
			// What this case pins is where a diagnostic points, which is a
			// fact about the source's layout. The printer promises meaning,
			// not layout, and its zero layout is nobody's — so running the
			// printed form and comparing the line it named would be checking
			// the wrong promise.
			continue
		}
		t.Run(c.ID, func(t *testing.T) {
			// Before the parse, so a case that changed the process is named
			// here rather than felt by whatever ran next. See processState.
			defer processStateNow(t).unchangedBy(t)
			f, err := syntax.Parse(c.Snippet, corpusGrammar())
			if err != nil {
				unread++
				t.Skipf("the grammar here does not read it: %v", err)
			}
			printed := syntax.Print(f)

			// A directory each, because a case that creates a file would
			// find it already there on the second run — and the name taken
			// out of what they printed, because two directories differ by
			// their names and that is not the printer's doing either.
			wantOut, wantStatus := runUnderBash(t, c.Snippet)
			gotOut, gotStatus := runUnderBash(t, printed)
			if gotOut != wantOut || gotStatus != wantStatus {
				t.Errorf("printing changed what it does\n  from:   %s\n  gave:   %s\n  before: %q (%d)\n  after:  %q (%d)",
					c.Snippet, printed, wantOut, wantStatus, gotOut, gotStatus)
			}
		})
	}
	// A skip is invisible in a passing run, so a grammar that quietly stopped
	// reading half the corpus would read as a green round-trip over half as
	// many cases. Every case here parses today; if one stops, that is the
	// finding rather than a smaller test.
	if unread != 0 {
		t.Errorf("%d corpus cases did not parse; corpusGrammar no longer reads the corpus", unread)
	}
}

// corpusGrammar is the grammar the round-trip reads the corpus with: the core,
// widened until it reads every case.
//
// Named here rather than taken from a dialect. What this test needs is a
// grammar wide enough to reach the cases and to mean what they meant — which
// shell happens to have that grammar is not the printer's business, and
// borrowing one made `interp`'s tests stop compiling without `dialect/bash`
// (#491).
//
// Seven of these are what the corpus cannot be *read* without, measured by
// turning each off against the cases that are neither syntax errors nor
// layout-sensitive: ParamIndirection (8 cases), Coproc (3), CaseContinue,
// CoprocName and PipeBothStreams (2 each), ExtendedPatternInCondition and
// FunctionKeywordParens (1 each) — 19 between them, which is what the plain
// core cannot read. ClobberOverrideMarker is an eighth, and three of its
// cases are the same kind: `>>|`, `&>>|` and `>>!` after a compound are text
// the core reads as a pipe or a stray word rather than as a redirection.
// FunctionMultipleNames is a ninth: eight cases giving one body several
// names, where the core reads the second name as the body and the brace group
// after it closes nothing, or reads the whole list as a command with a stray
// `(` after it. FunctionKeywordBodyIsOptional is a tenth and the newest: two
// cases whose keyword form has a `;` before its body or no body at all. The rest change
// what a case *means* rather than whether it parses, and they are here
// because the printer should be exercised on those meanings rather than on
// whatever a narrower reading turns them into.
//
// The unread count above is what keeps the six honest.
func corpusGrammar() syntax.Dialect {
	d := syntax.Core()
	d.ArraySubscriptFlags = true
	// A `!` with no pipeline after it, and a second `!` that toggles the
	// first. An eleventh and twelfth thing the corpus cannot be read
	// without: three cases write one, and the core drops the `!` outright
	// rather than reading the line.
	d.BareNegationReach = syntax.BareNegationBeforeATerminator
	d.RepeatedNegationToggles = true
	d.CaseContinue = true
	// And zsh's spelling of the same terminator. No real shell has both —
	// each refuses the other's — so this pair is a superset the printer is
	// given on purpose: the corpus records both spellings, and a reading
	// dialect narrow enough to be a shell could not read all of it.
	d.CaseContinuePipe = true
	d.ClobberOverrideMarker = true
	d.CasePatternListSpansNewlines = true
	// And a blank in the same place. One case is written that way and the
	// core reads none of it: inside the arm's parentheses the list is one
	// word, so `(a b)` is a pattern rather than two words.
	d.CasePatternListSpansBlanks = true
	d.Coproc = true
	d.CoprocName = true
	d.CurrentShellSubstitution = true
	d.DollarDoubleQuote = true
	d.ExtendedPatternInCondition = true
	d.FdVariableSubscript = true
	d.FuncDefAtParen = true
	d.FunctionKeywordParens = true
	d.FunctionKeywordNameIsAnyWord = true
	// And the stage two dialects check such a name at. Two cases are keyword
	// definitions whose name holds an expansion, and without this the
	// grammar refuses them while reading where those shells read them and
	// complain when the definition runs.
	d.FunctionNameCheckedWhenTheDefinitionRuns = true
	// And the `name()` spelling of the same rule. Four cases name a function
	// with a space, an operator, nothing at all or quotes, and the core
	// refuses every one of them at the parenthesis.
	d.FunctionNameIsAnyWord = true
	// One body under several names, in both spellings. Eight cases, and the
	// core reads none of them: after the keyword the second name is read as
	// the *body* and the brace group after it has nothing to be part of, and
	// before the parentheses the words are a command's arguments with a `(`
	// behind them.
	d.FunctionMultipleNames = true
	// And a name list that ends without a body, plus the `;` that may stand
	// between the names and a body it does have. Two cases, one of them
	// inside an `eval` where the printer never reaches it.
	d.FunctionKeywordBodyIsOptional = true
	d.MultiDigitFdNumber = true
	d.ParamCaseChange = true
	// `<(cmd)`. Three cases are written with it — the ones about `sysopen`,
	// where the substitution *is* the subject — and the core reads `<` as a
	// redirection and the parenthesis as a subshell after it.
	d.ProcessSubstitution = true
	// And `=(cmd)`, the temp-file spelling. Six cases are written with it —
	// the ones about the file the construct makes — and without the flag the
	// core reads the `=` as an ordinary character and stops at the `(`.
	d.ProcessSubstitutionToFile = true
	d.ParamIndirection = true
	d.ParamTransformations = true
	d.PipeBothStreams = true
	d.RegexTakesAlternation = true
	d.TimePosixFlag = true
	// A here-document whose delimiter carries the `)` that closes the
	// construct it is in. Three cases are written that way and two of the six
	// shells refuse them, so this is one of the flags the corpus cannot be
	// read without.
	d.HeredocEndsAtClosingParen = true
	// `declare` beside the four the core already reads as declarations, so a
	// snippet using it keeps the assignment rule when it is printed back.
	d.DeclarationUtilities = map[string]bool{
		"declare": true, "export": true, "local": true,
		"readonly": true, "typeset": true,
	}
	return d
}

// guardedBuffer is a buffer that may be read while the shell is still writing
// it.
//
// `Run` returning does not mean every goroutine the script started has
// finished: a background job outlives the script that started it, which is
// what a background job is for, and in a real shell it is a separate process
// that goes on writing to the terminal after the shell has gone. Here it is a
// goroutine holding the caller's io.Writer, so reading that writer the
// instant Run returns is a read racing a write.
//
// The shell's own lock cannot help — it guards the writers it hands out, and
// this is the far side of one. So the buffer carries its own, and the same
// lock covers the String that reads it.
type guardedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (g *guardedBuffer) Write(p []byte) (int, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.buf.Write(p)
}

func (g *guardedBuffer) String() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.buf.String()
}

// runUnderBash runs a snippet the way the conformance harness would, and
// returns what it printed and reported.
func runUnderBash(t *testing.T, src string) (string, int) {
	t.Helper()
	f, err := syntax.Parse(src, corpusGrammar())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var out guardedBuffer
	dir := t.TempDir()
	sem, dg := testSemantics(), interp.PosixDiagnostics()
	r := newTestRunner(t, &interp.Runner{
		Semantics: &sem, Diagnostics: &dg,
		Stdout: &out, Stderr: &out,
		Stdin: strings.NewReader(""),
		// TMPDIR is the Runner's own, so a corpus case with a process
		// substitution in it makes its directory somewhere the framework
		// takes away — not in a real one, where nothing calls CleanUp.
		// Its own directory rather than Dir: the path a substitution
		// expands to would otherwise be rewritten by the <dir> masking
		// below, which is meant for what the *snippet* prints.
		Name: "sh", Dir: dir, Env: append(testPATH(), "TMPDIR="+t.TempDir()),
	})
	status, rerr := r.Run(context.Background(), f)
	text := strings.ReplaceAll(out.String(), dir, "<dir>")
	if rerr != nil {
		return text + "refused: " + rerr.Error(), -1
	}
	return text, status
}

// processState is the state a corpus case could change that is not the
// Runner's, taken before a case runs and checked after it.
//
// This loop runs every corpus snippet in the test binary's own process, so
// anything a case leaves behind is inherited by every test after it — and by
// every child those tests exec. The failure has no locality at all: the
// symptom is four SIGPIPE errors in unrelated tests hours of reading later,
// which is how #2446 was first written off as a load-dependent flake.
//
// A guard rather than a scrub, and deliberately. Signal dispositions are
// prevented at the source now — a `trap` borrows one and Finish gives it back
// (restoreDispositions) — so nothing here has to be undone. What this catches
// is the *next* thing, whatever it turns out to be, at the case that did it
// and by name. A scrub around the loop would have hidden that case instead.
//
// The three things checked are the three a snippet can still reach in
// principle. The working directory and the environment are already the
// Runner's own — `cd` declines to call os.Chdir and nothing calls os.Setenv,
// both by design — so those two are expected to be silent forever, and their
// silence is the evidence rather than an oversight. umask and the resource
// limits are absent because interp cannot reach them at all without the
// SetUmask and SetRlimit hooks, which this test does not supply: a case that
// runs `umask 077` changes the Runner's idea of the mask and not the
// process's.
type processState struct {
	ignored []os.Signal
	dir     string
	env     []string
}

// leakableSignals are the signals whose ignored-ness the runtime will report,
// which is the disposition that survives exec and therefore the one a case
// can hand to a later test's children.
//
// SIGKILL and SIGSTOP cannot be ignored. SIGURG and SIGPROF belong to the Go
// runtime. The four job-control signals are left out for the reason
// internal/oracle leaves them out of its own list: the runtime keeps an
// inherited SIG_IGN for them and does not report it, so watching them would
// be watching an answer known to be wrong.
var leakableSignals = []os.Signal{
	syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT, syscall.SIGILL,
	syscall.SIGTRAP, syscall.SIGABRT, syscall.SIGFPE, syscall.SIGBUS,
	syscall.SIGSEGV, syscall.SIGSYS, syscall.SIGPIPE, syscall.SIGALRM,
	syscall.SIGTERM, syscall.SIGCHLD, syscall.SIGXCPU, syscall.SIGXFSZ,
	syscall.SIGVTALRM, syscall.SIGWINCH, syscall.SIGUSR1, syscall.SIGUSR2,
}

func processStateNow(t *testing.T) processState {
	t.Helper()
	var st processState
	for _, sig := range leakableSignals {
		if signal.Ignored(sig) {
			st.ignored = append(st.ignored, sig)
		}
	}
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("reading the working directory: %v", err)
	}
	st.dir = dir
	st.env = os.Environ()
	slices.Sort(st.env)
	return st
}

// unchangedBy reports what this case left behind, named as such: the point is
// that the case which caused it is the one that fails.
func (before processState) unchangedBy(t *testing.T) {
	t.Helper()
	after := processStateNow(t)
	if !slices.Equal(before.ignored, after.ignored) {
		t.Errorf("this case changed the process's ignored signals from %v to %v, which every test after it inherits", before.ignored, after.ignored)
	}
	if before.dir != after.dir {
		t.Errorf("this case changed the process's working directory from %s to %s", before.dir, after.dir)
	}
	if !slices.Equal(before.env, after.env) {
		t.Errorf("this case changed the process's environment")
	}
}
