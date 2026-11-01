// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/internal/asttree"
	"github.com/blairham/sh/syntax"
)

// The prelude this binary ships already parsed is the tree a parse of the
// text makes now (#6397). See the same test in dialect/bash for why the
// second check is there: DeepEqual cannot tell a node reached twice from two
// copies of it, and a literal can only be the second.
func TestPreludeTree(t *testing.T) {
	t.Parallel()
	want, err := syntax.Parse(zsh.Prelude(), zsh.Dialect())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(zsh.PreludeTree(), want) {
		t.Fatal(
			"dialect/zsh/preludetree.go is not what the prelude parses to; regenerate it with `go run ./internal/preludegen`",
		)
	}
	if _, err := asttree.Emit(want, zsh.Prelude(), "zsh", "preludeTree", "", nil); err != nil {
		t.Fatalf("the prelude's tree cannot be written as a literal: %v", err)
	}
}

// What a script can see of the prelude is the same whether the front end
// parsed it at start or took it from the binary, run both ways and compared
// byte for byte. The last check is the control, as in dialect/bash: the same
// text a line lower must be told apart by at least one row.
func TestPreludeTreeIsUnobservable(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	nofile := filepath.Join(dir, "noshebang")
	if err := os.WriteFile(nofile, []byte("whence -w pushd __dirs_rotate\npopd\nprint $LINENO\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	rows := []string{
		// The listings, and the names a listing leaves out (#1035, #2464).
		`functions; typeset -f; print -r -- "[$?]" ${(k)functions}`,
		`whence -v pushd popd dirs __dirs_rotate __compinit_dump; whence -w pushd; which pushd; print "[$?]"`,
		`type pushd; functions pushd; print "[$?]"; print -r -- ${+functions[pushd]} ${+builtins[pushd]}`,
		// A refusal from inside one, located at the caller (#603, #5447).
		"print one\npopd\npushd +9\ndirs -z\npopd extra",
		"f() {\n  popd\n}\nf",
		// Tracing a call into one.
		"set -x\npushd " + dir + "\npopd\nset +x",
		`PS4='+%N:%i> '; set -x; pushd ` + dir + `; dirs -v; popd`,
		// The call stack, seen from a function the prelude calls back into.
		`cd() { print -r -- "$funcstack" "$functrace" "$funcfiletrace"; builtin cd "$@"; }; pushd ` + dir + `; popd`,
		`trap 'print -r -- "trap $LINENO"' DEBUG; pushd ` + dir + ` >/dev/null`,
		// Taking the voice away: unset, then redefined.
		`unset -f pushd; pushd /; print "[$?]"; whence -w pushd`,
		`pushd() { print "mine $LINENO"; }; pushd; functions pushd; popd`,
		// What the prelude's top level leaves behind.
		`print -r -- "$ZSH_VERSION" "$WORDCHARS" "[$_]" "$LINENO" "[$pipestatus]"; alias`,
		// The fresh shell a file with no shebang runs in sources it too.
		nofile,
	}
	run := func(tree *syntax.File, src string) string {
		var out, errs bytes.Buffer
		sh := zshWriting(&out, &errs)
		sh.PreludeTree = tree
		code := driver.MainArgs(sh, []string{"zsh", "-c", src})
		return fmt.Sprintf("%s\x00%s\x00%d", out.String(), errs.String(), code)
	}
	for _, src := range rows {
		parsed, built := run(nil, src), run(zsh.PreludeTree(), src)
		if parsed != built {
			t.Errorf("%q\nparsed at start: %q\nbuilt in:        %q", src, parsed, built)
		}
	}

	// Numbered from line 2 rather than pasted under a newline, which is the
	// same tree one line down without making the text a snippet.
	lp := syntax.NewParserAt(zsh.Prelude(), zsh.Dialect(), 2)
	lower := lp.Parse()
	if err := lp.Err(); err != nil {
		t.Fatal(err)
	}
	seen := false
	for _, src := range rows {
		if run(lower, src) != run(zsh.PreludeTree(), src) {
			seen = true
			break
		}
	}
	if !seen {
		t.Error("no row told the prelude from the same text a line lower, so no row could have seen a wrong tree")
	}
}

// A start does not parse the prelude. `zsh -c :` allocated 4,266 times in
// process with the text parsed at start and 629 with the tree built in
// (measured on main at 0db5fe1a6 and on this change, darwin). Allocations
// rather than time, so the row is deterministic; the bound has room above
// what a start costs now and sits far below the parse coming back.
//
// Not parallel: it reads the process's allocation counters.
func TestAStartDoesNotParseThePrelude(t *testing.T) {
	const bound = 1500
	var out, errs bytes.Buffer
	sh := zshWriting(&out, &errs)
	got := testing.AllocsPerRun(10, func() { driver.MainArgs(sh, []string{"zsh", "-c", ":"}) })
	if got > bound {
		t.Errorf("zsh -c : allocates %v times, want at most %d — is the prelude being parsed at start again?", got, bound)
	}
}
