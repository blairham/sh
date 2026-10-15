// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package opened

import (
	"errors"
	"io/fs"
	"os"
)

// StatAtSupported says whether StatAt and AccessAt can answer on this
// platform. Where it is false both return errStatAtUnsupported and a caller
// asks by name instead, which is what it did before these existed.
const StatAtSupported = statAtSupported

var errStatAtUnsupported = errors.New("opened: no stat relative to a descriptor on this platform")

// StatAt asks about name **from the directory dir holds** rather than from
// the place dir's name currently leads: fstatat(2), following a final symbolic
// link when follow is set and answering for the link itself when it is not.
//
// It is the question a process whose working directory is dir puts when it
// stats a relative path, and that is the whole reason it is here. A shell
// that keeps its directory as a descriptor (interp's heldDirectory) can ask
// a relative path's question in one call this way, where joining a name and
// stating the result took two — one to check the name still leads to the
// descriptor, one for the path — and gave a different answer from every real
// shell once the name had been taken by a different directory (#6494).
//
// The FileInfo it returns carries a *syscall.Stat_t in Sys, as os.Stat's does,
// so the readers in interp that decompose a stat — owner, access time — read
// it unchanged. It is not one os.SameFile recognizes; a caller comparing
// identities uses os.Stat.
func StatAt(dir *os.File, name string, follow bool) (fs.FileInfo, error) {
	if !statAtSupported {
		return nil, errStatAtUnsupported
	}
	conn, err := dir.SyscallConn()
	if err != nil {
		return nil, err
	}
	var (
		info    fs.FileInfo
		statErr error
	)
	if err := conn.Control(func(dirfd uintptr) {
		info, statErr = retrying(func() (fs.FileInfo, error) {
			return statAtOnce(int(dirfd), name, follow)
		})
	}); err != nil {
		return nil, err
	}
	if statErr != nil {
		return nil, &fs.PathError{Op: "fstatat", Path: name, Err: statErr}
	}
	return info, nil
}

// AccessAt is access(2) asked from the directory dir holds: faccessat(2) with
// the real identity, which is the question access(2) itself puts. mode is
// the R_OK/W_OK/X_OK bits.
func AccessAt(dir *os.File, name string, mode uint32) error {
	if !statAtSupported {
		return errStatAtUnsupported
	}
	conn, err := dir.SyscallConn()
	if err != nil {
		return err
	}
	var accessErr error
	if err := conn.Control(func(dirfd uintptr) {
		_, accessErr = retrying(func() (struct{}, error) {
			return struct{}{}, accessAtOnce(int(dirfd), name, mode)
		})
	}); err != nil {
		return err
	}
	return accessErr
}
