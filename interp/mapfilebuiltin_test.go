// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
	"github.com/blairham/sh/syntax"
)

// `mapfile` reads a stream into an indexed array, one element per delimiter,
// and `readarray` is the same command under its other name. The elements land
// in the calling shell, which is the builtin's whole reason to exist — the
// `while read` loop it replaces loses its array to the pipeline's subshell.
func TestMapfileReadsAStreamIntoAnArray(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
		status          int
	}{
		{
			// No operand: the elements land in MAPFILE, and without -t each
			// keeps its trailing delimiter.
			"defaults to MAPFILE and keeps the newline",
			`printf 'a\nb\n' | { mapfile; printf '[%s]%s' "${MAPFILE[1]}" "${#MAPFILE[@]}"; }`,
			"[b\n]2", 0,
		},
		{
			"-t strips the trailing delimiter",
			`printf 'a\nb\n' | { mapfile -t arr; echo "${arr[0]}-${arr[1]}"; }`,
			"a-b", 0,
		},
		{
			// A final line the stream never terminated is still an element,
			// and reaching the end of the input is not a failure.
			"a missing final newline still yields the element",
			`printf 'a\nb' | { mapfile -t arr; echo "${arr[1]}:${#arr[@]}:$?"; }`,
			"b:2:0", 0,
		},
		{
			"-d renames the delimiter",
			`printf 'a:b:' | { mapfile -t -d : arr; echo "${arr[0]}${arr[1]}"; }`,
			"ab", 0,
		},
		{
			"the renamed delimiter is kept without -t",
			`printf 'a:b:' | { mapfile -d : arr; echo "${arr[0]}"; }`,
			"a:", 0,
		},
		{
			"-n caps the elements",
			`printf '1\n2\n3\n' | { mapfile -t -n 2 arr; echo "${arr[1]}:${#arr[@]}"; }`,
			"2:2", 0,
		},
		{
			"-n 0 is no cap at all",
			`printf '1\n2\n3\n' | { mapfile -t -n 0 arr; echo "${#arr[@]}"; }`,
			"3", 0,
		},
		{
			// The bundle splits: -tn is -t -n, through the shared reader.
			"-n leaves the rest of the stream unread",
			`printf '1\n2\n' | { mapfile -tn 1 arr; read -r rest; echo "${arr[0]}+$rest"; }`,
			"1+2", 0,
		},
		{
			"-s throws away the first elements",
			`printf '1\n2\n3\n' | { mapfile -t -s 2 arr; echo "${arr[0]}:${#arr[@]}"; }`,
			"3:1", 0,
		},
		{
			// -s and -n compose: skip, then count.
			"-s then -n",
			`printf '1\n2\n3\n4\n' | { mapfile -t -s 1 -n 2 arr; echo "${arr[0]}${arr[1]}:${#arr[@]}"; }`,
			"23:2", 0,
		},
		{
			// Giving -O at all is what changes the write: the elements land
			// at their subscripts and the rest of the array survives.
			"-O writes into the array it finds",
			`a=(x y z); printf 'q\n' | { mapfile -t -O 1 a; echo "${a[0]}${a[1]}${a[2]}"; }`,
			"xqz", 0,
		},
		{
			"without -O the array is replaced",
			`a=(x y z); printf 'q\n' | { mapfile -t a; echo "${a[0]}:${#a[@]}"; }`,
			"q:1", 0,
		},
		{
			"readarray is the same command",
			`printf 'a\n' | { readarray -t arr; echo "${arr[0]}"; }`,
			"a", 0,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := mapfileRun(t, c.src, nil)
			if !strings.Contains(out, c.want) {
				t.Errorf("said %q, want %q in it", out, c.want)
			}
			if st != c.status {
				t.Errorf("status = %d, want %d (%q)", st, c.status, out)
			}
		})
	}
}

