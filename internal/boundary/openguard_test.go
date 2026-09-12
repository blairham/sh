// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package boundary_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The hole #942 named was not one bad line. It was an API shape: this package
// answered a bool and each caller then opened the file itself, so nine call
// sites each had to remember to be honest and the tenth would be written by
// copying one of them.
//
// The shape is fixed — Boundary makes the descriptor now, so there is nothing
// to ask permission for and then do differently — but that only holds while
// the front end goes through it. A package that reaches for os.Open again has
// the old arrangement back, and it compiles, and every test about the nine
// still passes.
//
// So the rule is enforced over the source. Every direct filesystem call in
// every package that can hand a script a command must be named here with a
// reason, and the reason has to be one the design already accepts: the path is
// fixed and chosen by the shell rather than by the script, the call is behind
// a wrapper that asks the gate first, or the access is the policy apparatus
// itself, which cannot be subject to the policy it is reading.
//
// # What #1808 changed, and why the scope is derived
//
// This used to read "the four packages that hold a Boundary", and that was the
// whole of #1808: a dialect holds no Boundary, so no test read one, and
// `zsh/system` opened any file a script named with a bare os.OpenFile (#1805)
// while `zsh/files` deleted, renamed and chmodded whatever it was pointed at
// (#1819). Neither was a line somebody forgot. Both were outside the set this
// walk enumerated, which is the same shape as #1416 and is worth saying in the
// same words: **a guarantee that is checked rather than enumerated is only as
// wide as the set it walks.**
//
// A widened hand-written list would have been that bug with a later date on
// it, because the next dialect directory is not in a list nobody edits. So the
// scope is *derived* from the tree — see inScope — on three rules that a new
// package cannot be outside of by accident: it is under dialect/, it declares
// or registers a command, or it holds a Boundary. TestTheGuardReadsEveryDialect
// and TestTheGuardReadsEveryPackageThatHoldsABoundary check the derivation
// against the filesystem rather than against a constant, so adding dialect/fish
// puts it in scope on the commit that creates the directory.

// pathCalls are the calls in os and syscall that name a path — the ones that
// cross the boundary, as against the ones that take a descriptor the gate has
// already agreed to. os.File methods and syscall.Fstat are absent for that
// reason and not by omission.
//
// The near neighbors of each verb are here on purpose. A guard that knows
// os.Remove and not os.RemoveAll, or os.Mkdir and not os.MkdirAll, is one
// rename away from quiet — and the .golangci.yml entry above forbidigo says
// why in its own words, having been widened once after os.MkdirTemp("") did
// what os.TempDir had just been forbidden for.
//
// os.Stat and its neighbors are on the list, which they were not. The old
// comment argued a probe is not an open, and that is still true — but it is an
// argument about what the *policy* should allow, not about which code has to
// ask. interp gates probes as ActionStat and has since fsgate.go, #1819 gave a
// dialect AllowProbe for the same reason, and `zstat` answering with the true
// size of a file `[[ -f ]]` had just refused into "not there" is what an
// unasked probe costs. Where the disclosure really is outside the boundary the
// exemption below says so in one line, which is cheaper than the hole.
var pathCalls = map[string]map[string]bool{
	"os": {
		"Open": true, "OpenFile": true, "OpenRoot": true, "Create": true,
		"CreateTemp": true, "ReadFile": true, "WriteFile": true, "ReadDir": true,
		"Remove": true, "RemoveAll": true, "Rename": true,
		"Mkdir": true, "MkdirAll": true, "MkdirTemp": true,
		"Link": true, "Symlink": true, "Readlink": true, "Truncate": true,
		"Chmod": true, "Chown": true, "Lchown": true, "Chtimes": true,
		"Stat": true, "Lstat": true,
	},
	"syscall": {
		"Open": true, "Openat": true, "Creat": true,
		"Unlink": true, "Unlinkat": true, "Rmdir": true,
		"Rename": true, "Renameat": true,
		"Mkdir": true, "Mkdirat": true, "Mkfifo": true, "Mknod": true,
		"Link": true, "Linkat": true, "Symlink": true, "Symlinkat": true,
		"Readlink": true, "Readlinkat": true, "Truncate": true,
		"Chmod": true, "Fchmodat": true, "Chown": true, "Lchown": true, "Fchownat": true,
		"Stat": true, "Lstat": true, "Fstatat": true,
		"Access": true, "Faccessat": true, "Utimes": true, "UtimesNano": true,
		"Chflags": true, "Lchflags": true, "Statfs": true, "Mount": true,
	},
}

