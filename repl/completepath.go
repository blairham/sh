// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package repl

import (
	"bufio"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strings"
)

// Completing a path: the directory that is read, the entries that are
// offered, and what is written back into the line.
//
// The two halves are deliberately separate. What is *read* is a real path —
// the tilde resolved, the shell's working directory in front of it — and what
// is *written* keeps the text that was typed, tilde and all, with only the
// last component added. Both shells do that, and it is why `~/Deve` completes
// to `~/Developer/` rather than to somebody's home directory spelled out.

// paths completes a word against the filesystem.
//
// keep, when it is not nil, narrows what is offered — which is how a command
// word with a slash in it is answered with the things that could run rather
// than with everything.
func (s shellCompleter) paths(word string, keep func(dir string, e os.DirEntry) bool) []string {
	if isUserWord(word) {
		return s.users(word[1:])
	}
	prefix := wordPrefix(word)
	base := strings.TrimPrefix(dequote(word), dequote(prefix))
	typed := dequote(prefix)
	dir := s.readable(typed)
	quote := wordQuote(word)
	// Through the boundary, not through os: the directory being listed is one
	// the person at the prompt typed, which is the definition of inside. A
	// refusal comes back as an error and is answered here the way a directory
	// that is not there is answered — no entries, and Tab offers nothing. See
	// boundary.Boundary.ReadDir, which carries the argument.
	entries, err := s.bound.ReadDir(s.context(), dir)
	if err != nil {
		fixed, ok := s.corrected(typed, quote)
		if !ok {
			return nil
		}
		dir, prefix = fixed.dir, fixed.prefix
		if entries, err = s.bound.ReadDir(s.context(), dir); err != nil {
			return nil
		}
	}
	// `~` and `#` mean something only at the very start of an unquoted word.
	atStart := prefix == "" && quote == 0

	var matched []os.DirEntry
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, base) {
			continue
		}
		// A name that was not asked for by its dot is not offered, unless the
		// dialect says otherwise: a bare Tab listing every dotfile is what
		// makes completion unusable in a home directory.
		if strings.HasPrefix(name, ".") && !strings.HasPrefix(base, ".") && !s.hidden {
			continue
		}
		if keep != nil && !keep(dir, e) {
			continue
		}
		matched = append(matched, e)
	}
	// Sorted by the name rather than by the text that will be inserted: a
	// backslash is not part of the name, and sorting by it would file every
	// awkward name together at the top.
	sort.Slice(matched, func(i, j int) bool { return matched[i].Name() < matched[j].Name() })

	out := make([]string, 0, len(matched))
	for _, e := range matched {
		text := prefix + escapeName(e.Name(), quote, atStart)
		if s.isDir(dir, e) {
			text += "/"
		}
		out = append(out, text)
	}
	return out
}

// correctedDir is a directory to read instead of the one that was typed, and
// the text to put in the line in front of what is found there.
type correctedDir struct{ dir, prefix string }

// corrected is the second attempt at a directory portion that would not read:
// bash's `dirspell`, which needs bash's `direxpand` beside it to be visible at
// all.
//
// The pair is measured, and it is the whole reason this is one method rather
// than two. Driven through a pseudo-terminal against bash 5.3.15 on
// 2026-09-08, in a directory holding `documents/target-file.txt`:
//
//	shopt -s …            ls documnets/tar<TAB>
//	(neither)             the bell, and the line as typed
//	dirspell              the bell, and the line as typed
//	direxpand             the bell, and the line as typed
//	dirspell direxpand    /…/documents/target-file.txt
//
// The control is in every one of those sessions — `ls documents/tar<TAB>`
// completes throughout — so what the first three rows show is a correction
// that happened and had nowhere to go. `dirspell` decides which directory is
// *read* and `direxpand` decides whether the directory that was read is what
// gets *written*, and a correction the line may not carry is a correction
// nobody sees. So a shell holding only the first offers nothing, which is what
// the second `return nil` below is.
//
// Two shapes are refused before the corrector is asked, and both are measured
// rather than reasoned. With both names on, bash corrects `documents/../
// documnets/` and `linkdir/../documnets/tar`, and refuses `./documnets/`,
// `././documnets/tar`, `../documnets/tar` and `alpha/./bteta/gam`. So a `.`
// anywhere in the typed directory stops it, and a `..` stops it only where it
// comes first. The test is on the text as *typed*, which is the only place it
// can be: `./documnets` and `documents/../documnets` resolve to the same
// directory and bash answers them differently.
func (s shellCompleter) corrected(typed string, quote rune) (correctedDir, bool) {
	if s.correctDir == nil || !correctableTyped(typed) {
		return correctedDir{}, false
	}
	fixed := s.correctDir(s.readable(typed))
	if fixed == "" || !s.expandsDirectory {
		return correctedDir{}, false
	}
	// Escaped the way a name is, because it is going into the line and a
	// directory with a space or a dollar in it would otherwise arrive as two
	// words or as an expansion. Not atStart: a corrected directory is an
	// absolute path, so it can neither begin with a `~` that means a home nor
	// with a `#` that means a comment.
	return correctedDir{dir: fixed, prefix: escapeName(fixed+"/", quote, false)}, true
}

