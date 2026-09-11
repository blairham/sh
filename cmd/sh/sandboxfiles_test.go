// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The modules that change the filesystem without opening anything, from the
// outside, through the flags a person types.
//
// #1805 was `sysopen` and `zsystem flock` opening files with no gate, and the
// tests beside this one cover those. This is the other half of the same
// hole, and it was live rather than theoretical: #1808 predicted that
// `zsh/files` would escape the moment it gained builtins, #1670 gave it nine,
// and every mutating one of them called the `os` package directly. Measured
// against the shipped binary under `default deny`, `zf_rm` deleted what it
// was pointed at and returned 0, `zf_mkdir` created outside the boundary, and
// `zf_ln` hard-linked a denied file into an allowed directory — after which
// its contents read back normally (#1819).
//
// # Every case is decided by the filesystem, not by the diagnostic
//
// A test that asserted on the word "refused" would pass for a shell whose
// builtin was broken for unrelated reasons, and that is not a small risk
// here: the whole class arrived because nothing checked, so a check that
// cannot tell "refused" from "did not work" would let the next one in the
// same way. So each case below asks the *filesystem* whether the thing
// happened, and each has a sibling asserting the same operation works when
// the policy permits it.

// filesFixture builds the two halves every case here needs: a workspace the
// policy allows, and the directory beside it that it does not.
//
// The denied half is the *parent* of the allowed one, so `..` out of the
// workspace lands in it. That is the shape a real caller has — an agent given
// a working directory inside a tree it may not otherwise touch — and it is
// the shape in which a rule that accidentally matched a prefix would show up.
func filesFixture(t *testing.T) (outside, ws, policy string) {
	t.Helper()
	outside = t.TempDir()
	ws = filepath.Join(outside, "ws")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	policy = writePolicy(t,
		"default deny",
		"allow read "+ws+"/**",
		"allow write "+ws+"/**",
		"allow stat "+ws+"/**",
		"allow list "+ws+"/**",
	)
	return outside, ws, policy
}

// zf runs a script with the module loaded, from inside the workspace.
func zf(t *testing.T, ws, policy, script string) outcome {
	t.Helper()
	return sandboxed(t, "zsh", "-policy", policy, "-c",
		"cd "+ws+"\nzmodload zsh/files\n"+script+"\n")
}

func TestEveryMutatingFilesBuiltinIsInsideTheBoundary(t *testing.T) {
	t.Parallel()
	// Each case names a path outside the workspace, the command aimed at it,
	// and how the filesystem says whether the boundary held. `sync` is the
	// one of the nine that is absent, because it names no path and there is
	// no rule anybody could write about it.
	cases := []struct {
		name   string
		make   func(t *testing.T, at string)
		script string // %s is the path outside the workspace
		held   func(at string) bool
		want   string
	}{{
		name:   "rm",
		make:   func(t *testing.T, at string) { writeFile(t, at, "PRECIOUS") },
		script: "zf_rm %s",
		held:   func(at string) bool { return exists(at) },
		want:   "the file is still there",
	}, {
		name:   "rm -r",
		make:   func(t *testing.T, at string) { mkdirAll(t, filepath.Join(at, "sub")) },
		script: "zf_rm -r %s",
		held:   func(at string) bool { return exists(at) },
		want:   "the tree is still there",
	}, {
		name:   "rm -f",
		make:   func(t *testing.T, at string) { writeFile(t, at, "PRECIOUS") },
		script: "zf_rm -f %s",
		held:   func(at string) bool { return exists(at) },
		want:   "-f silences diagnostics, not the gate",
	}, {
		name:   "rmdir",
		make:   func(t *testing.T, at string) { mkdirAll(t, at) },
		script: "zf_rmdir %s",
		held:   func(at string) bool { return exists(at) },
		want:   "the directory is still there",
	}, {
		name:   "mkdir",
		make:   func(*testing.T, string) {},
		script: "zf_mkdir %s",
		held:   func(at string) bool { return !exists(at) },
		want:   "no directory was created outside",
	}, {
		name:   "mkdir -p",
		make:   func(*testing.T, string) {},
		script: "zf_mkdir -p %s/a/b",
		held:   func(at string) bool { return !exists(at) },
		want:   "-p creates parents, and each one is a directory it may not make",
	}, {
		name:   "mv",
		make:   func(t *testing.T, at string) { writeFile(t, at, "PRECIOUS") },
		script: "zf_mv %s %s.moved",
		held:   func(at string) bool { return exists(at) && !exists(at+".moved") },
		want:   "the source is where it was and nothing arrived at the target",
	}, {
		name:   "chmod",
		make:   func(t *testing.T, at string) { writeFile(t, at, "PRECIOUS"); chmod(t, at, 0o600) },
		script: "zf_chmod 777 %s",
		held:   func(at string) bool { return mode(at) == 0o600 },
		want:   "the mode is the one it had",
	}, {
		name:   "chmod -R",
		make:   func(t *testing.T, at string) { mkdirAll(t, at); chmod(t, at, 0o700) },
		script: "zf_chmod -R 777 %s",
		held:   func(at string) bool { return mode(at) == 0o700 },
		want:   "a recursive change is a change to each path in it",
	}, {
		name: "chown",
		make: func(t *testing.T, at string) { writeFile(t, at, "PRECIOUS"); chmod(t, at, 0o600) },
		// Owner 0 rather than this process's own, because the assertion is
		// that the gate answers before the system call is reached: a change
		// that would have failed on privilege anyway proves nothing, and the
		// mode staying put is what says the walk stopped.
		script: "zf_chown 0 %s",
		held:   func(at string) bool { return mode(at) == 0o600 && exists(at) },
		want:   "the file is untouched",
	}, {
		name:   "chgrp",
		make:   func(t *testing.T, at string) { writeFile(t, at, "PRECIOUS"); chmod(t, at, 0o600) },
		script: "zf_chgrp 0 %s",
		held:   func(at string) bool { return mode(at) == 0o600 && exists(at) },
		want:   "the file is untouched",
	}}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			outside, ws, policy := filesFixture(t)
			at := filepath.Join(outside, "target")
			c.make(t, at)
			script := strings.ReplaceAll(c.script, "%s", at)
			got := zf(t, ws, policy, script)
			if !c.held(at) {
				t.Errorf("%s escaped the boundary: want %s\nerrs = %q",
					c.name, c.want, got.errs)
			}
			// Whichever way it was refused, it must not have said anything
			// the policy was withholding. The two wordings are pinned
			// deliberately in the pair of tests below; here the only claim
			// is that neither of them leaks.
			if strings.Contains(got.out, "PRECIOUS") {
				t.Errorf("out = %q: the contents of a denied file reached the script", got.out)
			}
		})
	}
}