// exempt is every direct filesystem call the shell still makes, and why.
//
// An entry is a package directory, a function, and the reason the access is
// outside the boundary. Adding one is the deliberate act the list exists to
// force: a call in one of these packages that is not here fails this test.
//
// Three reasons recur, and each is one the design document already gives:
//
//   - the shell chose the path, not the script — a fixed name, a temporary
//     this shell made, or a descriptor it was handed;
//   - the call is the gated wrapper itself, or sits immediately behind one
//     that asked about this very path;
//   - the access is the apparatus — the gate, or the audit log — which cannot
//     be subject to the policy it enforces.
var exempt = map[string]string{
	"driver.controllingTerminal": "/dev/tty, opened to name a terminal in an ioctl rather " +
		"than to read anything. A fixed path the front end chose, and gating it would stop " +
		"^C and ^Z reaching commands while protecting no file. A redirection a script writes " +
		"to /dev/tty is an ordinary open and is gated.",
	"driver.readFdDir": "/dev/fd, listed to learn which descriptors this process was started " +
		"holding. The capabilities are in the table before a line is read, so there is nothing " +
		"a refusal could prevent — which is the argument ActionInherit already makes.",
	"driver.holdLowDescriptors": "os.DevNull, opened once and duplicated to occupy the low " +
		"descriptor numbers before main. A fixed path, nothing is read from it, and it happens " +
		"before there is a script — or a policy — to ask about.",
	"repl.lookupTerminal": "/dev, listed to name this session's terminal. A fixed path, and " +
		"no content is read.",
	"repl.readTerminalDescription": "the terminal's compiled description, read out of a " +
		"terminfo database to answer `$terminfo` and `$termcap` (#2076). The path is " +
		"`<database>/<x>/<name>`: the name is `$TERM` with terminalNameIsOneComponent " +
		"having refused anything that is not a single component — no separator, no `.` or " +
		"`..`, nothing beginning with a dot — so a script setting `$TERM` selects a file " +
		"inside a database directory and cannot leave one. The directories are the terminfo " +
		"environment plus a fixed system list, which is the argument interp.CommandsOnPath " +
		"already makes about `$PATH`: the shell chose them, and this is the lookup every " +
		"curses program on the machine makes before this shell starts. A file that is not a " +
		"compiled description reads as no description at all, so nothing that is not a " +
		"terminal's capability table can reach a script through it, and a refusal here would " +
		"hide `cuu1` from a prompt while telling it nothing — which is the silent " +
		"substitution #2076 is about.",
	"repl.userHomes": "the account file, read to answer `~name` completion. A fixed path the " +
		"front end chose — the person types a prefix, never the path — and the standard " +
		"library has no call that enumerates accounts. It is the last of the three #951 " +
		"named and the one #951 did not close: gating this read would hide the prefix " +
		"listing while leaving `~name` itself resolving, because expandTilde falls back to " +
		"user.Lookup for a name the file does not hold and that is a library call no gate is " +
		"on. Closing it means gating account lookup as a whole, which is a different " +
		"question from a directory listing.",
	"repl.isDir": "one bit about an entry a listing the gate already allowed has just " +
		"named — whether a symbolic link among the completions points at a directory, which " +
		"is what decides the trailing slash. No content is read and no name is disclosed that " +
		"the allowed ReadDir did not already return. It is a probe, and Boundary has no probe " +
		"seam to route it through the way interp's AllowProbe routes a dialect's; that " +
		"asymmetry is #1824 rather than this line.",
	"repl.trim": "the history file's rewrite, on the path the append already passed the gate " +
		"on, through a temporary in the same directory. The shell's own scaffolding, which " +
		"ActionOpen's rule places outside the boundary.",
	"internal/acp.newSession": "os.DevNull, so an agent this client starts does not inherit a terminal.",
	"driver.OpenAudit": "the audit stream's own file. The apparatus is outside the boundary it " +
		"enforces — a policy that could hide its own log would be a policy nobody could check, " +
		"which docs/design/sandboxing.md states under `The apparatus is outside the boundary`.",

	// interp. The gate lives here, so most of these are the apparatus: the
	// wrapper that consults before it calls is not a way around the boundary,
	// it is the boundary.
	"interp.stat": "fsgate.go's own os.Stat, on both sides of the ActionStat consultation. " +
		"This is the gate rather than a caller of it.",
	"interp.statEntering": "fsgate.go's own os.Stat again, asked as a chdir asks it — of the " +
		"directory's own entry rather than of the directory, which is what makes the execute " +
		"bit decide. Behind the same ActionStat consultation as stat, and that consultation " +
		"names the *directory*, so a rule written against the name a script used still " +
		"matches (#1492).",
	"interp.lstat":    "fsgate.go's own os.Lstat, behind the same ActionStat consultation as stat.",
	"interp.readLink": "fsgate.go's own os.Readlink, behind the same ActionStat consultation as stat.",
	"interp.readDir": "fsgate.go's own listing, behind ActionReadDir. The gated path opens the " +
		"directory through internal/opened and takes the entries from that descriptor; the " +
		"os.ReadDir calls are the no-gate and watch-only forms.",
	"interp.openGatedFile": "verifyopen.go: the open the gate has just agreed to, verified against " +
		"the object it reached before the descriptor is returned. The seam itself.",
	"interp.readFileGatedBytes": "verifyopen.go: the same, for the read-whole-file form. The\n\touter openGated and readFileGated are the errno-recording wrappers around\n\tthese two and reach nothing themselves.",
	"interp.settleBackgroundJobBeforeABlockingOpen": "a stat of the path a redirection is about " +
		"to open, to learn whether the open can block. The open itself goes through the gate a " +
		"moment later and is refused there; this answers a question about waiting, and a " +
		"refusal here would change nothing except which of the two says no.",
	"interp.CommandsOnPath": "the PATH directories, listed to answer a view of what is " +
		"runnable. The shell chose the paths — they are $PATH — and this is the command search " +
		"every execution already makes, asked once instead of per word.",
	"interp.openFifoWriteEnd": "a named pipe this shell made for a process substitution, in a " +
		"directory this shell made. The script named the command, never the path.",
	"interp.openFifoReadEnd": "the other end of the same pipe. See openFifoWriteEnd.",
	"interp.nudgeFifoEOF":    "the same pipe again, opened to give a waiting reader end-of-file.",
	"interp.mkfifo":          "the named pipe this shell makes for a substitution, in its own directory.",
	"interp.procSub":         "the same pipe, removed when the substitution that made it is done.",
	"interp.newSubstFile": "the regular file `=(cmd)` writes instead of a pipe, made in the " +
		"same directory as the pipes, numbered by the same counter and removed by the same " +
		"removeProcSubs. The script named the command, never the path — see Runner.ownPipe, " +
		"which holds the argument for both spellings.",
	"interp.procSubDir":     "the directory this shell makes for its own pipes, under r.tempHome().",
	"interp.removeProcSubs": "the same pipes, removed with the command that named them.",
	"interp.CleanUp":        "the same directory, removed when this shell stops being one (#1284).",

	// dialect/zsh. filesgate.go is the module's gate — every call in it is
	// behind the consultation above it — and the rest are the mutating system
	// calls, each one behind a fileMayModify about that exact path (#1819).
	"dialect/zsh.fileMayModifyTarget": "the link chain chmod and chown follow, walked one hop " +
		"at a time with AllowModify asked about every hop before the Lstat and Readlink that " +
		"find the next one.",
	"dialect/zsh.fileWriteWhole": "filesgate.go's gated whole-file write, which asks AllowModify " +
		"about the name first. The removal in front of the write is what lets a " +
		"second `zcompile` replace a product the first one made read-only.",
	"dialect/zsh.fileLstat": "filesgate.go's gated Lstat: AllowProbe first, and a refusal reads as a path that is not there.",
	"dialect/zsh.fileStat":  "filesgate.go's gated Stat, behind the same AllowProbe.",
	"dialect/zsh.fileReadDir": "filesgate.go's gated listing, behind AllowList — the action a " +
		"recursive `zf_rm` would otherwise learn a denied tree's shape from.",
	"dialect/zsh.fileChown": "chown of a path fileWalk has just asked AllowModify about, " +
		"following the link chain unless -h said otherwise.",
	"dialect/zsh.fileChmod": "chmod of a path fileWalk has just asked AllowModify about, on the " +
		"same terms as fileChown.",
	"dialect/zsh.fileWalk": "the recursive descent's own Lstat, on a path the allow callback " +
		"above it has already passed. The descent is this package's rather than WalkDir's " +
		"precisely so that every directory it lists goes through fileReadDir.",
	"dialect/zsh.fileLn": "the unlink `-f` does before a link, and the link itself, both after " +
		"fileMayModify on the target and fileMayRead on the source — a hard link puts the " +
		"contents inside the new name's directory, so that is where the read is asked about.",
	"dialect/zsh.fileMv":      "the rename, after fileMayModify on both the source and the target.",
	"dialect/zsh.fileMakeDir": "the directory creation, after fileMayModify on it and on every parent `-p` would make.",
	"dialect/zsh.fileRemoveTree": "the unlink at the end of a recursive removal, after " +
		"fileMayModify on the path and fileReadDir on the directory it came from.",
	"dialect/zsh.fileRmdir": "the directory removal, after fileMayModify on it.",
	"dialect/zsh.fileWritable": "the access check `-i` and `zf_mv`'s default ask about, on a " +
		"path fileConfirm has just passed through fileLstat's AllowProbe and the caller through " +
		"fileMayModify. Two spellings, one per platform.",
	"dialect/zsh.fileUnlink": "the unlink itself, after fileMayModify on the path. Two spellings, one per platform.",
	"dialect/zsh.statPath": "the stat behind `zstat`, which statmodule.go asks AllowProbe about " +
		"before calling — a refusal is the kernel's own ENOENT, so the answer for a hidden path " +
		"and a missing one is the same sentence (#1819).",
	"dialect/zsh.sysOpenFile": "the open behind `sysopen`, which systemio.go asks AllowOpen " +
		"about before and VerifyOpened about after. The open is this package's own only because " +
		"`-o nonblock` needs a flag os.OpenFile cannot carry and has to be taken off again (#1805).",
	"dialect/zsh.systemLockOpen": "the open behind `zsystem flock`, between the same AllowOpen " +
		"and VerifyOpened pair (#1805).",
	// dialect/zsh. mapfile.go is the whole of `zsh/mapfile`'s gate, and the
	// module is the one that reaches the filesystem without naming a command:
	// a read is an expansion and a write is an assignment, so these four are
	// the only places it touches a path at all (#2260).
	"dialect/zsh.mapfileRead": "the read behind `${mapfile[p]}`, after AllowReadPath on the " +
		"path the subscript named. A refusal reads back the same as a denied path holding " +
		"nothing, so the wording cannot be an oracle for what the policy hides.",
	"dialect/zsh.mapfileWrite": "the write behind `mapfile[p]=v`, after AllowModify on the name. " +
		"0666 before the umask, which is the mode zsh leaves behind.",
	"dialect/zsh.mapfileRemove": "the unlink behind `unset \"mapfile[p]\"`, after AllowModify on " +
		"the name — an unlink changes what is at a name, so it asks the write question and not " +
		"the read one.",
	"dialect/zsh.mapfileNames": "the listing behind `${(k)mapfile}`, after AllowList on the " +
		"shell's own directory. It is the route with no path in it to hang a check on, which is " +
		"why the roster has a sandboxcheck row of its own.",
	// dialect/bash. history.go is the whole of the `history` builtin's gate.
	// bash keeps a history list and writes a history file with no terminal
	// anywhere, so `-w`, `-a`, `-r` and `-n` are four letters that reach a
	// path a script named in an ordinary script (#2271).
	"dialect/bash.historyWriteFile": "the history file `-w` and `-a` put down, after AllowModify " +
		"on the path — which is the operand, or $HISTFILE when there is none. 0600 rather than " +
		"the umask's answer, which is the mode bash leaves behind: a history file holds what " +
		"somebody typed.",
	"dialect/bash.historyReadFile": "the history file `-r` and `-n` take entries from, after " +
		"AllowReadPath on the path. Reading it is an open, so a refusal is reported the way a " +
		"redirection's is rather than read back as an empty list.",
	// The smoke suite is in scope because it holds a Boundary to read a block
	// store back — and it is the one package here that is not the shell. The
	// rest of this list explains a path the shell reaches; these two explain
	// why an instrument is being asked at all.
	"internal/smoke.home": "the scratch home the suite builds for one session: an rc file, a " +
		"few files to complete against, and the directories the autocd and cdspell rows move " +
		"into. Every path is one the suite composed under a temporary root it was given, and no " +
		"script has run yet — there is nothing for a policy to be about. It is in scope only " +
		"because reading a block store needs a Boundary value.",
	"internal/smoke.Run": "the temporary root the whole run is given, made once before any " +
		"shell starts. The same argument as home, one level up.",
}

