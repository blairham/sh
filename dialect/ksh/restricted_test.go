// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package ksh_test

import (
	"strings"
	"testing"
)

// What this shell's restricted shell refuses, measured 2026-09-22 on ksh93u+
// 2012-08-01 from a script file under `env -i PATH=/usr/bin:/bin LC_ALL=C` with
// a scratch HOME, one spelling at a time with `echo tail` behind it.
//
// It is **not** bash's mode under another name, which is the reason #4205 was a
// separate issue from #4168: every sentence is the single word `restricted`
// where bash writes a sentence apiece, three of the refusals end the script
// where none of bash's does, and the frozen names differ. See
// interp/restricted.go for the sites and Semantics.SetHasTheRestrictedLetter
// for why the letter and the mode are one question.
func TestTheRestrictedShellRefusals(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct {
		name, src, want string
		// ends is whether the refusal takes the rest of the script with it.
		ends bool
	}{
		{name: "cd with an operand", src: `cd /`, want: "cd: restricted"},
		{name: "cd with none", src: `cd`, want: "cd: restricted"},
		{name: "a command word with a separator", src: `/bin/echo hi`, want: "/bin/echo: restricted"},
		{name: "a relative one", src: `./x`, want: "./x: restricted"},
		{name: "an output redirection", src: `echo x > f`, want: "f: restricted"},
		{name: "an appending one", src: `echo x >> f`, want: "f: restricted"},
		{name: "one that reads and writes", src: `echo x <> f`, want: "f: restricted"},
		{name: "unset of a frozen name", src: `unset PATH`, want: "unset: PATH: restricted"},
		// The three that end the script, and they are three rather than all
		// eight — see Semantics.RestrictedBuiltinRefusalIsFatal.
		{name: "sourcing a path", src: `. /etc/profile`, want: ".: /etc/profile: restricted", ends: true},
		{name: "exec", src: `exec echo hi`, want: "exec: echo: restricted", ends: true},
		{name: "command -p", src: `command -p echo hi`, want: "-p: restricted", ends: true},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runKsh(t, dir, "set -r\n"+c.src+"\necho tail\n")
			if !strings.Contains(out, c.want) {
				t.Errorf("output %q, want %q in it", out, c.want)
			}
			if got := strings.Contains(out, "tail"); got == c.ends {
				t.Errorf("output %q: the script ran on = %v, want %v", out, got, !c.ends)
			}
		})
	}
}

// The frozen names, and the two ways a script can see that this shell does not
// call them readonly.
//
// Measured: the assignment is `PATH: restricted` rather than `PATH: is read
// only`, it ends the script the way a readonly assignment does here, and
// `readonly -p` in the mode lists none of them — where bash lists `declare -r
// ENV`. See Semantics.RestrictedFreezeIsAReadonly.
func TestTheRestrictedShellFreezesItsOwnNames(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"PATH", "SHELL", "ENV", "FPATH"} {
		t.Run(name, func(t *testing.T) {
			out, _ := runKsh(t, dir, "set -r\n"+name+"=/x\necho tail\n")
			if want := name + ": restricted"; !strings.Contains(out, want) {
				t.Errorf("output %q, want %q", out, want)
			}
			if strings.Contains(out, "is read only") {
				t.Errorf("output %q: this shell does not word it as a readonly", out)
			}
			if strings.Contains(out, "tail") {
				t.Errorf("output %q, want the assignment to end the script", out)
			}
		})
	}
	// HISTFILE is bash's extra name and not this shell's, which is why the list
	// is a dialect's and not the substrate's.
	out, _ := runKsh(t, dir, "set -r\nHISTFILE=/x\necho tail\n")
	if strings.Contains(out, "restricted") || !strings.Contains(out, "tail") {
		t.Errorf("output %q, want HISTFILE assigned without complaint", out)
	}
	// And nothing is listed as readonly by it.
	out, _ = runKsh(t, dir, "set -r\nreadonly -p\n")
	for _, name := range []string{"PATH", "SHELL", "ENV", "FPATH"} {
		if strings.Contains(out, name) {
			t.Errorf("readonly -p wrote %q; this shell lists none of the frozen names", out)
		}
	}
}

