package timeline

import "fmt"

// BlockState is the lifecycle state of a timeline block. GC reachability is a
// separate orthogonal dimension tracked in the local index database.
type BlockState string

const (
	StatePending    BlockState = "pending"    // created locally, not yet flushed/sealed
	StateCreating   BlockState = "creating"   // being assembled or uploaded
	StateSealed     BlockState = "sealed"     // content finalized and verified
	StateActive     BlockState = "active"     // referenced by current timeline/ref
	StateFrozen     BlockState = "frozen"     // archived into a larger block, no longer independently mutable
	StateSuperseded BlockState = "superseded" // replaced by a newer repacked block
	StateGarbage    BlockState = "garbage"    // unreferenced and past grace period
	StateDeleting   BlockState = "deleting"   // remote delete requested
	StateDeleted    BlockState = "deleted"    // remote delete confirmed
)

// validTransitions defines the allowed lifecycle transitions.
var validTransitions = map[BlockState][]BlockState{
	StatePending:    {StateCreating, StateActive, StateSuperseded, StateGarbage},
	StateCreating:   {StateSealed, StateGarbage},
	StateSealed:     {StateActive, StateSuperseded},
	StateActive:     {StateFrozen, StateSuperseded},
	StateFrozen:     {StateSuperseded},
	StateSuperseded: {StateGarbage},
	StateGarbage:    {StateDeleting},
	StateDeleting:   {StateDeleted},
}

// kindStates defines which lifecycle states each block kind may occupy.
var kindStates = map[Kind][]BlockState{
	KindNode: {
		StatePending, StateActive, StateSuperseded,
		StateGarbage, StateDeleting, StateDeleted,
	},
	KindSegment: {
		StatePending, StateCreating, StateSealed, StateActive,
		StateFrozen, StateSuperseded, StateGarbage, StateDeleting, StateDeleted,
	},
	KindArchive: {
		StateCreating, StateSealed, StateActive, StateSuperseded,
		StateGarbage, StateDeleting, StateDeleted,
	},
	KindCheckpoint: {
		StateCreating, StateSealed, StateActive, StateSuperseded,
		StateGarbage, StateDeleting, StateDeleted,
	},
}

// CanTransition reports whether a block may move from one state to another.
func CanTransition(from, to BlockState) bool {
	for _, next := range validTransitions[from] {
		if next == to {
			return true
		}
	}
	return false
}

// ValidStatesFor returns the lifecycle states a block kind may occupy.
func ValidStatesFor(kind Kind) []BlockState {
	states, ok := kindStates[kind]
	if !ok {
		return nil
	}
	out := make([]BlockState, len(states))
	copy(out, states)
	return out
}

// ValidateState ensures a kind/state combination is legal.
func ValidateState(kind Kind, state BlockState) error {
	for _, s := range ValidStatesFor(kind) {
		if s == state {
			return nil
		}
	}
	return fmt.Errorf("invalid state %q for kind %q", state, kind)
}
