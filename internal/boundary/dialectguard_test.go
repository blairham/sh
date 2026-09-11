// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package boundary_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

// The same rule as the file beside this one, applied to the other half of the
// program — and #1808 is the issue that says the two halves were never the
// same set.
//
// The guard next door reads "every package that holds a Boundary", which is
// the front end. A dialect holds none: `interp` has the gate, `internal/
// boundary` has the front end's, and `dialect/zsh` has neither — so no test
// anywhere read a dialect's source, and a builtin it registers could open,
// create, delete and rename files with nothing objecting.
//
// That is not hypothetical and it is why this file exists. `sysopen` and
// `zsystem flock` opened any file with a bare os.OpenFile (#1805). `autoload`
// read a function's file with os.ReadFile and then *ran* it (#1812). Every
// mutating builtin in `zsh/files` — nine of them, a module whose entire
// purpose is modifying the filesystem — called the `os` package directly, so
// `default deny` still let `zf_rm` delete what it was pointed at and `zf_ln`
// hard-link a denied file into an allowed directory (#1819). Three separate
// discoveries, each found by someone running the shell rather than by a test.
//
// # Why the verb list is longer here
//
// The front end's guard watches opens, because that is all the front end
// does. A dialect's builtins are the shell's own hands: they unlink, rename,
// create directories and change modes, and none of those is an open. The
// seams for them — interp's AllowModify, AllowReadPath, AllowProbe and
// AllowList — arrived with #1819 precisely because there was nothing for a
// dialect to call, and this list is what makes using them the only way
// through.
//
// Probes are watched too, which the front end's guard deliberately does not
// do. The reason they differ: the front end asks about a path a *person*
// typed into a completion, and the dialect asks about one a *script* named,
// which is the subject the policy is about. `zstat` answering with the true
// size of a file `[ -f ]` refuses into "not there" was one of #1819's three.

// dialects is every package that can register a builtin, relative to this
// one. Read from the filesystem rather than listed, because a list is the
// thing #1808 is about: a fifth dialect added to a hand-written list only
// when somebody remembers is a fifth dialect this rule does not cover.
func dialectDirs(t *testing.T) []string {
	t.Helper()
	const root = "../../dialect"
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("reading %s: %v", root, err)
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			out = append(out, filepath.Join(root, e.Name()))
		}
	}
	if len(out) == 0 {
		t.Fatal("no dialect packages found: the guard is reading the wrong place")
	}
	return out
}

// touchers is every call that reaches the filesystem on a path.
//
// Three groups, and the second and third are what this adds to the front
// end's list. The openers read or write a file's contents; the mutators
// change what is at a name without reading anything; the probes answer a
// question about a path, which is a disclosure rather than a change and is
// refused quietly rather than loudly.
var touchers = map[string]map[string]bool{
	"os": {
		// Contents.
		"Open": true, "OpenFile": true, "Create": true, "CreateTemp": true,
		"ReadFile": true, "WriteFile": true, "ReadDir": true,
		// Names.
		"Remove": true, "RemoveAll": true, "Rename": true, "Mkdir": true,
		"MkdirAll": true, "MkdirTemp": true, "Symlink": true, "Link": true,
		"Truncate": true, "Chmod": true, "Chown": true, "Lchown": true,
		// Questions.
		"Stat": true, "Lstat": true, "Readlink": true,
	},
	// The same calls one layer down. A builtin needing flags os.OpenFile
	// cannot express reaches for these — `sysopen`'s non-blocking form does
	// — and going around the standard library must not also go around the
	// gate.
	"syscall": {
		"Open": true, "Stat": true, "Lstat": true, "Unlink": true,
		"Rename": true, "Mkdir": true, "Rmdir": true, "Chmod": true,
		"Chown": true, "Symlink": true, "Link": true, "Truncate": true,
	},
}

