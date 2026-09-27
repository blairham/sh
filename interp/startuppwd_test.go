// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/blairham/sh/interp"
)

// The name a shell gives the directory it starts in. Tests name the axis and
// never a shell — interp/inheritedpwd.go has the measurements.
//
// The probe needs a directory with two true names, so each case builds a
// symbolic link to the one the runner is started in: without it every answer
// spells the directory the same way and a broken policy reads as a working
// one.

// linkedDir is a directory and a link that reaches it by another name.
func linkedDir(t *testing.T) (real, via string) {
	t.Helper()
	base := t.TempDir()
	real = filepath.Join(base, "real")
	if err := os.Mkdir(real, 0o755); err != nil {
		t.Fatal(err)
	}
	via = filepath.Join(base, "via")
	if err := os.Symlink(real, via); err != nil {
		t.Fatal(err)
	}
	return real, via
}

func TestTheNameAShellGivesItsStartingDirectory(t *testing.T) {
	real, via := linkedDir(t)
	elsewhere := filepath.Dir(real)

	for _, c := range []struct {
		name   string
		policy StartupPwdNamePolicy
		env    []string
		want   string
	}{
		{
			"from the kernel: a true handed name is still ignored",
			StartupPwdNameFromTheKernel,
			[]string{"PWD=" + via},
			real,
		},
		{
			"from the kernel: HOME is ignored too",
			StartupPwdNameFromTheKernel,
			[]string{"HOME=" + via},
			real,
		},

		{
			"when it fits: a true handed name is taken",
			StartupPwdNameFromTheEnvironmentWhenItFits,
			[]string{"PWD=" + via},
			via,
		},
		{
			"when it fits: a name of somewhere else is dropped",
			StartupPwdNameFromTheEnvironmentWhenItFits,
			[]string{"PWD=" + elsewhere},
			real,
		},
		{
			"when it fits: a relative name is dropped",
			StartupPwdNameFromTheEnvironmentWhenItFits,
			[]string{"PWD=relative"},
			real,
		},
		{
			"when it fits: HOME is not consulted",
			StartupPwdNameFromTheEnvironmentWhenItFits,
			[]string{"HOME=" + via},
			real,
		},

		{
			"or home: a true handed name is taken",
			StartupPwdNameFromTheEnvironmentOrHome,
			[]string{"PWD=" + via},
			via,
		},
		{
			"or home: HOME names the directory itself",
			StartupPwdNameFromTheEnvironmentOrHome,
			[]string{"HOME=" + via},
			via,
		},
		{
			"or home: HOME above it, with the path appended",
			StartupPwdNameFromTheEnvironmentOrHome,
			[]string{"HOME=" + filepath.Dir(via)},
			filepath.Dir(via) + "/real",
		},
		{
			"or home: a HOME the directory is not under",
			StartupPwdNameFromTheEnvironmentOrHome,
			[]string{"HOME=/usr"},
			real,
		},
		{
			"or home: nothing handed over at all",
			StartupPwdNameFromTheEnvironmentOrHome, nil, real,
		},

		{
			"unanswered: the kernel's answer, as it was before the axis",
			StartupPwdNameUnspecified,
			[]string{"PWD=" + via},
			real,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			out := &strings.Builder{}
			sem := PosixSemantics()
			sem.StartupPwdName = c.policy
			r := newTestRunner(t, &Runner{
				Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh", Dir: real,
				Env: c.env, Stdout: out, Stderr: &strings.Builder{},
			})
			runCd(t, r, `echo "[$PWD][$(pwd)]"`)
			if got := strings.TrimSpace(out.String()); got != "["+c.want+"]["+c.want+"]" {
				t.Errorf("PWD and pwd = %s, want [%s][%s]", got, c.want, c.want)
			}
		})
	}
}

