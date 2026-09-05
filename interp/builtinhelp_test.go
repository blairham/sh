// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// helpRun keeps the two streams apart, which optRun does not — and the
// separation is the whole point here: the answer to `--help` goes to standard
// output where every refusal beside it goes to standard error, and a merged
// buffer could not tell an implementation that got that backwards from one
// that did not.
func helpRun(t *testing.T, dg Diagnostics, src string) (out, errOut string, status int) {
	t.Helper()
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	var o, e bytes.Buffer
	sem := permissive()
	sem.BadOptionToSpecialBuiltinFatal = No
	// A letter that takes an argument, so the reader has one to claim the
	// word with; and a letter that does not, so there is an option region to
	// answer in past the first word.
	sem.ReadOptions = "rd:"
	r := newTestRunner(t, &Runner{
		Stdout: &o, Stderr: &e, Semantics: &sem, Diagnostics: &dg, Name: "testsh",
	})
	st, rerr := r.Run(context.Background(), f)
	if rerr != nil {
		t.Fatalf("run %q: %v", src, rerr)
	}
	return o.String(), e.String(), st
}

func helpDiagnostics() Diagnostics {
	return Diagnostics{
		BuiltinBadOption:  "%[1]s: %[2]s: invalid option",
		BuiltinHelpStatus: 7,
		BuiltinHelp: map[string]string{
			"export": "export: export [-fn] [name[=value] ...]",
		},
	}
}

// The answer is on standard output and the refusal beside it is not, which is
// how a script tells them apart.
func TestTheHelpOptionIsAnsweredOnStandardOutput(t *testing.T) {
	out, errOut, _ := helpRun(t, helpDiagnostics(), `export --help; echo "st=$?"`)
	if !strings.Contains(out, "export: export [-fn] [name[=value] ...]") {
		t.Errorf("stdout %q, want the help text", out)
	}
	if strings.Contains(errOut, "export") {
		t.Errorf("stderr %q, want nothing about the builtin there", errOut)
	}
	if !strings.Contains(out, "st=7") {
		t.Errorf("stdout %q, want the dialect's help status", out)
	}
	// And it did not go on to do the work: `export --help` exports nothing.
	out, _, _ = helpRun(t, helpDiagnostics(), `export --help x=1; echo "[${x-unset}]"`)
	if !strings.Contains(out, "[unset]") {
		t.Errorf("stdout %q, want the builtin not to have run", out)
	}
}

// A builtin with no entry has no answer, and the word is then the option
// nobody has — which is what three of the panel's five do for every builtin.
func TestABuiltinWithNoHelpRefusesTheHelpOption(t *testing.T) {
	out, errOut, _ := helpRun(t, helpDiagnostics(), `unset --help; echo "st=$?"`)
	if !strings.Contains(errOut, "unset: --: invalid option") {
		t.Errorf("stderr %q, want the ordinary bad-option refusal", errOut)
	}
	if !strings.Contains(out, "st=2") {
		t.Errorf("stdout %q, want the bad-option status", out)
	}
	// And with no map at all, the same: nothing answers.
	_, errOut, _ = helpRun(t, Diagnostics{BuiltinBadOption: "%[1]s: %[2]s: invalid option"},
		`export --help`)
	if !strings.Contains(errOut, "invalid option") {
		t.Errorf("stderr %q, want the refusal", errOut)
	}
}

// The status defaults rather than falling to zero, because the option reader
// says "this builtin is finished" with a nonzero code and has no other way to
// say it: a zero here would print the answer and then run the builtin.
func TestTheHelpStatusDefaultsToTheUsageStatus(t *testing.T) {
	dg := helpDiagnostics()
	dg.BuiltinHelpStatus = 0
	out, _, _ := helpRun(t, dg, `export --help x=1; echo "st=$?"; echo "[${x-unset}]"`)
	if !strings.Contains(out, "st=2") {
		t.Errorf("stdout %q, want the default status", out)
	}
	if !strings.Contains(out, "[unset]") {
		t.Errorf("stdout %q, want the builtin not to have run", out)
	}
}

// It has to be the exact word, and it has to stand where an option stands.
func TestTheHelpOptionIsTheWholeWordInTheOptionRegion(t *testing.T) {
	for _, src := range []string{
		// An abbreviation is not it, and neither is a word with anything
		// after it.
		`export --hel`,
		`export --help=x`,
		// Past the `--` that ends the options it is an operand, so this is a
		// complaint about a name rather than about an option.
		`export -- --help`,
	} {
		out, errOut, _ := helpRun(t, helpDiagnostics(), src)
		if strings.Contains(out, "[name[=value] ...]") {
			t.Errorf("%s: stdout %q, want no help answer", src, out)
		}
		if errOut == "" {
			t.Errorf("%s: stderr empty, want a complaint", src)
		}
	}
	// But it is answered where an option stands, not only as the first word.
	dg := helpDiagnostics()
	dg.BuiltinHelp["read"] = "read: read [-r] [-d delim] [name ...]"
	out, _, _ := helpRun(t, dg, `read -r --help`)
	if !strings.Contains(out, "read: read [-r] [-d delim] [name ...]") {
		t.Errorf("stdout %q, want the help answer after another option", out)
	}
}

// A letter that takes an argument has already claimed the word, so the reader
// answers this rather than a scan of the argument list.
func TestALetterThatTakesAnArgumentClaimsTheHelpWord(t *testing.T) {
	dg := helpDiagnostics()
	dg.BuiltinHelp["read"] = "read: read [-r] [-d delim] [name ...]"
	out, _, _ := helpRun(t, dg, `printf 'a-b\n' | { read -d --help v; echo "[$v]"; }`)
	if strings.Contains(out, "read: read [-r]") {
		t.Errorf("stdout %q, want --help taken as the delimiter argument", out)
	}
	if !strings.Contains(out, "[a]") {
		t.Errorf("stdout %q, want the read to have stopped at the `-`", out)
	}
}
