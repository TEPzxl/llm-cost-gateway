package embedding

import "context"

type Adapter interface {
	Embed(ctx context.Context, text string) ([]float64, error)
}