// An empty -d means NUL, and a NUL delimiter is stripped from the elements
// even without -t — measured by the length of what a NUL-delimited element
// leaves behind.
func TestMapfileTakesAnEmptyDelimiterAsNUL(t *testing.T) {
	out, st := mapfileRun(t, `mapfile -d '' arr; echo "${arr[0]}${arr[1]}:${#arr[@]}"`,
		func(r *Runner) { r.Stdin = strings.NewReader("a\x00b\x00") })
	if !strings.Contains(out, "ab:2") {
		t.Errorf("said %q, want ab:2 in it", out)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}

// -u resolves the number against the shell's own table, where `exec 3<file`
// put it — the same table `read -u` reads.
func TestMapfileReadsTheDescriptorUNames(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f"), []byte("held\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, st := mapfileRun(t, `exec 3<f; mapfile -t -u 3 arr; echo "[${arr[0]}]"`,
		func(r *Runner) { r.Dir = dir })
	if !strings.Contains(out, "[held]") {
		t.Errorf("said %q, want the line off descriptor 3", out)
	}
	if st != 0 {
		t.Errorf("status = %d, want 0", st)
	}
}

// The refusals, each with its own wording and each at 1 — where an option the
// builtin does not have at all is 2, through the shared reader.
func TestMapfileRefusesWhatItCannotUse(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
		status          int
	}{
		{"an option it does not have", `mapfile -q arr`, "mapfile: -q: invalid option", 2},
		{"under its other name too", `readarray -q arr`, "readarray: -q: invalid option", 2},
		{"a count that is not one", `mapfile -n x arr`, "mapfile: x: invalid line count", 1},
		{"a negative count", `mapfile -n -3 arr`, "mapfile: -3: invalid line count", 1},
		{"a skip that is not a count", `mapfile -s x arr`, "mapfile: x: invalid line count", 1},
		{"an origin that is not one", `mapfile -O -1 arr`, "mapfile: -1: invalid array origin", 1},
		{
			"a descriptor that is not a number",
			`mapfile -u x arr`, "mapfile: x: invalid file descriptor specification", 1,
		},
		{"a descriptor nothing is open at", `mapfile -u 9 arr`, "mapfile: 9: invalid file descriptor", 1},
		{"a name that is not one", `mapfile -t 1bad`, "mapfile: `1bad': not a valid identifier", 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := mapfileRun(t, c.src, nil)
			if !strings.Contains(out, c.want) {
				t.Errorf("said %q, want %q in it", out, c.want)
			}
			if st != c.status {
				t.Errorf("status = %d, want %d (%q)", st, c.status, out)
			}
		})
	}
}

// -C names a command run every -c elements as the array fills, and -c is how
// many that is. The schedule is the part worth pinning: the call comes *before*
// the element is assigned, and only on a multiple of the quantum, so a -C with
// the default quantum of 5000 calls nothing at all on a short list.
func TestMapfileCallsBackAsTheArrayFills(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
		status          int
	}{
		{
			// Every element, and the subscript it is about to get.
			"-c 1 calls for each element",
			`printf 'a\nb\n' | { mapfile -t -C "echo cb" -c 1 arr; echo "n=${#arr[@]}"; }`,
			"cb 0 a\ncb 1 b\nn=2\n", 0,
		},
		{
			// Not every element: the second and the fourth, which are the
			// multiples of two, and the fifth line never reaches one.
			"-c 2 calls on the multiples",
			`printf '1\n2\n3\n4\n5\n' | { mapfile -t -C "echo cb" -c 2 arr; echo "n=${#arr[@]}"; }`,
			"cb 1 2\ncb 3 4\nn=5\n", 0,
		},
		{
			"the default quantum is 5000, so a short list calls nothing",
			`printf 'a\nb\nc\n' | { mapfile -t -C "echo cb" arr; echo "n=${#arr[@]}"; }`,
			"n=3\n", 0,
		},
		{
			// -O moves the subscript the callback is handed with it.
			"-O moves the subscript",
			`printf 'a\nb\n' | { mapfile -t -O 5 -C "echo cb" -c 1 arr; }`,
			"cb 5 a\ncb 6 b\n", 0,
		},
		{
			// -s does not: a skipped line is never an element, so it is
			// neither counted toward the quantum nor given a subscript.
			"-s does not",
			`printf '1\n2\n3\n4\n' | { mapfile -t -s 2 -C "echo cb" -c 1 arr; }`,
			"cb 0 3\ncb 1 4\n", 0,
		},
		{
			// The element arrives without its delimiter only where -t asked
			// for that; the callback is handed exactly what is stored.
			"the element is what will be stored",
			`printf 'a\n' | { mapfile -C "printf [%s]" -c 1 arr; }`,
			"[0][a\n]", 0,
		},
		{
			// The callback runs before the assignment, and the replacing
			// form has already emptied the array by the time of the first
			// call: it sees neither the element being read nor the three
			// that were there.
			"the array is empty at the first call",
			`cb() { echo "at:${#arr[@]}"; }; arr=(x y z)
printf '1\n2\n' | { mapfile -t -C cb -c 1 arr; echo "n=${#arr[@]}"; }`,
			"at:0\nat:1\nn=2\n", 0,
		},
		{
			// With -O the array is not emptied, so the callback sees what
			// was there.
			"-O leaves the array for the callback to see",
			`cb() { echo "at:${#arr[@]}"; }; arr=(x y z)
printf '1\n' | { mapfile -t -O 1 -C cb -c 1 arr; }`,
			"at:3\n", 0,
		},
		{
			// It runs in the calling shell, which is what makes a progress
			// counter possible at all.
			"the callback runs in the calling shell",
			`n=0; cb() { n=$((n+1)); }; printf 'a\nb\nc\n' | { mapfile -t -C cb -c 1 arr; echo "n=$n"; }`,
			"n=3\n", 0,
		},
		{
			// And a callback that fails is not the builtin's failure.
			"a failing callback is not a failing mapfile",
			`printf 'a\n' | { mapfile -t -C false -c 1 arr; echo "st=$? n=${#arr[@]}"; }`,
			"st=0 n=1\n", 0,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := mapfileRun(t, c.src, nil)
			if out != c.want {
				t.Errorf("said %q, want %q", out, c.want)
			}
			if st != c.status {
				t.Errorf("status = %d, want %d (%q)", st, c.status, out)
			}
		})
	}
}

