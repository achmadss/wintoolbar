package win

import "unsafe"

const (
	TRANSPARENT         = 1
	ANTIALIASED_QUALITY = 4
	FW_NORMAL           = 400
	FW_SEMIBOLD         = 600
	DEFAULT_CHARSET     = 1
	OUT_TT_PRECIS       = 4
	CLIP_DEFAULT_PRECIS = 0
	DIB_RGB_COLORS      = 0
	DT_LEFT             = 0x0000
	DT_RIGHT            = 0x0002
	DT_VCENTER          = 0x0004
	DT_SINGLELINE       = 0x0020
	DT_NOPREFIX         = 0x0800
	DT_NOCLIP           = 0x0100
)

type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

var (
	procCreateCompatibleDC   = gdi32.proc("CreateCompatibleDC")
	procDeleteDC             = gdi32.proc("DeleteDC")
	procCreateDIBSection     = gdi32.proc("CreateDIBSection")
	procSelectObject         = gdi32.proc("SelectObject")
	procDeleteObject         = gdi32.proc("DeleteObject")
	procCreateFont           = gdi32.proc("CreateFontW")
	procSetTextColor         = gdi32.proc("SetTextColor")
	procSetBkMode            = gdi32.proc("SetBkMode")
	procDrawText             = user32.proc("DrawTextW")
	procGetTextExtentPoint32 = gdi32.proc("GetTextExtentPoint32W")
	procCreateBitmap         = gdi32.proc("CreateBitmap")
	procGdiFlush             = gdi32.proc("GdiFlush")
)

func CreateCompatibleDC(dc uintptr) uintptr { return procCreateCompatibleDC.Call0(dc) }
func DeleteDC(dc uintptr)                   { procDeleteDC.Call(dc) }
func SelectObject(dc, obj uintptr) uintptr  { return procSelectObject.Call0(dc, obj) }
func DeleteObject(obj uintptr)              { procDeleteObject.Call(obj) }
func SetTextColor(dc uintptr, bgr uint32)   { procSetTextColor.Call(dc, uintptr(bgr)) }
func SetBkMode(dc uintptr, mode int32)      { procSetBkMode.Call(dc, uintptr(mode)) }
func GdiFlush()                             { procGdiFlush.Call() }

// CreateDIB32 makes a top-down 32-bit DIB section. pixels aliases the bitmap
// memory as BGRA words (0xAARRGGBB on little endian).
func CreateDIB32(w, h int32) (bmp uintptr, pixels []uint32) {
	bi := bitmapInfoHeader{Width: w, Height: -h, Planes: 1, BitCount: 32}
	bi.Size = uint32(unsafe.Sizeof(bi))
	var bits unsafe.Pointer
	bmp = procCreateDIBSection.Call0(0, uintptr(unsafe.Pointer(&bi)), DIB_RGB_COLORS, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if bmp == 0 || bits == nil {
		return 0, nil
	}
	return bmp, unsafe.Slice((*uint32)(bits), int(w)*int(h))
}

// CreateMonoBitmap makes a 1-bit bitmap, all bits zero.
func CreateMonoBitmap(w, h int32) uintptr {
	return procCreateBitmap.Call0(uintptr(w), uintptr(h), 1, 1, 0)
}

// CreateFont makes a font with pixel height px (negative = character height).
func CreateFont(px int32, weight int32, face string) uintptr {
	return procCreateFont.Call0(uintptr(px), 0, 0, 0, uintptr(weight), 0, 0, 0,
		DEFAULT_CHARSET, OUT_TT_PRECIS, CLIP_DEFAULT_PRECIS, ANTIALIASED_QUALITY, 0,
		uintptr(unsafe.Pointer(UTF16Ptr(face))))
}

// DrawText draws UTF-16 text (no terminator needed) inside r.
func DrawText(dc uintptr, text []uint16, r *Rect, format uint32) {
	if len(text) == 0 {
		return
	}
	procDrawText.Call(dc, uintptr(unsafe.Pointer(&text[0])), uintptr(len(text)), uintptr(unsafe.Pointer(r)), uintptr(format))
}

func TextExtent(dc uintptr, text []uint16) Size {
	var s Size
	if len(text) > 0 {
		procGetTextExtentPoint32.Call(dc, uintptr(unsafe.Pointer(&text[0])), uintptr(len(text)), uintptr(unsafe.Pointer(&s)))
	}
	return s
}
