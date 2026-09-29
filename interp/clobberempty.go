// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package interp

// ClobbersAnEmptyFile narrows `noclobber`: an existing regular file of size
// zero may be truncated by a plain `>` after all.
//
// A session switch rather than a Semantics axis because there is no
// disagreement to settle — one shell in the panel has the option and the
// other five have nothing to answer with — and because it is meaningless on
// its own. With `noclobber` off, `>` truncates whatever is there and this
// switch changes nothing that can be observed.
//
// Measured 2026-09-29 on zsh 5.9.2, the status and the file's contents
// reported on **standard error**, since the row under test is redirecting
// standard output at the file it is about:
//
//	                       noclobber   noclobber + clobberempty
//	file is empty          refused     truncated, status 0
//	file has bytes in it   refused     refused
//	file does not exist    created     created
//
// **The boundary is the size, and only for a regular file.** With both
// options on, a directory is still refused, and a device and a fifo are
// still allowed — but those two were already allowed by `noclobber` alone,
// so they say nothing about this switch. The one row that moves is a regular
// file of size zero; a symlink follows what it points at, which is the
// resolution every other open here uses.
//
// `>|` and `>>` are untouched, being the override and a different operator,
// and `&>` follows the same rule as `>`.
func (r *Runner) SetClobbersAnEmptyFile(on bool) { r.clobberEmpty = on }

// ClobbersAnEmptyFile reports that switch, for the option that sets it.
func (r *Runner) ClobbersAnEmptyFile() bool { return r.clobberEmpty }
