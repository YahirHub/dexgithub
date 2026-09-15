package dexgithub

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
)

// VerifyWebhookSignature validates GitHub's X-Hub-Signature-256 header using
// the webhook secret created for the GitHub App manifest.
func VerifyWebhookSignature(secret string, body []byte, signature string) bool {
	if secret == "" || len(body) == 0 {
		return false
	}
	const prefix = "sha256="
	if !strings.HasPrefix(signature, prefix) {
		return false
	}
	provided, err := hex.DecodeString(strings.TrimPrefix(signature, prefix))
	if err != nil || len(provided) != sha256.Size {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	expected := mac.Sum(nil)
	return hmac.Equal(expected, provided)
}

func (s *Service) VerifyWebhook(body []byte, signature string) (bool, error) {
	app, err := s.requireApp()
	if err != nil {
		return false, err
	}
	if app.WebhookSecret == "" {
		return false, errors.New("GitHub App sin webhook secret configurado")
	}
	return VerifyWebhookSignature(app.WebhookSecret, body, signature), nil
}
