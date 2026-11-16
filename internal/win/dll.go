// Package win holds the small set of Win32 bindings wintoolbar uses.
//
// Only the standard library syscall package is used. DLLs are loaded from
// System32 only (LOAD_LIBRARY_SEARCH_SYSTEM32), so a DLL planted next to the
// executable or in the current directory is never picked up.
package win

import (
	"syscall"
	"unsafe"
)

const loadLibrarySearchSystem32 = 0x00000800

var (
	kernel32          = syscall.NewLazyDLL("kernel32.dll") // KnownDLL, always from System32.
	procLoadLibraryEx = kernel32.NewProc("LoadLibraryExW")
)

// DLL is a System32 DLL whose procedures resolve on first use.
type DLL struct {
	name   string
	handle uintptr
}

// Proc is one exported procedure of a DLL.
type Proc struct {
	dll  *DLL
	name string
	addr uintptr
}

func systemDLL(name string) *DLL { return &DLL{name: name} }

func (d *DLL) load() uintptr {
	if d.handle != 0 {
		return d.handle
	}
	p, err := syscall.UTF16PtrFromString(d.name)
	if err != nil {
		panic(err)
	}
	h, _, e := procLoadLibraryEx.Call(uintptr(unsafe.Pointer(p)), 0, loadLibrarySearchSystem32)
	if h == 0 {
		panic("win: cannot load " + d.name + ": " + e.Error())
	}
	d.handle = h
	return h
}

func (d *DLL) proc(name string) *Proc { return &Proc{dll: d, name: name} }

// Find reports whether the procedure exists, resolving it if needed.
func (p *Proc) Find() bool {
	if p.addr != 0 {
		return true
	}
	a, err := syscall.GetProcAddress(syscall.Handle(p.dll.load()), p.name)
	if err != nil {
		return false
	}
	p.addr = a
	return true
}

// Call invokes the procedure. It panics if the procedure does not exist.
func (p *Proc) Call(args ...uintptr) (uintptr, uintptr, syscall.Errno) {
	if !p.Find() {
		panic("win: missing " + p.dll.name + "!" + p.name)
	}
	return syscall.SyscallN(p.addr, args...)
}

// Call0 invokes the procedure and returns only the first result.
func (p *Proc) Call0(args ...uintptr) uintptr {
	r, _, _ := p.Call(args...)
	return r
}

var (
	user32   = systemDLL("user32.dll")
	gdi32    = systemDLL("gdi32.dll")
	shell32  = systemDLL("shell32.dll")
	iphlpapi = systemDLL("iphlpapi.dll")
	advapi32 = systemDLL("advapi32.dll")
	kern32   = systemDLL("kernel32.dll")
)

// UTF16Ptr converts s to a NUL-terminated UTF-16 pointer. Not for hot paths.
func UTF16Ptr(s string) *uint16 {
	p, err := syscall.UTF16PtrFromString(s)
	if err != nil {
		panic(err)
	}
	return p
}