// TestEveryCommandPackageOpenGoesThroughTheBoundaryOrSaysWhyNot.
func TestEveryCommandPackageOpenGoesThroughTheBoundaryOrSaysWhyNot(t *testing.T) {
	t.Parallel()
	var found []string
	for _, pkg := range inScope(t) {
		for _, call := range directCalls(t, pkg) {
			found = append(found, call)
			if _, ok := exempt[call]; !ok {
				t.Errorf("%s reaches the filesystem through the os or syscall package and "+
					"nothing says why.\n"+
					"\t%s is read by this guard because it %s.\n"+
					"\tA path a script named goes through the gate — Boundary.OpenFile, "+
					".ReadFile, .WriteFile or .ReadDir out here, and\n"+
					"\tRunner.AllowOpen, .AllowModify, .AllowReadPath, .AllowProbe or "+
					".AllowList inside a builtin — or add %s\n"+
					"\tto exempt in this file with the reason it is outside the boundary.",
					call, pkg.dir, pkg.why, call)
			}
		}
	}
	// A walk that found nothing passes for the wrong reason, which is the
	// failure mode of every assertion made over a traversal.
	if len(found) < len(exempt) {
		t.Fatalf("the walk found %d direct calls and the exemption list names %d; "+
			"the detector is not reading these packages", len(found), len(exempt))
	}
	// And nothing is exempted that no longer exists: a list that outlives its
	// subject is a rule nobody is following.
	sort.Strings(found)
	for call := range exempt {
		if i := sort.SearchStrings(found, call); i == len(found) || found[i] != call {
			t.Errorf("exempt names %s and no such call is there any more", call)
		}
	}
}

