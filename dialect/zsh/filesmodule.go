// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/blairham/sh/interp"
)

// The `zsh/files` module: nine file operations as builtins, so a shell can
// still move and remove files when there is no room to start a process.
//
// Measured 2026-09-09 against zsh 5.9.2 with `zsh -f`. The module provides
// each of the nine under two names — `mv` and `zf_mv` — and **only the `zf_`
// half is registered here**, for the reason statmodule.go sets out at length
// for `stat`: this shell's builtins are registered before any script runs, so
// registering `rm` at all is registering it always, and every `rm -v` and
// `mv -n` in every script this shell runs would stop reaching the system's
// command. The manual names the `zf_` spellings as the ones to load on their
// own, and every real caller writes them — `zmodload -F zsh/files b:zf_mv
// b:zf_rm` is a prompt theme's line and `zmodload -F zsh/files b:zf_rm ||
// return` is a plugin manager's.
//
// So the nine plain names are in the feature table as what they are — features
// of the module this shell has not got — and `zmodload -F zsh/files b:rm`
// refuses by that name rather than answering yes and leaving a script with
// whatever `rm` was already on its `$PATH`.
//
// **`-s` is refused by name.** Four of the commands take it, and it is not a
// variation on what they do: it asks that no symbolic link be followed *during
// the descent*, so that a recursive `rm` of a deep tree cannot be walked out of
// its own subtree by a link or by a directory moved under it while it runs.
// That is a promise about how each component is opened, and an implementation
// that only checked the operand would be claiming the guarantee while leaving
// the hole it exists to close. Named as missing rather than accepted, which is
// the same call zmodload.go makes for its own letters: a script can tell a
// shell that lacks one from a typo.

// fileOp is one of the nine, and everything about it that is not its own work:
// the name it is registered under, the option letters it takes, and how many
// operands it needs before it will try.
type fileOp struct {
	name    string
	letters string
	// The context is carried to every operation because each of them asks
	// the gate before it touches anything, and the gate is asked with the
	// context the builtin was invoked under — see filesgate.go.
	run func(r *interp.Runner, ctx context.Context, f fileFlags, args []string) int
}

// fileParanoid is the letter that is refused by name wherever it is valid.
const fileParanoid = 's'

// fileFlags is what the letters asked for, across all nine — a command only
// ever reads the ones its own letter set allows, so an unset field is a letter
// that command does not have rather than one that was not given.
type fileFlags struct {
	dirs        bool // -d: unlink a directory (rm), link one (ln)
	force       bool // -f
	noDeref     bool // -h and -n
	interactive bool // -i
	parents     bool // -p
	recursive   bool // -R and -r
	symbolic    bool // -s, on `ln` only, where it means a symbolic link
	mode        string
	haveMode    bool
}

func registerFilesModule(r *interp.Runner) {
	for _, op := range fileOps() {
		if op.name == "sync" && !fileSyncSupported {
			// A platform whose flush-the-buffers call this package has not
			// been taught. Unregistered rather than registered and refusing,
			// so `zmodload -F zsh/files b:zf_sync` answers honestly.
			continue
		}
		r.Register("zf_"+op.name, fileBuiltin(op))
	}
}

// fileOps is the table, in the order `zmodload -lF zsh/files` lists them.
//
// `ln`'s `-s` is the one `s` here that is not the paranoid letter: on `ln` it
// asks for a symbolic link, which is the whole reason anyone calls it. The two
// meanings share a letter across commands and never within one, so the letter
// set is where they are told apart and fileLetter is where it happens.
func fileOps() []fileOp {
	return []fileOp{
		{name: "chgrp", letters: "hRs", run: fileChgrp},
		{name: "chmod", letters: "Rs", run: fileChmod},
		{name: "chown", letters: "hRs", run: fileChown},
		{name: "ln", letters: "dfhins", run: fileLn},
		{name: "mkdir", letters: "pm", run: fileMkdir},
		{name: "mv", letters: "fi", run: fileMv},
		{name: "rm", letters: "dfiRrs", run: fileRm},
		{name: "rmdir", letters: "", run: fileRmdir},
		{name: "sync", letters: "", run: fileSyncOp},
	}
}

