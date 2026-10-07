// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/blairham/sh/interp"
)

// The completion dump: what `compinit` read from `$fpath`, kept in a file of
// this shell's own so the next start need not read it again (#6307).
//
// # Why there is one, and why it is not zsh's
//
// `compinit` reads the first line of every `_name` file on `$fpath` — about
// 1,250 files on a machine with Homebrew's zsh library and a plugin manager's
// completions — and that read cost about 0.10 s of every interactive start.
// zsh keeps the same tables in `${ZDOTDIR:-$HOME}/.zcompdump` and reads that
// instead. This shell must never read or write that file: the two shells
// would each take the other's for their own, and a real zsh reading a file
// this one wrote would be trusting a format it never agreed to. So the dump
// is ours, in our own format, under `$XDG_CACHE_HOME/sh/` (or `~/.cache/sh/`),
// and `compinit -d` keeps naming zsh's file and nothing else.
//
// # What makes it safe to read back
//
// A dump that is out of date must never drop a completion or keep one that
// has gone. What compinit's scan reads is, for each directory in order, the
// `_name` files in it that are regular files after links, and the first line
// of each — the rest of a file is read only when its function is first
// called, by its path. So the dump is read back only when all of that is as
// it was:
//
//   - this build, by module version, VCS revision and the executable's size
//     and modification time, and the format the shipped compinit names — a
//     scan's result depends on the interpreter as well as on the files;
//   - the directories, in order, as written and as resolved against the
//     shell's directory;
//   - in each, the same `_` names, each a regular file or not as before;
//   - and for each regular file, either the same size, modification time,
//     change time, inode and device, or — where any of those moved — the same
//     first line, read again and compared by its SHA-256.
//
// The last clause is not a nicety. A plugin manager's framework rewrites some
// completion files on every start: oh-my-zsh's kubectl plugin renames a fresh
// `_kubectl` into its cache directory each time it loads, the same bytes under
// a new inode. Keyed on the stat alone, the dump would never be read on a
// configuration that loads it, which is the configuration it was built for.
// A file whose stat is unchanged is not opened; one whose stat moved costs an
// open and a short read; one whose first line moved means a scan.
//
// # What makes it safe to write
//
// The stats are taken before the scan and again when the dump is written, and
// it is written only when the two agree, so a file edited while compinit read
// it is never recorded as what it read. **A file changed in the last two
// seconds means no dump is written**: on a filesystem whose times are whole
// seconds, a file edited twice inside one second could otherwise keep the stat
// the first edit gave it — the race git calls a racy timestamp. That start's
// scan stands, and the next start writes the dump.
//
// The file is written to a temporary name of this process's own in the same
// directory and renamed over the old one, so a shell starting while another
// writes reads the old dump or the new one and never half of one; two shells
// writing at once each rename a whole file and the last one stays. It ends in
// a SHA-256 of everything before it, so a truncated or damaged dump fails the
// check and the start falls back to the scan, which then writes a good one.
//
// # How compinit reaches it
//
// Through a prelude-private function in front of a prelude-only command —
// see interp/preludecommand.go for why that is not a builtin. Three
// subcommands; NAME is an array compinit declared, DIR a directory it scans:
//
//	load FORMAT NAME... -- DIR...      0 and the arrays set, where the dump holds
//	key  VAR FORMAT DIR...             VAR gets the stat snapshot, or empty
//	                                   where a file is too recent to record
//	save KEY FORMAT NAME... -- DIR...  the arrays written, where the snapshot
//	                                   still matches KEY
//
// Nothing here says anything on standard error: a dump that cannot be read
// or written is a slower start, not a failure of `compinit`.
const compdumpCommand = "compdumpengine"

// compdumpPrelude is the function the shipped compinit calls. Private by its
// prefix, so `whence`, `functions` and `$functions` know nothing of it.
const compdumpPrelude = `
__compinit_dump() {
	` + compdumpCommand + ` "$@"
}
`

// compdumpMagic opens every dump, with the version of this file's layout.
const compdumpMagic = "sh-compdump 2\n"

// compdumpRacyWindow is how recent a change has to be for the snapshot to be
// refused. Two seconds covers a filesystem with one-second times. A variable
// for the tests, whose fixtures are always seconds old.
var compdumpRacyWindow = 2 * time.Second

// compdumpLineCap bounds how much of a file is read for its first line. A
// first line longer than this is recorded as unreadable, so a change to that
// file's stat always means a scan.
const compdumpLineCap = 64 << 10

// The sections a dump holds before compinit's own arrays.
const (
	compdumpMeta  = ".meta"
	compdumpDirs  = ".dirs"
	compdumpFiles = ".files"
)

func registerCompdump(r *interp.Runner) {
	r.RegisterPreludeCommand(compdumpCommand, compdumpBuiltin)
}

