package json

import (
	"testing"
)

func TestEscapeString(t *testing.T) {
	t.Parallel()

	b := EscapeStringAppend(nil, []byte("foo"))
	if string(b) != `"foo"` {
		t.Fatalf("Expected: %v\nGot: %v", `"foo"`, b)
	}

	b = EscapeStringAppend(nil, []byte(`f"oo`))
	if string(b) != `"f\"oo"` {
		t.Fatalf("Expected: %v\nGot: %v", `"f\"oo"`, b)
	}
}
