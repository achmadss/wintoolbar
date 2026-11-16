// Command mkrsrc writes rsrc_windows_amd64.syso: a COFF object with the
// resources the Go linker embeds into the executable.
//
// Resources:
//   - RT_MANIFEST 1: Common Controls v6 (themed settings window), per-monitor-v2
//     DPI awareness, and Windows 10/11 support (needed for layered child windows).
//   - RT_ICON 1..4 and RT_GROUP_ICON 1: the app icon, the same four bars the
//     tray icon draws (see internal/widget/tray.go), at 16, 32, 48 and 256 px.
//
// Run with: go generate
package main

import (
	"bytes"
	"encoding/binary"
	"log"
	"os"
	"sort"
)

const manifest = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<assembly xmlns="urn:schemas-microsoft-com:asm.v1" manifestVersion="1.0" xmlns:asmv3="urn:schemas-microsoft-com:asm.v3">
  <assemblyIdentity type="win32" name="wintoolbar" version="1.0.0.0" processorArchitecture="amd64"/>
  <compatibility xmlns="urn:schemas-microsoft-com:compatibility.v1">
    <application>
      <supportedOS Id="{8e0f7a12-bfb3-4fe8-b9a5-48fd50a15a9a}"/>
    </application>
  </compatibility>
  <asmv3:application>
    <asmv3:windowsSettings>
      <dpiAwareness xmlns="http://schemas.microsoft.com/SMI/2016/WindowsSettings">PerMonitorV2</dpiAwareness>
    </asmv3:windowsSettings>
  </asmv3:application>
  <dependency>
    <dependentAssembly>
      <assemblyIdentity type="win32" name="Microsoft.Windows.Common-Controls" version="6.0.0.0" processorArchitecture="*" publicKeyToken="6595b64144ccf1df" language="*"/>
    </dependentAssembly>
  </dependency>
