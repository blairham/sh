// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/blairham/sh/driver"
)

// A compatibility level outside the range this shell accepts is complained
// about where it is **assigned**, and stored all the same.
//
// Measured 2026-09-22 on `/opt/homebrew/bin/bash` 5.3.20 under `LC_ALL=C`:
// the range runs from the oldest release with a compatibility reading to the
// shell's own, the spelling is `NN` or `N.N` and nothing else, and the
// complaint costs the script neither its value nor its status — `echo
// "$BASH_COMPAT"` after the refusal prints what was written and `$?` is 0.
func TestACompatibilityLevelOutOfRangeIsComplainedAboutAndStored(t *testing.T) {
	dir := t.TempDir()
	const complaint = "compatibility value out of range"
	for _, c := range []struct {
		name, value string
		want        bool
	}{
		{"the oldest level", "31", false},
		{"this shell's own release", "53", false},
		{"one in between", "44", false},
		{"spelled with a dot", "5.1", false},
		{"the oldest level with a dot", "3.1", false},
		{"not a number at all", "abc", true},
		{"below the floor", "30", true},
		{"zero", "0", true},
		{"past the ceiling", "54", true},
		{"far past it", "99", true},
		// The dot is a removal and not a decimal point, so the spellings
		// around it are each a value rather than a rounding.
		{"two digits after the dot", "4.10", true},
		{"a leading dot", ".44", true},
		{"a trailing dot", "44.", true},
		{"a leading zero", "044", true},
		{"padded with a space", " 44", true},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := runBash(t, dir, `BASH_COMPAT=`+quoteForTest(c.value)+`; echo "st=$? [$BASH_COMPAT]"`)
			if got := strings.Contains(out, complaint); got != c.want {
				t.Errorf("%q: output %q, want the complaint present=%v", c.value, out, c.want)
			}
			if c.want {
				wantWholeLines(t, out, "bash: line 1: BASH_COMPAT: "+c.value+": "+complaint)
			}
			// Complained about or not, the value is stored and the status is
			// the assignment's own.
			wantWholeLines(t, out, "st=0 ["+c.value+"]")
			if st != 0 {
				t.Errorf("%q: status %d, want 0", c.value, st)
			}
		})
	}
}

// An empty value is silent, which is the row that keeps the complaint from
// firing on the way a script takes the level back.
func TestAnEmptyCompatibilityLevelIsSilent(t *testing.T) {
	dir := t.TempDir()
	out, _ := runBash(t, dir, `BASH_COMPAT=51; BASH_COMPAT=; echo "[$BASH_COMPAT]"`)
	if strings.Contains(out, "out of range") {
		t.Errorf("output %q, want no complaint", out)
	}
}

// Every spelling of an assignment is a store, so every one of them complains.
// Measured on bash 5.3.20, which says the same sentence at the same line for
// all five.
func TestEveryAssignmentSpellingComplainsAboutABadLevel(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct{ name, src string }{
		{"plain", `BASH_COMPAT=abc`},
		{"exported", `export BASH_COMPAT=abc`},
		{"declared", `declare BASH_COMPAT=abc`},
		{"appended", `BASH_COMPAT+=abc`},
		{"a command's prefix", `BASH_COMPAT=abc :`},
		{"local to a call", `f(){ local BASH_COMPAT=abc; }; f`},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runBash(t, dir, c.src)
			wantWholeLines(t, out, "bash: line 1: BASH_COMPAT: abc: compatibility value out of range")
		})
	}
}

// The `shopt compatNN` letters and `BASH_COMPAT` are one state, and they move
// together in both directions.
//
// Measured 2026-09-22 on bash 5.3.20. `shopt -s compatNN` writes the level;
// `shopt -u compatNN` resets to this shell's own release only when the level
// *is* NN, and writes the level back either way — which is the half a model
// that wrote only on a change gets wrong, and is why `BASH_COMPAT=4.4; shopt
// -u compat42` re-spells the parameter as `44`.
func TestTheShoptLettersAndTheParameterAreOneState(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct{ name, src, want string }{
		{"a letter writes the parameter", `shopt -s compat44`, "44"},
		{"the oldest letter", `shopt -s compat31`, "31"},
		{"the last letter named wins", `shopt -s compat44 compat43`, "43"},
		{"unsetting the level in force resets it", `shopt -s compat44; shopt -u compat44`, "53"},
		{"unsetting another letter leaves it", `BASH_COMPAT=42; shopt -u compat44`, "42"},
		{"and still writes it", `BASH_COMPAT=4.4; shopt -u compat42`, "44"},
		{"unsetting with nothing set writes the default", `shopt -u compat44`, "53"},
		{"a value out of range is the default", `BASH_COMPAT=abc 2>/dev/null; shopt -u compat44`, "53"},
		{"over a value out of range the letter still writes", `BASH_COMPAT=abc 2>/dev/null; shopt -s compat44`, "44"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runBash(t, dir, c.src+`; echo "[$BASH_COMPAT]"`)
			wantWholeLines(t, out, "["+c.want+"]")
		})
	}
}

