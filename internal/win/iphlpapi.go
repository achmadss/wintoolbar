package win

import "unsafe"

// MibIfRow2 mirrors MIB_IF_ROW2 (netioapi.h). Size on amd64 is 1352 bytes;
// iphlpapi_test.go checks size and key offsets.
type MibIfRow2 struct {
	InterfaceLuid               uint64
	InterfaceIndex              uint32
	InterfaceGuid               [16]byte
	Alias                       [257]uint16
	Description                 [257]uint16
	PhysicalAddressLength       uint32
	PhysicalAddress             [32]byte
	PermanentPhysicalAddress    [32]byte
	Mtu                         uint32
	Type                        uint32
	TunnelType                  uint32
	MediaType                   uint32
	PhysicalMediumType          uint32
	AccessType                  uint32
	DirectionType               uint32
	InterfaceAndOperStatusFlags uint8
	_                           [3]byte
	OperStatus                  uint32
	AdminStatus                 uint32
	MediaConnectState           uint32
	NetworkGuid                 [16]byte
	ConnectionType              uint32
	_                           [4]byte
	TransmitLinkSpeed           uint64
	ReceiveLinkSpeed            uint64
	InOctets                    uint64
	InUcastPkts                 uint64
	InNUcastPkts                uint64
	InDiscards                  uint64
	InErrors                    uint64
	InUnknownProtos             uint64
	InUcastOctets               uint64
	InMulticastOctets           uint64
	InBroadcastOctets           uint64
	OutOctets                   uint64
	OutUcastPkts                uint64
	OutNUcastPkts               uint64
	OutDiscards                 uint64
	OutErrors                   uint64
	OutUcastOctets              uint64
	OutMulticastOctets          uint64
	OutBroadcastOctets          uint64
	OutQLen                     uint64
}

// InterfaceAndOperStatusFlags bits.
const (
	IfFlagHardwareInterface = 1 << 0
	IfFlagFilterInterface   = 1 << 1
)

// Interface types (ipifcons.h).
const (
	IF_TYPE_ETHERNET_CSMACD   = 6
	IF_TYPE_SOFTWARE_LOOPBACK = 24
	IF_TYPE_IEEE80211         = 71
	IF_TYPE_TUNNEL            = 131
	IfOperStatusUp            = 1
)

// MibIpForwardRow2 mirrors MIB_IPFORWARD_ROW2. Size 104 bytes.
type MibIpForwardRow2 struct {
	InterfaceLuid     uint64
	InterfaceIndex    uint32
	DestinationPrefix [28]byte // SOCKADDR_INET
	PrefixLength      uint8
	_                 [3]byte
	NextHop           [28]byte
	SitePrefixLength  uint8
	_                 [3]byte
	ValidLifetime     uint32
	PreferredLifetime uint32
	Metric            uint32
	Protocol          uint32
	Loopback          uint8
	AutoconfigureAddr uint8
	Publish           uint8
	Immortal          uint8
	Age               uint32
	Origin            uint32
}

var (
	procGetIfTable2        = iphlpapi.proc("GetIfTable2")
	procGetIfEntry2        = iphlpapi.proc("GetIfEntry2")
	procGetIpForwardTable2 = iphlpapi.proc("GetIpForwardTable2")
	procFreeMibTable       = iphlpapi.proc("FreeMibTable")
)

// GetIfEntry2 refreshes row in place. row.InterfaceLuid (or InterfaceIndex) must be set.
func GetIfEntry2(row *MibIfRow2) bool {
	return procGetIfEntry2.Call0(uintptr(unsafe.Pointer(row))) == 0
}

// IfTable calls fn for each interface row. The rows are only valid inside fn.
func IfTable(fn func(*MibIfRow2)) bool {
	var table unsafe.Pointer
	if procGetIfTable2.Call0(uintptr(unsafe.Pointer(&table))) != 0 || table == nil {
		return false
	}
	defer procFreeMibTable.Call(uintptr(table))
	n := *(*uint32)(table)
	rows := unsafe.Slice((*MibIfRow2)(unsafe.Add(table, 8)), n)
	for i := range rows {
		fn(&rows[i])
	}
	return true
}

// DefaultRouteLuids calls fn with the LUID of each interface that has a
// default route (prefix length 0), IPv4 or IPv6.
func DefaultRouteLuids(fn func(luid uint64)) bool {
	var table unsafe.Pointer
	if procGetIpForwardTable2.Call0(0 /* AF_UNSPEC */, uintptr(unsafe.Pointer(&table))) != 0 || table == nil {
		return false
	}
	defer procFreeMibTable.Call(uintptr(table))
	n := *(*uint32)(table)
	rows := unsafe.Slice((*MibIpForwardRow2)(unsafe.Add(table, 8)), n)
	for i := range rows {
		if rows[i].PrefixLength == 0 {
			fn(rows[i].InterfaceLuid)
		}
	}
	return true
}
