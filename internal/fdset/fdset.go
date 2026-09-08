// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

// Package fdset asks the kernel which descriptors a read would not wait on.
//
// Two questions, and they are the same question asked with two different
// amounts of patience:
//
//   - ReadableNow, of one descriptor, waiting no time at all. It is what an
//     interpreter asks before a read it does not want to block on.
//   - Wait, of a set of them, waiting until something happens. It is what a
//     line editor asks so that it can sit idle on a terminal and still notice
//     a descriptor a shell asked it to watch.
//
// **They are one package because they are one mechanism.** The descriptor set,
// how many descriptors one of its words holds, and the fact that this call's
// result is spelled differently on Linux than on the BSDs are the substrate
// underneath both, and the substrate is where the platform mistakes live. A
// second copy of it beside the first is the shape of bug this tree has now
// found four times: the copy is written from the same understanding and then
// the fix goes into only one of them. The callers are in packages that cannot
// see each other's unexported names — an interpreter and a line editor — so
// the shared thing has to be a package.
//
// Neither question ever reads. That is the whole point of both: a poll that
// consumed the byte it reported would leave a caller eating its own input.
package fdset

import "errors"

// ErrUnsupported is what Wait answers on a system with no descriptor set to
// wait on. A caller that cannot wait can still read; see Wait.
var ErrUnsupported = errors.New("waiting on descriptors is not supported on this system")
