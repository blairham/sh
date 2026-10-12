// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/internal/asttree"
	"github.com/blairham/sh/syntax"
)

// The prelude this binary ships already parsed is the tree a parse of the
// text makes now, on this target (#6397).
//
// Run on every target CI builds, and that is half the point: the literal is
// written once, on one machine, and the bash prelude's text names the machine
// the binary is for, so every offset after that word is arithmetic over a
// constant. A Linux run and a macOS run of this test are the proof that the
// arithmetic is right on both.
//
// The second check is the one DeepEqual cannot make. A tree that reaches one
// node from two places compares equal to one holding two copies, and a
// literal can only ever be the second; asking the generator to write the
// fresh parse is what fails if the parser starts sharing a node.
func TestPreludeTree(t *testing.T) {
	want, err := syntax.Parse(bash.Prelude(), bash.Dialect())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(bash.PreludeTree(), want) {
		t.Fatal("dialect/bash/preludetree.go is not what the prelude parses to; regenerate it with `go run ./internal/preludegen`")
	}
	if _, err := asttree.Emit(want, bash.Prelude(), "bash", "preludeTree", "", nil); err != nil {
		t.Fatalf("the prelude's tree cannot be written as a literal: %v", err)
	}
}

// What a script can see of the prelude is the same whether the front end
// parsed it at start or took it from the binary — every surface #603, #1035,
// #1081, #2464 and #5447 put on a prelude function, run both ways and
// compared byte for byte.
//
// The last row is the control, and it is what makes the others mean
// something: the same text parsed one line lower, standing in for a tree that
// is wrong by one position. At least one row must tell that tree from the
// right one, or the comparison could not have seen a tree that differed.
func TestPreludeTreeIsUnobservable(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	nofile := filepath.Join(dir, "noshebang")
	if err := os.WriteFile(nofile, []byte("type pushd __dirs_rotate\npopd\necho \"$LINENO\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	rows := []string{
		// The listings, and the names a listing leaves out (#1035, #2464).
		`declare -f; declare -F; typeset -f; compgen -A function; echo "[$?]"`,
		`type pushd popd dirs __dirs_rotate; type -a pushd; echo "[$?]"`,
		`declare -f pushd; echo "[$?]"; compgen -A builtin pushd; command -V dirs`,
		`shopt -s extdebug; declare -F pushd dirs __dirs_rotate; echo "[$?]"`,
		// A refusal from inside one, located at the caller (#603, #5447).
		"echo one\npopd\npushd +9\ndirs -z\npushd -L\npopd extra",
		"f() {\n  popd\n}\nf",
		// Tracing a call into one.
		"set -x\npushd " + dir + "\npopd\nset +x",
		`PS4='+${LINENO}:${FUNCNAME[0]-}: '; set -x; pushd ` + dir + `; dirs -v; popd`,
		// A DEBUG trap and the call stack, seen from a function the prelude
		// calls back into.
		`trap 'echo "trap $LINENO ${FUNCNAME[0]-} $BASH_COMMAND"' DEBUG; pushd ` + dir + ` >/dev/null`,
		`set -T; trap 'echo "trap $LINENO ${FUNCNAME[0]-}"' DEBUG; pushd ` + dir + ` >/dev/null; popd >/dev/null`,
		`cd() { caller 0; caller 1; echo "${BASH_LINENO[*]} ${FUNCNAME[*]} ${BASH_SOURCE[*]}"; builtin cd "$@"; }; pushd ` + dir + `; popd`,
		// Taking the voice away: unset, then redefined.
		`unset -f pushd; pushd /; echo "[$?]"; type pushd`,
		`pushd() { echo "mine $LINENO"; }; pushd; declare -f pushd; declare -F; popd`,
		// What the prelude's top level leaves behind.
		`echo "$BASH_VERSION" "${BASH_VERSINFO[*]}" "$BASH" "[$_]" "$LINENO"; declare -p BASH_VERSINFO; echo "${#PIPESTATUS[@]}"`,
		// The fresh shell a file with no shebang runs in sources it too.
		nofile,
	}
	run := func(tree *syntax.File, src string) string {
		var out, errs bytes.Buffer
		sh := bashShell(&out, &errs)
		sh.PreludeTree = tree
		code := driver.MainArgs(sh, []string{"bash", "-c", src})
		return fmt.Sprintf("%s\x00%s\x00%d", out.String(), errs.String(), code)
	}
	for _, src := range rows {
		parsed, built := run(nil, src), run(bash.PreludeTree(), src)
		if parsed != built {
			t.Errorf("%q\nparsed at start: %q\nbuilt in:        %q", src, parsed, built)
		}
	}

	// Numbered from line 2 rather than pasted under a newline, which is the
	// same tree one line down without making the text a snippet.
	lp := syntax.NewParserAt(bash.Prelude(), bash.Dialect(), 2)
	lower := lp.Parse()
	if err := lp.Err(); err != nil {
		t.Fatal(err)
	}
	seen := false
	for _, src := range rows {
		if run(lower, src) != run(bash.PreludeTree(), src) {
			seen = true
			break
		}
	}
	if !seen {
		t.Error("no row told the prelude from the same text a line lower, so no row could have seen a wrong tree")
	}
}

// A start does not parse the prelude. `bash -c :` allocated 3,608 times in
// process with the text parsed at start, 3,396 of them the parse, and 212 with
// the tree built in (measured on main at 0db5fe1a6 and on this change, darwin;
// 3,662 against 212 under the race detector). Allocations rather than time,
// so the row is deterministic; the bound has room above what a start costs
// now and sits far below the parse coming back.
//
// Not parallel: it reads the process's allocation counters.
func TestAStartDoesNotParseThePrelude(t *testing.T) {
	const bound = 1000
	var out, errs bytes.Buffer
	sh := bashShell(&out, &errs)
	got := testing.AllocsPerRun(10, func() { driver.MainArgs(sh, []string{"bash", "-c", ":"}) })
	if got > bound {
		t.Errorf("bash -c : allocates %v times, want at most %d — is the prelude being parsed at start again?", got, bound)
	}
}
