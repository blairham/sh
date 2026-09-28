// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"runtime"
	"strings"
	"testing"
)

// The platform table, graded against what a reference answers on each
// platform — **every row on both runners**, which is the reason `tripleFor`
// takes the platform rather than reading it.
//
// A table that asked `runtime.GOOS` could only ever assert the row of the
// machine it ran on, so the other rows would be graded by nothing and would
// read exactly like rows that agreed. Handing the platform in makes the Linux
// rows a real assertion on a Mac and the Darwin row a real assertion on the
// Linux runner.
//
// The reference figures and the image digest are in buildTriple's own
// comment, along with which rows were measured and which are derived.
func TestTheBuildTripleIsWhatEachPlatformAnswers(t *testing.T) {
	for _, c := range []struct {
		name                         string
		goos, goarch, release        string
		cpu, machine, vendor, ostype string
	}{
		{
			// zsh 5.9.2 on this machine, with that build's own release, and
			// the one row where the kernel's machine word and the triple's
			// CPU are different words.
			"darwin on 64-bit ARM", "darwin", "arm64", "25.4.0",
			"arm64", "aarch64", "apple", "darwin25.4.0",
		},
		{
			// Debian's zsh 5.9 in the pinned image. The vendor there is
			// `debian` and here is `unknown`, which is what this build is —
			// and `unknown` is one of the two spellings that image's own two
			// builds use.
			"linux on x86-64", "linux", "amd64", "",
			"x86_64", "x86_64", "unknown", "linux-gnu",
		},
		{
			// The same image on the other architecture, where the reference
			// itself says `unknown`.
			"linux on 64-bit ARM", "linux", "arm64", "",
			"aarch64", "aarch64", "unknown", "linux-gnu",
		},
		{
			// Derived rather than measured — there is no Intel Mac here and
			// no zsh of that build to ask — from the rule the three rows
			// above state. It is in the table so that the derivation is a
			// row somebody can contradict rather than a sentence in a
			// comment.
			"darwin on x86-64", "darwin", "amd64", "23.0.0",
			"x86_64", "x86_64", "apple", "darwin23.0.0",
		},
		{
			// A kernel that will not say its release leaves the bare
			// platform name, which is still the prefix every script that
			// branches on this parameter reads.
			"darwin with no release to ask for", "darwin", "arm64", "",
			"arm64", "aarch64", "apple", "darwin",
		},
		{
			// A platform nobody has measured is still four words.
			"a platform with no row", "plan9", "amd64", "",
			"x86_64", "x86_64", "unknown", "plan9",
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := tripleFor(c.goos, c.goarch, c.release)
			want := buildTriple{cpu: c.cpu, machine: c.machine, vendor: c.vendor, ostype: c.ostype}
			if got != want {
				t.Errorf("tripleFor(%q, %q, %q) = %+v, want %+v", c.goos, c.goarch, c.release, got, want)
			}
		})
	}
}

// And the row this binary actually holds is one of them, which is the half a
// table of literals cannot say: a table nothing consulted would pass every
// row above while the shell answered something else.
//
// It asserts the relation rather than a literal, so it is true on either
// runner: the kernel's release is the machine's and cannot be written down.
func TestTheRunningPlatformTakesARowOfTheTable(t *testing.T) {
	got := tripleFor(runtime.GOOS, runtime.GOARCH, kernelRelease())
	for _, f := range []struct{ name, value string }{
		{"cpu", got.cpu},
		{"machine", got.machine},
		{"vendor", got.vendor},
		{"ostype", got.ostype},
	} {
		if f.value == "" {
			t.Errorf("%s is empty on %s/%s", f.name, runtime.GOOS, runtime.GOARCH)
		}
	}
	if !strings.HasPrefix(got.ostype, runtime.GOOS) {
		t.Errorf("ostype = %q, want it to start with %q — a script branches on the prefix",
			got.ostype, runtime.GOOS)
	}
	if got.machine != canonicalCPU(runtime.GOARCH) {
		t.Errorf("machine = %q, want the triple's CPU %q", got.machine, canonicalCPU(runtime.GOARCH))
	}
}