// pkg is a package this guard reads, and the rule that put it in scope.
type pkg struct {
	dir  string // repo-relative, which is also the prefix its exemption keys use
	path string // the same directory as this test sees it on disk
	why  string
}

// inScope derives the packages this guard reads from the tree.
//
// Three rules, and a package is in scope if any of them holds:
//
//  1. **It is a dialect.** Everything under dialect/, whether or not it has
//     registered anything yet — `zsh/files` was "safe by accident" for exactly
//     as long as it took #1670 to give it builtins, and a guard that waits for
//     the registration is a guard that arrives after the code it was meant to
//     read. The directory is the trigger.
//  2. **It declares or registers a command.** A function shaped like
//     interp.Builtin, or a call to Runner.Register or .Unregister. This is what
//     puts interp itself in scope — it declares the built-in table — and it is
//     what would catch a plugin host or a future dialect living somewhere other
//     than dialect/.
//  3. **It holds a Boundary.** The original scope, now read off the source
//     instead of written down: the five packages the old list named are exactly
//     the five that name the type.
//
// The rules are read syntactically, so none of them can be true of a package
// and false here. A rule that is wrong in the widening direction costs an
// exemption line; one that is wrong the other way is the bug this exists to
// prevent, which is why nothing here is a list of names.
func inScope(t *testing.T) []pkg {
	t.Helper()
	root := filepath.Join("..", "..")
	var out []pkg
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		name := d.Name()
		if p != root && (strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") ||
			name == "testdata" || name == "vendor") {
			return fs.SkipDir
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			rel = ""
		}
		if why := scopeReason(t, p, rel); why != "" {
			out = append(out, pkg{dir: rel, path: p, why: why})
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the tree: %v", err)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].dir < out[j].dir })
	return out
}

// scopeReason is the three rules, applied to one directory. It returns the
// reason the package is read, or "" when it is not.
func scopeReason(t *testing.T, dir, rel string) string {
	t.Helper()
	files := sourceFiles(t, dir)
	if len(files) == 0 {
		return ""
	}
	if rel == "dialect" || strings.HasPrefix(rel, "dialect/") {
		return "is a dialect, and a dialect registers builtins"
	}
	declares, holds := false, false
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.FuncDecl:
				declares = declares || isBuiltinSignature(n.Type)
			case *ast.FuncLit:
				declares = declares || isBuiltinSignature(n.Type)
			case *ast.CallExpr:
				if sel, ok := n.Fun.(*ast.SelectorExpr); ok &&
					(sel.Sel.Name == "Register" || sel.Sel.Name == "Unregister") {
					declares = true
				}
			case *ast.SelectorExpr:
				if id, ok := n.X.(*ast.Ident); ok && id.Name == "boundary" && n.Sel.Name == "Boundary" {
					holds = true
				}
			}
			return true
		})
	}
	switch {
	case declares:
		return "declares or registers a builtin"
	case holds:
		return "holds a Boundary"
	}
	return ""
}

