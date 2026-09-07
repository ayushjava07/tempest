package providers

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
)

func TestSecretMaskingBridge(t *testing.T) {
	bridge := NewMaskingBridge()

	bridge.RegisterSecret([]byte("ghp_ABC123456789xyzSecretToken"))
	bridge.RegisterSecret([]byte("mysql://root:p@ssw0rd99@db:3306/app"))
	// Below MinMaskingLength (3 bytes), should not be registered
	bridge.RegisterSecret([]byte("foo"))

	if bridge.SecretCount() != 2 {
		t.Fatalf("expected 2 registered secrets, got %d", bridge.SecretCount())
	}

	logLine := "Error executing step: failed to connect to mysql://root:p@ssw0rd99@db:3306/app with token ghp_ABC123456789xyzSecretToken for foo"
	masked := bridge.MaskText(logLine)

	if strings.Contains(masked, "p@ssw0rd99") {
		t.Fatalf("masked output leaked mysql password: %s", masked)
	}
	if strings.Contains(masked, "ghp_ABC123456789xyzSecretToken") {
		t.Fatalf("masked output leaked token: %s", masked)
	}
	if !strings.Contains(masked, "for foo") {
		t.Fatalf("short string 'foo' should not have been masked: %s", masked)
	}
	if !strings.Contains(masked, RedactedPlaceholder) {
		t.Fatalf("expected redaction placeholder in output: %s", masked)
	}
}

func TestMaskingSecretProviderIntegration(t *testing.T) {
	ctx := context.Background()
	envP := NewEnvProvider("APP_")
	_ = envP.PutSecret(ctx, "aws.secret_key", []byte("wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"))

	bridge := NewMaskingBridge()
	maskedProvider := NewMaskingSecretProvider(envP, bridge)

	// Initially bridge knows 0 secrets
	if bridge.SecretCount() != 0 {
		t.Fatalf("expected 0 secrets before lookup")
	}

	// Fetch secret
	sec, err := maskedProvider.GetSecret(ctx, "aws.secret_key")
	if err != nil {
		t.Fatalf("failed to get secret: %v", err)
	}
	if string(sec.Value) != "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY" {
		t.Fatalf("unexpected secret value: %s", string(sec.Value))
	}

	// Bridge should have auto-registered the fetched secret
	if bridge.SecretCount() != 1 {
		t.Fatalf("expected 1 auto-registered secret, got %d", bridge.SecretCount())
	}

	traceLog := fmt.Sprintf("Authorization: AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE, Signature=%s", string(sec.Value))
	sanitized := bridge.MaskText(traceLog)

	if strings.Contains(sanitized, "wJalrXUtnFEMI") {
		t.Fatalf("trace log leaked AWS secret key: %s", sanitized)
	}
	if !strings.Contains(sanitized, RedactedPlaceholder) {
		t.Fatalf("expected RedactedPlaceholder in sanitized trace: %s", sanitized)
	}
}

func TestConcurrentSecretMasking(t *testing.T) {
	bridge := NewMaskingBridge()
	var wg sync.WaitGroup

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			token := fmt.Sprintf("secret-token-value-%04d", idx)
			bridge.RegisterSecret([]byte(token))

			text := fmt.Sprintf("Log entry with %s included", token)
			out := bridge.MaskText(text)
			if strings.Contains(out, token) {
				t.Errorf("token %s was not masked in %s", token, out)
			}
		}(i)
	}

	wg.Wait()
	if bridge.SecretCount() != 20 {
		t.Fatalf("expected 20 registered secrets, got %d", bridge.SecretCount())
	}
}
