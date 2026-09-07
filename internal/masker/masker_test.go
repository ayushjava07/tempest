package masker

import (
	"strings"
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestMasker_CreditCardLuhn(t *testing.T) {
	maskerFull := NewMasker(StrategyRedactFull, nil)
	maskerPart := NewMasker(StrategyMaskPartial, nil)

	// Valid Luhn Visa test card: 4111111111111111
	validCC := "Charged card 4111-1111-1111-1111 for order"
	redacted := maskerFull.MaskText(validCC)
	if !strings.Contains(redacted, "[CC_REDACTED]") {
		t.Errorf("expected [CC_REDACTED], got %q", redacted)
	}

	partial := maskerPart.MaskText(validCC)
	if !strings.Contains(partial, "4111-****-****-1111") {
		t.Errorf("expected partial mask 4111-****-****-1111, got %q", partial)
	}

	// Invalid Luhn (last digit altered): should not be masked as a credit card
	invalidCC := "Reference number 4111-1111-1111-1112 in log"
	preserved := maskerFull.MaskText(invalidCC)
	if !strings.Contains(preserved, "4111-1111-1111-1112") {
		t.Errorf("invalid Luhn should be preserved, got %q", preserved)
	}
}

func TestMasker_SSN(t *testing.T) {
	masker := NewMasker(StrategyRedactFull, nil)
	text := "User SSN is 123-45-6789 verified"
	res := masker.MaskText(text)
	if !strings.Contains(res, "[SSN_REDACTED]") {
		t.Errorf("expected [SSN_REDACTED], got %q", res)
	}
}

func TestMasker_Email(t *testing.T) {
	maskerPart := NewMasker(StrategyMaskPartial, nil)
	text := "Contact alice.smith@example.com for help"
	res := maskerPart.MaskText(text)
	if !strings.Contains(res, "al***@example.com") {
		t.Errorf("expected al***@example.com, got %q", res)
	}
}

func TestMasker_JWTAndBearer(t *testing.T) {
	masker := NewMasker(StrategyRedactFull, nil)
	text := "Header: Bearer eyJhbGciOi.eyJzdWIiOi.signature_val for auth"
	res := masker.MaskText(text)
	if !strings.Contains(res, "Bearer [REDACTED]") {
		t.Errorf("expected Bearer [REDACTED], got %q", res)
	}
}

func TestMasker_MaskMap(t *testing.T) {
	masker := NewMasker(StrategyRedactFull, nil)
	input := map[string]any{
		"username": "alice",
		"password": "super-secret-password",
		"nested": map[string]any{
			"api_key": "secret-key-12345",
			"email":   "test@company.org",
		},
		"tags": []any{"user-tag", "admin@domain.com"},
	}

	masked := masker.MaskMap(input)

	if masked["password"] != "[REDACTED]" {
		t.Errorf("expected password to be [REDACTED], got %v", masked["password"])
	}

	nested := masked["nested"].(map[string]any)
	if nested["api_key"] != "[REDACTED]" {
		t.Errorf("expected api_key to be [REDACTED], got %v", nested["api_key"])
	}
	if nested["email"] != "[EMAIL_REDACTED]" {
		t.Errorf("expected email to be masked, got %v", nested["email"])
	}

	tags := masked["tags"].([]any)
	if tags[1] != "[EMAIL_REDACTED]" {
		t.Errorf("expected slice email to be masked, got %v", tags[1])
	}
}

func TestMasker_Pseudonymize(t *testing.T) {
	key := []byte("test-key-32-bytes-long-for-hmac")
	masker := NewMasker(StrategyPseudonymize, key)

	text1 := "User email is user@example.com"
	text2 := "User email is user@example.com"

	res1 := masker.MaskText(text1)
	res2 := masker.MaskText(text2)

	if !strings.Contains(res1, "pseudo:") {
		t.Fatalf("expected pseudonym format, got %q", res1)
	}
	// Deterministic: same input should yield identical pseudonym
	if res1 != res2 {
		t.Errorf("expected deterministic pseudonyms: %q != %q", res1, res2)
	}
}
