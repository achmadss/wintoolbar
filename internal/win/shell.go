package win

import (
	"syscall"
	"unsafe"
)

const (
	NIM_ADD    = 0
	NIM_MODIFY = 1
	NIM_DELETE = 2

	NIF_MESSAGE = 0x01
	NIF_ICON    = 0x02
	NIF_TIP     = 0x04
	NIF_SHOWTIP = 0x80
)

type NotifyIconData struct {
	Size            uint32
	Wnd             uintptr
	ID              uint32
	Flags           uint32
	CallbackMessage uint32
	Icon            uintptr
	Tip             [128]uint16
	State           uint32
	StateMask       uint32
	Info            [256]uint16
	Version         uint32
	InfoTitle       [64]uint16
	InfoFlags       uint32
	GuidItem        [16]byte
	BalloonIcon     uintptr
}

var (
	procShellNotifyIcon = shell32.proc("Shell_NotifyIconW")
	procShellExecute    = shell32.proc("ShellExecuteW")
)

func ShellNotifyIcon(msg uint32, d *NotifyIconData) bool {
	d.Size = uint32(unsafe.Sizeof(*d))
	return procShellNotifyIcon.Call0(uintptr(msg), uintptr(unsafe.Pointer(d))) != 0
}

// ShellOpen opens path with its default handler.
func ShellOpen(path string) {
	procShellExecute.Call(0, uintptr(unsafe.Pointer(UTF16Ptr("open"))), uintptr(unsafe.Pointer(UTF16Ptr(path))), 0, 0, 1)
}

// --- kernel32 ---

var (
	procCreateMutex             = kern32.proc("CreateMutexW")
	procGetModuleHandle         = kern32.proc("GetModuleHandleW")
	procK32GetProcessMemoryInfo = kern32.proc("K32GetProcessMemoryInfo")
	procGetCurrentProcess       = kern32.proc("GetCurrentProcess")
)

const errorAlreadyExists = 183

// SingleInstance returns false if another process holds the named mutex.
func SingleInstance(name string) bool {
	h, _, e := procCreateMutex.Call(0, 0, uintptr(unsafe.Pointer(UTF16Ptr(name))))
	if h == 0 {
		return true // cannot tell; do not block startup
	}
	return e != errorAlreadyExists
}

func ModuleHandle() uintptr { return procGetModuleHandle.Call0(0) }

type processMemoryCounters struct {
	Cb                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
	PrivateUsage               uintptr
}

// ProcessMemory returns the working set and private bytes of this process.
func ProcessMemory() (workingSet, private uint64) {
	var c processMemoryCounters
	c.Cb = uint32(unsafe.Sizeof(c))
	p := procGetCurrentProcess.Call0()
	procK32GetProcessMemoryInfo.Call(p, uintptr(unsafe.Pointer(&c)), uintptr(c.Cb))
	return uint64(c.WorkingSetSize), uint64(c.PrivateUsage)
}

// --- advapi32 ---

var procRegGetValue = advapi32.proc("RegGetValueW")

const (
	HKEY_CURRENT_USER = 0x80000001
	RRF_RT_REG_DWORD  = 0x00000010
)

// RegDWORD reads a DWORD from HKCU. ok is false if missing.
func RegDWORD(subkey, value string) (v uint32, ok bool) {
	size := uint32(4)
	r := procRegGetValue.Call0(HKEY_CURRENT_USER, uintptr(unsafe.Pointer(UTF16Ptr(subkey))),
		uintptr(unsafe.Pointer(UTF16Ptr(value))), RRF_RT_REG_DWORD, 0,
		uintptr(unsafe.Pointer(&v)), uintptr(unsafe.Pointer(&size)))
	return v, r == 0
}

// UTF16PtrToString reads a NUL-terminated UTF-16 string.
func UTF16PtrToString(p *uint16) string {
	if p == nil {
		return ""
	}
	n := 0
	for ptr := unsafe.Pointer(p); *(*uint16)(ptr) != 0; n++ {
		ptr = unsafe.Add(ptr, 2)
	}
	return syscall.UTF16ToString(unsafe.Slice(p, n))
}

