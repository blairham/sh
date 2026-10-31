// SPDX-FileCopyrightText: 2026 Blair Hamilton
// SPDX-License-Identifier: Apache-2.0

package testenv

import (
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"
)

// keepCrashReportsOut answers, on this process's behalf, the crash of every
// process it starts, so that macOS writes no report of it into the person's
// Library.
//
// # Why a suite needs it
//
// Several suites re-execute their own binary to be a shell that a signal
// kills — QUIT, ABRT, SEGV, BUS and the rest of the class whose default action
// dumps core — because the death is what is under test. To macOS each of those
// is a crash: the kernel raises an EXC_CRASH exception on the way out, the
// host's handler passes it to ReportCrash, and a .ips file lands in
// ~/Library/Logs/DiagnosticReports. One local run of one cross-dialect test
// left twenty there (#7154). That is a test writing to real user state, which
// is what this package exists to stop.
//
// RLIMIT_CORE, which those shell halves already set to 0, stops a core file
// and not the report. Nor does anything the dying process can do for itself:
// measured, a process that registers a crash handler of its own and then dies
// still gets a report.
//
// # What does
//
// An exception goes to the dying task's own handler before the host's, and a
// handler that answers KERN_SUCCESS has handled it, so the host's handler and
// ReportCrash behind it are never asked. Task exception ports are inherited
// across fork and kept across exec, so a port this process registers before it
// starts anything is the port every shell below it dies through, and
// answerCrashes answers each crash that arrives there. The death itself is
// untouched: the kernel goes on to end the process by its signal, and the
// parent reads the same wait status it always did.
//
// Measured with one dying row, `TestACrashingSignalKillsTheShellQuietly`, from
// a driver test binary copied to a name with no reports yet: one .ips and one
// ReportCrash entry in the unified log without this, none of either with it.
// The copy matters. macOS keeps at most 25 reports per process name, and for
// `driver.test` the cap had already been reached, so counting files under that
// name read zero with or without the handler. The log still showed ReportCrash
// handling every death, so a zero under that name is the cap, not this fix.
//
// # The handler must not stop
//
// The exception is a request the kernel waits on. A dying process whose
// handler holds the port and never answers does not finish dying — measured,
// not even SIGKILL lets its parent reap it — so a stalled handler would turn
// every death below it into a hang. answerCrashes runs for the life of the
// process, and if it ever has to stop it gives up the port, which sends any
// later death back to the host's handler instead of leaving it waiting.
//
// # Only in the process that started the suite
//
// The port has to belong to a process that outlives the deaths, which is why
// Run calls this only when the environment was not already assembled. A
// re-executed child that registered a port of its own would die through it,
// and would get the report anyway.
//
// # Without cgo
//
// The Mach calls are libSystem functions, reached the way package syscall
// reaches libc on darwin: each one is imported by name, with a trampoline in
// crashreports_darwin.s, and called through the runtime's libc entry.
func keepCrashReportsOut() {
	crashHandler.Do(registerCrashHandler)
}

var crashHandler sync.Once

// crashesAnswered counts the crashes that reached answerCrashes, which is how
// a test sees that a death came here rather than going to the host. Each is
// counted before it is answered: the dying process cannot finish until the
// answer goes, so a parent whose wait has returned always finds it counted.
var crashesAnswered atomic.Int64

func registerCrashHandler() {
	task, _, _ := libcCall(taskSelfTrapAddr, 0, 0, 0)
	if kr, _, _ := libcCall(machPortAllocateAddr, task, machPortRightReceive,
		uintptr(unsafe.Pointer(&crashPort))); kr != kernSuccess {
		return
	}
	if kr, _, _ := libcCall6(machPortInsertRightAddr, task, uintptr(crashPort), uintptr(crashPort),
		machMsgTypeMakeSend, 0, 0); kr != kernSuccess {
		releaseCrashPort(task)
		return
	}
	if kr, _, _ := libcCall6(taskSetExceptionPortsAddr, task, excMaskCrash, uintptr(crashPort),
		exceptionDefault, threadStateNone(), 0); kr != kernSuccess {
		releaseCrashPort(task)
		return
	}
	go answerCrashes(task)
}

// answerCrashes receives each crash sent to crashPort and answers that it was
// handled.
func answerCrashes(task uintptr) {
	defer releaseCrashPort(task)
	head := (*machMsgHeader)(unsafe.Pointer(&crashRequest))
	for {
		*head = machMsgHeader{size: uint32(unsafe.Sizeof(crashRequest)), local: crashPort}
		switch kr, _, _ := libcCall(machMsgReceiveAddr, uintptr(unsafe.Pointer(head)), 0, 0); kr {
		case kernSuccess:
		case machRcvInterrupted, machRcvTooLarge:
			// A message too large for the buffer has been destroyed, which
			// hands that one crash back to the host; the next is ours.
			continue
		default:
			return
		}
		crashesAnswered.Add(1)
		// The reply MIG would build: the request's id plus 100, sent on the
		// reply right the request carried, with the code that says handled.
		crashReply = exceptionReply{
			head: machMsgHeader{
				bits:   head.bits & machMsghBitsRemoteMask,
				size:   uint32(unsafe.Sizeof(crashReply)),
				remote: head.remote,
				id:     head.id + 100,
			},
			ndr:     ndrRecord,
			retCode: kernSuccess,
		}
		_, _, _ = libcCall(machMsgSendAddr, uintptr(unsafe.Pointer(&crashReply)), 0, 0)
		// The request also carried send rights to the dead process's thread
		// and task. The reply right has been spent, so it is cleared before
		// the rest is released, or each death would leave its task behind.
		head.remote = 0
		_, _, _ = libcCall(machMsgDestroyAddr, uintptr(unsafe.Pointer(head)), 0, 0)
	}
}

