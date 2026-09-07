package masker

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Strategy defines how sensitive substrings are obfuscated.
type Strategy int

const (
	StrategyRedactFull Strategy = iota
	StrategyMaskPartial
	StrategyPseudonymize
)

// Masker detects and obfuscates PII and credentials in strings and data structures.
type Masker struct {
	strategy      Strategy
	hmacKey       []byte
	sensitiveKeys map[string]bool
	emailRegex    *regexp.Regexp
	ssnRegex      *regexp.Regexp
	jwtRegex      *regexp.Regexp
	bearerRegex   *regexp.Regexp
	creditCardReg *regexp.Regexp
}

func NewMasker(strat Strategy, hmacKey []byte) *Masker {
	sensitive := map[string]bool{
		"password":      true,
		"passwd":        true,
		"secret":        true,
		"token":         true,
		"api_key":       true,
		"apikey":        true,
		"access_token":  true,
		"refresh_token": true,
		"authorization": true,
		"ssn":           true,
		"credit_card":   true,
	}

	return &Masker{
		strategy:      strat,
		hmacKey:       hmacKey,
		sensitiveKeys: sensitive,
		emailRegex:    regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`),
		ssnRegex:      regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`),
		jwtRegex:      regexp.MustCompile(`\beyJ[a-zA-Z0-9_-]+\.eyJ[a-zA-Z0-9_-]+\.[a-zA-Z0-9_-]+\b`),
		bearerRegex:   regexp.MustCompile(`(?i)\bBearer\s+[a-zA-Z0-9._~+\-/=]+\b`),
		creditCardReg: regexp.MustCompile(`\b(?:\d{4}[-\s]?){3}\d{4}\b`),
	}
}

// MaskText scans and replaces all detected PII and credentials within a text buffer.
func (m *Masker) MaskText(input string) string {
	if input == "" {
		return ""
	}

	// 1. Bearer headers
	input = m.bearerRegex.ReplaceAllStringFunc(input, func(s string) string {
		return "Bearer [REDACTED]"
	})

	// 2. JWT tokens
	input = m.jwtRegex.ReplaceAllStringFunc(input, func(s string) string {
		return m.applyStrategy(s, "JWT")
	})

	// 3. Credit cards (only if passes Luhn checksum)
	input = m.creditCardReg.ReplaceAllStringFunc(input, func(s string) string {
		digits := strings.Map(func(r rune) rune {
			if r >= '0' && r <= '9' {
				return r
			}
			return -1
		}, s)
		if len(digits) == 16 && isValidLuhn(digits) {
			if m.strategy == StrategyMaskPartial {
				return digits[:4] + "-****-****-" + digits[12:]
			}
			return m.applyStrategy(s, "CC")
		}
		return s
	})

	// 4. SSN
	input = m.ssnRegex.ReplaceAllStringFunc(input, func(s string) string {
		if m.strategy == StrategyMaskPartial {
			return "***-**-" + s[7:]
		}
		return m.applyStrategy(s, "SSN")
	})

	// 5. Emails
	input = m.emailRegex.ReplaceAllStringFunc(input, func(s string) string {
		if m.strategy == StrategyMaskPartial {
			parts := strings.SplitN(s, "@", 2)
			if len(parts[0]) > 2 {
				return parts[0][:2] + "***@" + parts[1]
			}
			return "***@" + parts[1]
		}
		return m.applyStrategy(s, "EMAIL")
	})

	return input
}

// MaskMap recursively scrubs sensitive key names and string values within a map.
func (m *Masker) MaskMap(data map[string]any) map[string]any {
	out := make(map[string]any, len(data))
	for k, v := range data {
		lowerKey := strings.ToLower(k)
		if m.sensitiveKeys[lowerKey] {
			out[k] = "[REDACTED]"
			continue
		}

		switch val := v.(type) {
		case string:
			out[k] = m.MaskText(val)
		case map[string]any:
			out[k] = m.MaskMap(val)
		case []any:
			out[k] = m.maskSlice(val)
		default:
			out[k] = val
		}
	}
	return out
}

func (m *Masker) maskSlice(slice []any) []any {
	out := make([]any, len(slice))
	for i, item := range slice {
		switch v := item.(type) {
		case string:
			out[i] = m.MaskText(v)
		case map[string]any:
			out[i] = m.MaskMap(v)
		case []any:
			out[i] = m.maskSlice(v)
		default:
			out[i] = v
		}
	}
	return out
}

func (m *Masker) applyStrategy(val, label string) string {
	switch m.strategy {
	case StrategyRedactFull:
		return fmt.Sprintf("[%s_REDACTED]", label)
	case StrategyPseudonymize:
		h := hmac.New(sha256.New, m.hmacKey)
		h.Write([]byte(val))
		return "pseudo:" + hex.EncodeToString(h.Sum(nil))[:16]
	default:
		return "[REDACTED]"
	}
}

// isValidLuhn computes the Luhn checksum on a sequence of numeric digits.
func isValidLuhn(s string) bool {
	sum := 0
	alternate := false

	for i := len(s) - 1; i >= 0; i-- {
		n, err := strconv.Atoi(string(s[i]))
		if err != nil {
			return false
		}

		if alternate {
			n *= 2
			if n > 9 {
				n = (n % 10) + 1
			}
		}

		sum += n
		alternate = !alternate
	}

	return (sum % 10) == 0
}