// The one case where `$PWD` and `pwd` are two answers: a kept name that says
// nowhere is still the parameter, and `pwd` reports where the shell really is.
// The other policy that reads the environment drops such a name outright,
// which is the control the pair is worth stating for.
func TestAKeptNameThatSaysNowhereLeavesPwdAlone(t *testing.T) {
	real, _ := linkedDir(t)
	for _, c := range []struct {
		policy    StartupPwdNamePolicy
		handed    string
		wantVar   string
		wantBuilt string
	}{
		{StartupPwdNameFromTheEnvironmentOrHome, "/nonexistent-zz", "/nonexistent-zz", real},
		{StartupPwdNameFromTheEnvironmentOrHome, "/usr", "/usr", real},
		{StartupPwdNameFromTheEnvironmentWhenItFits, "/nonexistent-zz", real, real},
	} {
		t.Run(c.policy.String()+" "+c.handed, func(t *testing.T) {
			out := &strings.Builder{}
			sem := PosixSemantics()
			sem.StartupPwdName = c.policy
			r := newTestRunner(t, &Runner{
				Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh", Dir: real,
				Env:    []string{"PWD=" + c.handed},
				Stdout: out, Stderr: &strings.Builder{},
			})
			runCd(t, r, `echo "[$PWD][$(pwd)]"`)
			if want := "[" + c.wantVar + "][" + c.wantBuilt + "]"; strings.TrimSpace(out.String()) != want {
				t.Errorf("PWD and pwd = %s, want %s", strings.TrimSpace(out.String()), want)
			}
		})
	}
}

// The name taken at startup is the one a later relative `cd` is written in,
// which is what says it became the shell's own and not only a parameter.
func TestTheNameTakenAtStartupCarriesIntoALaterCd(t *testing.T) {
	real, via := linkedDir(t)
	if err := os.Mkdir(filepath.Join(real, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	out := &strings.Builder{}
	sem := PosixSemantics()
	sem.StartupPwdName = StartupPwdNameFromTheEnvironmentOrHome
	r := newTestRunner(t, &Runner{
		Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh", Dir: real,
		Env: []string{"PWD=" + via}, Stdout: out, Stderr: &strings.Builder{},
	})
	runCd(t, r, "cd sub")
	runCd(t, r, `echo "[$PWD]"`)
	if want := "[" + via + "/sub]"; strings.TrimSpace(out.String()) != want {
		t.Errorf("PWD after a relative cd = %s, want %s", strings.TrimSpace(out.String()), want)
	}
}

// `$PWD` carries the export attribute however the name was settled.
//
// Not an axis: every shell in the panel hands the name down and none of the
// dialect binaries in this tree did, so a child of any of them saw no `PWD` at
// all — see exportStartupPwd for the measurement. Stated over all four
// policies because the export is a fact about the parameter and not about
// where its value came from, and the two that read the environment would
// otherwise be passing only because an inherited name is exported anyway.
func TestTheStartupPwdCarriesTheExportAttribute(t *testing.T) {
	real, via := linkedDir(t)
	for _, c := range []struct {
		name   string
		policy StartupPwdNamePolicy
		env    []string
	}{
		{"asked of the kernel, nothing handed over", StartupPwdNameFromTheKernel, nil},
		{"asked of the kernel, a handed name ignored", StartupPwdNameFromTheKernel, []string{"PWD=" + via}},
		{"a handed name taken", StartupPwdNameFromTheEnvironmentWhenItFits, []string{"PWD=" + via}},
		{"a handed name dropped", StartupPwdNameFromTheEnvironmentWhenItFits, []string{"PWD=/nonexistent-zz"}},
		{"named under home", StartupPwdNameFromTheEnvironmentOrHome, []string{"HOME=" + via}},
		{"unanswered", StartupPwdNameUnspecified, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			sem := PosixSemantics()
			sem.StartupPwdName = c.policy
			r := newTestRunner(t, &Runner{
				Semantics: &sem, Diagnostics: &Diagnostics{}, Name: "sh", Dir: real,
				Env: c.env, Stdout: &strings.Builder{}, Stderr: &strings.Builder{},
			})
			runCd(t, r, `:`)
			a, ok := r.ParameterAttributes("PWD")
			if !ok {
				t.Fatalf("PWD is not a parameter of a shell that has just started in %s", real)
			}
			if !a.Exported {
				t.Errorf("PWD = %+v, want the export attribute on it", a)
			}
			// The control, and it is the reason this is not simply "mark
			// everything the startup writes": the name beside it in the same
			// startup is settled by a policy of its own and is not exported
			// under the answer this runner holds.
			if b, ok := r.ParameterAttributes("OPTIND"); ok && b.Exported {
				t.Errorf("OPTIND = %+v, want no export attribute on it", b)
			}
		})
	}
}
