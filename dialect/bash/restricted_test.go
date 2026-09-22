// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package bash_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairham/sh/internal/dialecttest"
)

// What bash's restricted shell refuses, measured 2026-09-22 on bash 5.3.20
// with a scratch HOME and no startup files, each spelling run from a script
// file with `echo st=$?` behind it.
//
// Here rather than in `interp` because every sentence and every status in it
// is one shell's, and bash is the only column whose mode this shell has
// built: ksh93 has a restricted mode of its own with different refusals, and
// the other four have none. See interp/restricted.go for the ten sites and
// Semantics.SetHasTheRestrictedLetter for why having the letter and having
// the mode are one question.
//
// The cases are written as output **and** status, because half of what a
// restricted shell is for is a caller reading the status: a refusal that
// printed the right sentence and reported 0 would let `cmd || fallback` take
// the wrong branch, which is the failure the whole mode exists to prevent.
func TestBashRestrictedShellRefusesInItsOwnWords(t *testing.T) {
	for _, c := range []struct {
		name, src, want string
		status          int
	}{
		{
			name:   "cd",
			src:    "set -r\ncd /\n",
			want:   "bash: line 2: cd: restricted\n",
			status: 1,
		}, {
			// The whole builtin, before the options and before the operand:
			// a bare `cd` is the same sentence and says nothing about HOME.
			name:   "cd with no operand",
			src:    "set -r\ncd\n",
			want:   "bash: line 2: cd: restricted\n",
			status: 1,
		}, {
			// The frozen names, which are refused as the ordinary readonly
			// they now are rather than by a check of their own.
			name:   "PATH",
			src:    "set -r\nPATH=/bin\n",
			want:   "bash: line 2: PATH: readonly variable\n",
			status: 1,
		}, {
			name:   "SHELL",
			src:    "set -r\nSHELL=x\n",
			want:   "bash: line 2: SHELL: readonly variable\n",
			status: 1,
		}, {
			name:   "BASH_ENV",
			src:    "set -r\nBASH_ENV=x\n",
			want:   "bash: line 2: BASH_ENV: readonly variable\n",
			status: 1,
		}, {
			// Before the search, which is the point: a word that names
			// nothing is refused for the separator and not for being absent.
			name:   "a command word with a separator",
			src:    "set -r\n/no/such\n",
			want:   "bash: line 2: /no/such: restricted: cannot specify `/' in command names\n",
			status: 1,
		}, {
			name:   "the same word behind command",
			src:    "set -r\ncommand /no/such\n",
			want:   "bash: line 2: /no/such: restricted: cannot specify `/' in command names\n",
			status: 1,
		}, {
			// `.` names itself by the word the script wrote.
			name:   "dot on a path",
			src:    "set -r\n. ./nosuch\n",
			want:   "bash: line 2: .: ./nosuch: restricted\n",
			status: 1,
		}, {
			name:   "source on a path",
			src:    "set -r\nsource d/nosuch\n",
			want:   "bash: line 2: source: d/nosuch: restricted\n",
			status: 1,
		}, {
			// The target word as written, and the command does not run.
			name:   "a truncating redirection",
			src:    "set -r\necho x > f\n",
			want:   "bash: line 2: f: restricted: cannot redirect output\n",
			status: 1,
		}, {
			name:   "an appending redirection",
			src:    "set -r\necho x >> f\n",
			want:   "bash: line 2: f: restricted: cannot redirect output\n",
			status: 1,
		}, {
			name:   "both streams at once",
			src:    "set -r\necho x &> f\n",
			want:   "bash: line 2: f: restricted: cannot redirect output\n",
			status: 1,
		}, {
			name:   "exec with a command",
			src:    "set -r\nexec nosuchcmd\n",
			want:   "bash: line 2: exec: restricted\n",
			status: 1,
		}, {
			name:   "the default path",
			src:    "set -r\ncommand -p ls\n",
			want:   "bash: line 2: command: -p: restricted\n",
			status: 1,
		}, {
			// The letter either way round in a bundle.
			name:   "the default path while reporting",
			src:    "set -r\ncommand -vp sed\n",
			want:   "bash: line 2: command: -p: restricted\n",
			status: 1,
		}, {
			name:   "a hand-written hash entry on a path",
			src:    "set -r\nhash -p /bin/sh zz\n",
			want:   "bash: line 2: hash: /bin/sh: restricted\n",
			status: 1,
		}, {
			// The second half of that rule: a bare name is searched for, and
			// a name nothing answers to is refused as missing.
			name:   "a hand-written hash entry nothing answers to",
			src:    "set -r\nhash -p zznosuchcmd zz\n",
			want:   "bash: line 2: hash: zznosuchcmd: not found\n",
			status: 1,
		}, {
			name:   "unloading a builtin",
			src:    "set -r\nenable -d printf\n",
			want:   "bash: line 2: enable: restricted\n",
			status: 1,
		}, {
			// The way back out, which there is not. The usage block under it
			// is `set`'s own, and the status is 1 where this shell's other
			// refused letters report 2.
			name: "the way back out",
			src:  "set -r\nset +r\n",
			want: "bash: line 2: set: +r: invalid option\n" +
				"set: usage: set [-abefhkmnptuvxBCEHPT] [-o option-name] [--] [-] [arg ...]\n",
			status: 1,
		}, {
			// And the long spelling, which bash has never had: `restricted`
			// is not one of its `set -o` names, so this is the ordinary
			// unknown-name refusal at that refusal's own status.
			name:   "the long way back out",
			src:    "set -r\nset +o restricted\n",
			want:   "bash: line 2: set: restricted: invalid option name\n",
			status: 2,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, st, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, c.src)
			if err != nil {
				t.Fatalf("run %q: %v", c.src, err)
			}
			if out != c.want {
				t.Errorf("said %q, want %q", out, c.want)
			}
			if st != c.status {
				t.Errorf("status %d, want %d", st, c.status)
			}
		})
	}
}

