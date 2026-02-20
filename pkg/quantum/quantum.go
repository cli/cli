// Package quantum provides a simulated quantum consensus function with dynamic
// node scaling. It mirrors the QuantumConsensusFunction described in the design
// spec, translating the Qiskit-based Python prototype into pure Go using sparse
// statevector arithmetic.
package quantum

import (
	"fmt"
	"math"
	"math/cmplx"
	"time"
)

const (
	// DefaultMinNodes is the minimum number of active nodes.
	DefaultMinNodes = 8
	// DefaultMaxNodes is the maximum number of active nodes.
	DefaultMaxNodes = 64
	// DefaultCoherenceTime is the coherence threshold in seconds (1 ns).
	DefaultCoherenceTime = 1e-9
	// DefaultSuperpositionAngle is the initial superposition angle in radians (90°).
	DefaultSuperpositionAngle = math.Pi / 2
)

// QuantumConsensusFunction manages a simulated quantum consensus protocol with
// dynamic node scaling between [NodeRange[0], NodeRange[1]].
type QuantumConsensusFunction struct {
	// NodeRange holds the [min, max] number of active nodes.
	NodeRange [2]int
	// ActiveNodes is the current number of active nodes.
	ActiveNodes int
	// CoherenceTime is the maximum time window (in seconds) used during
	// consensus validation.
	CoherenceTime float64
	// SuperpositionAngle is the base Rz phase angle applied during entanglement.
	SuperpositionAngle float64
}

// New creates a QuantumConsensusFunction with the default parameters:
// min=8, max=64, coherence=1 ns, angle=π/2.
func New() *QuantumConsensusFunction {
	return NewWithOptions(DefaultMinNodes, DefaultMaxNodes)
}

// NewWithOptions creates a QuantumConsensusFunction with the given node range.
func NewWithOptions(minNodes, maxNodes int) *QuantumConsensusFunction {
	return &QuantumConsensusFunction{
		NodeRange:          [2]int{minNodes, maxNodes},
		ActiveNodes:        minNodes,
		CoherenceTime:      DefaultCoherenceTime,
		SuperpositionAngle: DefaultSuperpositionAngle,
	}
}

// AdjustNodes dynamically adjusts the number of active nodes based on the ratio
// currentLoad/maxLoad, clamped to [NodeRange[0], NodeRange[1]].
//
// This fixes the undefined max_load variable present in the original Python
// prototype by requiring callers to supply it explicitly.
func (q *QuantumConsensusFunction) AdjustNodes(currentLoad, maxLoad float64) (int, error) {
	if maxLoad <= 0 {
		return 0, fmt.Errorf("maxLoad must be positive, got %g", maxLoad)
	}
	loadFactor := currentLoad / maxLoad
	nodes := int(float64(q.NodeRange[1]) * loadFactor)
	if nodes < q.NodeRange[0] {
		nodes = q.NodeRange[0]
	}
	if nodes > q.NodeRange[1] {
		nodes = q.NodeRange[1]
	}
	q.ActiveNodes = nodes
	return q.ActiveNodes, nil
}

// QuantumState is a sparse statevector over q.ActiveNodes qubits.
// State indices are uint64 to support up to 64 qubits (2^64 basis states).
// Only non-zero amplitudes are stored.
type QuantumState struct {
	// Amplitudes maps computational-basis indices to complex amplitudes.
	Amplitudes map[uint64]complex128
	// NumQubits is the number of qubits represented by this state.
	NumQubits int
}

// EntangleNodes creates a full-entanglement (GHZ-like) quantum state across
// all active nodes.
//
// The circuit applied is:
//  1. H on qubit 0  (create superposition)
//  2. CNOT(0, k)    for k in [1, ActiveNodes)  (entangle)
//  3. Rz(angle_k)   on qubit k, where angle_k = SuperpositionAngle * k/ActiveNodes
//
// inputState is accepted for API compatibility; the simulation always starts
// from the |0…0⟩ computational basis state.
func (q *QuantumConsensusFunction) EntangleNodes(_ []float64) (*QuantumState, error) {
	n := q.ActiveNodes
	if n <= 0 {
		return nil, fmt.Errorf("ActiveNodes must be positive, got %d", n)
	}

	// |0…0⟩
	state := &QuantumState{
		Amplitudes: map[uint64]complex128{0: 1 + 0i},
		NumQubits:  n,
	}

	// H on qubit 0
	state = applyHadamard(state, 0)

	// CNOT(0, k) + Rz(angle_k) for each remaining qubit
	for qubit := 1; qubit < n; qubit++ {
		state = applyCNOT(state, 0, qubit)
		angle := q.SuperpositionAngle * float64(qubit) / float64(n)
		state = applyRz(state, qubit, angle)
	}

	return state, nil
}

