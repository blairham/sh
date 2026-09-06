// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// cshRedir is the vector these tests vary, with everything else answered so
// that the only question a snippet asks is the one under test.
func cshRedir(dir string, form GreatAmpTargetForm, dg Diagnostics) func(*Runner) {
	return func(r *Runner) {
		sem := CoreSemantics()
		sem.SplitParamExpansion = Yes
		sem.SplitCommandSubstitution = Yes
		sem.GlobExpansionResults = Yes
		sem.GlobNoMatchIsError = No
		sem.RedirectTargetIsAnOrdinaryWord = No
		sem.MultiDigitDuplicationTargetIsAnError = No
		sem.RedirectErrorOnSpecialBuiltinFatal = No
		sem.DuplicationTargetErrorOnABuiltinIsFatal = No
		sem.GreatAmpTarget = form
		r.Semantics, r.Diagnostics, r.Dir = &sem, &dg, dir
	}
}

// GreatAmpTargetNamesAFile is the csh spelling: `>&word` opens the word and
// sends *both* output streams there, exactly as `&>word` does.
//
// The second stream is what makes it `&>` and not `>`, and is the half a
// reading that only moved standard output would pass without.
func TestGreatAmpTargetNamesAFileOpensItForBothStreams(t *testing.T) {
	dir := t.TempDir()
	src := `{ echo out; echo err >&2; } >&qq; printf "[%s]" "$?"`
	out, st := run(t, src, cshRedir(dir, GreatAmpTargetNamesAFile, Diagnostics{}))
	if st != 0 {
		t.Fatalf("status %d, out %q", st, out)
	}
	if out != "[0]" {
		t.Errorf("out = %q, want %q — nothing should have reached the caller", out, "[0]")
	}
	if got := readFile(t, dir, "qq"); got != "out\nerr\n" {
		t.Errorf("qq = %q, want both streams in it", got)
	}
}

// GreatAmpTargetIsADescriptor keeps `>&` a duplication and refuses the word,
// leaving no file behind.
func TestGreatAmpTargetIsADescriptorRefusesAName(t *testing.T) {
	dir := t.TempDir()
	dg := Diagnostics{DuplicationTargetIsNotADescriptor: "%[2]s: bad file unit number"}
	out, st := run(t, `echo hi >&qq; printf "[%s]" "$?"`, cshRedir(dir, GreatAmpTargetIsADescriptor, dg))
	if out != "sh: qq: bad file unit number\n[1]" {
		t.Errorf("out = %q, want the refusal in the dialect's words and a status of 1", out)
	}
	if st != 0 {
		t.Errorf("status %d, want the script to have carried on", st)
	}
	if got := readFile(t, dir, "qq"); got != "" {
		t.Errorf("qq = %q, want no file made by a refusal", got)
	}
}

// The two forms that open a file part company over a word that expanded to
// nothing: one takes it as a name and fails on the empty path, the other
// takes it as a descriptor and refuses it.
//
// This is the difference a script reaches by writing `>&"${COPROC[1]}"` in a
// shell with no such array, which is the whole reason it is a form.
func TestAnEmptyGreatAmpTargetSplitsTheTwoFileForms(t *testing.T) {
	dg := Diagnostics{
		CannotCreate:           "%[2]s: %[1]s",
		EmptyDuplicationTarget: "%[1]s: Bad file descriptor",
	}
	for _, tc := range []struct {
		name string
		form GreatAmpTargetForm
		want string
	}{
		{
			"a name, so the open is attempted and fails on the empty path",
			GreatAmpTargetNamesAnyFile,
			"sh: No such file or directory: \n[1]",
		},
		{
			"a descriptor, so it is refused before any open",
			GreatAmpTargetNamesAFile,
			"sh: \"\": Bad file descriptor\n[1]",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			out, _ := run(t, `echo hi >&""; printf "[%s]" "$?"`, cshRedir(dir, tc.form, dg))
			if out != tc.want {
				t.Errorf("out = %q, want %q", out, tc.want)
			}
		})
	}
}

