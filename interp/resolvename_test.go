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

// What a name would resolve to, handed out without a wording attached.
//
// The point of the API is that the two vary independently: every dialect
// resolves a name the same way and none of them says so in the same words, so
// a dialect with a builtin of its own asking this question needs the
// resolution rather than a sentence — and a resolution it redid itself is one
// that can disagree with the shell's.

// resolveRunner is a runner with a function defined, a PATH holding one
// executable, and nothing else.
func resolveRunner(t *testing.T) (*Runner, string) {
	t.Helper()
	dir := t.TempDir()
	tool := filepath.Join(dir, "tool431")
	if err := os.WriteFile(tool, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	f, err := syntax.Parse("f() { :; }\n", syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	sem := permissive()
	r := newTestRunner(t, &Runner{Semantics: &sem, Dir: dir, Vars: map[string]string{"PATH": dir}})
	if _, rerr := r.Run(context.Background(), f); rerr != nil {
		t.Fatal(rerr)
	}
	return r, tool
}

func TestResolveNameAnswersTheKindAndThePath(t *testing.T) {
	r, tool := resolveRunner(t)
	for _, c := range []struct {
		name string
		kind NameKind
		path string
	}{
		{"f", NameFunction, ""},
		{"echo", NameBuiltin, ""},
		{"if", NameReserved, ""},
		{"tool431", NameFile, tool},
		{"nosuchcmd431", NameNotFound, ""},
	} {
		kind, path := r.ResolveName(c.name)
		if kind != c.kind || path != c.path {
			t.Errorf("%q resolved to (%v, %q), want (%v, %q)", c.name, kind, path, c.kind, c.path)
		}
	}
}

// The order is the resolution's own, so a name that is several things answers
// with the one the shell would actually run.
func TestResolveNameFollowsTheResolutionOrder(t *testing.T) {
	r, _ := resolveRunner(t)
	f, err := syntax.Parse("echo() { :; }\n", syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	if _, rerr := r.Run(context.Background(), f); rerr != nil {
		t.Fatal(rerr)
	}
	if kind, _ := r.ResolveName("echo"); kind != NameFunction {
		t.Errorf("a function shadowing a builtin resolved to %v, want a function", kind)
	}
}

// A name the shell must answer itself is never resolved from PATH, even where
// the builtin is missing. Reporting the file would say the shell would run it,
// which is the silent failure reserved.go exists to prevent.
func TestResolveNameDoesNotHandOutAReservedBuiltinsFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "umask")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	sem := permissive()
	r := newTestRunner(t, &Runner{Semantics: &sem, Dir: dir, Vars: map[string]string{"PATH": dir}})
	r.Unregister("umask")
	if kind, p := r.ResolveName("umask"); kind == NameFile {
		t.Errorf("resolved to the file %q, want the name refused instead", p)
	}
}

func TestNameKindNamesItself(t *testing.T) {
	for _, c := range []struct {
		kind NameKind
		want string
	}{
		{NameFunction, "function"},
		{NameBuiltin, "builtin"},
		{NameReserved, "reserved"},
		{NameFile, "file"},
		{NameNotFound, "not found"},
	} {
		if got := c.kind.String(); got != c.want {
			t.Errorf("%d.String() = %q, want %q", c.kind, got, c.want)
		}
	}
}

// FunctionText is the definition written back, for a builtin that shows a
// body rather than naming one.
func TestFunctionTextIsTheDefinition(t *testing.T) {
	r, _ := resolveRunner(t)
	body, ok := r.FunctionText("f")
	if !ok {
		t.Fatal("no text for a function that is defined")
	}
	if !strings.Contains(body, "f ()") || !strings.Contains(body, ":") {
		t.Errorf("text = %q, want the definition", body)
	}
	if _, ok := r.FunctionText("nosuchcmd431"); ok {
		t.Error("a name no function has answered with text")
	}
}

// LookPathAll is every hit rather than the first, which is what a listing
// that shows them all needs.
func TestLookPathAllListsEveryHit(t *testing.T) {
	first, second := t.TempDir(), t.TempDir()
	for _, dir := range []string{first, second} {
		if err := os.WriteFile(filepath.Join(dir, "tool431"), []byte("#!/bin/sh\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	sem := permissive()
	r := newTestRunner(t, &Runner{Semantics: &sem, Dir: first, Vars: map[string]string{"PATH": first + ":" + second}})
	hits := r.LookPathAll("tool431")
	want := []string{filepath.Join(first, "tool431"), filepath.Join(second, "tool431")}
	if len(hits) != len(want) {
		t.Fatalf("got %v, want %v", hits, want)
	}
	for i := range hits {
		if hits[i] != want[i] {
			t.Errorf("hit %d = %q, want %q", i, hits[i], want[i])
		}
	}
	if got := r.LookPathAll("nosuchcmd431"); len(got) != 0 {
		t.Errorf("a name nothing holds gave %v, want none", got)
	}
}
