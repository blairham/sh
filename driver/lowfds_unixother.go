// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

//go:build unix && !linux

package driver

// lowDescriptorCeiling is 99 wherever the hold does not cost more than its
// fcntl calls: see lowfds_unix.go for why 99, and lowfds_linux.go for the one
// platform where it does.
//
// The grace period that makes Linux's table growth expensive is Linux's, and
// nothing measured on darwin shows the hold costing more than its fcntl
// calls, so it keeps every number bash can name up to 99.
const lowDescriptorCeiling = 99
