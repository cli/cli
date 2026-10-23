package api

import (
	"encoding/base64"
	"encoding/binary"
	"math"
)

// EncodeNodeID encodes ids as a base64 MessagePack array of compact ints,
// the payload of a GraphQL node ID such as "PR_<payload>".
func EncodeNodeID(ids ...int64) string {
	b := []byte{0x90 | byte(len(ids))}
	for _, id := range ids {
		switch {
		case id < 0x80:
			b = append(b, byte(id))
		case id <= math.MaxUint8:
			b = append(b, 0xcc, byte(id))
		case id <= math.MaxUint16:
			b = binary.BigEndian.AppendUint16(append(b, 0xcd), uint16(id))
		case id <= math.MaxUint32:
			b = binary.BigEndian.AppendUint32(append(b, 0xce), uint32(id))
		default:
			b = binary.BigEndian.AppendUint64(append(b, 0xcf), uint64(id))
		}
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
