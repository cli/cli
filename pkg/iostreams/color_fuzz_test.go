//go:build go1.18
// +build go1.18

package iostreams

import "testing"

func FuzzHexToRGB(f *testing.F) {
	seeds := []string{"fc0303", "c3406b", "060a5d", "fooo"}

	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, in string) {
		cs := NewColorScheme(true, true, true)
		cs.HexToRGB(in, in)
	})
}
