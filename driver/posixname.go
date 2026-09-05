// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package driver

import "strings"

// What a shell was *called* is the front end's fact, the same as how it
// arrived. `$0`, the alias route and login-ness are already read here for that
// reason, and this is the fourth of them: a shell invoked under the standard's
// own name starts in POSIX mode.
//
// It is the front end's rather than a dialect's because a dialect answers
// "which shell am I", and this is a question about one invocation of it. The
// same binary answers it both ways depending on the word it was exec'd with,
// so an axis keyed on it would write the accident down and lose the rule —
// which is the reasoning that put the mode in the core in the first place, and
// what #691 left for this to finish.

// PosixNamed reports whether the invocation was made under the standard's own
// name for the shell.
//
// The name is the last element of argv[0], with one leading dash removed —
// `sh`, `/bin/sh`, `./sh` and the login spelling `-sh` are all it, and `shx`,
// `bsh`, `SH` and `--sh` are not.
//
// Both halves of that are measured rather than assumed. A path is reduced to
// its last element because `/bin/sh` and `/usr/bin/sh` both mean it, and one
// dash is stripped because `login` and every terminal emulator's "run as a
// login shell" prepend exactly one — `-/bin/sh` is the name too, and `--sh` is
// not, which is what says one and not any number.
func PosixNamed(argv []string) bool {
	if len(argv) == 0 {
		return false
	}
	name := strings.TrimPrefix(argv[0], "-")
	if i := strings.LastIndexByte(name, '/'); i >= 0 {
		name = name[i+1:]
	}
	return name == "sh"
}
