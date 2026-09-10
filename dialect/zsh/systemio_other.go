// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build !unix

package zsh

import "os"

// A platform with no descriptor of its own to hand a script, which is the
// shape socketmodule_other.go has and for the same reason: the three names
// are not registered at all, so `zmodload -F zsh/system b:sysopen` refuses by
// the builtin's name rather than loading a module with nothing behind it.

const systemIOSupported = false

func sysopenFlag(string) (flag int, cloexec, known bool) { return 0, false, false }

func sysOpenFile(path string, flags int, perm os.FileMode) (*os.File, error) {
	return os.OpenFile(path, flags, perm)
}
