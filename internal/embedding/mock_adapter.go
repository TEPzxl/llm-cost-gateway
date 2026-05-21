package embedding

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"math"
	"strings"
	"unicode"
)

const mockDimension = 64

type MockAdapter struct{}

func NewMockAdapter() *MockAdapter {
	return &MockAdapter{}
}

func (a *MockAdapter) Embed(_ context.Context, text string) ([]float64, error) {
	vector := make([]float64, mockDimension)
	for _, token := range tokenize(text) {
		sum := sha256.Sum256([]byte(token))
		index := binary.BigEndian.Uint64(sum[:8]) % mockDimension
		vector[index]++
	}
	normalize(vector)
	return vector, nil
}

func TextForMessages(messages []struct {
	Role    string
	Content string
}) string {
	var builder strings.Builder
	for _, message := range messages {
		if builder.Len() > 0 {
			builder.WriteByte('\n')
		}
		builder.WriteString(message.Role)
		builder.WriteByte(':')
		builder.WriteString(message.Content)
	}
	return builder.String()
}

func tokenize(value string) []string {
	return strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

func normalize(vector []float64) {
	var sum float64
	for _, value := range vector {
		sum += value * value
	}
	if sum == 0 {
		return
	}
	norm := math.Sqrt(sum)
	for index := range vector {
		vector[index] /= norm
	}
}
