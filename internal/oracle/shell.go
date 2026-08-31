// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package oracle records what real shells do.
//
// It implements no shell behavior. It runs shell binaries over a corpus of
// snippets and records their output, which is what makes it safe under the
// rules in CLEANROOM.md: the snippets are ours, and the output of a binary is
// a fact about that binary rather than anyone's expression.
//
// The result is the evidence behind docs/spec. A spec entry that cites an
// oracle run can be re-run; one that cites a memory cannot.
package oracle

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// Shell is one member of the reference panel.
type Shell struct {
	// Name is the stable identifier used in tables and golden files. It
	// survives the binary moving, which paths do not.
	Name string

	// Lookup are candidate paths, tried in order. The first that exists wins.
	Lookup []string

	// Argv0 overrides the name the shell is invoked under. bash behaves as a
	// different language when called as sh — it loses process substitution —
	// so the panel carries both and must be able to say which it ran.
	Argv0 string

	// Args are extra flags placed before -c. The implementation under test
	// needs them: it defaults to a strict core that refuses anything the
	// panel disagrees about, so grading it against bash means telling it to
	// be bash rather than relying on a default to happen to match.
	Args []string

	// Why records what this panel member is here to represent.
	Why string

	// MustReport, when set, is a substring the shell's version string has to
	// contain for this entry to be believed. /bin/sh is bash on macOS and
	// dash on Debian, so the bash-as-sh entry silently recorded dash on a
	// Linux runner until this existed — a mislabeled column is worse than a
	// missing one, because nothing looks wrong.
	MustReport string
}

// Panel is the reference set. Membership is a deliberate claim: dash stands in
// for the minimal /bin/sh implementations, whose absence from the core
// language is the decision recorded in docs/spec/core.md.
var Panel = []Shell{
	{
		Name:   "dash",
		Lookup: []string{"/bin/dash", "/usr/bin/dash"},
		Why:    "the minimal /bin/sh; Debian and Ubuntu ship it as sh",
	},
	{
		Name:   "bash",
		Lookup: []string{"/opt/homebrew/bin/bash", "/usr/local/bin/bash", "/bin/bash", "/usr/bin/bash"},
		Why:    "the dominant scripting target",
	},
	{
		// Deliberately the same binary as the bash entry, not /bin/sh.
		// /bin/sh is bash on macOS and dash on Debian, so pointing here made
		// the column mean different things on different machines; and even
		// where it was bash it was a different build, which conflated the
		// version with the invocation. Same binary, two columns, one variable.
		Name:       "bash-as-sh",
		Lookup:     []string{"/opt/homebrew/bin/bash", "/usr/local/bin/bash", "/bin/bash", "/usr/bin/bash"},
		Argv0:      "sh",
		MustReport: "bash",
		Why:        "argv[0] alone changes the language: the same bash loses constructs when called sh",
	},
	{
		// macOS still ships bash 3.2 (2007, the last GPLv2 release), and it
		// is the oldest build anything has to run on. Absent on Linux, which
		// the run reports rather than hides.
		Name:       "bash32",
		Lookup:     []string{"/bin/bash"},
		MustReport: "version 3.",
		Why:        "the oldest bash that matters: what macOS ships, and what rejects ${x^^}",
	},
	{
		Name:   "ksh93",
		Lookup: []string{"/bin/ksh", "/usr/bin/ksh", "/opt/homebrew/bin/ksh93"},
		Why:    "the other ksh-family lineage; the only panel member without `local`",
	},
	{
		Name:   "zsh",
		Lookup: []string{"/opt/homebrew/bin/zsh", "/usr/local/bin/zsh", "/bin/zsh", "/usr/bin/zsh"},
		Why:    "the interactive incumbent, and the most divergent semantics",
	},
}

// Found is a panel member that exists on this machine, with the build string
// it reported. Versions are recorded because they change answers: ${x^^} is a
// bash 4 feature and bash 3.2 rejects it.
type Found struct {
	Shell
	Path    string
	Version string
}

// Resolve returns the panel members present on this machine, and the names of
// those that are missing.
//
// A missing shell is not an error. It is reported, because a table generated
// from three shells is a weaker claim than the same table generated from five,
// and silently narrowing the panel would overstate the evidence.
func Resolve(ctx context.Context) (found []Found, missing []string) {
	for _, s := range Panel {
		path, ok := locate(s.Lookup)
		if !ok {
			missing = append(missing, s.Name)
			continue
		}
		v := version(ctx, path)
		if s.MustReport != "" && !strings.Contains(strings.ToLower(v), s.MustReport) {
			// The path exists but is not the shell this entry names.
			missing = append(missing, s.Name)
			continue
		}
		found = append(found, Found{Shell: s, Path: path, Version: v})
	}
	return found, missing
}

func locate(candidates []string) (string, bool) {
	for _, c := range candidates {
		if p, err := exec.LookPath(c); err == nil {
			return p, true
		}
	}
	return "", false
}

// version asks the shell what it is. There is no portable spelling, so this
// tries the common ones and falls back to "unknown" rather than failing: an
// unknown version still permits a comparison, it just weakens the record.
func version(ctx context.Context, path string) string {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// --version covers bash and zsh. ksh and dash have no such flag; ksh
	// answers ${.sh.version}, and dash reports nothing at all.
	for _, probe := range [][]string{
		{"--version"},
		{"-c", "echo ${.sh.version}"},
	} {
		out, err := exec.CommandContext(ctx, path, probe...).CombinedOutput()
		if err != nil {
			continue
		}
		if line := firstLine(string(out)); line != "" && !strings.Contains(line, "not found") {
			return line
		}
	}
	return "unknown"
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}
