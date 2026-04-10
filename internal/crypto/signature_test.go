package crypto

import (
	"strings"
	"testing"
)

func TestGenerateSecret_Length(t *testing.T) {
	s, err := GenerateSecret()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 32 bytes → 64 hex chars
	if len(s) != 64 {
		t.Errorf("expected 64 hex chars, got %d", len(s))
	}
}

func TestGenerateSecret_Unique(t *testing.T) {
	a, _ := GenerateSecret()
	b, _ := GenerateSecret()
	if a == b {
		t.Error("two generated secrets should not be equal")
	}
}

func TestBuildPayload_Deterministic(t *testing.T) {
	p1 := BuildPayload("tx1", "n1", "u1", "w1", "checkout", 1234567890)
	p2 := BuildPayload("tx1", "n1", "u1", "w1", "checkout", 1234567890)
	if p1 != p2 {
		t.Error("BuildPayload must be deterministic")
	}
	if !strings.Contains(p1, "|") {
		t.Error("payload separator missing")
	}
}

func TestVerifySignature_Valid(t *testing.T) {
	secret := "test_secret_for_unit_tests_only_!!"
	sig := ComputeHMAC(BuildPayload("tx1", "n1", "u1", "w1", "checkout", 100), secret)

	if !VerifySignature("tx1", "n1", "u1", "w1", "checkout", 100, secret, sig) {
		t.Error("expected signature verification to pass")
	}
}

func TestVerifySignature_WrongSecret(t *testing.T) {
	sig := ComputeHMAC(BuildPayload("tx1", "n1", "u1", "w1", "checkout", 100), "secret_A")
	if VerifySignature("tx1", "n1", "u1", "w1", "checkout", 100, "secret_B", sig) {
		t.Error("verification should fail with wrong secret")
	}
}

func TestVerifySignature_TamperedField(t *testing.T) {
	secret := "test_secret_for_unit_tests_only_!!"
	sig := ComputeHMAC(BuildPayload("tx1", "n1", "u1", "w1", "checkout", 100), secret)
	// Tamper with weapon ID
	if VerifySignature("tx1", "n1", "u1", "TAMPERED", "checkout", 100, secret, sig) {
		t.Error("verification should fail with tampered field")
	}
}

func TestVerifySignature_InvalidHex(t *testing.T) {
	if VerifySignature("tx1", "n1", "u1", "w1", "checkout", 100, "secret", "not-valid-hex!!!") {
		t.Error("verification should fail with invalid hex signature")
	}
}