// dialectExempt is every direct filesystem call a dialect still makes, and
// why it is outside the boundary or already inside it.
//
// Adding an entry is the deliberate act this list exists to force, and the
// reasons come in exactly three kinds:
//
//   - **It is the gate layer.** dialect/zsh's filesgate.go is where the
//     asking happens, so it is the one place that must call the `os` package
//     with a script's path. Everything else in the module goes through it.
//   - **It asked first.** The call sits below an AllowModify, AllowOpen or
//     AllowProbe on the same path, named in the entry so a reader can check.
//   - **The path is not the script's.** A fixed path the shell chose is
//     inside the boundary already, which is the rule docs/design.md states.
//
// A reason that is none of those three is a hole being written down rather
// than an exemption.
var dialectExempt = map[string]string{
	// The gate layer itself.
	"zsh.fileMayModifyTarget": "filesgate.go: walks a symbolic-link chain asking AllowModify " +
		"about every hop, so the Lstat and Readlink here are how the question is asked rather " +
		"than a way around it.",
	"zsh.fileLstat":   "filesgate.go: os.Lstat behind AllowProbe, which is the gated probe itself.",
	"zsh.fileStat":    "filesgate.go: os.Stat behind AllowProbe.",
	"zsh.fileReadDir": "filesgate.go: os.ReadDir behind AllowList.",
	"zsh.fileUnlink":  "the unlink itself, on both platforms, reached only from fileUnlinkOne, which asks fileMayModify first.",

	// zsh/files: each asked before it acted.
	"zsh.fileChown": "asks through fileWalk, which calls fileMayModify or fileMayModifyTarget " +
		"for every path before apply runs.",
	"zsh.fileChmod":   "the same, through fileWalk.",
	"zsh.fileWalk":    "the walk that does the asking: allow() is fileMayModify or fileMayModifyTarget, called before apply and before descending.",
	"zsh.fileLn":      "asks fileMayRead for a hard link's source and fileMayModify for its target before linking.",
	"zsh.fileMv":      "asks fileMayModify for both names before renaming — a rename changes each of them.",
	"zsh.fileMakeDir": "asks fileMayModify for every directory it would create, parents included, through fileMissingParents.",
	"zsh.fileRemoveTree": "asks fileReadDir to enumerate and fileMayModify before removing the " +
		"directory itself; each entry goes back through fileRemove.",
	"zsh.fileRmdir": "asks fileLstat and then fileMayModify before removing.",

	// zsh/system and zsh/stat: the #1805 and #1819 fixes, from the other side.
	"zsh.sysOpenFile": "the open `sysopen` performs — on both platforms — behind " +
		"Runner.AllowOpen and confirmed with VerifyOpened, which is the seam #1807 added for " +
		"exactly this. The syscall.Open is the non-blocking form, which os.OpenFile cannot " +
		"express, and it is still inside the boundary because the asking happens above it.",
	"zsh.systemLockOpen": "the open a `zsystem flock` is taken by, behind Runner.AllowOpen.",
	"zsh.statPath":       "the stat `zstat` performs, behind Runner.AllowProbe in zstatOne.",
}

// TestNoDialectReachesTheFilesystemWithoutSayingWhy.
func TestNoDialectReachesTheFilesystemWithoutSayingWhy(t *testing.T) {
	t.Parallel()
	var found []string
	for _, dir := range dialectDirs(t) {
		for _, call := range directCalls(t, dir, touchers) {
			found = append(found, call)
			if _, ok := dialectExempt[call]; !ok {
				t.Errorf("%s reaches the filesystem through the os or syscall package and "+
					"nothing says why.\n"+
					"\tA builtin acts on a path the script named, which is the subject a policy "+
					"is about.\n"+
					"\tAsk first — Runner.AllowModify to change a path, AllowReadPath to make its "+
					"contents\n"+
					"\treachable, AllowProbe to ask about it, AllowList to enumerate it, or "+
					"AllowOpen with\n"+
					"\tVerifyOpened to open it — or add %s to dialectExempt in this file with the "+
					"reason\n"+
					"\tit is outside the boundary. See #1808 and #1819.", call, call)
			}
		}
	}
	// A walk that found nothing passes for the wrong reason, which is the
	// failure mode of every assertion made over a traversal — and the one
	// #1808 is about, since the guard next door was reading a set that did
	// not contain these packages at all.
	if len(found) < len(dialectExempt) {
		t.Fatalf("the walk found %d direct calls and the exemption list names %d; "+
			"the detector is not reading the dialect packages", len(found), len(dialectExempt))
	}
	// And nothing is exempted that no longer exists. An entry that outlives
	// its call is permission nobody is using, which reads on review as a
	// decision somebody made about code that is not there.
	sort.Strings(found)
	for call := range dialectExempt {
		if i := sort.SearchStrings(found, call); i == len(found) || found[i] != call {
			t.Errorf("dialectExempt names %s and no such call is there any more", call)
		}
	}
}

// TestTheDialectGuardSeesTheCallsTheFrontEndGuardWouldMiss.
//
// A detector that has gone quiet passes over a broken tree and a clean one
// alike, so it is handed the violations it exists to find — and specifically
// the two kinds the guard next door does not look for, since those are the
// ones this file was added for. Neither a `syscall.Unlink` nor an `os.Rename`
// is an open, and #1819 was nine functions doing exactly that.
func TestTheDialectGuardSeesTheCallsTheFrontEndGuardWouldMiss(t *testing.T) {
	t.Parallel()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "x.go", `
package zsh

func removes(path string) error { return os.Remove(path) }

func renames(a, b string) error { return os.Rename(a, b) }

func unlinksBeneath(path string) error { return syscall.Unlink(path) }

func probes(path string) (any, error) { return os.Lstat(path) }

func asks(r *Runner, ctx any, path string) bool { return r.AllowModify(ctx, path) }
`, 0)
	if err != nil {
		t.Fatal(err)
	}
	var caught []string
	for _, d := range f.Decls {
		fd := d.(*ast.FuncDecl)
		if callsDirectly(fd.Body, touchers) {
			caught = append(caught, fd.Name.Name)
		}
	}
	want := []string{"removes", "renames", "unlinksBeneath", "probes"}
	if len(caught) != len(want) {
		t.Fatalf("the detector read %v, want %v", caught, want)
	}
	for i, name := range want {
		if caught[i] != name {
			t.Errorf("the detector read %v, want %v", caught, want)
			break
		}
	}
}