const RRF_RT_REG_BINARY = 0x00000008

// RegBinary reads a REG_BINARY value from HKCU into buf and returns its length.
func RegBinary(subkey, value string, buf []byte) (int, bool) {
	if len(buf) == 0 {
		return 0, false
	}
	size := uint32(len(buf))
	r := procRegGetValue.Call0(HKEY_CURRENT_USER, uintptr(unsafe.Pointer(UTF16Ptr(subkey))),
		uintptr(unsafe.Pointer(UTF16Ptr(value))), RRF_RT_REG_BINARY, 0,
		uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	return int(size), r == 0
}

// --- wtsapi32: session lock/unlock notifications ---

var (
	wtsapi32                     = systemDLL("wtsapi32.dll")
	procWTSRegisterSessionNotify = wtsapi32.proc("WTSRegisterSessionNotification")
)

const (
	WM_WTSSESSION_CHANGE   = 0x02B1
	WM_POWERBROADCAST      = 0x0218
	WTS_SESSION_LOGON      = 0x5
	WTS_SESSION_UNLOCK     = 0x8
	PBT_APMRESUMEAUTOMATIC = 0x12
)

// RegisterSessionNotification delivers WM_WTSSESSION_CHANGE for this session to h.
func RegisterSessionNotification(h uintptr) bool {
	return procWTSRegisterSessionNotify.Call0(h, 0 /*NOTIFY_FOR_THIS_SESSION*/) != 0
}

var procSetProcessWorkingSetSize = kern32.proc("SetProcessWorkingSetSize")

// TrimWorkingSet asks Windows to page out this process's working set. Pages
// still in use fault back in; pages touched only during startup stay out.
func TrimWorkingSet() {
	p := procGetCurrentProcess.Call0()
	procSetProcessWorkingSetSize.Call(p, ^uintptr(0), ^uintptr(0))
}

var (
	procRegSetKeyValue    = advapi32.proc("RegSetKeyValueW")
	procRegDeleteKeyValue = advapi32.proc("RegDeleteKeyValueW")
)

const (
	REG_SZ          = 1
	RRF_RT_REG_SZ   = 0x00000002
	errFileNotFound = 2
)

// RegString reads a REG_SZ value from HKCU.
func RegString(subkey, value string) (string, bool) {
	var buf [1024]uint16
	size := uint32(len(buf) * 2)
	r := procRegGetValue.Call0(HKEY_CURRENT_USER, uintptr(unsafe.Pointer(UTF16Ptr(subkey))),
		uintptr(unsafe.Pointer(UTF16Ptr(value))), RRF_RT_REG_SZ, 0,
		uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	if r != 0 {
		return "", false
	}
	return syscall.UTF16ToString(buf[:]), true
}

// RegSetString writes a REG_SZ value under HKCU, creating the key if needed.
func RegSetString(subkey, value, data string) error {
	d, err := syscall.UTF16FromString(data)
	if err != nil {
		return err
	}
	r := procRegSetKeyValue.Call0(HKEY_CURRENT_USER, uintptr(unsafe.Pointer(UTF16Ptr(subkey))),
		uintptr(unsafe.Pointer(UTF16Ptr(value))), REG_SZ, uintptr(unsafe.Pointer(&d[0])), uintptr(len(d)*2))
	if r != 0 {
		return syscall.Errno(r)
	}
	return nil
}

// RegDeleteValue removes a value under HKCU; a missing value is not an error.
func RegDeleteValue(subkey, value string) error {
	r := procRegDeleteKeyValue.Call0(HKEY_CURRENT_USER, uintptr(unsafe.Pointer(UTF16Ptr(subkey))),
		uintptr(unsafe.Pointer(UTF16Ptr(value))))
	if r != 0 && r != errFileNotFound {
		return syscall.Errno(r)
	}
	return nil
}