// The two wordings, pinned on purpose, because which one a command gets is
// decided by whether it asks a question before it acts and that is worth
// being deliberate about.
//
// `rm` starts by asking what the operand *is* — it has to, since `-d`, `-r`
// and a plain file are three different removals — and that question is a
// probe, so a path the policy hides answers the way a path that is not there
// answers. A script may not be told that a file it cannot see is being
// withheld, which is the same rule `zstat` and `[[ -f ]]` follow.
func TestARefusedRemovalReadsAsANameThatIsNotThere(t *testing.T) {
	t.Parallel()
	outside, ws, policy := filesFixture(t)
	secret := filepath.Join(outside, "secret")
	writeFile(t, secret, "PRECIOUS")
	missing := filepath.Join(outside, "nosuch")

	refused := zf(t, ws, policy, "zf_rm "+secret)
	absent := zf(t, ws, policy, "zf_rm "+missing)

	norm := func(s, path string) string { return strings.ReplaceAll(s, path, "PATH") }
	if got, want := norm(refused.errs, secret), norm(absent.errs, missing); got != want {
		t.Errorf("refused = %q, absent = %q: a script can tell a file it may not remove "+
			"from one that is not there", got, want)
	}
	if !exists(secret) {
		t.Error("the file was removed")
	}
}

// `mkdir` asks nothing first — there is nothing to ask, since the name is
// supposed not to exist — so its refusal is the loud one every other
// modification gets, in the words a refused redirection uses. There is no
// honest way to carry on: a directory that was not created is not there, and
// a command that stayed silent about it would be lying about what it did.
func TestARefusedDirectoryCreationSaysSo(t *testing.T) {
	t.Parallel()
	outside, ws, policy := filesFixture(t)
	at := filepath.Join(outside, "made")

	got := zf(t, ws, policy, "zf_mkdir "+at)
	if !strings.Contains(got.errs, "refused") {
		t.Errorf("errs = %q, want the refusal reported the way a redirection's is", got.errs)
	}
	if exists(at) {
		t.Error("the directory was created")
	}
}