// fileBuiltin wraps one operation in the option reading every one of them
// shares.
func fileBuiltin(op fileOp) interp.Builtin {
	return func(r *interp.Runner, ctx context.Context, args []string) int {
		flags, rest, code := fileOptions(r, op, args)
		if code != 0 {
			return code
		}
		return op.run(r, ctx, flags, rest)
	}
}

// fileOptions reads the leading option words for one command.
func fileOptions(r *interp.Runner, op fileOp, args []string) (flags fileFlags, rest []string, code int) {
	rest = args
	for len(rest) > 0 && strings.HasPrefix(rest[0], "-") && len(rest[0]) > 1 {
		word := rest[0]
		rest = rest[1:]
		if word == "--" {
			break
		}
		for i := 1; i < len(word); i++ {
			letter := word[i]
			if letter == 'm' && strings.ContainsRune(op.letters, 'm') {
				value, ok := fileLetterValue(word, &rest, i)
				if !ok {
					r.Diagnosef("not enough arguments\n")
					return flags, nil, 1
				}
				flags.mode, flags.haveMode = value, true
				break
			}
			if !strings.ContainsRune(op.letters, rune(letter)) {
				r.Diagnosef("bad option: -%c\n", letter)
				return flags, nil, 1
			}
			if letter == fileParanoid && op.name != "ln" {
				r.Diagnosef("-%c is not implemented yet\n", letter)
				return flags, nil, 1
			}
			fileLetter(&flags, letter)
		}
	}
	return flags, rest, 0
}

// fileLetterValue is the argument of `-m`: the rest of the word it is in, or
// the word after it.
func fileLetterValue(word string, rest *[]string, i int) (string, bool) {
	if i+1 < len(word) {
		return word[i+1:], true
	}
	if len(*rest) == 0 {
		return "", false
	}
	value := (*rest)[0]
	*rest = (*rest)[1:]
	return value, true
}

func fileLetter(flags *fileFlags, letter byte) {
	switch letter {
	case 'd':
		flags.dirs = true
	case 'f':
		flags.force = true
	case 'h', 'n':
		// Two letters for one thing, which the manual says outright: "-h and
		// -n options are identical and both exist for compatibility".
		flags.noDeref = true
	case 'i':
		flags.interactive = true
	case 'p':
		flags.parents = true
	case 'R', 'r':
		flags.recursive = true
	case 's':
		flags.symbolic = true
	}
}

// fileChgrp is `chgrp group file ...`, which the manual defines as `chown`
// with a user-spec of `:group` — so it is that, rather than a second
// implementation of the same walk.
func fileChgrp(r *interp.Runner, ctx context.Context, flags fileFlags, args []string) int {
	if len(args) < 2 {
		r.Diagnosef("not enough arguments\n")
		return 1
	}
	return fileChown(r, ctx, flags, append([]string{":" + args[0]}, args[1:]...))
}

// fileChown is `chown user-spec file ...`.
func fileChown(r *interp.Runner, ctx context.Context, flags fileFlags, args []string) int {
	if len(args) < 2 {
		r.Diagnosef("not enough arguments\n")
		return 1
	}
	uid, gid, why := fileOwnerSpec(args[0])
	if why != "" {
		r.Diagnosef("%s\n", why)
		return 1
	}
	status := 0
	for _, path := range args[1:] {
		if !fileWalk(r, ctx, flags, path, func(p string) error {
			if flags.noDeref {
				return os.Lchown(p, uid, gid)
			}
			return os.Chown(p, uid, gid)
		}) {
			status = 1
		}
	}
	return status
}