// The callback is source text with the two arguments appended to it, not a
// command with two arguments handed to it — measured three ways in bash, and
// the third is the decisive one because nothing but a parse can fail on it.
//
// The consequence is that the element has to be quoted on the way in. A line
// holding a semicolon is data; joined raw it would be program.
func TestMapfileCallbackIsSourceTextWithTheArgumentsAppended(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
	}{
		{
			// The arguments land on the *last* command in the string.
			"the arguments reach the last command",
			`printf 'a\n' | { mapfile -t -C "echo one; echo two" -c 1 arr; }`,
			"one\ntwo 0 a\n",
		},
		{
			// And the decisive one, because nothing but a parse can fail on
			// it: an unterminated quote in the callback is a syntax error,
			// which a command with two words appended could not have. The
			// label is `stdin`, measured, where `eval` says `eval`.
			//
			// Nothing follows it here because what a syntax error inside an
			// eval does to the rest of the script is a question this runner
			// already answers, and the substrate's answer is to abandon it.
			// In the one dialect that has the command the script runs on.
			"an unterminated quote in the string is a syntax error",
			`printf 'a\n' | { mapfile -t -C 'echo "' -c 1 arr; }`,
			"sh: stdin: unterminated double quote\n",
		},
		{
			// And the element is one word however it is spelled: two spaces
			// survive, a semicolon does not start a command, and a star is
			// not a pattern.
			"the element is one word",
			`cb() { echo "n=$# [$2]"; }; printf 'a b; echo NO\nx  y\n*\n' | { mapfile -t -C cb -c 1 arr; }`,
			"n=2 [a b; echo NO]\nn=2 [x  y]\nn=2 [*]\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st := mapfileRun(t, c.src, nil)
			if out != c.want {
				t.Errorf("said %q, want %q", out, c.want)
			}
			if st != 0 {
				t.Errorf("status = %d, want 0 (%q)", st, out)
			}
		})
	}
}

// Zero means no cap to -n and is a refusal to -c, and the refusal stands with
// no -C to call: writing the letter at all is what asks for it to be read.
// The whole read is lost with it, which is the part a script notices.
func TestMapfileRefusesAnInvalidCallbackQuantum(t *testing.T) {
	for _, src := range []string{
		`printf 'a\n' | { mapfile -t -C cb -c 0 arr; echo "st=$? n=${#arr[@]}"; }`,
		`printf 'a\n' | { mapfile -t -c 0 arr; echo "st=$? n=${#arr[@]}"; }`,
		`printf 'a\n' | { mapfile -t -c x arr; echo "st=$? n=${#arr[@]}"; }`,
	} {
		out, _ := mapfileRun(t, src, nil)
		if !strings.Contains(out, "invalid callback quantum") || !strings.Contains(out, "st=1 n=0") {
			t.Errorf("%s said %q, want the refusal and an untouched array", src, out)
		}
	}
}

func mapfileRun(t *testing.T, src string, setup func(*Runner)) (string, int) {
	t.Helper()
	var buf strings.Builder
	sem := PosixSemantics()
	r := newTestRunner(t, &Runner{
		Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &Diagnostics{},
		Name: "sh", Stdin: strings.NewReader(""),
	})
	if setup != nil {
		setup(r)
	}
	f, err := syntax.Parse(src, syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	st, err := r.Run(context.Background(), f)
	if err != nil {
		t.Fatal(err)
	}
	return buf.String(), st
}
