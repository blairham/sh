// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"os"
	"path/filepath"
	"strings"
)

// StartupPwdNamePolicy is the name a shell gives the directory it starts in.
//
// It reads like a question about `pwd` and is really a question about *which
// spelling* of one directory the shell will use for the rest of the session.
// A directory reached through a symbolic link has at least two true names, and
// `$PWD`, `pwd` and every relative `cd` afterwards are written in whichever
// one the shell picked at startup.
//
// Measured 2026-09-13 on macOS, where `/var` is a link to `/private/var`, by
// starting each shell with its working directory set to a path under `/var`:
//
//	                                  handed a PWD    handed only a HOME
//	ksh93u+                           as handed       named under HOME
//	BusyBox ash                       as handed       the resolved path
//	bash 5.3, bash 3.2, zsh, dash     the resolved    the resolved path
//
// Three answers, and the split is not cosmetic: it decides what a script
// building a path out of `$PWD` writes into a file, and it is why eight rows
// of the conformance corpus read `/private` against a recorded answer that
// does not — the harness starts each case in a scratch directory under the
// macOS `TMPDIR`, which is exactly such a link, and hands it over as `HOME`.
//
// **What separates the two shells that do not simply ask the kernel.**
//
// With `PWD` handed over, they agree while it is *true* and part company over
// rubbish: `PWD=/usr` in a directory that is not `/usr` is still what `$PWD`
// expands to in ksh93 — while `pwd` there answers the real directory, so the
// two can disagree — and BusyBox ash drops it and uses the real directory for
// both. `PWD=relative`, `PWD=` and `PWD=/nonexistent` go the same way in each.
// A name that reaches the directory by another route is taken by both:
// `PWD=/tmp/d/../d` is what `pwd` prints there, dot-dot and all, so the test
// is a stat and never a comparison of text.
//
// With no `PWD` at all, ksh93 looks at `$HOME`. Where the directory is at or
// under the home directory, the name is `$HOME` with the path from there
// appended — measured a case at a time: `HOME` naming the directory itself
// gives exactly `$HOME`, `HOME` naming its parent through a dot-dot gives
// `$HOME/basename`, dot-dot and all, and a `HOME` the directory is not under
// gives the kernel's answer. It is `HOME` and no other name: the same value in
// a variable called anything else changes nothing.
type StartupPwdNamePolicy int

const (
	// StartupPwdNameUnspecified is no answer, and is refused like any other.
	StartupPwdNameUnspecified StartupPwdNamePolicy = iota
	// StartupPwdNameFromTheKernel asks where the directory *is* and ignores
	// every name it was handed. bash 5.3, bash 3.2, zsh and dash.
	StartupPwdNameFromTheKernel
	// StartupPwdNameFromTheEnvironmentWhenItFits takes the `PWD` it was
	// handed where that really names the directory, and otherwise asks the
	// kernel. BusyBox ash, where `$PWD` and `pwd` are always one answer.
	StartupPwdNameFromTheEnvironmentWhenItFits
	// StartupPwdNameFromTheEnvironmentOrHome keeps the `PWD` it was handed
	// whatever it says, and where none was handed over names the directory
	// under `$HOME` if it sits there. ksh93.
	StartupPwdNameFromTheEnvironmentOrHome
)

func (p StartupPwdNamePolicy) String() string {
	switch p {
	case StartupPwdNameFromTheKernel:
		return "StartupPwdNameFromTheKernel"
	case StartupPwdNameFromTheEnvironmentWhenItFits:
		return "StartupPwdNameFromTheEnvironmentWhenItFits"
	case StartupPwdNameFromTheEnvironmentOrHome:
		return "StartupPwdNameFromTheEnvironmentOrHome"
	}
	return "StartupPwdNameUnspecified"
}

// settleStartupPwd applies the policy, and is the whole of what ensurePWD asks
// before it writes an answer of its own.
//
// It reports whether `$PWD` has been settled, so the caller falls back to the
// kernel's answer only where no name from the environment was taken. Where one
// *was*, r.Dir moves with it: the name is the shell's own from then on, so a
// later `cd ..` and a later `pwd` are written in the same spelling.
func (r *Runner) settleStartupPwd() bool {
	policy := r.sem().StartupPwdName
	if policy == StartupPwdNameFromTheKernel || policy == StartupPwdNameUnspecified {
		return false
	}
	if handed, ok := r.inheritedValue("PWD"); ok {
		switch {
		case r.namesWorkingDirectory(handed):
			r.Dir = handed
		case policy != StartupPwdNameFromTheEnvironmentOrHome:
			// The value says nowhere, so this shell starts as though none
			// had been handed over at all.
			return false
		}
		// Kept as the parameter either way in the shell that keeps it, which
		// is how `$PWD` and `pwd` come to disagree there.
		r.setVarQuietly("PWD", handed)
		return true
	}
	if policy != StartupPwdNameFromTheEnvironmentOrHome {
		return false
	}
	named, ok := r.workingDirectoryUnderHome()
	if !ok {
		return false
	}
	r.Dir = named
	r.setVarQuietly("PWD", named)
	return true
}

// namesWorkingDirectory reports whether an absolute path is another spelling
// of the directory the shell is in.
//
// By what the kernel says rather than by text, since the whole point of an
// inherited name is that it is spelled differently — and through the gate,
// because this is a question about the filesystem asked on the caller's
// behalf.
func (r *Runner) namesWorkingDirectory(path string) bool {
	if !filepath.IsAbs(path) {
		return false
	}
	here, err := r.stat(r.workDir())
	if err != nil {
		return false
	}
	there, err := r.stat(path)
	return err == nil && os.SameFile(here, there)
}

// workingDirectoryUnderHome is the shell's directory named through `$HOME`,
// and whether it sits there at all.
//
// The home name is used as written and the path below it is appended as text,
// which is what keeps a dot-dot in `$HOME` in the answer: joining the two
// would clean it away and give a third spelling neither shell has.
func (r *Runner) workingDirectoryUnderHome() (string, bool) {
	home, ok := r.getVar("HOME")
	if !ok || !filepath.IsAbs(home) {
		return "", false
	}
	homeHere, err := r.physicalPath(home)
	if err != nil {
		return "", false
	}
	here, err := r.physicalPath(r.workDir())
	if err != nil {
		return "", false
	}
	rel, err := filepath.Rel(homeHere, here)
	if err != nil || rel == ".." || strings.HasPrefix(rel, "../") {
		return "", false
	}
	if rel == "." {
		return home, true
	}
	return strings.TrimSuffix(home, "/") + "/" + rel, true
}