// releaseCrashPort gives up the receive right, which is what turns a death
// that would wait on this process into one the host's handler answers.
func releaseCrashPort(task uintptr) {
	_, _, _ = libcCall6(machPortModRefsAddr, task, uintptr(crashPort), machPortRightReceive,
		uintptr(minusOne), 0, 0)
}

// The port and both messages live here rather than on a stack, so that the
// addresses handed to libSystem cannot move while it holds them.
var (
	crashPort    uint32
	crashRequest [256]uint32
	crashReply   exceptionReply
)

// minusOne is the reference delta that releases a right.
var minusOne int32 = -1

// machMsgHeader is mach_msg_header_t.
type machMsgHeader struct {
	bits, size             uint32
	remote, local, voucher uint32
	id                     int32
}

// exceptionReply is the reply to exception_raise: a header, the NDR record and
// a return code.
type exceptionReply struct {
	head    machMsgHeader
	ndr     [8]byte
	retCode int32
}

// ndrRecord is NDR_record: everything zero but the integer representation,
// which is little-endian on every Mac this builds for.
var ndrRecord = [8]byte{4: 1}

// From <mach/*.h>.
const (
	kernSuccess            = 0
	machPortRightReceive   = 1          // MACH_PORT_RIGHT_RECEIVE
	machMsgTypeMakeSend    = 20         // MACH_MSG_TYPE_MAKE_SEND
	excMaskCrash           = 1 << 10    // EXC_MASK_CRASH
	exceptionDefault       = 1          // EXCEPTION_DEFAULT
	machMsghBitsRemoteMask = 0x1f       // MACH_MSGH_BITS_REMOTE_MASK
	machRcvTooLarge        = 0x10004004 // MACH_RCV_TOO_LARGE
	machRcvInterrupted     = 0x10004005 // MACH_RCV_INTERRUPTED
)

// threadStateNone is THREAD_STATE_NONE, which differs by architecture. The
// kernel checks the flavor is one this machine has even though
// EXCEPTION_DEFAULT carries no thread state.
func threadStateNone() uintptr {
	if runtime.GOARCH == "amd64" {
		return 13
	}
	return 5
}

// The libSystem entry points, filled in by crashreports_darwin.s.
var (
	taskSelfTrapAddr          uintptr
	machPortAllocateAddr      uintptr
	machPortInsertRightAddr   uintptr
	machPortModRefsAddr       uintptr
	taskSetExceptionPortsAddr uintptr
	machMsgReceiveAddr        uintptr
	machMsgSendAddr           uintptr
	machMsgDestroyAddr        uintptr
)

//go:cgo_import_dynamic shtestenv_task_self_trap task_self_trap "/usr/lib/libSystem.B.dylib"
//go:cgo_import_dynamic shtestenv_mach_port_allocate mach_port_allocate "/usr/lib/libSystem.B.dylib"
//go:cgo_import_dynamic shtestenv_mach_port_insert_right mach_port_insert_right "/usr/lib/libSystem.B.dylib"
//go:cgo_import_dynamic shtestenv_mach_port_mod_refs mach_port_mod_refs "/usr/lib/libSystem.B.dylib"
//go:cgo_import_dynamic shtestenv_task_set_exception_ports task_set_exception_ports "/usr/lib/libSystem.B.dylib"
//go:cgo_import_dynamic shtestenv_mach_msg_receive mach_msg_receive "/usr/lib/libSystem.B.dylib"
//go:cgo_import_dynamic shtestenv_mach_msg_send mach_msg_send "/usr/lib/libSystem.B.dylib"
//go:cgo_import_dynamic shtestenv_mach_msg_destroy mach_msg_destroy "/usr/lib/libSystem.B.dylib"

// libcCall and libcCall6 are package syscall's own entry to a libc function on
// darwin, which it keeps for exactly this use.
//
//go:linkname libcCall syscall.syscall
func libcCall(fn, a1, a2, a3 uintptr) (r1, r2 uintptr, err syscall.Errno)

//go:linkname libcCall6 syscall.syscall6
func libcCall6(fn, a1, a2, a3, a4, a5, a6 uintptr) (r1, r2 uintptr, err syscall.Errno)
