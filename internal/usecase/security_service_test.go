package usecase

import (
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func TestVerifyTOTPMatchesRFC6238SixDigitCode(t *testing.T) {
	// RFC 6238's SHA-1 test secret produces 94287082 in eight-digit mode;
	// this application displays six digits, so the expected code is 287082.
	secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	if !verifyTOTP(secret, "287082", time.Unix(59, 0).UTC()) {
		t.Fatal("expected the current RFC 6238 TOTP code to validate")
	}
	if verifyTOTP(secret, "000000", time.Unix(59, 0).UTC()) {
		t.Fatal("did not expect an incorrect TOTP code to validate")
	}
}

func TestPasswordHashSupportsLongPassphrases(t *testing.T) {
	password := strings.Repeat("correct-horse-battery-staple-", 20)
	if len(password) <= 72 {
		t.Fatal("test password must exceed bcrypt's 72-byte limit")
	}
	encoded, err := hashPassword(password)
	if err != nil {
		t.Fatalf("hashPassword() error = %v", err)
	}
	if !strings.HasPrefix(encoded, "$argon2id$") {
		t.Fatalf("hashPassword() prefix = %q, want Argon2id", encoded[:min(len(encoded), 16)])
	}
	if !verifyPassword(encoded, password) {
		t.Fatal("verifyPassword() rejected the correct long passphrase")
	}
	if verifyPassword(encoded, password+"x") {
		t.Fatal("verifyPassword() accepted an incorrect passphrase")
	}
}

func TestPasswordVerificationSupportsExistingBcryptHashes(t *testing.T) {
	const password = "existing-password"
	encoded, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("GenerateFromPassword() error = %v", err)
	}
	if !verifyPassword(string(encoded), password) {
		t.Fatal("verifyPassword() rejected a legacy bcrypt hash")
	}
}

func TestMFASecretEncryptionRoundTripsAndRejectsWrongKey(t *testing.T) {
	key := []byte("01234567890123456789012345678901")
	service := &SecurityService{mfaKey: key}
	ciphertext, err := service.encryptMFASecret("JBSWY3DPEHPK3PXP")
	if err != nil {
		t.Fatalf("encryptMFASecret() error = %v", err)
	}
	got, err := service.decryptMFASecret(ciphertext)
	if err != nil || got != "JBSWY3DPEHPK3PXP" {
		t.Fatalf("decryptMFASecret() = %q, %v; want original secret", got, err)
	}

	wrongKey := &SecurityService{mfaKey: []byte("11234567890123456789012345678901")}
	if _, err := wrongKey.decryptMFASecret(ciphertext); err == nil {
		t.Fatal("expected encrypted MFA secret to reject a different key")
	}
}
