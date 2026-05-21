package policy

import (
	"regexp"
	"sort"
)

const (
	PIIEmail          = "email"
	PIIPhone          = "phone"
	PIICreditCardLike = "credit_card_like"
	PIIAPIKeyLike     = "api_key_like"
)

var (
	emailPattern      = regexp.MustCompile(`(?i)\b[A-Z0-9._%+\-]+@[A-Z0-9.\-]+\.[A-Z]{2,}\b`)
	phonePattern      = regexp.MustCompile(`(?:\+?\d[\d\s().-]{7,}\d)`)
	creditCardPattern = regexp.MustCompile(`(?:\d[ -]?){13,19}`)
	apiKeyPattern     = regexp.MustCompile(`(?i)\b(?:sk-[A-Za-z0-9_-]{16,}|llmgw_[A-Za-z0-9_-]{16,}|api[_-]?key[:=][A-Za-z0-9_-]{12,})\b`)
)

type PIIHit struct {
	Type  string
	Start int
	End   int
}

type Detector struct{}

func NewDetector() *Detector {
	return &Detector{}
}

func (d *Detector) Detect(text string) []PIIHit {
	var hits []PIIHit
	hits = appendRegexHits(hits, PIIEmail, emailPattern, text)
	hits = appendCreditCardHits(hits, text)
	hits = appendPhoneHits(hits, text)
	hits = appendRegexHits(hits, PIIAPIKeyLike, apiKeyPattern, text)
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Start != hits[j].Start {
			return hits[i].Start < hits[j].Start
		}
		return hits[i].End < hits[j].End
	})
	return hits
}

func appendPhoneHits(hits []PIIHit, text string) []PIIHit {
	for _, match := range phonePattern.FindAllStringIndex(text, -1) {
		if overlapsType(hits, match[0], match[1], PIICreditCardLike) {
			continue
		}
		hits = append(hits, PIIHit{Type: PIIPhone, Start: match[0], End: match[1]})
	}
	return hits
}

func HitTypes(hits []PIIHit) []string {
	seen := map[string]struct{}{}
	types := make([]string, 0, len(hits))
	for _, hit := range hits {
		if _, ok := seen[hit.Type]; ok {
			continue
		}
		seen[hit.Type] = struct{}{}
		types = append(types, hit.Type)
	}
	sort.Strings(types)
	return types
}

func appendRegexHits(hits []PIIHit, hitType string, pattern *regexp.Regexp, text string) []PIIHit {
	for _, match := range pattern.FindAllStringIndex(text, -1) {
		hits = append(hits, PIIHit{Type: hitType, Start: match[0], End: match[1]})
	}
	return hits
}

func appendCreditCardHits(hits []PIIHit, text string) []PIIHit {
	for _, match := range creditCardPattern.FindAllStringIndex(text, -1) {
		candidate := text[match[0]:match[1]]
		digits := digitsOnly(candidate)
		if len(digits) < 13 || len(digits) > 19 || !luhnValid(digits) {
			continue
		}
		hits = append(hits, PIIHit{Type: PIICreditCardLike, Start: match[0], End: match[1]})
	}
	return hits
}

func overlapsType(hits []PIIHit, start int, end int, hitType string) bool {
	for _, hit := range hits {
		if hit.Type != hitType {
			continue
		}
		if start < hit.End && end > hit.Start {
			return true
		}
	}
	return false
}

func digitsOnly(value string) string {
	result := make([]byte, 0, len(value))
	for index := 0; index < len(value); index++ {
		if value[index] >= '0' && value[index] <= '9' {
			result = append(result, value[index])
		}
	}
	return string(result)
}

func luhnValid(digits string) bool {
	var sum int
	double := false
	for index := len(digits) - 1; index >= 0; index-- {
		value := int(digits[index] - '0')
		if double {
			value *= 2
			if value > 9 {
				value -= 9
			}
		}
		sum += value
		double = !double
	}
	return sum%10 == 0
}