// What a restricted shell still takes, which is as measured as what it
// refuses and is the half a test suite forgets.
//
// A gate graded only on refusals cannot tell a working mode from one that
// refuses everything, and four of these rows are spellings that look like the
// refused ones and are not: a descriptor duplication is not a redirection
// that opens a file, a here-string opens nothing at all, `command -p` with
// nothing to search for searches for nothing, and switching a builtin off
// neither adds one nor takes one away.
func TestBashRestrictedShellStillTakesWhatItWasMeasuredTaking(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{"a descriptor duplication", "set -r\necho x >&2\n", "x\n"},
		{"the other direction", "set -r\necho x 2>&1\n", "x\n"},
		{"a here-string", "set -r\nread zza <<< hi\necho \"$zza\"\n", "hi\n"},
		{"the default path with nothing to search for", "set -r\ncommand -p\necho st=$?\n", "st=0\n"},
		{"switching a builtin off", "set -r\nenable -n printf\necho st=$?\n", "st=0\n"},
		{"a second request for the mode", "set -r\nset -r\necho st=$?\n", "st=0\n"},
		{"the letter in $-", "set -r\ncase $- in *r*) echo yes;; *) echo no;; esac\n", "yes\n"},
		// A shell that never entered the mode grants the request for the
		// state it is already in, silently — which is why the refusal above
		// hangs on the state rather than on the letter.
		{"leaving a mode never entered", "set +r\necho st=$?\n", "st=0\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, c.src)
			if err != nil {
				t.Fatalf("run %q: %v", c.src, err)
			}
			if out != c.want {
				t.Errorf("said %q, want %q", out, c.want)
			}
		})
	}
}

// The refusals hold inside a subshell, a command substitution and a function,
// because the mode is the shell's rather than the statement's.
//
// Measured on bash 5.3.20: each of the three is `cd: restricted` at 1, which
// is what a clone copying the field already gives. Written down because the
// alternative — a flag consulted only on the outermost runner — would pass
// every case above and leave `( cd / )` as the way out.
func TestBashRestrictedModeReachesInsideANestedShell(t *testing.T) {
	for _, c := range []struct{ name, src string }{
		{"a subshell", "set -r\n( cd / )\n"},
		{"a command substitution", "set -r\nx=$(cd /)\n"},
		{"a function body", "set -r\nf() { cd /; }\nf\n"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, c.src)
			if err != nil {
				t.Fatalf("run %q: %v", c.src, err)
			}
			if !strings.Contains(out, "cd: restricted") {
				t.Errorf("said %q, want the refusal", out)
			}
		})
	}
}

