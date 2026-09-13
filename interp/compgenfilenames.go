// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

import (
	"path/filepath"
	"strings"
)

// `compgen -f`, `compgen -d` and the three `-o` names that generate: the half
// of the builtin that answers from the filesystem rather than from what the
// shell knows about itself.
//
// Measured 2026-09-12 against bash 5.3.15 in a directory holding `adir`,
// `bdir`, `.hid` and `sp ace`, the files `afile`, `bfile` and `.hidfile`, and
// the links `alink` -> `afile`, `dlink` -> `adir` and `blink` -> nowhere:
//
//	compgen -f a        alink adir afile
//	compgen -d a        adir
//	compgen -d d        dlink
//	compgen -f b        blink bdir bfile
//	compgen -d b        bdir
//	compgen -f ""       every entry, dotfiles included, `.` and `..` not
//	compgen -f .        . .. .hid .hidfile
//	compgen -f ./a      ./alink ./adir ./afile
//	compgen -f "a*"     nothing, status 1
//
// Four rules come out of that, and each is one this could plausibly have got
// wrong:
//
//   - The word is a *prefix*, not a pattern. `a*` matches nothing rather than
//     three names, so no glob machinery is involved.
//   - It is split at the last `/`, the part in front names the directory to
//     read, and that part is written back onto every answer exactly as the
//     script spelled it. `../a` answers `../alink`, not a cleaned path.
//   - Dotfiles are never hidden — this is not a glob, so there is nothing for
//     the hidden rule to apply to — but `.` and `..` appear only when the
//     part after the last `/` begins with a dot. They are not entries the
//     directory read returns, so they are put in rather than filtered out.
//   - `-d` asks whether the name *resolves* to a directory, so a symlink to
//     one is a directory and a broken link is nothing. `-f` asks nothing and
//     takes the entry as it stands, broken link included.
//
// Order is this shell's own and is stated rather than borrowed: bash answers
// in readdir order, which on the machine this was measured on is a hash order
// that differs between two directories holding the same names, so there is no
// order to copy. Runner.readDir sorts for the same reason the glob machinery
// does, and these read through it, so the answers come out sorted.
//
// Through the gate rather than through `os`, which is what makes a completion
// a probe a policy can see: `compgen -f /etc/` enumerates a directory, and
// that is exactly the act ActionReadDir exists to record.

// compgenFilenames is the names in the directory the word points into that
// carry the word's last component as a prefix. dirsOnly keeps the ones that
// resolve to a directory, which is `-d` and `-A directory`.
func (r *Runner) compgenFilenames(word string, dirsOnly bool) []string {
	prefix, base := "", word
	if at := strings.LastIndex(word, "/"); at >= 0 {
		prefix, base = word[:at+1], word[at+1:]
	}
	// Where to read. An empty prefix is the working directory — the runner's
	// own, never the process's — and a relative one is resolved against it
	// for the same reason. See workDir.
	dir := r.workDir()
	if prefix != "" {
		if filepath.IsAbs(prefix) {
			dir = prefix
		} else {
			dir = filepath.Join(dir, prefix)
		}
	}
	entries, err := r.readDir(dir)
	if err != nil {
		// A directory that is not there generates nothing, which is the same
		// answer a prefix matching nothing gives. The gate's refusal reaches
		// here as the same error a missing directory does, deliberately —
		// see fsgate.go.
		return nil
	}
	names := make([]string, 0, len(entries)+2)
	if strings.HasPrefix(base, ".") {
		// The two the directory read does not return. Only when the word asks
		// for them: `compgen -f ""` lists `.hidfile` and not `.`, so this is
		// about what was typed rather than about hidden names.
		names = append(names, ".", "..")
	}
	for _, e := range entries {
		names = append(names, e.Name())
	}
	out := make([]string, 0, len(names))
	for _, name := range names {
		if !strings.HasPrefix(name, base) {
			continue
		}
		if dirsOnly && !r.compgenIsDir(dir, name) {
			continue
		}
		out = append(out, prefix+name)
	}
	return out
}

// compgenIsDir reports whether the entry resolves to a directory, following a
// link the way `-d` does. A stat rather than the DirEntry's own type, which is
// the opposite of what the glob machinery wants and is measured: `dlink` is a
// link to `adir` and `compgen -d d` answers `dlink`, while `blink` points at
// nothing and `compgen -d b` does not answer it.
func (r *Runner) compgenIsDir(dir, name string) bool {
	switch name {
	case ".", "..":
		// Both are directories by construction, and neither is worth a stat
		// of a path that would have to be built for it.
		return true
	}
	info, err := r.stat(filepath.Join(dir, name))
	return err == nil && info.IsDir()
}