// isBuiltinSignature reports whether a function is shaped like interp.Builtin:
// a runner, a context, the operands, and an exit status. A package holding one
// of these can hand a script a command whether or not the registration is in
// the same package.
func isBuiltinSignature(ft *ast.FuncType) bool {
	if ft.Params == nil || ft.Results == nil || len(ft.Results.List) != 1 {
		return false
	}
	var params []ast.Expr
	for _, f := range ft.Params.List {
		n := len(f.Names)
		if n == 0 {
			n = 1
		}
		for range n {
			params = append(params, f.Type)
		}
	}
	if len(params) != 3 {
		return false
	}
	star, ok := params[0].(*ast.StarExpr)
	if !ok || !named(star.X, "Runner") {
		return false
	}
	if !named(params[1], "Context") {
		return false
	}
	slice, ok := params[2].(*ast.ArrayType)
	if !ok || slice.Len != nil || !named(slice.Elt, "string") {
		return false
	}
	res := ft.Results.List[0]
	return len(res.Names) == 0 && named(res.Type, "int")
}

// named reports whether an expression is an identifier or a qualified one with
// this name, so that Runner and interp.Runner are one answer.
func named(e ast.Expr, name string) bool {
	switch e := e.(type) {
	case *ast.Ident:
		return e.Name == name
	case *ast.SelectorExpr:
		return e.Sel.Name == name
	}
	return false
}

