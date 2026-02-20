package quantum_test

import (
	"math"
	"testing"

	"github.com/cli/cli/v2/pkg/quantum"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── constructor ─────────────────────────────────────────────────────────────

func TestNew(t *testing.T) {
	q := quantum.New()
	assert.Equal(t, [2]int{quantum.DefaultMinNodes, quantum.DefaultMaxNodes}, q.NodeRange)
	assert.Equal(t, quantum.DefaultMinNodes, q.ActiveNodes)
	assert.Equal(t, quantum.DefaultCoherenceTime, q.CoherenceTime)
	assert.InDelta(t, quantum.DefaultSuperpositionAngle, q.SuperpositionAngle, 1e-12)
}

func TestNewWithOptions(t *testing.T) {
	q := quantum.NewWithOptions(4, 16)
	assert.Equal(t, [2]int{4, 16}, q.NodeRange)
	assert.Equal(t, 4, q.ActiveNodes)
}

// ─── AdjustNodes ─────────────────────────────────────────────────────────────

func TestAdjustNodes(t *testing.T) {
	tests := []struct {
		name        string
		minNodes    int
		maxNodes    int
		currentLoad float64
		maxLoad     float64
		wantNodes   int
		wantErr     bool
	}{
		{
			name:        "zero load clamps to min",
			minNodes:    8, maxNodes: 64,
			currentLoad: 0, maxLoad: 100,
			wantNodes: 8,
		},
		{
			name:        "full load clamps to max",
			minNodes:    8, maxNodes: 64,
			currentLoad: 100, maxLoad: 100,
			wantNodes: 64,
		},
		{
			name:        "50% load gives half of max",
			minNodes:    8, maxNodes: 64,
			currentLoad: 50, maxLoad: 100,
			wantNodes: 32,
		},
		{
			name:        "overload clamps to max",
			minNodes:    8, maxNodes: 64,
			currentLoad: 200, maxLoad: 100,
			wantNodes: 64,
		},
		{
			name:        "tiny load clamps to min",
			minNodes:    8, maxNodes: 64,
			currentLoad: 1, maxLoad: 100,
			wantNodes: 8, // int(64 * 0.01) = 0, clamped to 8
		},
		{
			name:        "maxLoad zero returns error",
			minNodes:    8, maxNodes: 64,
			currentLoad: 50, maxLoad: 0,
			wantErr: true,
		},
		{
			name:        "negative maxLoad returns error",
			minNodes:    8, maxNodes: 64,
			currentLoad: 50, maxLoad: -1,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := quantum.NewWithOptions(tt.minNodes, tt.maxNodes)
			got, err := q.AdjustNodes(tt.currentLoad, tt.maxLoad)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantNodes, got)
			// ActiveNodes must be updated on the struct
			assert.Equal(t, tt.wantNodes, q.ActiveNodes)
		})
	}
}

// ─── EntangleNodes ───────────────────────────────────────────────────────────

func TestEntangleNodes_returnsState(t *testing.T) {
	q := quantum.New()
	state, err := q.EntangleNodes(nil)
	require.NoError(t, err)
	require.NotNil(t, state)
	assert.Equal(t, quantum.DefaultMinNodes, state.NumQubits)
}

func TestEntangleNodes_normalizationPreserved(t *testing.T) {
	q := quantum.New()
	state, err := q.EntangleNodes(nil)
	require.NoError(t, err)

	// Sum of |amplitude|^2 must equal 1 (within floating-point tolerance).
	var total float64
	for _, amp := range state.Amplitudes {
		r := real(amp)
		i := imag(amp)
		total += r*r + i*i
	}
	assert.InDelta(t, 1.0, total, 1e-10)
}

func TestEntangleNodes_GHZLikeStructure(t *testing.T) {
	// For 2 qubits: H(q0) + CNOT(0,1) should yield Bell state
	// |Φ+⟩ = (|00⟩ + e^{iφ}|11⟩) / √2
	q := quantum.NewWithOptions(2, 64)
	state, err := q.EntangleNodes(nil)
	require.NoError(t, err)

	// Only basis states 00 (index 0) and 11 (index 3) should be non-zero.
	for idx, amp := range state.Amplitudes {
		if idx != 0 && idx != 3 {
			assert.InDelta(t, 0.0, math.Pow(real(amp), 2)+math.Pow(imag(amp), 2), 1e-12,
				"unexpected non-zero amplitude at index %d", idx)
		}
	}
	// Both |00⟩ and |11⟩ should each have probability ≈ 0.5
	amp00 := state.Amplitudes[0]
	amp11 := state.Amplitudes[3]
	prob00 := math.Pow(real(amp00), 2) + math.Pow(imag(amp00), 2)
	prob11 := math.Pow(real(amp11), 2) + math.Pow(imag(amp11), 2)
	assert.InDelta(t, 0.5, prob00, 1e-10)
	assert.InDelta(t, 0.5, prob11, 1e-10)
}

func TestEntangleNodes_zeroActiveNodesError(t *testing.T) {
	q := quantum.NewWithOptions(1, 64)
	q.ActiveNodes = 0
	_, err := q.EntangleNodes(nil)
	require.Error(t, err)
}

// ─── ValidateConsensus ───────────────────────────────────────────────────────

func TestValidateConsensus_validGHZState(t *testing.T) {
	q := quantum.New()
	state, err := q.EntangleNodes(nil)
	require.NoError(t, err)
	// A properly entangled GHZ state has marginal probability 0.5 per qubit.
	assert.True(t, q.ValidateConsensus(state))
}

func TestValidateConsensus_nilState(t *testing.T) {
	q := quantum.New()
	assert.False(t, q.ValidateConsensus(nil))
}

func TestValidateConsensus_emptyState(t *testing.T) {
	q := quantum.New()
	empty := &quantum.QuantumState{
		Amplitudes: map[uint64]complex128{},
		NumQubits:  0,
	}
	assert.False(t, q.ValidateConsensus(empty))
}

func TestValidateConsensus_allZeroAmplitudes(t *testing.T) {
	q := quantum.New()
	// A state where only |0…0⟩ has non-zero amplitude: all marginals for
	// qubits 1..n-1 are zero, so consensus should fail.
	state := &quantum.QuantumState{
		Amplitudes: map[uint64]complex128{0: 1 + 0i},
		NumQubits:  2,
	}
	assert.False(t, q.ValidateConsensus(state))
}
