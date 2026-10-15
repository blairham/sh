// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build darwin || linux

package opened

import (
	"io/fs"
	"path/filepath"
	"syscall"
	"time"
)

// statInfo is the fs.FileInfo StatAt answers with: the raw stat, decomposed
// the way the os package decomposes one, so a caller cannot tell the two
// apart by anything it reads off the interface.
type statInfo struct {
	name string
	sys  syscall.Stat_t
}

func newStatInfo(name string, st *syscall.Stat_t) *statInfo {
	return &statInfo{name: filepath.Base(name), sys: *st}
}

func (s *statInfo) Name() string       { return s.name }
func (s *statInfo) Size() int64        { return s.sys.Size }
func (s *statInfo) ModTime() time.Time { return statModTime(&s.sys) }
func (s *statInfo) IsDir() bool        { return s.Mode().IsDir() }
func (s *statInfo) Sys() any           { return &s.sys }

// Mode is the os package's own mapping from st_mode, bit for bit: the
// permission bits, one type bit from S_IFMT, and the three special bits.
func (s *statInfo) Mode() fs.FileMode {
	raw := uint32(s.sys.Mode)
	mode := fs.FileMode(raw & 0o777)
	switch raw & syscall.S_IFMT {
	case syscall.S_IFBLK:
		mode |= fs.ModeDevice
	case syscall.S_IFCHR:
		mode |= fs.ModeDevice | fs.ModeCharDevice
	case syscall.S_IFDIR:
		mode |= fs.ModeDir
	case syscall.S_IFIFO:
		mode |= fs.ModeNamedPipe
	case syscall.S_IFLNK:
		mode |= fs.ModeSymlink
	case syscall.S_IFSOCK:
		mode |= fs.ModeSocket
	}
	if raw&syscall.S_ISGID != 0 {
		mode |= fs.ModeSetgid
	}
	if raw&syscall.S_ISUID != 0 {
		mode |= fs.ModeSetuid
	}
	if raw&syscall.S_ISVTX != 0 {
		mode |= fs.ModeSticky
	}
	return mode
}