// sourceFiles parses a package's non-test files.
//
// Files are read and parsed one at a time rather than through parser.ParseDir,
// which does not consider build tags and is deprecated for saying so. A
// `_unix.go` and its `_windows.go` sibling therefore both count — an exemption
// that held only on one platform is exactly the kind that goes unnoticed, and
// `zf_rm` is spelled os.Remove on one and syscall.Unlink on the other.
func sourceFiles(t *testing.T, dir string) []*ast.File {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	fset := token.NewFileSet()
	var out []*ast.File
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		out = append(out, f)
	}
	return out
}

// directCalls names every os or syscall call on a path in a package's non-test
// files, as "package directory.enclosing declaration".
//
// Package-level declarations are read as well as function bodies. An
// initializer is the one place a filesystem call can sit with no function
// around it, and a walk that looked only at bodies would have been a guard
// with a hole in it of exactly the kind this file is about.
func directCalls(t *testing.T, p pkg) []string {
	t.Helper()
	var out []string
	for _, f := range sourceFiles(t, p.path) {
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				if d.Body != nil && callsThePathVerbs(d.Body) {
					out = append(out, fmt.Sprintf("%s.%s", p.dir, d.Name.Name))
				}
			case *ast.GenDecl:
				for _, s := range d.Specs {
					vs, ok := s.(*ast.ValueSpec)
					if !ok || len(vs.Names) == 0 {
						continue
					}
					for _, v := range vs.Values {
						if callsThePathVerbs(v) {
							out = append(out, fmt.Sprintf("%s.%s", p.dir, vs.Names[0].Name))
							break
						}
					}
				}
			}
		}
	}
	return out
}

// callsThePathVerbs reports whether a piece of syntax reaches the filesystem
// through the os or syscall package on a path.
func callsThePathVerbs(n ast.Node) bool {
	found := false
	ast.Inspect(n, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		id, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		if pathCalls[id.Name][sel.Sel.Name] {
			found = true
		}
		return true
	})
	return found
}

// TestTheGuardReadsEveryDialect. The scope is derived so that a dialect added
// tomorrow is read on the commit that creates its directory, and this is the
// assertion that says so: every directory under dialect/ that holds Go is in
// the set, checked against the filesystem rather than against a list.
//
// #1808 is the reason. `dialect/zsh` was outside the old scope for as long as
// the old scope was a constant, and a widened constant would only have moved
// the date.
func TestTheGuardReadsEveryDialect(t *testing.T) {
	t.Parallel()
	read := map[string]bool{}
	for _, p := range inScope(t) {
		read[p.dir] = true
	}
	dirs, err := os.ReadDir(filepath.Join("..", "..", "dialect"))
	if err != nil {
		t.Fatalf("reading dialect/: %v", err)
	}
	seen := 0
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		rel := path.Join("dialect", d.Name())
		if len(sourceFiles(t, filepath.Join("..", "..", rel))) == 0 {
			continue
		}
		seen++
		if !read[rel] {
			t.Errorf("%s holds Go and this guard does not read it", rel)
		}
	}
	if seen < 4 {
		t.Errorf("found %d dialects and there are at least four; the walk is not reading dialect/", seen)
	}
}