// The leading number is the whole of the question: `2>&word` is a duplication
// in every form, including the ones that open a file for the bare spelling.
func TestANumberedGreatAmpIsNeverAFile(t *testing.T) {
	dir := t.TempDir()
	dg := Diagnostics{DuplicationTargetIsNotADescriptor: "%[2]s: ambiguous redirect"}
	out, _ := run(t, `echo hi 2>&qq; printf "[%s]" "$?"`, cshRedir(dir, GreatAmpTargetNamesAFile, dg))
	if out != "sh: qq: ambiguous redirect\n[1]" {
		t.Errorf("out = %q, want the numbered form refused, no file opened and the command not run", out)
	}
	if got := readFile(t, dir, "qq"); got != "" {
		t.Errorf("qq = %q, want no file made by a refusal", got)
	}
}

// And the reading side is never a file, whatever the form says about writing.
func TestLessAmpIsNeverAFile(t *testing.T) {
	dir := t.TempDir()
	touch(t, dir, "qq")
	dg := Diagnostics{DuplicationTargetIsNotADescriptor: "file number expected"}
	for _, form := range []GreatAmpTargetForm{
		GreatAmpTargetIsADescriptor, GreatAmpTargetNamesAFile, GreatAmpTargetNamesAnyFile,
	} {
		t.Run(form.String(), func(t *testing.T) {
			out, _ := run(t, `read -r l <&qq; printf "[%s]" "$?"`, cshRedir(dir, form, dg))
			if out != "sh: file number expected\n[1]" {
				t.Errorf("out = %q, want the reading side refused in every form", out)
			}
		})
	}
}

// A word that *is* a descriptor is nobody's question, so a vector that has
// not answered the form still runs `>&2`.
func TestTheGreatAmpFormIsAskedAboutOnlyWhenItMatters(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		refuses   bool
	}{
		{"a descriptor number", `echo hi >&2`, false},
		{"a close", `exec 3>&-`, false},
		{"a name", `echo hi >&qq`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			out, st := run(t, tc.src, cshRedir(dir, GreatAmpTargetUnspecified, Diagnostics{}))
			refused := strings.Contains(out, "unanswered") || st == 2
			if refused != tc.refuses {
				t.Errorf("out = %q status %d; want refused=%v", out, st, tc.refuses)
			}
		})
	}
}

// DuplicationTargetErrorOnABuiltinIsFatal ends the shell over a `<&word` that
// named no descriptor — and only where the command runs *in* the shell. An
// external command takes the same refusal and the script carries on, which is
// what makes the boundary the command rather than the redirection.
func TestABadDuplicationTargetEndsTheShellOnlyOnABuiltin(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		wantAfter bool
	}{
		{"a builtin stops it", `read -r l <&qq; echo after`, false},
		{"an external command does not", `/bin/cat <&qq; echo after`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			touch(t, dir, "qq")
			out, _ := run(t, tc.src, func(r *Runner) {
				cshRedir(dir, GreatAmpTargetNamesAFile,
					Diagnostics{DuplicationTargetIsNotADescriptor: "file number expected"})(r)
				sem := *r.Semantics
				sem.DuplicationTargetErrorOnABuiltinIsFatal = Yes
				sem.FatalErrorStatusIsOne = Yes
				r.Semantics = &sem
			})
			if got := strings.Contains(out, "after"); got != tc.wantAfter {
				t.Errorf("out = %q, want the script to have carried on = %v", out, tc.wantAfter)
			}
		})
	}
}

// `set -C` refuses `&>f` over a file that is there, exactly as it refuses a
// plain `>f` — and the csh spelling inherits that by being the same operator
// rather than a copy of it.
//
// Unanimous across the panel, so there is no axis: `>|` is the documented
// override for `>` and there is no `>|&` anywhere to exempt this one.
func TestNoclobberRefusesBothStreamsToOneFile(t *testing.T) {
	for _, tc := range []struct{ name, src string }{
		{"the ampersand spelling", `set -C; : > qq; true &>qq; printf "[%s]" "$?"`},
		{"and the csh spelling", `set -C; : > qq; true >&qq; printf "[%s]" "$?"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			dg := Diagnostics{NoclobberRefusal: "%[1]s: cannot overwrite existing file"}
			out, _ := runGrammar(t, tc.src, func(d *syntax.Dialect) { d.AmpersandRedirect = true },
				cshRedir(dir, GreatAmpTargetNamesAFile, dg))
			if out != "sh: qq: cannot overwrite existing file\n[1]" {
				t.Errorf("out = %q, want the refusal and a status of 1", out)
			}
		})
	}
}