// fileOwnerSpec reads `user`, `user::`, `user:`, `user:group` and `:group`,
// and answers -1 for the half that is not being changed — which is what the
// system call reads as "leave this one alone".
//
// The separator is `:` when there is one, otherwise `.` when there is one,
// otherwise there is none: the manual states the rule in that order and it
// matters for a user whose name contains a dot. A name is looked up before a
// number, so an all-numeric account name wins over the same digits read as an
// id — also the manual's rule, and the reverse would silently change owner to
// somebody else on a machine that has such an account.
func fileOwnerSpec(spec string) (uid, gid int, why string) {
	uid, gid = -1, -1
	sep := ""
	switch {
	case strings.Contains(spec, ":"):
		sep = ":"
	case strings.Contains(spec, "."):
		sep = "."
	}
	name, group := spec, ""
	if sep != "" {
		name, group, _ = strings.Cut(spec, sep)
	}
	var u *user.User
	if name != "" {
		found, err := fileLookupUser(name)
		if err != nil {
			return -1, -1, name + ": no such user"
		}
		u = found
		uid, _ = strconv.Atoi(found.Uid)
	}
	switch {
	case group != "":
		id, err := fileLookupGroup(group)
		if err != nil {
			return -1, -1, group + ": no such group"
		}
		gid = id
	case sep == ":" && strings.HasSuffix(spec, ":") && u != nil:
		// `user:` with nothing after it is the user's own primary group,
		// where `user::` is the owner alone. The two are told apart by what
		// follows the separator, so `user::` leaves an empty group *and* a
		// trailing colon that this branch has already skipped.
		if !strings.HasSuffix(spec, "::") {
			gid, _ = strconv.Atoi(u.Gid)
		}
	}
	return uid, gid, ""
}

func fileLookupUser(name string) (*user.User, error) {
	if u, err := user.Lookup(name); err == nil {
		return u, nil
	}
	return user.LookupId(name)
}

func fileLookupGroup(name string) (int, error) {
	g, err := user.LookupGroup(name)
	if err != nil {
		if g, err = user.LookupGroupId(name); err != nil {
			return 0, err
		}
	}
	return strconv.Atoi(g.Gid)
}

// fileChmod is `chmod mode file ...`, and the mode is octal and nothing else —
// measured, `zf_chmod u+x f` is “invalid mode `u+x'“ and so is `999`.
func fileChmod(r *interp.Runner, ctx context.Context, flags fileFlags, args []string) int {
	if len(args) < 2 {
		r.Diagnosef("not enough arguments\n")
		return 1
	}
	mode, ok := fileOctalMode(args[0])
	if !ok {
		r.Diagnosef("invalid mode `%s'\n", args[0])
		return 1
	}
	status := 0
	for _, path := range args[1:] {
		if !fileWalk(r, ctx, flags, path, func(p string) error { return os.Chmod(p, mode) }) {
			status = 1
		}
	}
	return status
}

// fileOctalMode reads a mode the one way this command accepts one.
func fileOctalMode(text string) (fs.FileMode, bool) {
	bits, err := strconv.ParseUint(text, 8, 32)
	if err != nil || bits > 0o7777 {
		return 0, false
	}
	mode := fs.FileMode(bits & 0o777)
	if bits&0o4000 != 0 {
		mode |= fs.ModeSetuid
	}
	if bits&0o2000 != 0 {
		mode |= fs.ModeSetgid
	}
	if bits&0o1000 != 0 {
		mode |= fs.ModeSticky
	}
	return mode, true
}

