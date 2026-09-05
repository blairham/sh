// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// type's remaining letters — TypeOptions — and the three axes measured
// inside them. The external is made here rather than borrowed from the
// host, the way the command -v tests already do.

func typeDir(t *testing.T) (dir, exe string) {
	t.Helper()
	dir = t.TempDir()
	exe = filepath.Join(dir, "echo")
	if err := os.WriteFile(exe, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir, exe
}

func typeRun(t *testing.T, src, path string, set func(*Semantics)) (string, string, int) {
	t.Helper()
	return declRun(t, "PATH="+path+"\n"+src, func(s *Semantics) {
		s.TypeEndsOptionsWithDashDash = Yes
		s.TypePrintsFunctionBody = No
		if set != nil {
			set(s)
		}
	}, Diagnostics{})
}

// TestTypeAListsEveryResolution: the shell's own answer and then every PATH
// hit, worded `name is /path` whatever the dialect's plain external wording.
func TestTypeAListsEveryResolution(t *testing.T) {
	dir, exe := typeDir(t)
	out, _, st := typeRun(t, "type -a echo\necho st=$?", dir, func(s *Semantics) {
		s.TypeOptions = "afpP"
	})
	want := "echo is a shell builtin\necho is " + exe + "\n"
	if !strings.Contains(out, want) || st != 0 {
		t.Errorf("stdout = %q (status %d), want %q in it", out, st, want)
	}
}

// TestTypePSpeaksOnlyForFiles in one answer, searches past the shell in the
// other — and the answer's shape is its own axis.
func TestTypePSpeaksOnlyForFiles(t *testing.T) {
	dir, exe := typeDir(t)
	out, errs, st := typeRun(t, "type -p echo\necho st=$?", dir, func(s *Semantics) {
		s.TypeOptions = "afpP"
		s.TypePSearchesPathPastTheShell = No
	})
	if !strings.HasPrefix(out, "st=0") || errs != "" || st != 0 {
		t.Errorf("stdout %q stderr %q, want silence and 0 for a builtin", out, errs)
	}

	out, _, _ = typeRun(t, "type -p echo", dir, func(s *Semantics) {
		s.TypeOptions = "afpP"
		s.TypePSearchesPathPastTheShell = Yes
		s.TypePathAnswerIsASentence = No
	})
	if out != exe+"\n" {
		t.Errorf("stdout = %q, want the bare path %q", out, exe)
	}

	out, _, _ = typeRun(t, "type -p echo", dir, func(s *Semantics) {
		s.TypeOptions = "afpP"
		s.TypePSearchesPathPastTheShell = Yes
		s.TypePathAnswerIsASentence = Yes
	})
	if out != "echo is "+exe+"\n" {
		t.Errorf("stdout = %q, want the sentence", out)
	}

	// A miss: silence and the failing status in the bare shape, the
	// dialect's complaint in the sentence one.
	out, errs, _ = typeRun(t, "type -p nosuchzz\necho st=$?", dir, func(s *Semantics) {
		s.TypeOptions = "afpP"
		s.TypePSearchesPathPastTheShell = Yes
		s.TypePathAnswerIsASentence = No
	})
	if !strings.Contains(out, "st=1") || errs != "" {
		t.Errorf("stdout %q stderr %q, want a silent 1", out, errs)
	}
}

// TestTypeCapitalPAlwaysSearches: the PATH answer even for a builtin.
func TestTypeCapitalPAlwaysSearches(t *testing.T) {
	dir, exe := typeDir(t)
	out, _, _ := typeRun(t, "type -P echo", dir, func(s *Semantics) {
		s.TypeOptions = "afpP"
	})
	if out != exe+"\n" {
		t.Errorf("stdout = %q, want %q past the builtin", out, exe)
	}
}

// TestTypeFSkipsOrSays: functions left out of the search, or printed whole.
func TestTypeFSkipsOrSays(t *testing.T) {
	dir, exe := typeDir(t)
	tool := filepath.Join(dir, "tool")
	if err := os.WriteFile(tool, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	out, _, _ := typeRun(t, "tool() { :; }\ntype -f tool", dir, func(s *Semantics) {
		s.TypeOptions = "afpP"
		s.TypeFSaysTheFunctionBack = No
	})
	if !strings.Contains(out, "tool is "+tool) || strings.Contains(out, "function") {
		t.Errorf("stdout = %q, want the function skipped and the file named", out)
	}
	_ = exe
	out, _, _ = typeRun(t, "hi() { echo hi; }\ntype -f hi", dir, func(s *Semantics) {
		s.TypeOptions = "afpP"
		s.TypeFSaysTheFunctionBack = Yes
	})
	if !strings.HasPrefix(out, "hi () \n{ \n") || !strings.Contains(out, "echo hi") {
		t.Errorf("stdout = %q, want the function said back whole", out)
	}
}