// TestTheGuardReadsEveryPackageThatHoldsABoundary. The other half of the
// derivation, checked the same way: the scope must still contain everything
// the rule this test used to be written as a list of would have named.
func TestTheGuardReadsEveryPackageThatHoldsABoundary(t *testing.T) {
	t.Parallel()
	read := map[string]bool{}
	for _, p := range inScope(t) {
		read[p.dir] = true
	}
	// The five the old frontEnd list named, which is what this guard read
	// before #1808 widened it. They are here as a floor rather than as the
	// scope: a derivation that stopped matching would otherwise pass quietly.
	for _, dir := range []string{"driver", "repl", "internal/acp", "internal/blocks", "cmd/sh"} {
		if !read[dir] {
			t.Errorf("%s holds a Boundary and this guard does not read it", dir)
		}
	}
	// And interp, which holds the gate the other seams call and declares the
	// built-in table. It was never in the old list either.
	if !read["interp"] {
		t.Errorf("interp declares the builtins and this guard does not read it")
	}
}

// TestTheOpenGuardSeesACallItWasNotToldAbout. A detector that has gone quiet
// passes over a broken tree and a clean one alike, so it is handed the
// violations it exists to find: an open, a change to the filesystem, the
// syscall spelling of one, and an initializer with no function around it.
func TestTheOpenGuardSeesACallItWasNotToldAbout(t *testing.T) {
	t.Parallel()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "x.go", `package zsh

var atInit = os.ReadDir(dir)

func forgot() ([]byte, error) { return os.ReadFile(path) }

func deleted() error { return os.Remove(path) }

func unlinked() error { return syscall.Unlink(path) }

func remembered() ([]byte, error) { return b.ReadFile(ctx, path) }

func alsoRemembered(f *os.File) (os.FileInfo, error) { return f.Stat() }
`, 0)
	if err != nil {
		t.Fatal(err)
	}
	var reached []string
	for _, d := range f.Decls {
		switch d := d.(type) {
		case *ast.FuncDecl:
			if callsThePathVerbs(d.Body) {
				reached = append(reached, d.Name.Name)
			}
		case *ast.GenDecl:
			for _, s := range d.Specs {
				vs := s.(*ast.ValueSpec)
				for _, v := range vs.Values {
					if callsThePathVerbs(v) {
						reached = append(reached, vs.Names[0].Name)
					}
				}
			}
		}
	}
	want := []string{"atInit", "forgot", "deleted", "unlinked"}
	if strings.Join(reached, ",") != strings.Join(want, ",") {
		t.Errorf("the detector read %v, want %v", reached, want)
	}
}

// TestABuiltinIsRecognizedByItsShape. The scope rests on it, and a signature
// test that quietly matched nothing would put every package holding only
// builtins back outside the walk — which is #1808.
func TestABuiltinIsRecognizedByItsShape(t *testing.T) {
	t.Parallel()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "x.go", `package p

func core(r *Runner, ctx context.Context, args []string) int { return 0 }

func qualified(r *interp.Runner, _ context.Context, args []string) int { return 0 }

func grouped(r *interp.Runner, ctx context.Context, args []string) int { return 0 }

func notOne(r *interp.Runner, args []string) int { return 0 }

func alsoNot(r *interp.Runner, ctx context.Context, args []string) error { return nil }

func norThis(r Runner, ctx context.Context, args []string) int { return 0 }
`, 0)
	if err != nil {
		t.Fatal(err)
	}
	var matched []string
	for _, d := range f.Decls {
		fd := d.(*ast.FuncDecl)
		if isBuiltinSignature(fd.Type) {
			matched = append(matched, fd.Name.Name)
		}
	}
	want := "core,qualified,grouped"
	if strings.Join(matched, ",") != want {
		t.Errorf("the signature test matched %v, want %s", matched, want)
	}
}
