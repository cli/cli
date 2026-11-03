package api

import (
	"encoding/base64"
	"encoding/binary"
	"math"
)

// EncodeNodeID encodes templateIndex and ids as a base64 MessagePack array
// of compact integers, the payload of a GraphQL node ID such as "PR_<payload>".
//
// templateIndex is an internal GitHub node ID detail. Clients should treat node
// IDs as opaque rather than construct them. This helper was introduced to
// preserve existing behaviour while removing the msgpack dependency. Ideally,
// node IDs would not be generated on the client, but removing that coupling is
// a larger undertaking than a direct dependency replacement.
//
// templateIndex and all IDs must be non-negative. Callers must supply fewer
// than 15 IDs.
func EncodeNodeID(templateIndex int64, ids ...int64) string {
	parts := append([]int64{templateIndex}, ids...)
	b := []byte{0x90 | byte(len(parts))}
	for _, id := range parts {
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
