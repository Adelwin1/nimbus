package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
)

var ErrInvalidCiphertext = errors.New("invalid encrypted value")

type Encryptor struct {
	aead cipher.AEAD
}

func NewEncryptor(encodedKey string) (*Encryptor, error) {
	key, err := decodeKey(strings.TrimSpace(encodedKey))
	if err != nil {
		return nil, err
	}

	if len(key) != 32 {
		return nil, fmt.Errorf(
			"WEBHOOK_ENCRYPTION_KEY must decode to exactly 32 bytes",
		)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create AES cipher: %w", err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM cipher: %w", err)
	}

	return &Encryptor{aead: aead}, nil
}

func (e *Encryptor) Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}

	nonce := make([]byte, e.aead.NonceSize())

	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate encryption nonce: %w", err)
	}

	ciphertext := e.aead.Seal(
		nil,
		nonce,
		[]byte(plaintext),
		nil,
	)

	combined := append(nonce, ciphertext...)

	return base64.RawURLEncoding.EncodeToString(combined), nil
}

func (e *Encryptor) Decrypt(encodedCiphertext string) (string, error) {
	if encodedCiphertext == "" {
		return "", nil
	}

	combined, err := base64.RawURLEncoding.DecodeString(
		encodedCiphertext,
	)
	if err != nil {
		return "", ErrInvalidCiphertext
	}

	nonceSize := e.aead.NonceSize()
	if len(combined) <= nonceSize {
		return "", ErrInvalidCiphertext
	}

	nonce := combined[:nonceSize]
	ciphertext := combined[nonceSize:]

	plaintext, err := e.aead.Open(
		nil,
		nonce,
		ciphertext,
		nil,
	)
	if err != nil {
		return "", ErrInvalidCiphertext
	}

	return string(plaintext), nil
}

func decodeKey(encodedKey string) ([]byte, error) {
	if encodedKey == "" {
		return nil, errors.New("WEBHOOK_ENCRYPTION_KEY is required")
	}

	key, err := base64.StdEncoding.DecodeString(encodedKey)
	if err == nil {
		return key, nil
	}

	key, rawErr := base64.RawStdEncoding.DecodeString(encodedKey)
	if rawErr == nil {
		return key, nil
	}

	return nil, errors.New(
		"WEBHOOK_ENCRYPTION_KEY must be a valid base64 value",
	)
}
