package utils

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
)

type EncryptionManager struct {
	key []byte
}

func NewEncryptionManager(key string) (*EncryptionManager, error) {

	if len(key) != 64 {
		return nil, errors.New("encryption key must be exactly 64 hex characters long (32 bytes)")
	}

	keyBytes, err := hex.DecodeString(key)
	if err != nil {
		return nil, errors.New("encryption key must be a valid hex string")
	}

	if len(keyBytes) != 32 {
		return nil, errors.New("decoded encryption key must be exactly 32 bytes long")
	}

	return &EncryptionManager{key: keyBytes}, nil
}

func (e *EncryptionManager) Encrypt(plaintext string) (string, error) {
	block, err := aes.NewCipher(e.key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func (e *EncryptionManager) Decrypt(ciphertext string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(e.key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("ciphertext too short")
	}

	nonce, ciphertextBytes := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertextBytes, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}


func (em *EncryptionManager) GetKeyString() string {
	return hex.EncodeToString(em.key)
}
