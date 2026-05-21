package embedding

import (
	"context"
	"testing"
)

func TestMockAdapterEmbeddingsAreDeterministic(t *testing.T) {
	ctx := context.Background()
	adapter := NewMockAdapter()

	first, err := adapter.Embed(ctx, "Hello, world!")
	if err != nil {
		t.Fatalf("Embed returned error: %v", err)
	}
	second, err := adapter.Embed(ctx, "hello world")
	if err != nil {
		t.Fatalf("Embed returned error: %v", err)
	}
	if len(first) != len(second) {
		t.Fatalf("vector length = %d/%d", len(first), len(second))
	}
	for index := range first {
		if first[index] != second[index] {
			t.Fatalf("vector[%d] = %f/%f, want identical normalized token embedding", index, first[index], second[index])
		}
	}
}
