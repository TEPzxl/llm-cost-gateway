package policy

import "strings"

func Redact(text string, hits []PIIHit) string {
	if len(hits) == 0 {
		return text
	}
	normalized := mergeHits(hits)
	var builder strings.Builder
	cursor := 0
	for _, hit := range normalized {
		if hit.Start < cursor {
			continue
		}
		builder.WriteString(text[cursor:hit.Start])
		builder.WriteString("[REDACTED:" + hit.Type + "]")
		cursor = hit.End
	}
	builder.WriteString(text[cursor:])
	return builder.String()
}

func mergeHits(hits []PIIHit) []PIIHit {
	if len(hits) == 0 {
		return nil
	}
	merged := make([]PIIHit, 0, len(hits))
	for _, hit := range hits {
		if hit.Start < 0 || hit.End <= hit.Start {
			continue
		}
		if len(merged) == 0 {
			merged = append(merged, hit)
			continue
		}
		last := &merged[len(merged)-1]
		if hit.Start < last.End {
			if hit.End > last.End {
				last.End = hit.End
			}
			continue
		}
		merged = append(merged, hit)
	}
	return merged
}
