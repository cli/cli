package api

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEncodeNodeID(t *testing.T) {
	tests := map[string]struct {
		templateIndex int64
		id            int64
		want          string
	}{
		"positive fixint max": {id: 127, want: "kgB_"},
		"uint8 min":           {id: 128, want: "kgDMgA"},
		"uint8 max":           {id: 255, want: "kgDM_w"},
		"uint16 min":          {id: 256, want: "kgDNAQA"},
		"uint16 max":          {id: 65535, want: "kgDN__8"},
		"uint32 min":          {id: 65536, want: "kgDOAAEAAA"},
		"uint32 max":          {id: math.MaxUint32, want: "kgDO_____w"},
		"uint64 min":          {id: math.MaxUint32 + 1, want: "kgDPAAAAAQAAAAA"},
		"nonzero template":    {templateIndex: 1, id: 127, want: "kgF_"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			// Given a template index, database ID, and expected payload
			templateIndex, id := tt.templateIndex, tt.id

			// When the node ID encoder receives the explicit template index
			got := EncodeNodeID(templateIndex, id)

			// Then it preserves the payload, including the template index
			assert.Equal(t, tt.want, got)
		})
	}
}
