// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package zsh

import (
	"fmt"
	"syscall"
)

// errnoNames is this platform's error names, indexed by errno number, with a
// hole at 0 that `$errnos` drops on its way out.
//
// **Not measured against a zsh on this platform**, and that is worth saying
// plainly rather than leaving to be inferred: the panel this project measures
// against runs on macOS, so the darwin table's roster came from a real
// `$errnos` and this one's did not. What is here instead is the platform's own
// numbering, taken from the constants Go's `syscall` package declares for it,
// which is the same source of truth zsh's table is generated from — the
// kernel's headers — reached without reading anybody's shell.
//
// The numbering runs 1 to 132 with two holes, at 41 and 58, which are numbers
// the platform leaves unused. They come out as empty strings, which is what a
// shell array with a hole in it reads as.
//
// Three numbers have two names, and the choice between them is this file's
// because no measurement settles it: `EAGAIN` over `EWOULDBLOCK` at 11,
// `EDEADLK` over `EDEADLOCK` at 35, and `EOPNOTSUPP` over `ENOTSUP` at 95. The
// rule is the same one each time — the name POSIX gives the condition, over
// the synonym the platform adds — and each pair is named here so that a reader
// comparing this against a Linux zsh knows which three rows to look at first.
var errnoNames = buildErrnoNames(map[syscall.Errno]string{
	syscall.EPERM: "EPERM", syscall.ENOENT: "ENOENT", syscall.ESRCH: "ESRCH",
	syscall.EINTR: "EINTR", syscall.EIO: "EIO", syscall.ENXIO: "ENXIO",
	syscall.E2BIG: "E2BIG", syscall.ENOEXEC: "ENOEXEC", syscall.EBADF: "EBADF",
	syscall.ECHILD: "ECHILD", syscall.EAGAIN: "EAGAIN", syscall.ENOMEM: "ENOMEM",
	syscall.EACCES: "EACCES", syscall.EFAULT: "EFAULT", syscall.ENOTBLK: "ENOTBLK",
	syscall.EBUSY: "EBUSY", syscall.EEXIST: "EEXIST", syscall.EXDEV: "EXDEV",
	syscall.ENODEV: "ENODEV", syscall.ENOTDIR: "ENOTDIR", syscall.EISDIR: "EISDIR",
	syscall.EINVAL: "EINVAL", syscall.ENFILE: "ENFILE", syscall.EMFILE: "EMFILE",
	syscall.ENOTTY: "ENOTTY", syscall.ETXTBSY: "ETXTBSY", syscall.EFBIG: "EFBIG",
	syscall.ENOSPC: "ENOSPC", syscall.ESPIPE: "ESPIPE", syscall.EROFS: "EROFS",
	syscall.EMLINK: "EMLINK", syscall.EPIPE: "EPIPE", syscall.EDOM: "EDOM",
	syscall.ERANGE: "ERANGE", syscall.EDEADLK: "EDEADLK",
	syscall.ENAMETOOLONG: "ENAMETOOLONG", syscall.ENOLCK: "ENOLCK",
	syscall.ENOSYS: "ENOSYS", syscall.ENOTEMPTY: "ENOTEMPTY", syscall.ELOOP: "ELOOP",
	syscall.ENOMSG: "ENOMSG", syscall.EIDRM: "EIDRM", syscall.ECHRNG: "ECHRNG",
	syscall.EL2NSYNC: "EL2NSYNC", syscall.EL3HLT: "EL3HLT", syscall.EL3RST: "EL3RST",
	syscall.ELNRNG: "ELNRNG", syscall.EUNATCH: "EUNATCH", syscall.ENOCSI: "ENOCSI",
	syscall.EL2HLT: "EL2HLT", syscall.EBADE: "EBADE", syscall.EBADR: "EBADR",
	syscall.EXFULL: "EXFULL", syscall.ENOANO: "ENOANO", syscall.EBADRQC: "EBADRQC",
	syscall.EBADSLT: "EBADSLT", syscall.EBFONT: "EBFONT", syscall.ENOSTR: "ENOSTR",
	syscall.ENODATA: "ENODATA", syscall.ETIME: "ETIME", syscall.ENOSR: "ENOSR",
	syscall.ENONET: "ENONET", syscall.ENOPKG: "ENOPKG", syscall.EREMOTE: "EREMOTE",
	syscall.ENOLINK: "ENOLINK", syscall.EADV: "EADV", syscall.ESRMNT: "ESRMNT",
	syscall.ECOMM: "ECOMM", syscall.EPROTO: "EPROTO", syscall.EMULTIHOP: "EMULTIHOP",
	syscall.EDOTDOT: "EDOTDOT", syscall.EBADMSG: "EBADMSG",
	syscall.EOVERFLOW: "EOVERFLOW", syscall.ENOTUNIQ: "ENOTUNIQ",
	syscall.EBADFD: "EBADFD", syscall.EREMCHG: "EREMCHG", syscall.ELIBACC: "ELIBACC",
	syscall.ELIBBAD: "ELIBBAD", syscall.ELIBSCN: "ELIBSCN", syscall.ELIBMAX: "ELIBMAX",
	syscall.ELIBEXEC: "ELIBEXEC", syscall.EILSEQ: "EILSEQ",
	syscall.ERESTART: "ERESTART", syscall.ESTRPIPE: "ESTRPIPE",
	syscall.EUSERS: "EUSERS", syscall.ENOTSOCK: "ENOTSOCK",
	syscall.EDESTADDRREQ: "EDESTADDRREQ", syscall.EMSGSIZE: "EMSGSIZE",
	syscall.EPROTOTYPE: "EPROTOTYPE", syscall.ENOPROTOOPT: "ENOPROTOOPT",
	syscall.EPROTONOSUPPORT: "EPROTONOSUPPORT",
	syscall.ESOCKTNOSUPPORT: "ESOCKTNOSUPPORT", syscall.EOPNOTSUPP: "EOPNOTSUPP",
	syscall.EPFNOSUPPORT: "EPFNOSUPPORT", syscall.EAFNOSUPPORT: "EAFNOSUPPORT",
	syscall.EADDRINUSE: "EADDRINUSE", syscall.EADDRNOTAVAIL: "EADDRNOTAVAIL",
	syscall.ENETDOWN: "ENETDOWN", syscall.ENETUNREACH: "ENETUNREACH",
	syscall.ENETRESET: "ENETRESET", syscall.ECONNABORTED: "ECONNABORTED",
	syscall.ECONNRESET: "ECONNRESET", syscall.ENOBUFS: "ENOBUFS",
	syscall.EISCONN: "EISCONN", syscall.ENOTCONN: "ENOTCONN",
	syscall.ESHUTDOWN: "ESHUTDOWN", syscall.ETOOMANYREFS: "ETOOMANYREFS",
	syscall.ETIMEDOUT: "ETIMEDOUT", syscall.ECONNREFUSED: "ECONNREFUSED",
	syscall.EHOSTDOWN: "EHOSTDOWN", syscall.EHOSTUNREACH: "EHOSTUNREACH",
	syscall.EALREADY: "EALREADY", syscall.EINPROGRESS: "EINPROGRESS",
	syscall.ESTALE: "ESTALE", syscall.EUCLEAN: "EUCLEAN", syscall.ENOTNAM: "ENOTNAM",
	syscall.ENAVAIL: "ENAVAIL", syscall.EISNAM: "EISNAM",
	syscall.EREMOTEIO: "EREMOTEIO", syscall.EDQUOT: "EDQUOT",
	syscall.ENOMEDIUM: "ENOMEDIUM", syscall.EMEDIUMTYPE: "EMEDIUMTYPE",
	syscall.ECANCELED: "ECANCELED", syscall.ENOKEY: "ENOKEY",
	syscall.EKEYEXPIRED: "EKEYEXPIRED", syscall.EKEYREVOKED: "EKEYREVOKED",
	syscall.EKEYREJECTED: "EKEYREJECTED", syscall.EOWNERDEAD: "EOWNERDEAD",
	syscall.ENOTRECOVERABLE: "ENOTRECOVERABLE", syscall.ERFKILL: "ERFKILL",
})

// errnoTextOverrides is empty here: every number this platform's roster names
// has a sentence in Go's own table. See errnoText, and the darwin file, which
// has one entry and says why.
var errnoTextOverrides = map[int]string{}

// errnoUnknownText is what this platform says about a number that names no
// error — zero, the two holes at 41 and 58, and anything past the end.
//
// Measured against glibc's `strerror` in a container rather than against a
// zsh, for the reason the roster above gives: the panel this project measures
// runs on macOS. Zero is `Success` where darwin says `Undefined error: 0`, and
// an unnamed number is `Unknown error 9999` **without a colon**, where darwin
// writes one.
func errnoUnknownText(n int) string {
	if n == 0 {
		return "Success"
	}
	return fmt.Sprintf("Unknown error %d", n)
}