// The other half of every case above, without which they would all pass for a
// module that simply does not work.
//
// One script rather than a table: the operations compose — a thing is made,
// linked, moved, changed and removed — so a single run in one allowed
// directory exercises each of them against the state the last one left, which
// is how a caller uses them.
func TestTheFilesBuiltinsStillWorkWhereThePolicyAllowsThem(t *testing.T) {
	t.Parallel()
	_, ws, policy := filesFixture(t)
	got := zf(t, ws, policy, strings.Join([]string{
		"echo hello > a.txt",
		"zf_mkdir d          && echo mkdir-ok",
		"zf_mkdir -p p/q/r   && echo mkdirp-ok",
		"zf_ln a.txt hard    && echo ln-ok",
		"zf_ln -s a.txt soft && echo lns-ok",
		"zf_mv a.txt b.txt   && echo mv-ok",
		"zf_chmod 600 b.txt  && echo chmod-ok",
		"zf_rm hard          && echo rm-ok",
		"zf_rmdir d          && echo rmdir-ok",
		"zf_mkdir -p t/u && echo x > t/u/f && zf_rm -r t && echo rmr-ok",
	}, "\n"))
	for _, want := range []string{
		"mkdir-ok", "mkdirp-ok", "ln-ok", "lns-ok", "mv-ok",
		"chmod-ok", "rm-ok", "rmdir-ok", "rmr-ok",
	} {
		if !strings.Contains(got.out, want) {
			t.Errorf("out = %q errs = %q, want %s: the gate refused something inside the workspace",
				got.out, got.errs, want)
		}
	}
}

// The case that makes this a disclosure rather than a mutation bug.
//
// A hard link is a second *name* for one object, not a link a resolving walk
// can notice — so `zf_ln secret ./inside` puts the object itself inside the
// allowed subtree, and every later read of that name is allowed on its own
// merits. The contents cross the boundary when the name is created, which is
// why creating it is a read of the source.
func TestAHardLinkCannotCarryADeniedFileIntoAnAllowedDirectory(t *testing.T) {
	t.Parallel()
	outside, ws, policy := filesFixture(t)
	secret := filepath.Join(outside, "secret")
	writeFile(t, secret, "TOPSECRET\n")

	got := zf(t, ws, policy, "zf_ln "+secret+" ./leak\nread L < ./leak\necho LEAKED=$L")
	if strings.Contains(got.out, "TOPSECRET") {
		t.Errorf("out = %q: a hard link carried a denied file's contents inside.\n"+
			"The same read is refused when the script names the file directly, so a "+
			"builtin that gives it a second name inside the workspace has handed over "+
			"exactly what the rule withheld.", got.out)
	}
	if exists(filepath.Join(ws, "leak")) {
		t.Error("the link was created: the source is a file the policy does not permit reading")
	}
}

// A symbolic link is the case that needs *no* new check, and it is here so
// that the rule is recorded rather than assumed: the link may be made, and
// reading through it is refused by the walk that resolves it. A future change
// that started refusing the link itself would be refusing something harmless,
// and one that stopped refusing the read would be the leak above.
func TestASymbolicLinkMayBeMadeAndStillCannotBeReadThrough(t *testing.T) {
	t.Parallel()
	outside, ws, policy := filesFixture(t)
	secret := filepath.Join(outside, "secret")
	writeFile(t, secret, "TOPSECRET\n")

	got := zf(t, ws, policy, "zf_ln -s "+secret+" ./sl && echo MADE\nread L < ./sl\necho GOT=$L")
	if !strings.Contains(got.out, "MADE") {
		t.Errorf("out = %q errs = %q, want the symbolic link made: it writes only inside the workspace",
			got.out, got.errs)
	}
	if strings.Contains(got.out, "TOPSECRET") {
		t.Errorf("out = %q: the read followed the link out of the workspace", got.out)
	}
}

// `chmod` and `chown` follow a link to do their work, so the operand being
// inside the workspace says nothing about what the call lands on. This is the
// same shape as the `sysopen` symlink case beside it: an allowed name and a
// denied object.
func TestAModeChangeCannotFollowALinkOutOfAnAllowedDirectory(t *testing.T) {
	t.Parallel()
	outside, ws, policy := filesFixture(t)
	secret := filepath.Join(outside, "secret")
	writeFile(t, secret, "TOPSECRET\n")
	chmod(t, secret, 0o600)
	if err := os.Symlink(secret, filepath.Join(ws, "sneaky")); err != nil {
		t.Fatal(err)
	}

	zf(t, ws, policy, "zf_chmod 777 ./sneaky")
	if got := mode(secret); got != 0o600 {
		t.Errorf("mode = %04o, want 0600: the change followed a link to a file the policy protects.\n"+
			"The name is allowed and the object is not, so every hop of the chain has to be asked about.", got)
	}
}

// Enumerating a denied tree is a disclosure even when every removal in it is
// refused: a recursive `rm` that could list it has learned its shape.
func TestARecursiveRemovalCannotListADeniedTree(t *testing.T) {
	t.Parallel()
	outside, ws, policy := filesFixture(t)
	tree := filepath.Join(outside, "tree")
	mkdirAll(t, filepath.Join(tree, "SECRETNAME"))

	got := zf(t, ws, policy, "zf_rm -r "+tree)
	if strings.Contains(got.out+got.errs, "SECRETNAME") {
		t.Errorf("out = %q errs = %q: the walk named an entry of a directory the policy hides",
			got.out, got.errs)
	}
	if !exists(filepath.Join(tree, "SECRETNAME")) {
		t.Error("the tree was removed")
	}
}