</assembly>
`

const (
	rtIcon      = 3
	rtGroupIcon = 14
	rtManifest  = 24
	langEnUS    = 0x0409

	iconID = 1 // RT_GROUP_ICON id; the executable's main icon
)

// iconSizes are the sizes embedded, in pixels. Explorer and the taskbar pick
// one of them; 256 keeps the file browser's large view sharp.
var iconSizes = []int{16, 32, 48, 256}

// resource is one leaf in the resource tree: type -> id -> language -> bytes.
type resource struct {
	typ, id, lang uint32
	data          []byte
}

func main() {
	out := "rsrc_windows_amd64.syso"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}
	res := []resource{{rtManifest, 1, langEnUS, []byte(manifest)}}
	for i := range iconSizes {
		res = append(res, resource{rtIcon, uint32(iconID + i), langEnUS, iconDIB(iconSizes[i])})
	}
	res = append(res, resource{rtGroupIcon, iconID, langEnUS, groupIcon()})
	if err := os.WriteFile(out, build(res), 0o644); err != nil {
		log.Fatal(err)
	}
}

// iconDIB returns a 32-bit bottom-up DIB for one RT_ICON image: a bitmap info
// header whose height is doubled (colour rows plus the AND mask), the pixels
// in BGRA, and an all-zero mask since the alpha channel carries the shape.
func iconDIB(px int) []byte {
	le := binary.LittleEndian
	b := make([]byte, 0, 40+px*px*4+px*4)
	b = le.AppendUint32(b, 40)              // biSize
	b = le.AppendUint32(b, uint32(px))      // biWidth
	b = le.AppendUint32(b, uint32(2*px))    // biHeight: colour and mask
	b = le.AppendUint16(b, 1)               // biPlanes
	b = le.AppendUint16(b, 32)              // biBitCount
	b = le.AppendUint32(b, 0)               // biCompression: BI_RGB
	b = le.AppendUint32(b, uint32(px*px*4)) // biSizeImage
	b = le.AppendUint32(b, 0)               // biXPelsPerMeter
	b = le.AppendUint32(b, 0)               // biYPelsPerMeter
	b = le.AppendUint32(b, 0)               // biClrUsed
	b = le.AppendUint32(b, 0)               // biClrImportant

	for y := px - 1; y >= 0; y-- { // bottom-up
		for x := 0; x < px; x++ {
			b = append(b, pixel(px, x, y)...)
		}
	}
	maskStride := (px + 31) / 32 * 4
	b = append(b, make([]byte, maskStride*px)...)
	return b
}

// pixel returns the BGRA bytes for one icon pixel: the tray icon's four bars on
// a transparent background. Keep in sync with makeTrayIcon in
// internal/widget/tray.go.
func pixel(px, x, y int) []byte {
	const (
		r, g, bl = 0x60, 0xCD, 0xFF // the tray icon's opaque light blue
		bars     = 4
	)
	heights := [bars]int{6, 12, 9, 15}
	bw := px / 6
	gap := (px - bars*bw) / 5
	for i, hh := range heights {
		bh := hh * px / 16
		x0 := gap + i*(bw+gap)
		if x >= x0 && x < x0+bw && y >= px-bh {
			return []byte{bl, g, r, 0xFF}
		}
	}
	return []byte{0, 0, 0, 0}
}

// groupIcon returns a GRPICONDIR naming the icons in order. Each entry repeats
// the size and byte count of its RT_ICON image.
func groupIcon() []byte {
	le := binary.LittleEndian
	b := make([]byte, 0, 6+14*len(iconSizes))
	b = le.AppendUint16(b, 0) // idReserved
	b = le.AppendUint16(b, 1) // idType: icon
	b = le.AppendUint16(b, uint16(len(iconSizes)))
	for i, px := range iconSizes {
		d := byte(px)
		if px >= 256 {
			d = 0 // 0 means 256
		}
		b = append(b, d, d, 0, 0)                        // width, height, colours, reserved
		b = le.AppendUint16(b, 1)                        // planes
		b = le.AppendUint16(b, 32)                       // bit count
		b = le.AppendUint32(b, uint32(len(iconDIB(px)))) // bytes in resource
		b = le.AppendUint16(b, uint16(iconID+i))         // RT_ICON id
	}
	return b
}

// build lays out one .rsrc section: the root directory, one directory per
// resource type, one language directory per resource, one data entry per
// resource, then the data itself. Every data entry's offset needs an ADDR32NB
// relocation because it is stored as an RVA.
func build(res []resource) []byte {
	le := binary.LittleEndian
	sort.Slice(res, func(i, j int) bool { return res[i].typ < res[j].typ })

	var types []uint32
	for _, r := range res {
		if len(types) == 0 || types[len(types)-1] != r.typ {
			types = append(types, r.typ)
		}
	}

	// Offsets of the three directory levels and of the data entries.
	typeDir := map[uint32]uint32{}
	langDir := map[int]uint32{}
	dataEntry := make([]uint32, len(res))
	cur := uint32(16 + 8*len(types))
	for _, t := range types {
		typeDir[t] = cur
		cur += uint32(16 + 8*countType(res, t))
	}
	for i := range res {
		langDir[i] = cur
		cur += 16 + 8
	}
	for i := range res {
		dataEntry[i] = cur
		cur += 16
	}
	dataOff := cur

	// Blob offsets inside the section. Each data entry stores its blob's
	// offset, and the linker's ADDR32NB relocation turns that into an RVA.
	blobOff := make([]uint32, len(res))
	at := dataOff
	for i, r := range res {
		blobOff[i] = at
		at += uint32(len(r.data))
		at = (at + 7) &^ 7
	}

	var sec bytes.Buffer
	w := func(v any) { binary.Write(&sec, le, v) }
	dir := func(ids []uint32, offs []uint32) {
		w([4]uint32{0, 0, 0, uint32(len(ids)) << 16}) // Characteristics, TimeDateStamp, Major/Minor, Named=0 | Id=n
		for i, id := range ids {
			w([2]uint32{id, offs[i]})
		}
	}

	// Root directory: one entry per type.
	var typeIDs, typeOffs []uint32
	for _, t := range types {
		typeIDs = append(typeIDs, t)
		typeOffs = append(typeOffs, 0x80000000|typeDir[t])
	}
	dir(typeIDs, typeOffs)

	// One directory per type: one entry per resource id.
	for _, t := range types {
		var ids, offs []uint32
		for i, r := range res {
			if r.typ == t {
				ids = append(ids, r.id)
				offs = append(offs, 0x80000000|langDir[i])
			}
		}
		dir(ids, offs)
	}

	// One directory per resource: a single language entry.
	for i, r := range res {
		dir([]uint32{r.lang}, []uint32{dataEntry[i]})
	}

	// Data entries: the first field is the blob offset, which the relocation
	// below rewrites as an RVA.
	for i := range res {
		w([4]uint32{blobOff[i], uint32(len(res[i].data)), 0, 0})
	}

	// Data, each blob 8-byte aligned.
	for i := range res {
		sec.Write(res[i].data)
		for sec.Len()%8 != 0 {
			sec.WriteByte(0)
		}
	}

	if uint32(sec.Len()) != at {
		log.Fatalf("resource layout mismatch: section is %d bytes, plan is %d", sec.Len(), at)
	}

	const headers = 20 + 40
	rawSize := uint32(sec.Len())
	relocOff := uint32(headers) + rawSize
	symOff := relocOff + uint32(10*len(res))

	var f bytes.Buffer
	wf := func(v any) { binary.Write(&f, le, v) }
	// IMAGE_FILE_HEADER
	wf([2]uint16{0x8664, 1}) // AMD64, 1 section
	wf(uint32(0))            // timestamp
	wf(symOff)               // symbol table
	wf(uint32(1))            // symbols
	wf(uint16(0))            // optional header size
	wf(uint16(0))            // characteristics
	// IMAGE_SECTION_HEADER
	f.WriteString(".rsrc\x00\x00\x00")
	wf(uint32(0))          // VirtualSize
	wf(uint32(0))          // VirtualAddress
	wf(rawSize)            // SizeOfRawData
	wf(uint32(headers))    // PointerToRawData
	wf(relocOff)           // PointerToRelocations
	wf(uint32(0))          // PointerToLinenumbers
	wf(uint16(len(res)))   // NumberOfRelocations
	wf(uint16(0))          // NumberOfLinenumbers
	wf(uint32(0x40000040)) // INITIALIZED_DATA | MEM_READ
	f.Write(sec.Bytes())
	// One IMAGE_RELOCATION per data entry: its OffsetToData becomes an RVA.
	for _, off := range dataEntry {
		wf(off)
		wf(uint32(0)) // symbol index
		wf(uint16(3)) // IMAGE_REL_AMD64_ADDR32NB
	}
	// IMAGE_SYMBOL for the section.
	f.WriteString(".rsrc\x00\x00\x00")
	wf(uint32(0)) // Value
	wf(int16(1))  // SectionNumber
	wf(uint16(0)) // Type
	wf(uint8(3))  // IMAGE_SYM_CLASS_STATIC
	wf(uint8(0))  // aux symbols
	wf(uint32(4)) // empty string table
	return f.Bytes()
}

func countType(res []resource, t uint32) int {
	n := 0
	for _, r := range res {
		if r.typ == t {
			n++
		}
	}
	return n
}
