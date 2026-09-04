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

// The callbacks are deferred, and a dialect whose letter table says so gets
// them named as missing rather than unknown — the difference between a shell
// that lacks something and a typo.
func TestMapfileSaysWhenACallbackIsMerelyMissing(t *testing.T) {
	out, st := mapfileRun(t, `mapfile -C cb arr`, func(r *Runner) {
		dg := Diagnostics{UnimplementedOptionLetters: map[string]string{"mapfile": "Cc"}}
		r.Diagnostics = &dg
	})
	if !strings.Contains(out, "mapfile: -C is not implemented yet") {
		t.Errorf("said %q, want the letter named as missing", out)
	}
	if st != 2 {
		t.Errorf("status = %d, want 2", st)
	}
}

func mapfileRun(t *testing.T, src string, setup func(*Runner)) (string, int) {
	t.Helper()
	var buf strings.Builder
	sem := PosixSemantics()
	r := &Runner{
		Stdout: &buf, Stderr: &buf, Semantics: &sem, Diagnostics: &Diagnostics{},
		Name: "sh", Stdin: strings.NewReader(""),
	}
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