// fileWalk applies one operation to a path, and to everything under it when
// `-R` asked for that.
//
// The directory itself is changed before its contents, which is the order the
// manual states and the order that matters: a `chmod -R 0` that took the
// contents first would still be able to read the directory to find them.
func fileWalk(r *interp.Runner, ctx context.Context, flags fileFlags, path string, apply func(string) error) bool {
	base := shellPath(r, path)
	// Whether the operation follows a link decides what has to be permitted.
	// `-h` makes it an `lchown`, which changes the link itself and therefore
	// asks only about the name; without it the call lands on whatever the
	// link points at, and the object at the end of the chain is what the
	// rule is about — see fileMayModifyTarget.
	allow := func(p string) bool {
		if flags.noDeref {
			return fileMayModify(r, ctx, p)
		}
		return fileMayModifyTarget(r, ctx, p)
	}
	if !flags.recursive {
		if !allow(base) {
			return false
		}
		if err := apply(base); err != nil {
			r.Diagnosef("%s\n", fileReason(path, err))
			return false
		}
		return true
	}
	// The walk is over where the name is and every complaint is about the
	// name as written, so each path the walk hands back is put back under the
	// operand it came from. A recursive failure that named an absolute path
	// the script never wrote would be a diagnostic about a different tree.
	said := func(p string) string {
		rel, err := filepath.Rel(base, p)
		if err != nil || rel == "." {
			return path
		}
		return filepath.Join(path, rel)
	}
	// The descent is this package's own rather than filepath.WalkDir's,
	// because WalkDir reads every directory it passes through with the `os`
	// package and a gate cannot see it do that. A recursive `zf_chmod` over a
	// denied tree would have learned the tree's whole shape even where every
	// change in it was refused, which is the disclosure ActionReadDir exists
	// to cover.
	//
	// Otherwise it is WalkDir's own shape: pre-order, so the directory is
	// changed before its contents as the manual requires, and a link is never
	// descended into, because the entry is what Lstat says it is.
	ok := true
	var walk func(p string)
	walk = func(p string) {
		if !allow(p) {
			ok = false
			return
		}
		if err := apply(p); err != nil {
			r.Diagnosef("%s\n", fileReason(said(p), err))
			ok = false
		}
		info, err := os.Lstat(p)
		if err != nil || !info.IsDir() {
			return
		}
		entries, err := fileReadDir(r, ctx, p)
		if err != nil {
			r.Diagnosef("%s\n", fileReason(said(p), err))
			ok = false
			return
		}
		for _, e := range entries {
			walk(filepath.Join(p, e.Name()))
		}
	}
	if _, err := fileLstat(r, ctx, base); err != nil {
		r.Diagnosef("%s\n", fileReason(path, err))
		return false
	}
	walk(base)
	return ok
}

// fileLn is `ln filename dest` and `ln filename ... dir`.
func fileLn(r *interp.Runner, ctx context.Context, flags fileFlags, args []string) int {
	if len(args) == 0 {
		r.Diagnosef("not enough arguments\n")
		return 1
	}
	sources, dest := fileTargets(args)
	if len(sources) > 1 && !fileIsDir(r, ctx, dest, flags) {
		r.Diagnosef("last of many arguments must be a directory\n")
		return 1
	}
	status := 0
	for _, src := range sources {
		if !flags.symbolic {
			// A hard link needs the source to be there, and the complaint
			// about one that is not names it plainly — measured, `zf_ln
			// nosuch d` is `nosuch: no such file or directory` where the same
			// command's `file exists` puts the name in quotes. A *symbolic*
			// link needs nothing: `zf_ln -s nosuch d` writes a dangling one
			// and is 0, which is what makes a link to a path that will exist
			// later possible at all.
			if _, err := fileLstat(r, ctx, src); err != nil {
				r.Diagnosef("%s\n", fileReason(src, err))
				status = 1
				continue
			}
			// A hard link is the one operation here that carries a file's
			// *contents* somewhere else without opening it, so the source
			// has to be readable — see fileMayRead. A symbolic link carries
			// nothing and needs no such permission: reading through one
			// walks to the object and is checked on what it reached.
			if !fileMayRead(r, ctx, src) {
				status = 1
				continue
			}
		}
		target := fileInDir(r, ctx, dest, src, flags)
		if !fileMayModify(r, ctx, target) {
			status = 1
			continue
		}
		// **A link never replaces by default** — the manual says so and it is
		// measured: a second `zf_ln -s b bl` is `file exists`, with no query
		// even when the name in the way cannot be written to. So `-f` and a
		// `-i` answered yes are the only two things that clear it, and both
		// clear it by unlinking first, because the system call itself will
		// not overwrite.
		if !fileConfirm(r, ctx, "zf_ln", "replace", flags, target, false) {
			continue
		}
		if flags.force || flags.interactive {
			_ = os.Remove(shellPath(r, target))
		}
		var err error
		if flags.symbolic {
			// The *unresolved* source, because that is what goes inside the
			// link: a symbolic link holds the text it was given, and
			// resolving `../b` against the shell's directory here would
			// write an absolute link where the script asked for a relative
			// one — a different object, which moves differently and breaks
			// differently.
			err = os.Symlink(src, shellPath(r, target))
		} else {
			err = os.Link(shellPath(r, src), shellPath(r, target))
		}
		if err != nil {
			r.Diagnosef("`%s': %s\n", src, fileErrText(err))
			status = 1
		}
	}
	return status
}

