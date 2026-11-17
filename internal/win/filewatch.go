package win

import (
	"syscall"
	"unsafe"
)

// Change filters for ReadDirectoryChangesW.
const (
	FILE_NOTIFY_CHANGE_FILE_NAME  = 0x00000001
	FILE_NOTIFY_CHANGE_LAST_WRITE = 0x00000010
)

const (
	FILE_LIST_DIRECTORY        = 0x0001
	FILE_SHARE_ALL             = 0x0007 // read | write | delete
	OPEN_EXISTING              = 3
	FILE_FLAG_BACKUP_SEMANTICS = 0x02000000 // required to open a directory handle
	FILE_FLAG_OVERLAPPED       = 0x40000000 // keeps ReadDirectoryChangesW asynchronous
	ERROR_IO_PENDING           = 17
	WAIT_INFINITE              = 0xFFFFFFFF
)

var (
	procCreateFileW            = kern32.proc("CreateFileW")
	procReadDirectoryChangesW  = kern32.proc("ReadDirectoryChangesW")
	procGetOverlappedResult    = kern32.proc("GetOverlappedResult")
	procCancelIoEx             = kern32.proc("CancelIoEx")
	procSetEvent               = kern32.proc("SetEvent")
	procWaitForMultipleObjects = kern32.proc("WaitForMultipleObjects")
)

// OpenDir opens dir for change notification.
func OpenDir(dir string) (uintptr, error) {
	h, _, e := procCreateFileW.Call(
		uintptr(unsafe.Pointer(UTF16Ptr(dir))),
		FILE_LIST_DIRECTORY,
		FILE_SHARE_ALL,
		0,
		OPEN_EXISTING,
		FILE_FLAG_BACKUP_SEMANTICS|FILE_FLAG_OVERLAPPED,
		0,
	)
	if h == uintptr(syscall.InvalidHandle) {
		return 0, e
	}
	return h, nil
}

// StartChanges queues one overlapped ReadDirectoryChangesW on h. buf must be
// non-empty and stays owned by the call until ChangeResult returns.
func StartChanges(h uintptr, ov *syscall.Overlapped, buf []byte, filter uint32) error {
	if len(buf) == 0 {
		return syscall.EINVAL
	}
	r, _, e := procReadDirectoryChangesW.Call(
		h,
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
		0, // not recursive
		uintptr(filter),
		0, // byte count is read by ChangeResult instead
		uintptr(unsafe.Pointer(ov)),
		0, // no completion routine
	)
	if r != 0 || e == ERROR_IO_PENDING {
		return nil
	}
	return e
}

// ChangeResult reports how many bytes the queued read produced. bWait blocks
// until the read finishes; a cancelled read reports ERROR_OPERATION_ABORTED.
func ChangeResult(h uintptr, ov *syscall.Overlapped, bWait bool) (uint32, error) {
	var n uint32
	var wait uintptr
	if bWait {
		wait = 1
	}
	r, _, e := procGetOverlappedResult.Call(h, uintptr(unsafe.Pointer(ov)),
		uintptr(unsafe.Pointer(&n)), wait)
	if r == 0 {
		return 0, e
	}
	return n, nil
}

// CancelChanges aborts the pending read on h, issued by any thread.
func CancelChanges(h uintptr) { procCancelIoEx.Call(h, 0) }

// SetEvent signals ev.
func SetEvent(ev uintptr) { procSetEvent.Call(ev) }

// WaitAny blocks until a or b is signaled and returns its index, or -1 on error.
func WaitAny(a, b uintptr) int {
	hs := [2]uintptr{a, b}
	r, _, _ := procWaitForMultipleObjects.Call(2, uintptr(unsafe.Pointer(&hs[0])),
		0 /* not wait-all */, WAIT_INFINITE)
	if r >= 2 {
		return -1
	}
	return int(r)
}
