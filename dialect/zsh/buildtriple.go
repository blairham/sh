// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

// The four words a shell's **build triple** gives it, and they are four
// separate facts rather than one string cut up.
//
// `$MACHTYPE`, `$VENDOR` and `$OSTYPE` are the three components of the triple
// the shell was configured for, and `$CPUTYPE` is the machine word the kernel
// reports — which is not the same as the triple's CPU and is measured to
// differ on one of the platforms here.
//
// # What each is, measured
//
// 2026-09-27, one run per column: `/opt/homebrew/bin/zsh`, `zsh 5.9.2
// (aarch64-apple-darwin25.4.0)`, and Debian's own `zsh 5.9` in a
// digest-pinned `debian:bookworm-slim`
// (sha256:3783cc01769c7b2b1b83a5c5ad96c815348e28ed7da68e2e3687004faa906251)
// on both architectures, each under `env -i PATH=/usr/bin:/bin`:
//
//	platform        CPUTYPE   MACHTYPE   VENDOR    OSTYPE          uname -m
//	darwin/arm64    arm64     aarch64    apple     darwin25.4.0    arm64
//	linux/amd64     x86_64    x86_64     debian    linux-gnu       x86_64
//	linux/arm64     aarch64   aarch64    unknown   linux-gnu       aarch64
//
// Three findings are in that table and none of them is derivable from the
// others:
//
//   - **`CPUTYPE` is `uname -m` and `MACHTYPE` is the triple's CPU**, and
//     they differ on exactly one row — `arm64` against `aarch64` on macOS.
//     A rule read off either Linux row would have written the same word
//     twice and been wrong there.
//   - **`OSTYPE` carries the release on one platform and not on the other.**
//     `darwin25.4.0` names a kernel version; `linux-gnu` names a C library.
//     It is the triple's operating-system component, whatever shape that
//     takes, rather than a name plus a number.
//   - **`VENDOR` is the packager**, and Debian's own two builds disagree with
//     each other about it: `debian` on amd64 and `unknown` on arm64, in the
//     same distribution at the same version. So there is no vendor to derive
//     and the only honest answer for a build is the one its builder gives.
//
// # What this build says, and where it is derived rather than measured
//
// `apple` is the vendor on Darwin because every triple on that platform has
// it; `unknown` is the vendor on Linux because this is not a distribution's
// package, and it is one of the two spellings measured above rather than an
// invention. The `darwin/amd64` row is **derived** — there is no Intel Mac
// here to measure and no zsh of that build to ask — from the rule the other
// three rows state: the kernel's machine word for `CPUTYPE`, the canonical
// CPU for `MACHTYPE`, `apple`, and `darwin` plus the release.
//
// **And the release is this kernel's rather than the one this binary was
// built against**, which is a difference from the reference and is stated
// rather than glossed: zsh records the release its `configure` ran on, so the
// reference reads `darwin25.4.0` on a machine whose `uname -r` is `25.6.0`.
// A Go binary carries no such record, and the running kernel is the closest
// true statement about where this shell is — and is exactly what every script
// that branches on this parameter reads, since `[[ $OSTYPE == darwin* ]]`
// asks about the prefix.
type buildTriple struct {
	// cpu is `$CPUTYPE`: the machine word the kernel reports.
	cpu string
	// machine is `$MACHTYPE`: the canonical CPU of the triple.
	machine string
	// vendor is `$VENDOR`: who built it.
	vendor string
	// ostype is `$OSTYPE`: the triple's operating-system component.
	ostype string
}

// tripleFor is the table, taking the platform rather than reading it, so that
// a test on either runner can grade the rows measured on the other.
//
// The release is handed in for the same reason: a pure function of three
// words has a row a test can state, where one that asked the kernel could
// only ever assert what the machine it ran on happens to say.
func tripleFor(goos, goarch, release string) buildTriple {
	t := buildTriple{cpu: kernelMachineWord(goos, goarch), machine: canonicalCPU(goarch)}
	switch goos {
	case "darwin":
		t.vendor, t.ostype = "apple", "darwin"+release
	case "linux":
		t.vendor, t.ostype = "unknown", "linux-gnu"
	default:
		// A platform nobody has measured. The words are still the two facts
		// this function does know — the architecture is not the operating
		// system's to say — and the operating system's own name is a better
		// answer than an empty one for a script that branches on the prefix.
		t.vendor, t.ostype = "unknown", goos
	}
	return t
}

// kernelMachineWord is what `uname -m` answers, which is `$CPUTYPE`.
//
// The one row that is not the canonical CPU is macOS on 64-bit ARM, where the
// kernel says `arm64` and the triple says `aarch64` — measured above, in the
// same run as the Linux rows that say the same word twice.
func kernelMachineWord(goos, goarch string) string {
	if goos == "darwin" && goarch == "arm64" {
		return "arm64"
	}
	return canonicalCPU(goarch)
}

// canonicalCPU is the CPU component of a GNU configuration triple, which is
// `$MACHTYPE`.
//
// Two rows are measured — `amd64` reading `x86_64` and `arm64` reading
// `aarch64` — and the rest follow the same convention, which is that the
// triple names the architecture the way the toolchain does rather than the
// way Go does. An architecture with no row is written as Go spells it, which
// is a word rather than a silence.
func canonicalCPU(goarch string) string {
	switch goarch {
	case "amd64":
		return "x86_64"
	case "arm64":
		return "aarch64"
	case "386":
		return "i386"
	}
	return goarch
}