// correctableTyped reports whether a typed directory is one bash would try to
// correct. See corrected, which carries the measurements.
func correctableTyped(typed string) bool {
	if typed == "" {
		return false
	}
	parts := strings.Split(strings.TrimSuffix(typed, "/"), "/")
	for i, part := range parts {
		if part == "." || (i == 0 && part == "..") {
			return false
		}
	}
	return true
}

// isDir reports whether an entry can be descended into.
//
// A symlink to a directory counts, and the stat that decides it is only paid
// for a symlink. It matters more than it looks: a link to a directory is a
// directory to everything else the shell does, and completing it without the
// slash stops the next Tab at a name that cannot be continued.
func (s shellCompleter) isDir(dir string, e os.DirEntry) bool {
	if e.IsDir() {
		return true
	}
	if e.Type()&os.ModeSymlink == 0 {
		return false
	}
	info, err := os.Stat(filepath.Join(dir, e.Name()))
	return err == nil && info.IsDir()
}

// runnable reports whether an entry could be run, or descended into on the
// way to something that could.
func (s shellCompleter) runnable(dir string, e os.DirEntry) bool {
	if s.isDir(dir, e) {
		return true
	}
	info, err := e.Info()
	return err == nil && info.Mode()&0o111 != 0
}

// readable turns the directory part of a typed word into a path to read.
func (s shellCompleter) readable(dir string) string {
	if dir == "" {
		dir = "."
	}
	return s.resolve(s.expandTilde(dir))
}

// isUserWord reports whether a word names an account rather than a file.
//
// A `~` with no slash after it yet is a user name in both shells — which is
// why zsh offers nothing at all for `~lea` even with a file called
// `~lead.txt` sitting in the directory. The test is on the text as typed, so
// an escaped or quoted tilde is a filename again.
func isUserWord(word string) bool {
	return strings.HasPrefix(word, "~") && !strings.Contains(word, "/")
}

// users completes an account name, with the slash that a home directory gets
// and no space — the same suffix a directory gets, because it is one.
func (s shellCompleter) users(prefix string) []string {
	var out []string
	for name := range s.userHomes() {
		if strings.HasPrefix(name, prefix) {
			out = append(out, "~"+name+"/")
		}
	}
	sort.Strings(out)
	return out
}

// expandTilde turns a leading `~` or `~name` into the directory it stands
// for, and leaves anything it cannot resolve exactly as it was: an
// unresolvable tilde is a directory that will not be read, which is the same
// answer as a directory that does not exist.
func (s shellCompleter) expandTilde(path string) string {
	if !strings.HasPrefix(path, "~") {
		return path
	}
	name, rest := path[1:], ""
	if i := strings.IndexByte(name, '/'); i >= 0 {
		name, rest = name[:i], name[i:]
	}
	if name == "" {
		if s.home == "" {
			return path
		}
		return s.home + rest
	}
	if home, ok := s.userHomes()[name]; ok {
		return home + rest
	}
	// Not in the account file, which on some systems holds only the machine's
	// own accounts. The library knows about the rest.
	if u, err := user.Lookup(name); err == nil {
		return u.HomeDir + rest
	}
	return path
}

// userHomes reads the accounts a `~name` could name.
//
// From the account file rather than from the library, because there is no
// call in the standard library that *enumerates* accounts — only one that
// looks a single name up, which cannot answer a prefix. A file that is not
// there yields no names rather than an error: a machine with no account file
// is one where `~name` completes nothing, not one where Tab is broken.
func (s shellCompleter) userHomes() map[string]string {
	path := s.passwd
	if path == "" {
		path = "/etc/passwd"
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()

	homes := map[string]string{}
	scan := bufio.NewScanner(f)
	for scan.Scan() {
		line := scan.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// name:password:uid:gid:gecos:home:shell — the name and the home are
		// the first and the sixth.
		fields := strings.Split(line, ":")
		if len(fields) < 6 || fields[0] == "" {
			continue
		}
		homes[fields[0]] = fields[5]
	}
	return homes
}
