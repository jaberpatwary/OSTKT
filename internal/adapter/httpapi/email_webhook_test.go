package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"testing"
	"time"
)

func TestVerifyEmailWebhookSignatureChecksPayloadSecretAndTimestamp(t *testing.T) {
	now := time.Date(2026, time.November, 28, 12, 30, 0, 0, time.UTC)
	timestamp := strconv.FormatInt(now.Unix(), 10)
	secret := "a-random-secret-with-at-least-thirty-two-bytes"
	body := []byte(`{"message_id":"mail-1","from":"staff@example.test","subject":"VPN","text":"Disconnected"}`)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp + "."))
	_, _ = mac.Write(body)
	signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if !verifyEmailWebhookSignature(secret, timestamp, signature, body, now) {
		t.Fatal("expected valid signature to be accepted")
	}
	if verifyEmailWebhookSignature(secret, timestamp, signature, append(body, ' '), now) {
		t.Fatal("expected modified body to be rejected")
	}
	if verifyEmailWebhookSignature("short", timestamp, signature, body, now) {
		t.Fatal("expected short secrets to be rejected")
	}
	staleTimestamp := strconv.FormatInt(now.Add(-6*time.Minute).Unix(), 10)
	staleMAC := hmac.New(sha256.New, []byte(secret))
	_, _ = staleMAC.Write([]byte(staleTimestamp + "."))
	_, _ = staleMAC.Write(body)
	staleSignature := hex.EncodeToString(staleMAC.Sum(nil))
	if verifyEmailWebhookSignature(secret, staleTimestamp, staleSignature, body, now) {
		t.Fatal("expected stale timestamp to be rejected")
	}
}