func compdumpBuiltin(r *interp.Runner, ctx context.Context, args []string) int {
	if len(args) == 0 {
		return 2
	}
	switch args[0] {
	case "key":
		if len(args) < 3 {
			return 2
		}
		files, racy := compdumpWalk(r, ctx, args[3:], false, time.Now())
		key := ""
		if !racy {
			key = compdumpKey(args[2], args[3:], files)
		}
		r.SetVar(args[1], key)
		return 0
	case "load":
		if len(args) < 2 {
			return 2
		}
		names, dirs := compdumpSplit(args[2:])
		if compdumpLoad(r, ctx, args[1], names, dirs) {
			return 0
		}
		return 1
	case "save":
		if len(args) < 3 {
			return 2
		}
		names, dirs := compdumpSplit(args[3:])
		if compdumpSave(r, ctx, args[1], args[2], names, dirs) {
			return 0
		}
		return 1
	}
	return 2
}

// compdumpSplit parts NAME... -- DIR... at the first `--`.
func compdumpSplit(args []string) (names, dirs []string) {
	if i := slices.Index(args, "--"); i >= 0 {
		return args[:i], args[i+1:]
	}
	return args, nil
}

// compdumpEntry is one `_name` in one scanned directory, as the walk found it.
type compdumpEntry struct {
	dir  int
	name string
	// sig is the stat of a regular file, or a one-letter marker for what the
	// scan does not read: `!` a directory it could not list, `?` a file the
	// gate hid, `-` a name that is not there after links, `n` one that is not
	// a regular file.
	sig string
	// path is where the file is, for reading its first line.
	path string
	// line is the SHA-256 of the first line, or `!` where it could not be
	// read whole; empty until asked for.
	line string
}

func (e compdumpEntry) regular() bool { return len(e.sig) > 1 }

// compdumpWalk lists every scanned directory and stats every `_name` in it,
// reading first lines too where asked. racy is whether any regular file
// changed within the racy window of now.
func compdumpWalk(r *interp.Runner, ctx context.Context, dirs []string, lines bool, now time.Time) (files []compdumpEntry, racy bool) {
	recent := now.Add(-compdumpRacyWindow).UnixNano()
	for i, dir := range dirs {
		// `$dir/_*` is what compinit globs, so an empty entry is the root
		// and a relative one is the shell's directory's.
		path := shellPath(r, dir+"/")
		if !r.AllowList(ctx, path) {
			files = append(files, compdumpEntry{dir: i, sig: "!"})
			continue
		}
		entries, err := compdumpReadDir(path)
		if err != nil {
			files = append(files, compdumpEntry{dir: i, sig: "!"})
			continue
		}
		for _, d := range entries {
			name := d.Name()
			if !strings.HasPrefix(name, "_") {
				continue
			}
			e := compdumpEntry{dir: i, name: name, path: filepath.Join(path, name)}
			switch fi, err := compdumpStatGated(r, ctx, e.path); {
			case errors.Is(err, errGateRefused):
				e.sig = "?"
			case err != nil:
				e.sig = "-"
			case !fi.Mode().IsRegular():
				e.sig = "n"
			default:
				ctime, ino, dev := compdumpIdentity(fi)
				mtime := fi.ModTime().UnixNano()
				if mtime > recent || ctime > recent {
					racy = true
				}
				e.sig = strconv.FormatInt(fi.Size(), 10) + " " + strconv.FormatInt(mtime, 10) + " " +
					strconv.FormatInt(ctime, 10) + " " + strconv.FormatUint(ino, 10) + " " +
					strconv.FormatUint(dev, 10) + " " + strconv.FormatUint(uint64(fi.Mode()), 8)
				if lines {
					e.line = compdumpFirstLine(r, ctx, e.path)
				}
			}
			files = append(files, e)
		}
	}
	return files, racy
}

func compdumpStatGated(r *interp.Runner, ctx context.Context, path string) (fs.FileInfo, error) {
	if !r.AllowProbe(ctx, path) {
		return nil, errGateRefused
	}
	return compdumpStat(path)
}

// compdumpFirstLine is the SHA-256 of a file's bytes up to its first newline
// or its end, which is what compinit's `read -r` reads, or `!` where it cannot
// be read or the line runs past compdumpLineCap.
func compdumpFirstLine(r *interp.Runner, ctx context.Context, path string) string {
	if !r.AllowReadPath(ctx, path) {
		return "!"
	}
	head, err := compdumpReadHead(path, compdumpLineCap+1)
	if err != nil {
		return "!"
	}
	line, _, found := bytes.Cut(head, []byte{'\n'})
	if !found && len(head) > compdumpLineCap {
		return "!"
	}
	sum := sha256.Sum256(line)
	return hex.EncodeToString(sum[:])
}

