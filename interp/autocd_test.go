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

// A bare directory name may be read as a `cd`, and whether the substitution is
// announced is the axis two shells disagree about.
//
// The capability is the core's because two of the panel have it, both under
// the same name, and a copy in each dialect would be two places to fix. What
// they do not share is the sentence: one writes the `cd` it substituted before
// moving and the other moves in silence, which is why that half is an Answer
// and not a default.
func TestABareDirectoryNameMayBeReadAsACd(t *testing.T) {
	for _, c := range []struct {
		name        string
		on          bool
		interactive bool
		announces   Answer
		operand     string
		moved       bool
		says        string
	}{
		{"announced", true, true, Yes, "sub", true, "cd -- sub\n"},
		{"in silence", true, true, No, "sub", true, ""},
		// Off, the word is a command that is not there, which is what this
		// shell said before the capability existed.
		{"off", false, true, Yes, "sub", false, "sub"},
		// A script does not get it even with the capability on: the two
		// shells that have the option keep it for a person, measured on
		// 2026-09-08 in both.
		{"in a script", true, false, Yes, "sub", false, "sub"},
		// A word that is not a directory is still a command that is not
		// there, so this is a fallback for directories and not a general one.
		{"not a directory", true, true, Yes, "nosuch", false, "nosuch"},
		// A file is not a directory either, and a *readable* one is the case
		// worth naming: the fallback asks what the path is, not whether it is
		// there.
		{"a plain file", true, true, Yes, "plain", false, "plain"},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "plain"), nil, 0o644); err != nil {
				t.Fatal(err)
			}
			sem := PosixSemantics()
			sem.AutoCdAnnouncesTheSubstitution = c.announces
			out, errOut := &strings.Builder{}, &strings.Builder{}
			r := newTestRunner(t, &Runner{
				Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh", Dir: dir,
				Stdout: out, Stderr: errOut, Interactive: c.interactive,
				// A PATH with nothing on it, so a word that is not
				// substituted fails the way a missing command fails rather
				// than resolving to the directory sitting in the working
				// directory — the two are told apart by what the shell says.
				Vars: map[string]string{"PATH": t.TempDir()},
			})
			r.SetAutoCd(c.on)
			f, err := syntax.Parse(c.operand+"\n", syntax.Core())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := r.Run(context.Background(), f); err != nil {
				t.Fatal(err)
			}
			if moved := r.Dir != dir; moved != c.moved {
				t.Errorf("Dir = %q from %q; moved = %v, want %v", r.Dir, dir, moved, c.moved)
			}
			said := errOut.String() + out.String()
			if c.says == "" && said != "" {
				t.Errorf("said %q, want silence", said)
			}
			if c.says != "" && !strings.Contains(said, c.says) {
				t.Errorf("said %q, want it to contain %q", said, c.says)
			}
			// A word that was not substituted must not have been announced as
			// one: a shell that printed the `cd` and then failed would look
			// like a working option to a check on the directory alone.
			if !c.moved && strings.Contains(said, "cd -- ") {
				t.Errorf("said %q, which announces a substitution that did not happen", said)
			}
		})
	}
}

// A directory never shadows a command.
//
// The condition that makes this a fallback rather than a lookup order: the
// substitution is reached only after the builtins, the functions and PATH have
// each had their turn. Measured with a *directory* named `echo`, which is the
// arrangement that tells the two orders apart.
func TestADirectoryDoesNotShadowACommand(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "echo"), 0o755); err != nil {
		t.Fatal(err)
	}
	sem := PosixSemantics()
	sem.AutoCdAnnouncesTheSubstitution = Yes
	out, errOut := &strings.Builder{}, &strings.Builder{}
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh", Dir: dir,
		Stdout: out, Stderr: errOut, Interactive: true,
		Vars: map[string]string{"PATH": t.TempDir()},
	})
	r.SetAutoCd(true)
	f, err := syntax.Parse("echo hi\n", syntax.Core())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Run(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if out.String() != "hi\n" || r.Dir != dir {
		t.Errorf("stdout %q, Dir %q; want the builtin to have run and the shell to have stayed", out, r.Dir)
	}
}
