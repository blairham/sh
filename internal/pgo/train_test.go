// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package pgo_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/dialect/ash"
	"github.com/blairham/sh/dialect/bash"
	"github.com/blairham/sh/dialect/dash"
	"github.com/blairham/sh/dialect/ksh"
	"github.com/blairham/sh/dialect/zsh"
	"github.com/blairham/sh/driver"
	"github.com/blairham/sh/internal/pgo"
)

// shells are the five dialects as their binaries build them, less the
// interactive and protocol seams a `-c` run never reaches. A field missing
// here costs the profile nothing it would weigh: what is trained is the code
// a script runs, and every field below is what decides which code that is.
func shells(out *bytes.Buffer) map[string]driver.Shell {
	return map[string]driver.Shell{
		"bash": {
			Name: "bash", SystemStartupDirectory: "/etc",
			Dialect: bash.Dialect(), Semantics: bash.Semantics(), Diagnostics: bash.Diagnostics(),
			Prelude: bash.Prelude(), PreludeTree: bash.PreludeTree(), Register: bash.Apply,
			PromptStyle: bash.PromptStyle(), EditorStyle: bash.EditorStyle(),
			HistoryStyle: bash.HistoryStyle(), HookStyle: bash.HookStyle(),
			Stdout: out, Stderr: out,
		},
		"zsh": {
			Name: "zsh", SystemStartupDirectory: zsh.SystemStartupDirectory("/etc"),
			Dialect: zsh.Dialect(), Semantics: zsh.Semantics(), Diagnostics: zsh.Diagnostics(),
			Prelude: zsh.Prelude(), PreludeTree: zsh.PreludeTree(), Register: zsh.Apply,
			CommandStringParameter: "ZSH_EXECUTION_STRING",
			PromptStyle:            zsh.PromptStyle(), EditorStyle: zsh.EditorStyle(),
			HistoryStyle: zsh.HistoryStyle(), HookStyle: zsh.HookStyle(), MailStyle: zsh.MailStyle(),
			Stdout: out, Stderr: out,
		},
		"ksh": {
			Name: "ksh", SystemStartupDirectory: "/etc",
			Dialect: ksh.Dialect(), Semantics: ksh.Semantics(), Diagnostics: ksh.Diagnostics(),
			Prelude: ksh.Prelude(), Register: ksh.Apply,
			PromptStyle: ksh.PromptStyle(), EditorStyle: ksh.EditorStyle(), HistoryStyle: ksh.HistoryStyle(),
			Stdout: out, Stderr: out,
		},
		"dash": {
			Name: "dash", SystemStartupDirectory: "/etc",
			Dialect: dash.Dialect(), Semantics: dash.Semantics(), Diagnostics: dash.Diagnostics(),
			Prelude: dash.Prelude(), Register: dash.Apply,
			PromptStyle: dash.PromptStyle(), EditorStyle: dash.EditorStyle(), HistoryStyle: dash.HistoryStyle(),
			Stdout: out, Stderr: out,
		},
		"ash": {
			Name: "ash", SystemStartupDirectory: "/etc",
			Dialect: ash.Dialect(), Semantics: ash.Semantics(), Diagnostics: ash.Diagnostics(),
			Prelude: ash.Prelude(), Register: ash.Apply,
			PromptStyle: ash.PromptStyle(), EditorStyle: ash.EditorStyle(), HistoryStyle: ash.HistoryStyle(),
			Stdout: out, Stderr: out,
		},
	}
}

// TestEveryTrainingProgramAnswers is what keeps the profile honest: a program
// this shell refused would train the compiler on the refusal, and the run
// would look exactly like one that did the work. Each program is run in each
// dialect it names and must print its answer and nothing else.
func TestEveryTrainingProgramAnswers(t *testing.T) {
	var out bytes.Buffer
	sh := shells(&out)
	for _, p := range pgo.Programs() {
		for _, d := range p.Dialects {
			t.Run(p.Name+"/"+d, func(t *testing.T) {
				out.Reset()
				s, ok := sh[d]
				if !ok {
					t.Fatalf("no shell named %q", d)
				}
				status := driver.RunCommand(s, p.Text, nil)
				if got := out.String(); status != 0 || got != p.Answer {
					t.Fatalf("exited %d and printed %q, want 0 and %q", status, got, p.Answer)
				}
			})
		}
	}
}

// BenchmarkTrain runs every program in every dialect it names, once per
// iteration. It is what `make pgo` profiles; its time means nothing on its own.
func BenchmarkTrain(b *testing.B) {
	var out bytes.Buffer
	sh := shells(&out)
	for b.Loop() {
		for _, p := range pgo.Programs() {
			for range p.Weight {
				for _, d := range p.Dialects {
					out.Reset()
					if status := driver.RunCommand(sh[d], p.Text, nil); status != 0 || out.String() != p.Answer {
						b.Fatalf("%s/%s exited %d and printed %q", p.Name, d, status, strings.TrimSpace(out.String()))
					}
				}
			}
		}
	}
}

// TestEveryShellBuildsWithTheProfile holds the arrangement the package
// comment describes: each shell's directory carries a default.pgo that is a
// link to this package's one profile, and the profile is there to be read.
//
// A binary directory without the link builds with no profile at all and says
// nothing about it — the shell runs, the tests pass, and it is slower — so a
// shell added under cmd/ fails here until it is given one.
func TestEveryShellBuildsWithTheProfile(t *testing.T) {
	data, err := os.ReadFile("default.pgo")
	if err != nil {
		t.Fatalf("the profile is missing: %v — run `make pgo`", err)
	}
	// A pprof profile is gzipped protobuf, which is what `go build -pgo`
	// reads; a text listing or an empty file would build as though there were
	// no profile.
	if len(data) < 1024 || data[0] != 0x1f || data[1] != 0x8b {
		t.Fatalf("default.pgo is %d bytes and not a gzipped profile", len(data))
	}
	entries, err := os.ReadDir("../../cmd")
	if err != nil {
		t.Fatal(err)
	}
	seen := 0
	for _, e := range entries {
		if !e.IsDir() || e.Name() == "shfmt" {
			continue
		}
		seen++
		link := filepath.Join("../../cmd", e.Name(), "default.pgo")
		target, err := os.Readlink(link)
		if err != nil {
			t.Errorf("%s: %v — every shell is built with the profile; link it with "+
				"`ln -s ../../internal/pgo/default.pgo %s`", link, err, link)
			continue
		}
		if target != "../../internal/pgo/default.pgo" {
			t.Errorf("%s points at %s, not at this package's profile", link, target)
		}
	}
	if seen < 6 {
		t.Errorf("found %d shells under cmd/, want at least the six: the walk is looking in the wrong place", seen)
	}
}