// compdumpKey is the stat snapshot: the format, this build, the directories
// and every entry's name and stat.
func compdumpKey(format string, dirs []string, files []compdumpEntry) string {
	h := sha256.New()
	field := func(s string) {
		h.Write([]byte(s))
		h.Write([]byte{0})
	}
	field(format)
	field(compdumpBuildIdentity())
	for _, d := range dirs {
		field(d)
	}
	for _, e := range files {
		field(strconv.Itoa(e.dir))
		field(e.name)
		field(e.sig)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// compdumpBuildIdentity is which build of this shell is running: a dump one
// build wrote is not read by another, since what a scan leaves depends on the
// interpreter as well as on the files.
var compdumpBuildIdentity = sync.OnceValue(func() string {
	var b strings.Builder
	if bi, ok := debug.ReadBuildInfo(); ok {
		b.WriteString(bi.GoVersion + " " + bi.Main.Path + " " + bi.Main.Version + " " + bi.Main.Sum)
		for _, s := range bi.Settings {
			switch s.Key {
			case "vcs.revision", "vcs.time", "vcs.modified":
				b.WriteString(" " + s.Key + "=" + s.Value)
			}
		}
	}
	if exe, err := os.Executable(); err == nil {
		if fi, err := compdumpExecutableStat(exe); err == nil {
			fmt.Fprintf(&b, " %s %d %d", exe, fi.Size(), fi.ModTime().UnixNano())
		}
	}
	return b.String()
})

// compdumpPath is where the dump lives: under `$XDG_CACHE_HOME`, or under
// `~/.cache` where that is unset or not absolute — the XDG base directory
// specification says a relative value is to be ignored. No home, no dump.
func compdumpPath(r *interp.Runner) (string, bool) {
	if x, ok := r.GetVar("XDG_CACHE_HOME"); ok && filepath.IsAbs(x) {
		return filepath.Join(x, "sh", "compdump"), true
	}
	if h, ok := r.GetVar("HOME"); ok && filepath.IsAbs(h) {
		return filepath.Join(h, ".cache", "sh", "compdump"), true
	}
	return "", false
}

// compdumpEncode is the file: the magic line, then each section as its name,
// its length and its elements, each NUL-terminated, and a line holding the
// SHA-256 of everything before it. NUL is the one byte no element can hold —
// they are path names, hashes, and words compinit split on blanks and NUL.
func compdumpEncode(names []string, arrays [][]string) []byte {
	var b bytes.Buffer
	b.WriteString(compdumpMagic)
	for i, name := range names {
		b.WriteString(name)
		b.WriteByte(0)
		b.WriteString(strconv.Itoa(len(arrays[i])))
		b.WriteByte(0)
		for _, v := range arrays[i] {
			b.WriteString(v)
			b.WriteByte(0)
		}
	}
	sum := sha256.Sum256(b.Bytes())
	b.WriteByte('\n')
	b.WriteString(hex.EncodeToString(sum[:]))
	b.WriteByte('\n')
	return b.Bytes()
}

// errCompdumpInvalid is any file that is not a whole dump of this layout.
var errCompdumpInvalid = errors.New("compdump: not a valid dump")

// compdumpDecode checks a file's layout and checksum and answers its
// sections in order.
func compdumpDecode(data []byte) (names []string, arrays [][]string, err error) {
	const trailer = 1 + sha256.Size*2 + 1
	if len(data) < len(compdumpMagic)+trailer || data[len(data)-1] != '\n' || data[len(data)-trailer] != '\n' {
		return nil, nil, errCompdumpInvalid
	}
	body := data[:len(data)-trailer]
	sum := sha256.Sum256(body)
	if hex.EncodeToString(sum[:]) != string(data[len(data)-trailer+1:len(data)-1]) {
		return nil, nil, errCompdumpInvalid
	}
	rest, ok := bytes.CutPrefix(body, []byte(compdumpMagic))
	if !ok {
		return nil, nil, errCompdumpInvalid
	}
	next := func() (string, bool) {
		v, after, ok := bytes.Cut(rest, []byte{0})
		if !ok {
			return "", false
		}
		rest = after
		return string(v), true
	}
	for len(rest) > 0 {
		name, ok := next()
		if !ok {
			return nil, nil, errCompdumpInvalid
		}
		count, ok := next()
		if !ok {
			return nil, nil, errCompdumpInvalid
		}
		n, err := strconv.Atoi(count)
		if err != nil || n < 0 || n > len(rest) {
			return nil, nil, errCompdumpInvalid
		}
		values := make([]string, n)
		for j := range values {
			if values[j], ok = next(); !ok {
				return nil, nil, errCompdumpInvalid
			}
		}
		names = append(names, name)
		arrays = append(arrays, values)
	}
	return names, arrays, nil
}

// compdumpLoad sets the named arrays from the dump, where it holds: written
// by this build in this format over these directories, and every file in them
// as it was then — by its stat, or failing that by its first line. Nothing is
// set unless every check passes.
func compdumpLoad(r *interp.Runner, ctx context.Context, format string, names, dirs []string) bool {
	path, ok := compdumpPath(r)
	if !ok || !r.AllowReadPath(ctx, path) {
		return false
	}
	data, err := compdumpReadFile(path)
	if err != nil {
		return false
	}
	got, arrays, err := compdumpDecode(data)
	if err != nil || !slices.Equal(got, append([]string{compdumpMeta, compdumpDirs, compdumpFiles}, names...)) {
		return false
	}
	if !slices.Equal(arrays[0], []string{compdumpBuildIdentity(), format}) || !slices.Equal(arrays[1], dirs) {
		return false
	}
	stored := arrays[2]
	files, _ := compdumpWalk(r, ctx, dirs, false, time.Now())
	if len(stored) != 4*len(files) {
		return false
	}
	for i, e := range files {
		dir, name, sig, line := stored[4*i], stored[4*i+1], stored[4*i+2], stored[4*i+3]
		if dir != strconv.Itoa(e.dir) || name != e.name {
			return false
		}
		if sig == e.sig {
			continue
		}
		// The stat moved. Only a regular file then and now can still be
		// what the scan read, and only if its first line is the same.
		if !e.regular() || len(sig) <= 1 || line == "!" || compdumpFirstLine(r, ctx, e.path) != line {
			return false
		}
	}
	for i, name := range names {
		r.SetArray(name, arrays[3+i])
	}
	return true
}

// compdumpSave writes the named arrays as what a scan of dirs found, provided
// nothing moved since the snapshot key was taken before the scan.
func compdumpSave(r *interp.Runner, ctx context.Context, key, format string, names, dirs []string) bool {
	path, ok := compdumpPath(r)
	if !ok || key == "" {
		return false
	}
	files, racy := compdumpWalk(r, ctx, dirs, true, time.Now())
	if racy || compdumpKey(format, dirs, files) != key {
		return false
	}
	sections := []string{compdumpMeta, compdumpDirs, compdumpFiles}
	arrays := [][]string{{compdumpBuildIdentity(), format}, dirs, nil}
	for _, e := range files {
		arrays[2] = append(arrays[2], strconv.Itoa(e.dir), e.name, e.sig, e.line)
	}
	for _, name := range names {
		values, ok := r.GetArray(name)
		if !ok {
			return false
		}
		sections = append(sections, name)
		arrays = append(arrays, values)
	}
	if !r.AllowModify(ctx, path) {
		return false
	}
	return compdumpWriteFile(path, compdumpEncode(sections, arrays)) == nil
}

// The filesystem calls, each behind the gate question its caller asked.

// compdumpReadDir lists a directory compinit would scan, after AllowList.
func compdumpReadDir(path string) ([]os.DirEntry, error) { return os.ReadDir(path) }

// compdumpStat is the stat behind the snapshot, after AllowProbe. It follows
// a link, as compinit's `-.` qualifier does.
func compdumpStat(path string) (fs.FileInfo, error) { return os.Stat(path) }

// compdumpReadHead reads at most n bytes from the front of a completion file,
// after AllowReadPath, for its first line.
func compdumpReadHead(path string, n int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var buf []byte
	chunk := make([]byte, 512)
	for len(buf) < n {
		m, err := f.Read(chunk[:min(len(chunk), n-len(buf))])
		buf = append(buf, chunk[:m]...)
		if bytes.IndexByte(chunk[:m], '\n') >= 0 || errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	return buf, nil
}

// compdumpExecutableStat is this binary's own size and time, for the build
// identity — a path the shell chose, not one a script named.
func compdumpExecutableStat(path string) (fs.FileInfo, error) { return os.Stat(path) }

// compdumpReadFile reads the dump, after AllowReadPath.
func compdumpReadFile(path string) ([]byte, error) { return os.ReadFile(path) }

// compdumpWriteFile replaces the dump through a temporary file in the same
// directory and a rename, after AllowModify. The directory is made private to
// the user where it does not exist yet.
func compdumpWriteFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	// A name of this process's own, opened exclusively, rather than
	// os.CreateTemp, which the library may not call (.golangci.yml): two
	// shells starting at once each write their own file and rename it.
	tmp := fmt.Sprintf("%s.%d.%d.tmp", path, os.Getpid(), time.Now().UnixNano())
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, werr := f.Write(data)
	cerr := f.Close()
	if werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = os.Rename(tmp, path)
	}
	if werr != nil {
		_ = os.Remove(tmp)
	}
	return werr
}
