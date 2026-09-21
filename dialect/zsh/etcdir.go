// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package zsh

import (
	"os"
	"path/filepath"
)

// SystemStartupDirectory is where this machine keeps zsh's own system-wide
// startup files, given the directory the machine's administrator owns —
// `/etc` on every Unix anyone runs this on.
//
// It exists because zsh is the one shell in the panel whose system directory
// is not that directory. The files named by [Semantics]'s SystemStartupFiles
// — `zshenv`, `zprofile`, `zshrc`, `zlogin` — live in a directory chosen when
// zsh itself was built, `--enable-etcdir`, and the two platforms this is run
// on answer it differently:
//
//	macOS 26, zsh 5.9.2        /etc/zprofile, /etc/zshrc; no /etc/zsh
//	Debian sid, zsh 5.9.2      /etc/zsh/{zshenv,zprofile,zshrc,zlogin}
//
// Both measured 2026-09-21, the second in the digest-pinned image the suite's
// zsh column is graded against, where `zsh -o sourcetrace -c :` opens with
// `+/etc/zsh/zshenv:1> <sourcetrace>` and nothing named `/etc/zsh*` exists
// beside the directory. Until then our `zsh` looked in `/etc` on both and so
// read the administrator's files in **none** of its four slots on Linux —
// and Debian's `/etc/zsh/zshenv` is where `$PATH` is set for a shell started
// without one, so the gap was a startup gap and not a traced line (#3987).
//
// # Why a probe and not a build tag
//
// Because it is not a fact about the operating system. Debian and Arch build
// zsh with an etcdir of `/etc/zsh`; Fedora and macOS do not; a binary built
// here and run in a container is answering for the machine it lands on rather
// than the one it was compiled on. `GOOS` cannot see any of that, and would
// be a guess that is wrong on the first Linux that packages zsh the other way.
// The directory's own existence is the fact, it is one `stat` at startup, and
// it is what driver.Shell.SystemStartupDirectory already says it wants: the
// install's answer, which zsh's manual states outright — the files "may be in
// another directory, depending on the installation".
//
// A directory rather than a file, and any of the four rather than a
// particular one: a machine with `/etc/zsh` has zsh's etcdir there whether or
// not the administrator has filled every slot, and looking for `zshenv`
// specifically would fall back to `/etc` on a Debian that ships only a
// `zshrc` and then read nothing at all.
//
// The empty string is returned unchanged, because that is the front end's way
// of saying it reads no system files at all — see
// driver.Shell.SystemStartupDirectory, whose zero value is that and whose
// reason is that these are absolute paths into a real machine.
func SystemStartupDirectory(etc string) string {
	if etc == "" {
		return ""
	}
	own := filepath.Join(etc, "zsh")
	if info, err := os.Stat(own); err == nil && info.IsDir() {
		return own
	}
	return etc
}
