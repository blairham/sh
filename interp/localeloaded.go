// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// A locale variable names a locale; whether the shell is then *in* it is the C
// library's answer, and a name the library has no data for leaves the shell in
// the locale it was already in. Measured 2026-10-02 (#5503), `x=é; echo ${#x}`
// after each assignment:
//
//	macOS, `env -i`, zsh 5.9.2 and ksh93u+
//	  LC_ALL=xx_XX.UTF-8 → 2, LC_ALL=en_US.UTF-8 → 1, LC_ALL=xx_XX.UTF-8 → 1
//	macOS, bash 5.3.20: `LC_ALL=C` → 2, then `LC_ALL=xx` → 2 (and bash warns
//	  `cannot change locale`)
//	debian sid-slim with only C, C.utf8 and POSIX installed, bash 5.3.15 and
//	  zsh 5.9.2: LC_ALL=C.UTF-8 → 1, then LC_ALL=en_US.UTF-8 → 1
//	the same image, zsh: LC_ALL=en_US.UTF-8 alone → 2, where this read 1
//	dash: bytes throughout
//	alpine (musl), BusyBox ash: every name but C and POSIX is UTF-8, even
//	  xx_XX.ISO8859-1, because musl loads any name and has no data to refuse
//
// and the encoding is the data's, not the name's: on macOS `en_US`'s LC_CTYPE
// is UTF-8, so `LC_ALL=en_US` is 1 there, and `LC_ALL=UTF-8` names only an
// LC_CTYPE, so it fails as a whole locale and is 2 while `LC_CTYPE=UTF-8` is 1.
//
// Answered through Runner.LocaleCharset, which a front end that is a shell
// fills in from the host and a library leaves nil — then the name is read as
// written, which is what this package did before.

// ctypeInForce is the locale name in force for the character type and its
// codeset, as Runner.LocaleCharset settles it: the name the variables point at
// where the library would load it, and otherwise the one in force before.
//
// Settled when it is read rather than at every assignment: a name that fails
// has no effect, so what the reader sees is the last name that loaded. Only a
// sequence of assignments with no read between them can tell the two apart —
// `LC_ALL=C; LC_ALL=xx` read once is C in the shells and here the name before
// the `C` — and that is the one difference, noted rather than modeled.
func (r *Runner) ctypeInForce() (name, codeset string) {
	cand, whole := "", false
	for _, v := range localeVariables {
		if s, _ := r.getVar(v); s != "" {
			cand, whole = s, v != "LC_CTYPE"
			break
		}
	}
	if r.ctype.settled && cand == r.ctype.asked && whole == r.ctype.whole {
		return r.ctype.name, r.ctype.codeset
	}
	r.ctype.settled, r.ctype.asked, r.ctype.whole = true, cand, whole
	switch cand {
	case "":
		r.ctype.name, r.ctype.codeset = "", ""
	case "C", "POSIX":
		r.ctype.name, r.ctype.codeset = cand, ""
	default:
		if codeset, ok := r.LocaleCharset(cand, whole); ok {
			r.ctype.name, r.ctype.codeset = cand, codeset
		}
		// Otherwise the locale in force stays in force.
	}
	return r.ctype.name, r.ctype.codeset
}

// ctypeState is what ctypeInForce settled: the candidate it was last asked
// about and whether as a whole locale — `UTF-8` is a character type on macOS
// and not a locale, so the same name can load through LC_CTYPE and fail
// through LC_ALL — and the name and codeset in force.
type ctypeState struct {
	settled, whole bool
	asked          string
	name, codeset  string
}
