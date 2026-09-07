package webhookdelivery

import (
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var (
	ErrSignatureMismatch      = errors.New("webhook: HMAC signature mismatch")
	ErrReplayTimestampExpired = errors.New("webhook: timestamp outside allowed replay tolerance window")
	ErrMalformedSignature     = errors.New("webhook: malformed signature or timestamp header")
)

const DefaultTimestampTolerance = 5 * time.Minute

// SignPayload computes an HMAC-SHA512 signature over timestamp and raw payload bytes.
func SignPayload(payload []byte, ts time.Time, secret string) (string, string) {
	tsUnix := ts.Unix()
	tsStr := strconv.FormatInt(tsUnix, 10)

	mac := hmac.New(sha512.New, []byte(secret))
	mac.Write([]byte(tsStr))
	mac.Write([]byte("."))
	mac.Write(payload)
	sum := mac.Sum(nil)

	sig := fmt.Sprintf("t=%s,v1=%s", tsStr, hex.EncodeToString(sum))
	return sig, tsStr
}

// VerifySignature validates that the HMAC-SHA512 matches and the timestamp is within the anti-replay window.
func VerifySignature(payload []byte, sigHeader string, secret string, tolerance time.Duration) error {
	if tolerance <= 0 {
		tolerance = DefaultTimestampTolerance
	}

	parts := strings.Split(sigHeader, ",")
	var tsStr, sigHex string

	for _, p := range parts {
		kv := strings.SplitN(strings.TrimSpace(p), "=", 2)
		if len(kv) == 2 {
			switch kv[0] {
			case "t":
				tsStr = kv[1]
			case "v1":
				sigHex = kv[1]
			}
		}
	}

	if tsStr == "" || sigHex == "" {
		return ErrMalformedSignature
	}

	tsUnix, err := strconv.ParseInt(tsStr, 10, 64)
	if err != nil {
		return fmt.Errorf("%w: invalid timestamp", ErrMalformedSignature)
	}

	// 1. Anti-replay check
	reqTime := time.Unix(tsUnix, 0)
	now := time.Now().UTC()
	diff := now.Sub(reqTime)
	if diff < 0 {
		diff = -diff
	}

	if diff > tolerance {
		return fmt.Errorf("%w: age %s exceeds tolerance %s", ErrReplayTimestampExpired, diff, tolerance)
	}

	// 2. Compute expected HMAC
	mac := hmac.New(sha512.New, []byte(secret))
	mac.Write([]byte(tsStr))
	mac.Write([]byte("."))
	mac.Write(payload)
	expectedSig := mac.Sum(nil)

	expectedHex := hex.EncodeToString(expectedSig)

	// Constant-time comparison
	if !hmac.Equal([]byte(sigHex), []byte(expectedHex)) {
		return ErrSignatureMismatch
	}

	return nil
}