// fileMv is `mv filename dest` and `mv filename ... dir`.
//
// It renames and never copies, which the manual is explicit about — "this mv
// will not move files across devices" — so a move off the filesystem reports
// what the system call said rather than falling back to a copy that would have
// different failure modes and a different meaning for an interrupted run.
func fileMv(r *interp.Runner, ctx context.Context, flags fileFlags, args []string) int {
	if len(args) < 2 {
		r.Diagnosef("not enough arguments\n")
		return 1
	}
	sources, dest := fileTargets(args)
	if len(sources) > 1 && !fileIsDir(r, ctx, dest, flags) {
		r.Diagnosef("last of many arguments must be a directory\n")
		return 1
	}
	status := 0
	for _, src := range sources {
		if _, err := fileLstat(r, ctx, src); err != nil {
			r.Diagnosef("`%s': %s\n", src, fileErrText(err))
			status = 1
			continue
		}
		target := fileInDir(r, ctx, dest, src, flags)
		// Both ends: a rename takes the source name away and puts the
		// target name there, so it is a change to each of them and a policy
		// permitting only one of the two has not permitted this.
		if !fileMayModify(r, ctx, src) || !fileMayModify(r, ctx, target) {
			status = 1
			continue
		}
		// A rename replaces what is there by itself, so nothing is unlinked
		// first: the question is only whether to go on. Asked about a
		// destination that exists and cannot be written to, which is the
		// manual's default and is what an unguarded `zf_mv` onto a read-only
		// file stops for.
		if !fileConfirm(r, ctx, "zf_mv", "replace", flags, target, true) {
			continue
		}
		if err := os.Rename(shellPath(r, src), shellPath(r, target)); err != nil {
			r.Diagnosef("`%s': %s\n", src, fileErrText(err))
			status = 1
		}
	}
	return status
}

// fileTargets splits `src ... dest` into its two halves. One operand is a
// source with the working directory for a destination, which is what `ln f`
// means.
func fileTargets(args []string) (sources []string, dest string) {
	if len(args) == 1 {
		return args, "."
	}
	return args[:len(args)-1], args[len(args)-1]
}

// fileIsDir reports whether a destination is a directory to put things in.
// With `-h` a symbolic link to one is not, which is what makes `ln -sfh t sl`
// replace the link rather than write inside what it points at.
func fileIsDir(r *interp.Runner, ctx context.Context, path string, flags fileFlags) bool {
	info, err := fileLstat(r, ctx, path)
	if err != nil {
		return false
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		if flags.noDeref {
			return false
		}
		if info, err = fileStat(r, ctx, path); err != nil {
			return false
		}
	}
	return info.IsDir()
}

// fileInDir is where one source lands: the destination itself, or a name of
// the same last component inside it.
func fileInDir(r *interp.Runner, ctx context.Context, dest, src string, flags fileFlags) string {
	if !fileIsDir(r, ctx, dest, flags) {
		return dest
	}
	return filepath.Join(dest, filepath.Base(strings.TrimRight(src, string(filepath.Separator))))
}