// A refused redirection leaves nothing behind, which is the half a sentence
// cannot prove.
//
// The measurement that matters is the file: a shell that printed the refusal
// and opened the target anyway would pass every wording case above, and a
// truncating redirection that got as far as the open has already destroyed
// what was there. So the fixture is a file with contents, and the assertion
// is that they are still in it.
func TestBashRestrictedRedirectionNeverOpensTheTarget(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "keep")
	if err := os.WriteFile(target, []byte("before\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, st, err := preset.Combined(t, dialecttest.Base{Dir: dir}, "set -r\necho after > keep\n")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if want := "bash: line 2: keep: restricted: cannot redirect output\n"; out != want {
		t.Errorf("said %q, want %q", out, want)
	}
	if st != 1 {
		t.Errorf("status %d, want 1", st)
	}
	held, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(held) != "before\n" {
		t.Errorf("target holds %q, want it untouched", held)
	}
}

// A refused hash entry leaves the name unrunnable, which is the third of
// these "the sentence is not the point" cases and the sharpest.
//
// Measured on bash 5.3.20: `hash -p /bin/sh zzr; zzr -c 'echo hello'` is the
// refusal and then `zzr: command not found` at 127, where the same two lines
// in an ordinary shell print `hello`. A shell that made the entry anyway
// would have handed the script the very command the refusal was about.
func TestBashRestrictedHashEntryIsNeverMade(t *testing.T) {
	out, _, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()},
		"set -r\nhash -p /bin/sh zzr\nzzr -c 'echo hello'\necho st=$?\n")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if strings.Contains(out, "hello") {
		t.Fatalf("said %q, want the refused entry never to have run", out)
	}
	for _, want := range []string{"hash: /bin/sh: restricted", "st=127"} {
		if !strings.Contains(out, want) {
			t.Errorf("said %q, want it to carry %q", out, want)
		}
	}
}

// And the parameter that presents the same table is the same refusal, with
// no builtin named and at an assignment's own status.
//
// The door this closes is the one a script looking for a way out tries
// second: the builtin is guarded and the parameter is a second spelling of
// it, so a mode that reached only the builtin would have left the hash
// writable. Measured on bash 5.3.20: `BASH_CMDS[a]=/bin/sh` is `/bin/sh:
// restricted` at 0 with nothing hashed, `BASH_CMDS[a]=zznosuchcmd` is
// `zznosuchcmd: not found` at 0, and an ordinary shell takes both in silence.
func TestBashRestrictedModeReachesTheCommandHashParameter(t *testing.T) {
	for _, c := range []struct{ name, src, want string }{
		{
			"a path", "set -r\nBASH_CMDS[zzr]=/bin/sh\necho st=$?\n",
			"bash: line 2: /bin/sh: restricted\nst=0\n",
		}, {
			"a name nothing answers to", "set -r\nBASH_CMDS[zzr]=zznosuchcmd\necho st=$?\n",
			"bash: line 2: zznosuchcmd: not found\nst=0\n",
		}, {
			// And nothing was written either way, which is what makes the
			// refusal a refusal rather than a remark.
			"and nothing is stored", "set -r\nBASH_CMDS[zzr]=/bin/sh\necho \"[${BASH_CMDS[zzr]}]\"\n",
			"bash: line 2: /bin/sh: restricted\n[]\n",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _, err := preset.Combined(t, dialecttest.Base{Dir: t.TempDir()}, c.src)
			if err != nil {
				t.Fatalf("run %q: %v", c.src, err)
			}
			if out != c.want {
				t.Errorf("said %q, want %q", out, c.want)
			}
		})
	}
}