// The other direction: the parameter answers the letter.
func TestTheParameterAnswersTheShoptLetter(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct{ name, set, want string }{
		{"the level named", `BASH_COMPAT=44;`, "on"},
		{"spelled with a dot", `BASH_COMPAT=4.4;`, "on"},
		{"a level below it", `BASH_COMPAT=43;`, "off"},
		{"a level above it", `BASH_COMPAT=51;`, "off"},
		{"unset", ``, "off"},
		{"emptied", `BASH_COMPAT=;`, "off"},
		{"out of range", `BASH_COMPAT=abc 2>/dev/null;`, "off"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runBash(t, dir, c.set+` shopt compat44`)
			// The listing row and not the whole output: the out-of-range
			// case writes its complaint first, and a redirection on a bare
			// assignment does not take it away — measured, in bash too.
			lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
			row := strings.Fields(lines[len(lines)-1])
			if len(row) != 2 || row[0] != "compat44" || row[1] != c.want {
				t.Errorf("%q: output %q, want compat44 %s", c.set, out, c.want)
			}
		})
	}
}

// The letter reaches the reading, which is the point of joining the two: the
// row #4256 answered under the parameter's spelling has to answer under this
// one. `unset a[@]` at a level of 5.1 or below removes the variable.
func TestAShoptLetterReachesACompatibilityRow(t *testing.T) {
	dir := t.TempDir()
	const tail = `; unset "a[@]"; if declare -p a >/dev/null 2>&1; then echo KEPT; else echo GONE; fi`
	for _, c := range []struct{ name, set, want string }{
		{"a letter below the threshold", `shopt -s compat44;`, "GONE"},
		{"the oldest letter", `shopt -s compat31;`, "GONE"},
		{"the letter taken back again", `shopt -s compat44; shopt -u compat44;`, "KEPT"},
		{"no letter at all", ``, "KEPT"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runBash(t, dir, c.set+`a=(1 2 3)`+tail)
			if got := strings.TrimSpace(out); got != c.want {
				t.Errorf("%q: got %q, want %q", c.set, got, c.want)
			}
		})
	}
}

// The letters stop where bash's do. 5.3.20 has no `shopt compat51`, so the
// letter door reaches only levels at or below 44 while the parameter reaches
// every level in the range, and a letter past the end is not a name.
func TestThereIsNoLetterPastTheLastOneBashHas(t *testing.T) {
	dir := t.TempDir()
	out, st := runBash(t, dir, `shopt -s compat51`)
	wantWholeLines(t, out, "bash: line 1: shopt: compat51: invalid shell option name")
	if st != 1 {
		t.Errorf("status %d, want 1", st)
	}
}

// quoteForTest puts single quotes round a value so that a space in it reaches
// the assignment as part of the word.
func quoteForTest(value string) string {
	return "'" + value + "'"
}

