// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// TestNoParallelTestRaisesASignalAtThisProcess.
//
// The tests whose scripts trap a signal run in parallel (#7290), and a trap
// in a Runner is a subscription on the process: a signal raised at the test
// binary through the kernel reaches every Runner holding one, which is
// exactly what #7040 was about. What keeps them hearing only their own
// `kill $$` — which a Runner delivers to its own traps without kill(2) — is
// that every test raising a signal at this process is serial, and every
// serial test has finished before the first parallel one starts.
//
// That is a property of tests nobody has written yet, so it is read off the
// source rather than remembered. A parallel test fails here if it, or any
// function in this package's tests that it reaches, raises a signal at this
// process: kill(2) or tgkill(2) aimed at this process or its group, a handle
// on this process from os.FindProcess, or a script that sends one from
// outside — the `kill` program, or its parent's pid.
func TestNoParallelTestRaisesASignalAtThisProcess(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob("*_test.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 100 {
		t.Fatalf("found %d test files, want the whole package — the guard is looking in the wrong place", len(files))
	}
	srcs := map[string][]byte{}
	for _, file := range files {
		src, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		srcs[file] = src
	}
	found, parallel, err := parallelSignalRaisers(srcs)
	if err != nil {
		t.Fatal(err)
	}
	// The tree is mostly parallel tests, so a count far below that is a guard
	// that did not see the call it keys on.
	if parallel < 1000 {
		t.Fatalf("read %d parallel tests, want thousands — the guard is not seeing t.Parallel()", parallel)
	}
	for _, f := range found {
		t.Errorf("%s runs in parallel and raises a signal at this process (%s); "+
			"a test that does must stay serial, or the trapping tests beside it hear it", f.test, f.how)
	}
}

// TestTheSignalGuardCatchesWhatItIsFor hands the guard sources of its own,
// because over a tree that obeys the rule it reports nothing whether it works
// or not.
//
// It is serial, and has to be: its cases are scripts that send from outside,
// written as strings, and the guard reading the tree would find them in a
// parallel test.
func TestTheSignalGuardCatchesWhatItIsFor(t *testing.T) {
	const head = "package interp_test\n"
	for _, tc := range []struct {
		name, src string
		want      int
	}{
		{"tgkill at this process", "func TestX(t *testing.T) {\n\tt.Parallel()\n\t_ = syscall.Tgkill(syscall.Getpid(), syscall.Gettid(), syscall.SIGUSR1)\n}", 1},
		{"kill at this process", "func TestX(t *testing.T) {\n\tt.Parallel()\n\t_ = syscall.Kill(os.Getpid(), syscall.SIGUSR1)\n}", 1},
		{"kill at this process's group", "func TestX(t *testing.T) {\n\tt.Parallel()\n\t_ = syscall.Kill(-syscall.Getpgrp(), syscall.SIGUSR1)\n}", 1},
		{"kill at every process in the group", "func TestX(t *testing.T) {\n\tt.Parallel()\n\t_ = syscall.Kill(0, syscall.SIGUSR1)\n}", 1},
		{"a handle on this process", "func TestX(t *testing.T) {\n\tt.Parallel()\n\tp, _ := os.FindProcess(os.Getpid())\n\t_ = p\n}", 1},
		{"the kill program in a script", "func TestX(t *testing.T) {\n\tt.Parallel()\n\trun(t, `/bin/kill -USR1 $$`, nil)\n}", 1},
		{"the parent's pid in a script", "func TestX(t *testing.T) {\n\tt.Parallel()\n\trun(t, `sh -c 'kill -USR1 $PPID'`, nil)\n}", 1},
		{"the parent's pid read and not sent to", "func TestX(t *testing.T) {\n\tt.Parallel()\n\trun(t, `[ -n \"$PPID\" ] && echo have`, nil)\n}", 0},
		{"through a helper", "func TestX(t *testing.T) {\n\tt.Parallel()\n\tsend()\n}\nfunc send() { _ = syscall.Kill(os.Getpid(), syscall.SIGUSR1) }", 1},
		{"through a helper's helper", "func TestX(t *testing.T) {\n\tt.Parallel()\n\ta()\n}\nfunc a() { b() }\nfunc b() { _ = externalKill() }\nfunc externalKill() string { return \"/usr/bin/kill\" }", 1},
		// What the rule leaves alone: the same calls in a serial test, a
		// signal aimed at a child, and a script aiming at itself, which a
		// Runner never sends through the kernel.
		{"a serial test may", "func TestX(t *testing.T) {\n\t_ = syscall.Kill(os.Getpid(), syscall.SIGUSR1)\n}", 0},
		{"parallel rows of a serial test may", "func TestX(t *testing.T) {\n\tt.Run(\"r\", func(t *testing.T) {\n\t\tt.Parallel()\n\t\t_ = syscall.Kill(os.Getpid(), syscall.SIGUSR1)\n\t})\n}", 0},
		{"a child's pid", "func TestX(t *testing.T) {\n\tt.Parallel()\n\t_ = syscall.Kill(cmd.Process.Pid, syscall.SIGTERM)\n}", 0},
		{"a child's group", "func TestX(t *testing.T) {\n\tt.Parallel()\n\t_ = syscall.Kill(-pgid, syscall.SIGKILL)\n}", 0},
		{"a script aiming at itself", "func TestX(t *testing.T) {\n\tt.Parallel()\n\trun(t, `trap 'echo T' USR1; kill -USR1 $$`, nil)\n}", 0},
		{"a program killing itself", "func TestX(t *testing.T) {\n\tt.Parallel()\n\trun(t, `/bin/sh -c 'kill -USR1 $$'`, nil)\n}", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			found, _, err := parallelSignalRaisers(map[string][]byte{"case_test.go": []byte(head + tc.src + "\n")})
			if err != nil {
				t.Fatal(err)
			}
			if len(found) != tc.want {
				t.Errorf("found %v, want %d", found, tc.want)
			}
		})
	}
}