// fileMkdir is `mkdir [-p] [-m mode] dir ...`.
func fileMkdir(r *interp.Runner, ctx context.Context, flags fileFlags, args []string) int {
	if len(args) == 0 {
		r.Diagnosef("not enough arguments\n")
		return 1
	}
	mode := fs.FileMode(0o777)
	if flags.haveMode {
		asked, ok := fileOctalMode(flags.mode)
		if !ok {
			r.Diagnosef("invalid mode `%s'\n", flags.mode)
			return 1
		}
		mode = asked
	}
	status := 0
	for _, dir := range args {
		if err := fileMakeDir(r, ctx, shellPath(r, dir), mode, flags); err != nil {
			if errors.Is(err, errFileRefused) {
				// The refusal has already been reported, in the same words
				// every other refused action gets. Saying "cannot make
				// directory" after it would be a second diagnostic about one
				// event, and the first one is the accurate one.
				status = 1
				continue
			}
			r.Diagnosef("cannot make directory `%s': %s\n", dir, fileErrText(err))
			status = 1
		}
	}
	return status
}

// errFileRefused marks a failure the gate has already reported, so a caller
// that would otherwise wrap it in a diagnostic of its own can stay quiet.
var errFileRefused = errors.New("refused")

// fileMakeDir creates one directory, with the parents `-p` asked for.
//
// `-p` forgives a directory that is already there and nothing else: measured,
// `zf_mkdir -p f` on a regular file is still “cannot make directory `f': file
// exists“.
//
// The mode is applied after the creation rather than passed to it, because the
// mode passed to the system call is masked by the umask and the letter is a
// statement about the result — measured, `zf_mkdir -m 700 d` is `drwx------`
// under a umask that would have taken bits off it.
func fileMakeDir(r *interp.Runner, ctx context.Context, dir string, mode fs.FileMode, flags fileFlags) error {
	// Every directory this call would create is asked about, not just the one
	// the script named: `-p` creates the missing ancestors too, and a policy
	// that permits `a/b/c` has not thereby permitted making `a` and `a/b`.
	// Collected deepest-first by walking up while the name is not there, so
	// the set is exactly what MkdirAll would go on to create.
	for _, missing := range fileMissingParents(r, ctx, dir, flags) {
		if !fileMayModify(r, ctx, missing) {
			return errFileRefused
		}
	}
	if flags.parents {
		if info, err := fileStat(r, ctx, dir); err == nil {
			if info.IsDir() {
				return nil
			}
			// The kernel's own errno rather than the standard library's
			// sentinel, because the two are not worded alike: `file exists`
			// is what every other failure here reports and what zsh writes,
			// where fs.ErrExist says `file already exists`.
			return syscall.EEXIST
		}
		if err := os.MkdirAll(dir, 0o777); err != nil {
			return err
		}
	} else if err := os.Mkdir(dir, 0o777); err != nil {
		return err
	}
	return os.Chmod(dir, mode)
}

// fileMissingParents is every directory a `mkdir` would bring into being, the
// named one included.
//
// Without `-p` that is just the name itself, because nothing else will be
// created. With it, the walk goes up while each name is absent and stops at
// the first that is there — which is where MkdirAll would stop too — so the
// answer is the set of names the command is about to add to the filesystem
// and no more.
//
// The existence questions go through the gate like every other probe here: a
// `zf_mkdir -p` over a denied tree would otherwise report, by how far it got,
// which of its ancestors exist.
func fileMissingParents(r *interp.Runner, ctx context.Context, dir string, flags fileFlags) []string {
	if !flags.parents {
		return []string{dir}
	}
	var missing []string
	for at := dir; ; {
		if _, err := fileLstat(r, ctx, at); err == nil {
			break
		}
		missing = append(missing, at)
		parent := filepath.Dir(at)
		if parent == at {
			break
		}
		at = parent
	}
	return missing
}