// `zstat` is the probe half. fsgate.go opens by saying every probe in the
// shell comes through the gate because a probe is an oracle, and this one was
// outside that sentence: on one path in one script, `[[ -f secret ]]` was
// refused into "not there" while `zstat +size secret` answered with the true
// size and mtime.
func TestZstatIsInsideTheBoundary(t *testing.T) {
	t.Parallel()
	outside, ws, policy := filesFixture(t)
	secret := filepath.Join(outside, "secret")
	writeFile(t, secret, "0123456789\n")

	got := sandboxed(t, "zsh", "-policy", policy, "-c",
		"cd "+ws+"\nzmodload zsh/stat\n"+
			"if [[ -f "+secret+" ]]; then echo PROBE-VISIBLE; else echo probe-hidden; fi\n"+
			"zstat -A s +size "+secret+"\necho SIZE=$s\n")
	if !strings.Contains(got.out, "probe-hidden") {
		t.Fatalf("out = %q: the ordinary probe was not refused, so this measures nothing", got.out)
	}
	if strings.Contains(got.out, "SIZE=11") {
		t.Errorf("out = %q: zstat answered with the true size of a file `[[ -f ]]` hides.\n"+
			"Two probes about one path must not disagree about whether it is there.", got.out)
	}
}

// And the refusal has to be indistinguishable from a missing path, because a
// refusal that identified itself would be the oracle the refusal exists to
// prevent. The two runs differ only in which name they ask about.
func TestARefusedZstatReadsAsANameThatIsNotThere(t *testing.T) {
	t.Parallel()
	outside, ws, policy := filesFixture(t)
	secret := filepath.Join(outside, "secret")
	writeFile(t, secret, "x\n")
	missing := filepath.Join(outside, "nosuch")

	refused := sandboxed(t, "zsh", "-policy", policy, "-c",
		"cd "+ws+"\nzmodload zsh/stat\nzstat +size "+secret+"\n")
	absent := sandboxed(t, "zsh", "-policy", policy, "-c",
		"cd "+ws+"\nzmodload zsh/stat\nzstat +size "+missing+"\n")

	norm := func(s, path string) string { return strings.ReplaceAll(s, path, "PATH") }
	if got, want := norm(refused.errs, secret), norm(absent.errs, missing); got != want {
		t.Errorf("refused = %q, absent = %q: a script can tell a hidden file from a missing one", got, want)
	}
	if refused.code != absent.code {
		t.Errorf("code = %d, want %d: the status tells them apart", refused.code, absent.code)
	}
}

func TestAnAllowedZstatStillAnswers(t *testing.T) {
	t.Parallel()
	_, ws, policy := filesFixture(t)
	writeFile(t, filepath.Join(ws, "data"), "0123456789\n")
	got := sandboxed(t, "zsh", "-policy", policy, "-c",
		"cd "+ws+"\nzmodload zsh/stat\nzstat -A s +size ./data\necho SIZE=$s\n")
	if !strings.Contains(got.out, "SIZE=11") {
		t.Errorf("out = %q errs = %q, want the size of a file inside the workspace", got.out, got.errs)
	}
}

// Binding a socket creates a filesystem object and leaves it there, which is
// as much a write as `> path` is.
func TestZsocketCannotBindOutsideTheBoundary(t *testing.T) {
	t.Parallel()
	outside, ws, policy := filesFixture(t)
	sock := filepath.Join(outside, "sock")

	got := sandboxed(t, "zsh", "-policy", policy, "-c",
		"cd "+ws+"\nzmodload zsh/net/socket\nzsocket -l "+sock+" && echo BOUND\n")
	if exists(sock) {
		t.Errorf("%s exists: a refused bind created the socket anyway.\nerrs = %q", sock, got.errs)
	}
	if strings.Contains(got.out, "BOUND") {
		t.Errorf("out = %q, want the bind refused", got.out)
	}
}

func TestAnAllowedZsocketStillBinds(t *testing.T) {
	t.Parallel()
	_, ws, policy := filesFixture(t)
	got := sandboxed(t, "zsh", "-policy", policy, "-c",
		"cd "+ws+"\nzmodload zsh/net/socket\nzsocket -l ./sock && echo BOUND\n")
	if !strings.Contains(got.out, "BOUND") {
		t.Errorf("out = %q errs = %q, want a bind inside the workspace to work", got.out, got.errs)
	}
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func mkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func chmod(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// mode is the permission bits, or an impossible value when the path is gone —
// which fails a comparison rather than passing one, because a test asking
// "did the mode stay" must not be satisfied by the file having been deleted.
func mode(path string) os.FileMode {
	info, err := os.Lstat(path)
	if err != nil {
		return os.FileMode(0o7777)
	}
	return info.Mode().Perm()
}