// signalRaiser is one parallel test that raises a signal at this process, and
// the first way the guard found it doing so.
type signalRaiser struct{ test, how string }

// parallelSignalRaisers reads test sources and returns every top-level test
// that calls t.Parallel() and raises a signal at this process, with the
// number of parallel tests it read. Functions are resolved by name within
// their own package, since the internal and external test packages each have
// helpers the other cannot call.
func parallelSignalRaisers(srcs map[string][]byte) (found []signalRaiser, parallel int, err error) {
	fset := token.NewFileSet()
	funcs := map[string]map[string]*ast.FuncDecl{}
	type test struct {
		pkg  string
		decl *ast.FuncDecl
	}
	var tests []test
	names := make([]string, 0, len(srcs))
	for name := range srcs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		f, err := parser.ParseFile(fset, name, srcs[name], parser.SkipObjectResolution)
		if err != nil {
			return nil, 0, err
		}
		pkg := f.Name.Name
		if funcs[pkg] == nil {
			funcs[pkg] = map[string]*ast.FuncDecl{}
		}
		for _, d := range f.Decls {
			fd, ok := d.(*ast.FuncDecl)
			if !ok || fd.Body == nil || fd.Recv != nil {
				continue
			}
			funcs[pkg][fd.Name.Name] = fd
			if strings.HasPrefix(fd.Name.Name, "Test") && fd.Name.Name != "TestMain" {
				tests = append(tests, test{pkg, fd})
			}
		}
	}
	for _, tc := range tests {
		if !callsParallelAtTheTop(tc.decl.Body) {
			continue
		}
		parallel++
		if how := raisesAtThisProcess(tc.decl.Body, funcs[tc.pkg], map[string]bool{}); how != "" {
			found = append(found, signalRaiser{tc.decl.Name.Name, how})
		}
	}
	return found, parallel, nil
}

// callsParallelAtTheTop reports whether a test's own body, not a row's, calls
// t.Parallel(): a serial test's parallel rows run while nothing else does.
func callsParallelAtTheTop(body *ast.BlockStmt) bool {
	for _, st := range body.List {
		if es, ok := st.(*ast.ExprStmt); ok {
			if c, ok := es.X.(*ast.CallExpr); ok {
				if sel, ok := c.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Parallel" && len(c.Args) == 0 {
					return true
				}
			}
		}
	}
	return false
}

// raisesAtThisProcess says how a body, or a function of the package it calls,
// raises a signal at this process, or "" if it does not.
func raisesAtThisProcess(body ast.Node, funcs map[string]*ast.FuncDecl, seen map[string]bool) (how string) {
	ast.Inspect(body, func(n ast.Node) bool {
		if how != "" {
			return false
		}
		switch n := n.(type) {
		case *ast.CallExpr:
			switch callee := selectorName(n.Fun); callee {
			case "syscall.Kill", "unix.Kill", "syscall.Tgkill", "unix.Tgkill", "os.FindProcess":
				if len(n.Args) > 0 && aimsAtThisProcess(n.Args[0]) {
					how = callee
					return false
				}
			}
			if id, ok := n.Fun.(*ast.Ident); ok && !seen[id.Name] {
				if fd := funcs[id.Name]; fd != nil {
					seen[id.Name] = true
					if h := raisesAtThisProcess(fd.Body, funcs, seen); h != "" {
						how = id.Name + ": " + h
						return false
					}
				}
			}
		case *ast.BasicLit:
			if n.Kind != token.STRING {
				break
			}
			s, err := strconv.Unquote(n.Value)
			if err != nil {
				break
			}
			// Each spelled in two halves, so that this function, which the
			// guard reaches from a parallel test, does not hold what it is
			// looking for.
			program, parent := "/bin/"+"kill", "$"+"PPID"
			if strings.Contains(s, program) || strings.Contains(s, parent) && strings.Contains(s, "kill") {
				how = "a script sending from outside"
				return false
			}
		}
		return true
	})
	return how
}

// aimsAtThisProcess reports whether a pid argument names this process or its
// group: anything asking for this process's pid or group, or 0, which is
// every process in the group.
func aimsAtThisProcess(arg ast.Expr) bool {
	if lit, ok := arg.(*ast.BasicLit); ok {
		return lit.Kind == token.INT && lit.Value == "0"
	}
	mine := false
	ast.Inspect(arg, func(n ast.Node) bool {
		if c, ok := n.(*ast.CallExpr); ok {
			switch selectorName(c.Fun) {
			case "os.Getpid", "syscall.Getpid", "unix.Getpid", "syscall.Getpgrp", "unix.Getpgrp":
				mine = true
			}
		}
		return !mine
	})
	return mine
}

// selectorName is "pkg.Name" for a call through a package selector.
func selectorName(e ast.Expr) string {
	sel, ok := e.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	x, ok := sel.X.(*ast.Ident)
	if !ok {
		return ""
	}
	return x.Name + "." + sel.Sel.Name
}