// ValidateConsensus checks that the quantum state exhibits coherent consensus
// across all nodes within the CoherenceTime window.
//
// Coherence is declared valid when every qubit's marginal probability (the
// probability of measuring |1⟩ on that qubit) is strictly greater than zero,
// meaning every node participates in the entangled consensus state.
//
// The check mirrors the original Python loop:
//
//	start_time = time.time()
//	while (time.time() - start_time) < self.coherence_time:
//	    probabilities = statevector.probabilities()
//	    if np.all(probabilities > 0):  ...
func (q *QuantumConsensusFunction) ValidateConsensus(state *QuantumState) bool {
	if state == nil || state.NumQubits == 0 {
		return false
	}

	deadline := time.Duration(q.CoherenceTime * float64(time.Second))
	start := time.Now()

	for {
		probs := marginalProbabilities(state)
		allPositive := len(probs) > 0
		for _, p := range probs {
			if p <= 0 {
				allPositive = false
				break
			}
		}
		if allPositive {
			return true
		}
		if time.Since(start) >= deadline {
			break
		}
	}
	return false
}

// ─── internal gate helpers ────────────────────────────────────────────────────

// applyHadamard applies the Hadamard gate to the given qubit.
func applyHadamard(state *QuantumState, qubit int) *QuantumState {
	newAmps := make(map[uint64]complex128, len(state.Amplitudes)*2)
	inv := complex(1/math.Sqrt2, 0)
	mask := uint64(1) << uint(qubit)

	for idx, amp := range state.Amplitudes {
		flipped := idx ^ mask
		if (idx>>uint(qubit))&1 == 0 {
			// |0⟩ → (|0⟩ + |1⟩) / √2
			newAmps[idx] += inv * amp
			newAmps[flipped] += inv * amp
		} else {
			// |1⟩ → (|0⟩ − |1⟩) / √2
			newAmps[flipped] += inv * amp
			newAmps[idx] -= inv * amp
		}
	}
	return &QuantumState{Amplitudes: newAmps, NumQubits: state.NumQubits}
}

// applyCNOT applies the CNOT gate with the given control and target qubits.
func applyCNOT(state *QuantumState, control, target int) *QuantumState {
	newAmps := make(map[uint64]complex128, len(state.Amplitudes))
	tgtMask := uint64(1) << uint(target)

	for idx, amp := range state.Amplitudes {
		if (idx>>uint(control))&1 == 1 {
			newAmps[idx^tgtMask] += amp
		} else {
			newAmps[idx] += amp
		}
	}
	return &QuantumState{Amplitudes: newAmps, NumQubits: state.NumQubits}
}

// applyRz applies the Rz(angle) gate to the given qubit.
// Rz(θ)|0⟩ = e^{−iθ/2}|0⟩,  Rz(θ)|1⟩ = e^{iθ/2}|1⟩
func applyRz(state *QuantumState, qubit int, angle float64) *QuantumState {
	newAmps := make(map[uint64]complex128, len(state.Amplitudes))

	phase0 := cmplx.Exp(complex(0, -angle/2))
	phase1 := cmplx.Exp(complex(0, angle/2))

	for idx, amp := range state.Amplitudes {
		if (idx>>uint(qubit))&1 == 0 {
			newAmps[idx] += phase0 * amp
		} else {
			newAmps[idx] += phase1 * amp
		}
	}
	return &QuantumState{Amplitudes: newAmps, NumQubits: state.NumQubits}
}

// marginalProbabilities returns, for each qubit k, the total probability of
// measuring |1⟩ on qubit k (summed over all basis states where qubit k = 1).
func marginalProbabilities(state *QuantumState) []float64 {
	probs := make([]float64, state.NumQubits)
	for idx, amp := range state.Amplitudes {
		p := math.Pow(cmplx.Abs(amp), 2)
		for qubit := 0; qubit < state.NumQubits; qubit++ {
			if (idx>>uint(qubit))&1 == 1 {
				probs[qubit] += p
			}
		}
	}
	return probs
}