// fileRm is `rm [-dfiRrs] file ...`.
func fileRm(r *interp.Runner, ctx context.Context, flags fileFlags, args []string) int {
	if len(args) == 0 {
		r.Diagnosef("not enough arguments\n")
		return 1
	}
	status := 0
	for _, path := range args {
		if !fileRemove(r, ctx, flags, path) {
			status = 1
		}
	}
	return status
}

// fileRemove is one operand of `rm`, and the whole of the letter precedence:
// `-d` unlinks whatever it is and takes precedence over `-R` and `-r`, and
// `-f` silences everything and takes precedence over `-i`.
func fileRemove(r *interp.Runner, ctx context.Context, flags fileFlags, path string) bool {
	info, err := fileLstat(r, ctx, path)
	if err != nil {
		if flags.force && errors.Is(err, fs.ErrNotExist) {
			// `-f` "suppresses all error indications", and a name that is not
			// there is the one everybody relies on: `zf_rm -f -- $tmp` is how
			// a prompt theme cleans up after itself whether or not it got as
			// far as writing the file.
			return true
		}
		r.Diagnosef("%s\n", fileReason(path, err))
		return false
	}
	switch {
	case flags.dirs:
		return fileUnlinkOne(r, ctx, flags, path)
	case info.IsDir() && !flags.recursive:
		r.Diagnosef("%s: is a directory\n", path)
		return false
	case info.IsDir():
		return fileRemoveTree(r, ctx, flags, path)
	}
	return fileUnlinkOne(r, ctx, flags, path)
}

// fileRemoveTree empties a directory and then removes it, which is the order
// the manual states: everything below goes before the directory itself does.
func fileRemoveTree(r *interp.Runner, ctx context.Context, flags fileFlags, path string) bool {
	entries, err := fileReadDir(r, ctx, path)
	if err != nil {
		if flags.force {
			return true
		}
		r.Diagnosef("%s\n", fileReason(path, err))
		return false
	}
	ok := true
	for _, e := range entries {
		if !fileRemove(r, ctx, flags, filepath.Join(path, e.Name())) {
			ok = false
		}
	}
	if !fileMayModify(r, ctx, path) {
		return false
	}
	if err := os.Remove(shellPath(r, path)); err != nil {
		if flags.force {
			return ok
		}
		r.Diagnosef("%s\n", fileReason(path, err))
		return false
	}
	return ok
}

// fileUnlinkOne removes one name, asking first where asking is what a real
// shell does.
func fileUnlinkOne(r *interp.Runner, ctx context.Context, flags fileFlags, path string) bool {
	// Asked before the question is put to the person, not after: a refused
	// removal must not first prompt about a file the script may not touch,
	// which would confirm the file is there and name its mode.
	if !fileMayModify(r, ctx, path) {
		return false
	}
	if !fileConfirm(r, ctx, "zf_rm", "remove", flags, path, true) {
		return true
	}
	if err := fileUnlink(shellPath(r, path)); err != nil {
		if flags.force {
			return true
		}
		r.Diagnosef("%s\n", fileReason(path, err))
		return false
	}
	return true
}

// fileRmdir is `rmdir dir ...`, which removes empty directories and nothing
// else — a name that is not a directory is refused rather than unlinked.
func fileRmdir(r *interp.Runner, ctx context.Context, _ fileFlags, args []string) int {
	if len(args) == 0 {
		r.Diagnosef("not enough arguments\n")
		return 1
	}
	status := 0
	for _, dir := range args {
		info, err := fileLstat(r, ctx, dir)
		switch {
		case err != nil:
			r.Diagnosef("cannot remove directory `%s': %s\n", dir, fileErrText(err))
			status = 1
			continue
		case !info.IsDir():
			r.Diagnosef("cannot remove directory `%s': not a directory\n", dir)
			status = 1
			continue
		}
		if !fileMayModify(r, ctx, dir) {
			status = 1
			continue
		}
		if err := os.Remove(shellPath(r, dir)); err != nil {
			r.Diagnosef("cannot remove directory `%s': %s\n", dir, fileErrText(err))
			status = 1
		}
	}
	return status
}