// The mode is a switch here and not a door that locks behind you, which is the
// one thing about it that bash cannot do.
//
// Measured: `set -r; set +r` is silent at 0, `$-` loses the letter, and `cd`,
// an assignment to PATH and a command word with a separator are each taken
// again. The **long** spelling is refused all the same, and fatally, which is
// why the two doors are not one.
func TestTheRestrictedModeIsLeftByTheLetterAndNotByTheName(t *testing.T) {
	dir := t.TempDir()
	out, st := runKsh(t, dir, `
set -r
set +r
echo "st=$? [$-]"
cd / && echo moved
PATH=/bin && echo assigned
/bin/echo ran
`)
	for _, want := range []string{"st=0 [hB]", "moved", "assigned", "ran"} {
		if !strings.Contains(out, want) {
			t.Errorf("output %q, want %q in it", out, want)
		}
	}
	if st != 0 {
		t.Errorf("status %d, want 0", st)
	}
	out, _ = runKsh(t, dir, "set -r\nset +o restricted\necho tail\n")
	if !strings.Contains(out, "set: restricted: restricted") {
		t.Errorf("output %q, want the long spelling refused", out)
	}
	if strings.Contains(out, "tail") {
		t.Errorf("output %q, want the refusal to end the script", out)
	}
}

// The long spelling enters the mode and the listing says so, which is the half
// that was a lie before #4205: the name was recorded — listed and acted on by
// nothing — so a script that asked for a restricted shell through it was told 0
// and then ran everything the mode forbids.
func TestTheRestrictedModesLongSpellingIsReal(t *testing.T) {
	dir := t.TempDir()
	out, _ := runKsh(t, dir, "set -o restricted\nset -o | grep restricted\ncd / && echo moved\necho tail\n")
	if !strings.Contains(out, "restricted") {
		t.Errorf("output %q, want the listing to carry the name", out)
	}
	if strings.Contains(out, "moved") {
		t.Errorf("output %q, want cd refused", out)
	}
	if !strings.Contains(out, "cd: restricted") {
		t.Errorf("output %q, want the mode's own refusal", out)
	}
}

// What the mode does not refuse is as measured as what it does.
func TestTheRestrictedShellTakesWhatItDoesNotRefuse(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct{ name, src, want string }{
		{"a descriptor duplication", `echo x >&2`, "x"},
		{"a bare command name", `command echo hi`, "hi"},
		{"exec's redirection form", `exec 3< /dev/null; echo opened`, "opened"},
		{"a name the mode does not freeze", `LD_LIBRARY_PATH=/x; echo assigned`, "assigned"},
		{"whence with a path", `whence -p echo; echo asked`, "asked"},
		// A bare name reaches the search this shell still has, so `.` on one is
		// not the mode's business — the refusal is about a path.
		{"sourcing a bare name", `. nosuch`, "cannot open"},
	} {
		t.Run(c.name, func(t *testing.T) {
			out, _ := runKsh(t, dir, "set -r\n"+c.src+"\n")
			if !strings.Contains(out, c.want) {
				t.Errorf("output %q, want %q in it", out, c.want)
			}
			if strings.Contains(out, "restricted") {
				t.Errorf("output %q, want nothing refused", out)
			}
		})
	}
}

// The mode reaches inside a nested shell, which here is a clone copying the
// state rather than a check of its own.
func TestTheRestrictedModeReachesANestedShell(t *testing.T) {
	dir := t.TempDir()
	for _, src := range []string{`( cd / )`, `f(){ cd /; }; f`, `x=$(cd /)`} {
		out, _ := runKsh(t, dir, "set -r\n"+src+"\necho tail\n")
		if !strings.Contains(out, "cd: restricted") {
			t.Errorf("%s: output %q, want the refusal", src, out)
		}
	}
}
