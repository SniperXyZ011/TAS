package crypto

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

const secretByteLen = 32

// GenerateSecret creates a cryptographically random 32-byte hex-encoded secret
// suitable for use as an HMAC signing key for an edge node.
func GenerateSecret() (string, error) {
	b := make([]byte, secretByteLen)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("crypto: failed to generate secret: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// GenerateAPIKey creates a cryptographically random 32-byte hex-encoded API key.
func GenerateAPIKey() (string, error) {
	return GenerateSecret()
}

// BuildPayload constructs the deterministic string that must be HMAC-signed.
// Field order is fixed — both the ESP32 and this server must agree on it.
func BuildPayload(transactionID, nodeID, userID, weaponID, action string, timestamp int64) string {
	return fmt.Sprintf("%s|%s|%s|%s|%s|%d",
		transactionID, nodeID, userID, weaponID, action, timestamp)
}

// ComputeHMAC returns the HMAC-SHA256 hex digest of the payload using the given secret.
func ComputeHMAC(payload, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifySignature checks whether the provided signature matches the expected HMAC-SHA256.
// It uses hmac.Equal (constant-time comparison) to prevent timing attacks.
func VerifySignature(transactionID, nodeID, userID, weaponID, action string, timestamp int64, plaintextSecret, signature string) bool {
	payload := BuildPayload(transactionID, nodeID, userID, weaponID, action, timestamp)
	expected := ComputeHMAC(payload, plaintextSecret)

	// Decode both to bytes for constant-time comparison
	sigBytes, err := hex.DecodeString(signature)
	if err != nil {
		return false
	}
	expectedBytes, _ := hex.DecodeString(expected)
	return hmac.Equal(sigBytes, expectedBytes)
}
