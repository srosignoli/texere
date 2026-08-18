package content

import "testing"

func TestNodeIDIsTheShortID(t *testing.T) {
	if got := NodeID("73dhx9t2"); got != "73dhx9t2" {
		t.Errorf("NodeID = %q, want the short id", got)
	}
}

// An untracked node has no id and cannot be scheduled — unchanged behaviour.
func TestNodeIDIsEmptyWithoutAShortID(t *testing.T) {
	if got := NodeID(""); got != "" {
		t.Errorf("NodeID(\"\") = %q, want \"\"", got)
	}
}

// The property the whole feature rests on: the key does not change when the
// collection moves. Once the signature drops filepath/chapterTitle this is
// close to tautological — that is the point. It documents the intent rather
// than exercising a code path that could fail on its own.
func TestNodeIDDoesNotDependOnLocation(t *testing.T) {
	if NodeID("73dhx9t2") != NodeID("73dhx9t2") {
		t.Fatal("NodeID is not stable")
	}
}
