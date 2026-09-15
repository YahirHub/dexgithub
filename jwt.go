package dexgithub

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"strconv"
	"time"
)

func appJWT(credentials AppCredentials, now time.Time) (string, error) {
	if credentials.AppID <= 0 || credentials.PrivateKeyPEM == "" {
		return "", errors.New("dexgithub: credenciales de aplicación incompletas")
	}
	key, err := parseRSAPrivateKey([]byte(credentials.PrivateKeyPEM))
	if err != nil {
		return "", err
	}
	header, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT"})
	issuer := credentials.ClientID
	if issuer == "" {
		issuer = strconv.FormatInt(credentials.AppID, 10)
	}
	claims, _ := json.Marshal(map[string]any{
		"iat": now.Add(-60 * time.Second).Unix(),
		"exp": now.Add(9 * time.Minute).Unix(),
		"iss": issuer,
	})
	unsigned := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(claims)
	hash := sha256.Sum256([]byte(unsigned))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hash[:])
	if err != nil {
		return "", fmt.Errorf("firmar JWT de GitHub App: %w", err)
	}
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func parseRSAPrivateKey(data []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, errors.New("dexgithub: private key PEM inválida")
	}
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, errors.New("dexgithub: private key no es RSA PKCS#1/PKCS#8")
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("dexgithub: GitHub App requiere una private key RSA")
	}
	return key, nil
}
