package win

import (
	"syscall"
	"unsafe"
)

// GUID is a Windows GUID.
type GUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

var (
	ole32                = systemDLL("ole32.dll")
	procCoInitializeEx   = ole32.proc("CoInitializeEx")
	procCoUninitialize   = ole32.proc("CoUninitialize")
	procCoCreateInstance = ole32.proc("CoCreateInstance")
	procCoTaskMemFree    = ole32.proc("CoTaskMemFree")

	procCreateEvent         = kern32.proc("CreateEventW")
	procWaitForSingleObject = kern32.proc("WaitForSingleObject")
	procCloseHandle         = kern32.proc("CloseHandle")
)

const (
	COINIT_MULTITHREADED     = 0x0
	COINIT_APARTMENTTHREADED = 0x2
	CLSCTX_ALL               = 0x17
	WAIT_OBJECT_0            = 0
	WAIT_TIMEOUT             = 0x102
)

// HRESULT is a COM status code.
type HRESULT uint32

func (h HRESULT) Failed() bool { return int32(h) < 0 }

func (h HRESULT) Error() string {
	return "HRESULT 0x" + hex32(uint32(h))
}

func hex32(v uint32) string {
	const d = "0123456789ABCDEF"
	var b [8]byte
	for i := 7; i >= 0; i-- {
		b[i] = d[v&0xF]
		v >>= 4
	}
	return string(b[:])
}

// Err returns nil for success codes and h otherwise.
func (h HRESULT) Err() error {
	if h.Failed() {
		return h
	}
	return nil
}

func CoInitializeEx(flags uint32) HRESULT {
	return HRESULT(procCoInitializeEx.Call0(0, uintptr(flags)))
}

func CoUninitialize() { procCoUninitialize.Call() }

func CoCreateInstance(clsid, iid *GUID) (uintptr, error) {
	var obj uintptr
	hr := HRESULT(procCoCreateInstance.Call0(uintptr(unsafe.Pointer(clsid)), 0, CLSCTX_ALL,
		uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&obj))))
	if hr.Failed() {
		return 0, hr
	}
	return obj, nil
}

func CoTaskMemFree(p uintptr) { procCoTaskMemFree.Call(p) }

// VCall calls method idx of COM object obj. Unused args must be 0; on
// amd64 and arm64 the caller cleans the stack, so extra args are harmless.
func VCall(obj uintptr, idx int, a1, a2, a3, a4, a5, a6 uintptr) HRESULT {
	// obj and its vtable are owned by COM, not the Go heap.
	self := *(**struct{ vtbl *[64]uintptr })(unsafe.Pointer(&obj))
	fn := self.vtbl[idx]
	r, _, _ := syscall.SyscallN(fn, obj, a1, a2, a3, a4, a5, a6)
	return HRESULT(r)
}

// Release calls IUnknown::Release if obj is not 0.
func Release(obj uintptr) {
	if obj != 0 {
		VCall(obj, 2, 0, 0, 0, 0, 0, 0)
	}
}

func CreateEvent() uintptr { return procCreateEvent.Call0(0, 0, 0, 0) }

func WaitForSingleObject(h uintptr, ms uint32) uint32 {
	return uint32(procWaitForSingleObject.Call0(h, uintptr(ms)))
}

func CloseHandle(h uintptr) {
	if h != 0 {
		procCloseHandle.Call(h)
	}
}

// NewGUID builds a GUID from its four parts.
func NewGUID(d1 uint32, d2, d3 uint16, d4 [8]byte) GUID {
	return GUID{Data1: d1, Data2: d2, Data3: d3, Data4: d4}
}
