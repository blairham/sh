// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repostatus

import (
	"os"
	"path/filepath"
	"strings"
)

// Finding the repository and reading what it says about itself.
//
// Every fact here comes from a file whose *format* is documented, read with
// the standard library and nothing else: the head is one line, and an
// operation in progress is a name existing. Nothing is parsed that a
// document does not describe, and nothing is inferred from how any
// implementation happens to store it.

// discover walks up from dir looking for the marker that says "a repository
// starts here", and answers the working tree's root and the directory
// holding the repository's own files.
//
// Upward and not downward, which is the only direction that terminates, and
// stopping at the filesystem root. A handful of stats on a deep path and one
// on a shallow one, which is why it is affordable per prompt.
func discover(dir string) (root, git string, ok bool) {
	if dir == "" {
		return "", "", false
	}
	at := filepath.Clean(dir)
	for {
		marker := filepath.Join(at, ".git")
		if info, err := os.Stat(marker); err == nil {
			if info.IsDir() {
				return at, marker, true
			}
			// A file rather than a directory, which is how a linked working
			// tree and a submodule say where their repository actually is:
			// one line, `gitdir: ` and a path, relative to the tree holding
			// the file. Documented, and the reason this is not simply a
			// directory test.
			if elsewhere, ok := gitdirFile(at, marker); ok {
				return at, elsewhere, true
			}
			return at, marker, true
		}
		parent := filepath.Dir(at)
		if parent == at {
			return "", "", false
		}
		at = parent
	}
}

// gitdirFile reads the one-line redirection a `.git` file holds.
func gitdirFile(root, path string) (string, bool) {
	text, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	line, ok := strings.CutPrefix(strings.TrimSpace(string(text)), "gitdir:")
	if !ok {
		return "", false
	}
	where := strings.TrimSpace(line)
	if where == "" {
		return "", false
	}
	if !filepath.IsAbs(where) {
		where = filepath.Join(root, where)
	}
	return filepath.Clean(where), true
}

// read is the whole of what this package computes, and it is two file reads
// and a few stats.
func read(root, git string) Status {
	status := Status{Root: root, Git: git}
	head, err := os.ReadFile(filepath.Join(git, "HEAD"))
	if err != nil {
		// A repository whose head cannot be read is one this cannot describe.
		// Naming the root and nothing else is the honest answer: the segment
		// draws what there is, which is no branch.
		return status
	}
	status.Branch, status.Commit = headOf(strings.TrimSpace(string(head)))
	status.Operation = operation(git)
	// A rebase knows which branch it is putting back, and a prompt that said
	// "detached" through one would be telling a person they had lost their
	// place. The file is one line holding the same `refs/heads/…` a head
	// holds.
	if status.Branch == "" && status.Operation == "rebase" {
		if name, ok := rebaseBranch(git); ok {
			status.Branch = name
		}
	}
	return status
}

// headOf reads a head's one line: either a symbolic reference to a branch, or
// the commit a detached head points at.
func headOf(line string) (branch, commit string) {
	if ref, ok := strings.CutPrefix(line, "ref:"); ok {
		name := strings.TrimSpace(ref)
		// The last component, because that is the branch's name and the rest
		// is where branches are kept.
		return strings.TrimPrefix(name, "refs/heads/"), ""
	}
	if !hex(line) {
		return "", ""
	}
	// Short enough to read and long enough to be the one meant. Seven is what
	// every tool that abbreviates one starts at.
	if len(line) > 7 {
		return "", line[:7]
	}
	return "", line
}

// hex reports whether a line is nothing but hexadecimal digits, which is what
// a detached head holds and what tells it apart from anything else a file
// might have in it.
func hex(s string) bool {
	if s == "" {
		return false
	}
	for i := range len(s) {
		c := s[i]
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'f', c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}

// operations are the half-finished states a repository says it is in by the
// existence of a name, most specific first.
//
// Order matters where two could be true at once: a cherry-pick inside a
// rebase is a rebase to the person in it, because that is what they will
// finish.
var operations = []struct{ path, name string }{
	{"rebase-merge", "rebase"},
	{"rebase-apply", "rebase"},
	{"MERGE_HEAD", "merge"},
	{"CHERRY_PICK_HEAD", "cherry-pick"},
	{"REVERT_HEAD", "revert"},
	{"BISECT_LOG", "bisect"},
}

// operation names what the repository is in the middle of, or nothing.
func operation(git string) string {
	for _, o := range operations {
		if _, err := os.Lstat(filepath.Join(git, o.path)); err == nil {
			return o.name
		}
	}
	return ""
}

// rebaseBranch is the branch a rebase in progress will put back.
func rebaseBranch(git string) (string, bool) {
	for _, dir := range []string{"rebase-merge", "rebase-apply"} {
		text, err := os.ReadFile(filepath.Join(git, dir, "head-name"))
		if err != nil {
			continue
		}
		name := strings.TrimPrefix(strings.TrimSpace(string(text)), "refs/heads/")
		if name != "" {
			return name, true
		}
	}
	return "", false
}
