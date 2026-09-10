// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build darwin

package zsh

import (
	"fmt"
	"syscall"
)

// errnoNames is this platform's error names, indexed by errno number, with a
// hole at 0 that `$errnos` drops on its way out.
//
// The numbers are Go's `syscall` constants rather than literals, because the
// number belongs to the platform's headers and a literal here would be this
// machine's copy of them going stale silently. The *roster* — which names
// exist and how far the table runs — was measured against zsh 5.9.2's own
// `$errnos` on macOS 26: 107 entries, 1 to 107, with no holes.
//
// Two of the 107 are written as literals, and the reason is not the same for
// both. `ENOTCAPABLE` at 107 is newer than the table Go's darwin `syscall`
// package was generated from, so no build has a constant for it. `EQFULL` at
// 106 is declared for **arm64 and not for amd64** — the two architectures'
// tables in that package were generated at different times — and the errno
// numbering is the kernel's rather than the architecture's, so one literal
// covers both. That one was found by cross-compiling: this file built on the
// machine it was written on and not for the other half of what this project
// releases.
var errnoNames = buildErrnoNames(map[syscall.Errno]string{
	syscall.EPERM: "EPERM", syscall.ENOENT: "ENOENT", syscall.ESRCH: "ESRCH",
	syscall.EINTR: "EINTR", syscall.EIO: "EIO", syscall.ENXIO: "ENXIO",
	syscall.E2BIG: "E2BIG", syscall.ENOEXEC: "ENOEXEC", syscall.EBADF: "EBADF",
	syscall.ECHILD: "ECHILD", syscall.EDEADLK: "EDEADLK", syscall.ENOMEM: "ENOMEM",
	syscall.EACCES: "EACCES", syscall.EFAULT: "EFAULT", syscall.ENOTBLK: "ENOTBLK",
	syscall.EBUSY: "EBUSY", syscall.EEXIST: "EEXIST", syscall.EXDEV: "EXDEV",
	syscall.ENODEV: "ENODEV", syscall.ENOTDIR: "ENOTDIR", syscall.EISDIR: "EISDIR",
	syscall.EINVAL: "EINVAL", syscall.ENFILE: "ENFILE", syscall.EMFILE: "EMFILE",
	syscall.ENOTTY: "ENOTTY", syscall.ETXTBSY: "ETXTBSY", syscall.EFBIG: "EFBIG",
	syscall.ENOSPC: "ENOSPC", syscall.ESPIPE: "ESPIPE", syscall.EROFS: "EROFS",
	syscall.EMLINK: "EMLINK", syscall.EPIPE: "EPIPE", syscall.EDOM: "EDOM",
	syscall.ERANGE: "ERANGE", syscall.EAGAIN: "EAGAIN",
	syscall.EINPROGRESS: "EINPROGRESS", syscall.EALREADY: "EALREADY",
	syscall.ENOTSOCK: "ENOTSOCK", syscall.EDESTADDRREQ: "EDESTADDRREQ",
	syscall.EMSGSIZE: "EMSGSIZE", syscall.EPROTOTYPE: "EPROTOTYPE",
	syscall.ENOPROTOOPT: "ENOPROTOOPT", syscall.EPROTONOSUPPORT: "EPROTONOSUPPORT",
	syscall.ESOCKTNOSUPPORT: "ESOCKTNOSUPPORT", syscall.ENOTSUP: "ENOTSUP",
	syscall.EPFNOSUPPORT: "EPFNOSUPPORT", syscall.EAFNOSUPPORT: "EAFNOSUPPORT",
	syscall.EADDRINUSE: "EADDRINUSE", syscall.EADDRNOTAVAIL: "EADDRNOTAVAIL",
	syscall.ENETDOWN: "ENETDOWN", syscall.ENETUNREACH: "ENETUNREACH",
	syscall.ENETRESET: "ENETRESET", syscall.ECONNABORTED: "ECONNABORTED",
	syscall.ECONNRESET: "ECONNRESET", syscall.ENOBUFS: "ENOBUFS",
	syscall.EISCONN: "EISCONN", syscall.ENOTCONN: "ENOTCONN",
	syscall.ESHUTDOWN: "ESHUTDOWN", syscall.ETOOMANYREFS: "ETOOMANYREFS",
	syscall.ETIMEDOUT: "ETIMEDOUT", syscall.ECONNREFUSED: "ECONNREFUSED",
	syscall.ELOOP: "ELOOP", syscall.ENAMETOOLONG: "ENAMETOOLONG",
	syscall.EHOSTDOWN: "EHOSTDOWN", syscall.EHOSTUNREACH: "EHOSTUNREACH",
	syscall.ENOTEMPTY: "ENOTEMPTY", syscall.EPROCLIM: "EPROCLIM",
	syscall.EUSERS: "EUSERS", syscall.EDQUOT: "EDQUOT", syscall.ESTALE: "ESTALE",
	syscall.EREMOTE: "EREMOTE", syscall.EBADRPC: "EBADRPC",
	syscall.ERPCMISMATCH: "ERPCMISMATCH", syscall.EPROGUNAVAIL: "EPROGUNAVAIL",
	syscall.EPROGMISMATCH: "EPROGMISMATCH", syscall.EPROCUNAVAIL: "EPROCUNAVAIL",
	syscall.ENOLCK: "ENOLCK", syscall.ENOSYS: "ENOSYS", syscall.EFTYPE: "EFTYPE",
	syscall.EAUTH: "EAUTH", syscall.ENEEDAUTH: "ENEEDAUTH",
	syscall.EPWROFF: "EPWROFF", syscall.EDEVERR: "EDEVERR",
	syscall.EOVERFLOW: "EOVERFLOW", syscall.EBADEXEC: "EBADEXEC",
	syscall.EBADARCH: "EBADARCH", syscall.ESHLIBVERS: "ESHLIBVERS",
	syscall.EBADMACHO: "EBADMACHO", syscall.ECANCELED: "ECANCELED",
	syscall.EIDRM: "EIDRM", syscall.ENOMSG: "ENOMSG", syscall.EILSEQ: "EILSEQ",
	syscall.ENOATTR: "ENOATTR", syscall.EBADMSG: "EBADMSG",
	syscall.EMULTIHOP: "EMULTIHOP", syscall.ENODATA: "ENODATA",
	syscall.ENOLINK: "ENOLINK", syscall.ENOSR: "ENOSR", syscall.ENOSTR: "ENOSTR",
	syscall.EPROTO: "EPROTO", syscall.ETIME: "ETIME",
	syscall.EOPNOTSUPP: "EOPNOTSUPP", syscall.ENOPOLICY: "ENOPOLICY",
	syscall.ENOTRECOVERABLE: "ENOTRECOVERABLE", syscall.EOWNERDEAD: "EOWNERDEAD",
	darwinEQFULL:      "EQFULL",
	darwinENOTCAPABLE: "ENOTCAPABLE",
})