// fileSyncOp is `sync`, which takes nothing at all.
// The context is unused because `sync` names no path: it flushes the
// system's own buffers, which is not an access to anything a rule could be
// written about. Kept in the signature so every one of the nine has the same
// shape and a future operand cannot arrive without one.
func fileSyncOp(r *interp.Runner, _ context.Context, _ fileFlags, args []string) int {
	if len(args) > 0 {
		r.Diagnosef("too many arguments\n")
		return 1
	}
	if err := fileSync(); err != nil {
		r.Diagnosef("%s\n", fileErrText(err))
		return 1
	}
	return 0
}

// fileConfirm asks whether to go ahead with a name that is in the way, and
// reports whether the caller should.
//
// Three answers and one order. `-f` asks nothing and takes precedence over
// everything, which is why every real caller writes it. `-i` asks about every
// name. Without either, a file the shell cannot write to is *still* asked
// about — the manual's default for `rm` and `mv`, and what makes an unguarded
// removal of a read-only file stop and wait — and the question names the mode
// it would be overriding. `ln` has no such default, so it passes false and a
// name in the way is simply refused by the system call.
//
// The question goes to standard error with no newline after it, measured, and
// end of input is a no: a script whose input came from nowhere is not
// answering yes by accident.
func fileConfirm(r *interp.Runner, ctx context.Context, command, verb string, flags fileFlags, path string, unwritable bool) bool {
	at := shellPath(r, path)
	info, err := fileLstat(r, ctx, path)
	switch {
	case err != nil:
		// Nothing is in the way, so there is nothing to ask about — which is
		// why `-i` is silent for a `mv` or `ln` onto a name that does not
		// exist and speaks for one that does.
		return true
	case flags.force:
		return true
	case flags.interactive:
		return fileAsk(r, fmt.Sprintf("%s: %s `%s'? ", command, verb, path))
	case !unwritable || fileWritable(at):
		return true
	}
	return fileAsk(r, fmt.Sprintf("%s: %s `%s', overriding mode %s? ",
		command, verb, path, fileModeDigits(info.Mode())))
}

// fileModeDigits writes a mode the way the query does: octal, with a leading
// zero, four digits wide when the top three bits are set.
func fileModeDigits(mode fs.FileMode) string {
	bits := uint32(mode.Perm())
	if mode&fs.ModeSetuid != 0 {
		bits |= 0o4000
	}
	if mode&fs.ModeSetgid != 0 {
		bits |= 0o2000
	}
	if mode&fs.ModeSticky != 0 {
		bits |= 0o1000
	}
	return fmt.Sprintf("%04o", bits)
}

// fileAsk writes one question and reads the answer, which is yes only when it
// begins with a `y`.
//
// Read a byte at a time rather than through a buffered reader, because the
// shell's standard input goes on being read after this command returns: a
// reader that filled a buffer would swallow the lines after the answer, and a
// script that piped its answers in would find the next `read` looking at
// nothing.
func fileAsk(r *interp.Runner, question string) bool {
	_, _ = fmt.Fprint(r.Err(), question)
	var line []byte
	buf := make([]byte, 1)
	for {
		n, err := r.In().Read(buf)
		if n > 0 && buf[0] != '\n' {
			line = append(line, buf[0])
		}
		if err != nil || (n > 0 && buf[0] == '\n') {
			break
		}
	}
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(string(line))), "y")
}

// fileReason is the plain `<name>: <reason>` wording, which is what `rm`,
// `chmod` and `ln` say about a name they could not act on.
func fileReason(path string, err error) string {
	return path + ": " + fileErrText(err)
}

// fileErrText is the system's own sentence for a failure, without the
// operation and path the standard library wraps round it — those are already
// in the wording each command uses.
func fileErrText(err error) string {
	var perr *fs.PathError
	if errors.As(err, &perr) {
		return perr.Err.Error()
	}
	var lerr *os.LinkError
	if errors.As(err, &lerr) {
		return lerr.Err.Error()
	}
	return err.Error()
}