// A bad level the shell was **launched** with is complained about too, and the
// complaint carries no location at all.
//
// The route a user actually reaches for — `BASH_COMPAT=44 make`, a level
// exported from a parent shell — and the one a store cannot hear, since a name
// that arrives in the environment is never written (#4267).
//
// Measured 2026-09-22 on `/opt/homebrew/bin/bash` 5.3.20 under `LC_ALL=C`: the
// shell's own name and nothing after it, on `-c`, a script file and standard
// input alike, where the same sentence from an assignment carries `line 1`. On a
// script file that is the *shell's* name and not the script's, which is the row
// that shows nothing of the script has been read when this is written.
func TestABadCompatibilityLevelInTheEnvironmentIsComplainedAbout(t *testing.T) {
	for _, c := range []struct {
		name, value string
		set         bool
		want        string
	}{
		{name: "not a number", value: "abc", set: true, want: "bash: BASH_COMPAT: abc: compatibility value out of range"},
		{name: "past the ceiling", value: "99", set: true, want: "bash: BASH_COMPAT: 99: compatibility value out of range"},
		{name: "below the floor", value: "0", set: true, want: "bash: BASH_COMPAT: 0: compatibility value out of range"},
		// In range, empty, and absent are each silent.
		{name: "a level", value: "44", set: true},
		{name: "a dotted level", value: "5.1", set: true},
		{name: "empty", value: "", set: true},
		{name: "absent"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if c.set {
				t.Setenv("BASH_COMPAT", c.value)
			} else {
				// t.Setenv first, so the suite puts whatever was there
				// back; then away, because absent is not empty.
				t.Setenv("BASH_COMPAT", "")
				if err := os.Unsetenv("BASH_COMPAT"); err != nil {
					t.Fatal(err)
				}
			}
			for _, route := range []struct {
				name string
				argv []string
			}{
				{"-c", []string{"bash", "-c", `echo ran`}},
				{"a script file", []string{"bash", writeBashScript(t, "echo ran\n")}},
			} {
				t.Run(route.name, func(t *testing.T) {
					var out, errs bytes.Buffer
					if code := driver.MainArgs(bashShell(&out, &errs), route.argv); code != 0 {
						t.Fatalf("status %d, out %q, stderr %q", code, out.String(), errs.String())
					}
					if strings.TrimSpace(out.String()) != "ran" {
						t.Errorf("stdout = %q, want the script to have run", out.String())
					}
					lines := nonEmptyLines(errs.String())
					if c.want == "" {
						if len(lines) != 0 {
							t.Errorf("stderr = %q, want nothing said", errs.String())
						}
						return
					}
					if len(lines) != 1 || lines[0] != c.want {
						t.Errorf("stderr = %q, want exactly the one line %q", errs.String(), c.want)
					}
				})
			}
		})
	}
}

// The environment's complaint comes before the inherited option list's, and the
// two carry different locations — which is why they are two doors rather than
// one with a zero in it.
//
// Measured on bash 5.3.20: `SHELLOPTS=nosuchopt BASH_COMPAT=abc bash -c 'echo
// hi'` writes the parameter's sentence with no location, then the list's at
// `line 0`, then `hi`.
func TestTheEnvironmentsLevelIsComplainedAboutBeforeItsOptionList(t *testing.T) {
	t.Setenv("BASH_COMPAT", "abc")
	t.Setenv("SHELLOPTS", "nosuchopt")
	var out, errs bytes.Buffer
	driver.MainArgs(bashShell(&out, &errs), []string{"bash", "-c", `echo hi`})
	want := []string{
		"bash: BASH_COMPAT: abc: compatibility value out of range",
		"bash: line 0: nosuchopt: invalid option name",
	}
	if got := nonEmptyLines(errs.String()); !slices.Equal(got, want) {
		t.Errorf("stderr lines = %q, want %q", got, want)
	}
	if strings.TrimSpace(out.String()) != "hi" {
		t.Errorf("stdout = %q, want hi", out.String())
	}
}

// Inherited and then assigned is two complaints and not one: the startup value
// is its own event, and a script that writes the name is heard separately.
func TestAnInheritedLevelAndAnAssignedOneAreEachComplainedAbout(t *testing.T) {
	t.Setenv("BASH_COMPAT", "abc")
	var out, errs bytes.Buffer
	driver.MainArgs(bashShell(&out, &errs), []string{"bash", "-c", "BASH_COMPAT=99\necho done"})
	want := []string{
		"bash: BASH_COMPAT: abc: compatibility value out of range",
		"bash: line 1: BASH_COMPAT: 99: compatibility value out of range",
	}
	if got := nonEmptyLines(errs.String()); !slices.Equal(got, want) {
		t.Errorf("stderr lines = %q, want %q", got, want)
	}
}

// writeBashScript puts a snippet in a file of its own, for the routes that are
// about how the input was read.
func writeBashScript(t *testing.T, src string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "case.sh")
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// nonEmptyLines is what a diagnostic stream said, one whole line per entry.
func nonEmptyLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			out = append(out, line)
		}
	}
	return out
}