// The two errnos on this platform that Go's syscall package does not name on
// every architecture. Both measured through zsh's own `$errnos`, which is
// where the whole roster came from, and both at the end of the table — which
// is what makes it cheap to notice if the platform ever moves one.
const (
	darwinEQFULL      = syscall.Errno(106)
	darwinENOTCAPABLE = syscall.Errno(107)
)

// errnoTextOverrides is the sentence for a number this platform names and
// Go's table has no wording for. See errnoText.
//
// One entry, and it is the same 107 the roster above writes as a literal for
// the same reason: `ENOTCAPABLE` is newer than the table Go's darwin package
// was generated from, so no build has either its constant or its sentence.
// Measured against this platform's own `strerror`.
var errnoTextOverrides = map[int]string{107: "Capabilities insufficient"}

// errnoUnknownText is what this platform says about a number that names no
// error: zero, and anything past the end of the table.
//
// The colon is this platform's and is not universal — Linux writes `Unknown
// error 9999` with none — which is why this sentence is per platform rather
// than shared. Measured against `strerror` here: `Undefined error: 0` for
// zero, where Linux says `Success`.
func errnoUnknownText(n int) string {
	if n == 0 {
		return "Undefined error: 0"
	}
	return fmt.Sprintf("Unknown error: %d", n)
}
