package timeline

import "testing"

func TestValidTransitions(t *testing.T) {
	cases := []struct {
		from, to BlockState
		want     bool
	}{
		{StatePending, StateCreating, true},
		{StateCreating, StateSealed, true},
		{StateSealed, StateActive, true},
		{StateActive, StateFrozen, true},
		{StateFrozen, StateSuperseded, true},
		{StateSuperseded, StateGarbage, true},
		{StateGarbage, StateDeleting, true},
		{StateDeleting, StateDeleted, true},
		{StatePending, StateDeleted, false},
		{StateActive, StateGarbage, false},
		{StateDeleted, StateActive, false},
	}
	for _, c := range cases {
		if got := CanTransition(c.from, c.to); got != c.want {
			t.Fatalf("CanTransition(%q,%q)=%v want %v", c.from, c.to, got, c.want)
		}
	}
}

func TestValidateStatePerKind(t *testing.T) {
	if err := ValidateState(KindArchive, StateCreating); err != nil {
		t.Fatalf("archive creating should be valid: %v", err)
	}
	if err := ValidateState(KindArchive, StatePending); err == nil {
		t.Fatal("archive pending should be invalid")
	}
	if err := ValidateState(KindNode, StateSealed); err == nil {
		t.Fatal("node sealed should be invalid")
	}
	if err := ValidateState(KindNode, StatePending); err != nil {
		t.Fatalf("node pending should be valid: %v", err)
	}
}
