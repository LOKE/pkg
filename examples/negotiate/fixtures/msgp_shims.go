package fixtures

import "net/netip"

// MessagePack carries addresses as the raw 4 or 16 bytes, which is what
// reflection-based msgpack libraries produce for netip.Addr. Zones are dropped
// and malformed lengths decode to the zero address.
func addrToBytes(addr netip.Addr) []byte {
	return addr.AsSlice()
}

func addrFromBytes(data []byte) netip.Addr {
	addr, _ := netip.AddrFromSlice(data)
	return addr
}
