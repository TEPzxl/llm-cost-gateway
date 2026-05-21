package policy

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tep/llm-cost-gateway/internal/store"
	db "github.com/tep/llm-cost-gateway/internal/store/sqlc"
	"github.com/tep/llm-cost-gateway/internal/testutil"
)

func TestDetectorDetectsSupportedPII(t *testing.T) {
	detector := NewDetector()
	tests := []struct {
		name string
		text string
		want string
	}{
		{name: "email", text: "contact alice@example.com", want: PIIEmail},
		{name: "phone", text: "call +1 (415) 555-0134 now", want: PIIPhone},
		{name: "credit card", text: "card 4111 1111 1111 1111", want: PIICreditCardLike},
		{name: "api key", text: "sk-abcdefghijklmnopqrstuvwxyz123456", want: PIIAPIKeyLike},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hits := detector.Detect(tt.text)
			types := HitTypes(hits)
			if len(types) != 1 || types[0] != tt.want {
				t.Fatalf("types = %v, want [%s]", types, tt.want)
			}
		})
	}
}

func TestRedactRemovesMatchedText(t *testing.T) {
	detector := NewDetector()
	text := "email alice@example.com and call +1 (415) 555-0134"
	redacted := Redact(text, detector.Detect(text))

	if redacted == text {
		t.Fatal("redacted text did not change")
	}
	for _, leaked := range []string{"alice@example.com", "+1 (415) 555-0134"} {
		if strings.Contains(redacted, leaked) {
			t.Fatalf("redacted text leaked %q: %s", leaked, redacted)
		}
	}
}

func TestContentPolicyServiceDefaultsToAllow(t *testing.T) {
	ctx := context.Background()
	st := store.New(testutil.OpenPostgres(t, ctx))
	resetPolicyTestDatabase(t, ctx, st)
	org := createPolicyTestOrg(t, ctx, st)

	service := NewService(st)
	action, err := service.ActivePIIAction(ctx, org.ID)
	if err != nil {
		t.Fatalf("ActivePIIAction returned error: %v", err)
	}
	if action != ActionAllow {
		t.Fatalf("action = %q, want allow", action)
	}
}

func TestContentPolicyServiceCreatesAndReadsActivePolicy(t *testing.T) {
	ctx := context.Background()
	st := store.New(testutil.OpenPostgres(t, ctx))
	resetPolicyTestDatabase(t, ctx, st)
	org := createPolicyTestOrg(t, ctx, st)

	service := NewService(st)
	if _, err := service.CreateContentPolicy(ctx, CreateContentPolicyParams{
		OrgID:     org.ID,
		Name:      "pii-redact",
		PIIAction: ActionRedact,
	}); err != nil {
		t.Fatalf("CreateContentPolicy returned error: %v", err)
	}
	action, err := service.ActivePIIAction(ctx, org.ID)
	if err != nil {
		t.Fatalf("ActivePIIAction returned error: %v", err)
	}
	if action != ActionRedact {
		t.Fatalf("action = %q, want redact", action)
	}
}

func resetPolicyTestDatabase(t *testing.T, ctx context.Context, st *store.Store) {
	t.Helper()

	_, err := st.Pool.Exec(ctx, `
		TRUNCATE
			content_policies,
			organizations
		CASCADE
	`)
	if err != nil {
		t.Fatalf("reset test database: %v", err)
	}
}

func createPolicyTestOrg(t *testing.T, ctx context.Context, st *store.Store) db.Organization {
	t.Helper()

	now := time.Now().UTC()
	org, err := st.Queries.CreateOrganization(ctx, db.CreateOrganizationParams{
		ID:        uuid.New(),
		Name:      "Policy Test Org",
		Slug:      "policy-test-" + uuid.NewString(),
		Status:    "active",
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("CreateOrganization returned error: %v", err)
	}
	return org
}
