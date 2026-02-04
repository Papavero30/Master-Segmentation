package utils

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

type JWTKeyManager struct {
	keyDir         string
	privateKeyPath string
	publicKeyPath  string
	keySize        int
}

type JWTKeyMetadata struct {
	Version      int       `json:"version"`
	KeySize      int       `json:"key_size"`
	CreatedAt    time.Time `json:"created_at"`
	RotateAfter  time.Time `json:"rotate_after"`
	Algorithm    string    `json:"algorithm"`
	IsProduction bool      `json:"is_production"`
}

func NewJWTKeyManager(keyDir string, keySize int) *JWTKeyManager {
	if keySize == 0 {
		keySize = 2048
	}

	return &JWTKeyManager{
		keyDir:         keyDir,
		privateKeyPath: filepath.Join(keyDir, "private.pem"),
		publicKeyPath:  filepath.Join(keyDir, "public.pem"),
		keySize:        keySize,
	}
}

func (km *JWTKeyManager) LoadOrGenerateKeys() (*rsa.PrivateKey, *rsa.PublicKey, error) {
	if km.keysExist() {
		log.Println(" JWT keys found, loading from disk...")
		privateKey, publicKey, err := km.loadKeysFromFile()
		if err != nil {
			log.Printf(" Failed to load existing keys: %v", err)
			return nil, nil, fmt.Errorf("failed to load existing keys: %w", err)
		}

		log.Printf(" JWT keys loaded successfully (key_size=%d, private=%s, public=%s)",
			km.keySize, km.privateKeyPath, km.publicKeyPath)

		return privateKey, publicKey, nil
	}

	log.Printf(" JWT keys not found, generating new keys (key_size=%d)...", km.keySize)

	privateKey, publicKey, err := km.generateAndSaveKeys()
	if err != nil {
		log.Printf(" Failed to generate and save keys: %v", err)
		return nil, nil, fmt.Errorf("failed to generate and save keys: %w", err)
	}

	log.Printf(" JWT keys generated and saved successfully (key_size=%d, private=%s, public=%s)",
		km.keySize, km.privateKeyPath, km.publicKeyPath)

	return privateKey, publicKey, nil
}

func (km *JWTKeyManager) keysExist() bool {
	_, errPrivate := os.Stat(km.privateKeyPath)
	_, errPublic := os.Stat(km.publicKeyPath)
	return errPrivate == nil && errPublic == nil
}

func (km *JWTKeyManager) loadKeysFromFile() (*rsa.PrivateKey, *rsa.PublicKey, error) {
	privateKeyBytes, err := os.ReadFile(km.privateKeyPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read private key file: %w", err)
	}

	privateBlock, _ := pem.Decode(privateKeyBytes)
	if privateBlock == nil {
		return nil, nil, fmt.Errorf("failed to decode private key PEM")
	}

	privateKey, err := x509.ParsePKCS1PrivateKey(privateBlock.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	publicKeyBytes, err := os.ReadFile(km.publicKeyPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read public key file: %w", err)
	}

	publicBlock, _ := pem.Decode(publicKeyBytes)
	if publicBlock == nil {
		return nil, nil, fmt.Errorf("failed to decode public key PEM")
	}

	publicKeyInterface, err := x509.ParsePKIXPublicKey(publicBlock.Bytes)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse public key: %w", err)
	}

	publicKey, ok := publicKeyInterface.(*rsa.PublicKey)
	if !ok {
		return nil, nil, fmt.Errorf("public key is not RSA")
	}

	return privateKey, publicKey, nil
}

func (km *JWTKeyManager) generateAndSaveKeys() (*rsa.PrivateKey, *rsa.PublicKey, error) {
	if err := os.MkdirAll(km.keyDir, 0700); err != nil {
		return nil, nil, fmt.Errorf("failed to create key directory: %w", err)
	}

	privateKey, err := rsa.GenerateKey(rand.Reader, km.keySize)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate RSA key: %w", err)
	}

	publicKey := &privateKey.PublicKey

	if err := km.savePrivateKey(privateKey); err != nil {
		return nil, nil, fmt.Errorf("failed to save private key: %w", err)
	}

	if err := km.savePublicKey(publicKey); err != nil {
		return nil, nil, fmt.Errorf("failed to save public key: %w", err)
	}

	return privateKey, publicKey, nil
}

func (km *JWTKeyManager) savePrivateKey(privateKey *rsa.PrivateKey) error {
	privateKeyBytes := x509.MarshalPKCS1PrivateKey(privateKey)

	privateKeyPEM := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privateKeyBytes,
	}

	file, err := os.OpenFile(km.privateKeyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to create private key file: %w", err)
	}
	defer file.Close()

	if err := pem.Encode(file, privateKeyPEM); err != nil {
		return fmt.Errorf("failed to encode private key PEM: %w", err)
	}

	return nil
}

func (km *JWTKeyManager) savePublicKey(publicKey *rsa.PublicKey) error {
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return fmt.Errorf("failed to marshal public key: %w", err)
	}

	publicKeyPEM := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	}

	file, err := os.OpenFile(km.publicKeyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to create public key file: %w", err)
	}
	defer file.Close()

	if err := pem.Encode(file, publicKeyPEM); err != nil {
		return fmt.Errorf("failed to encode public key PEM: %w", err)
	}

	return nil
}

func (km *JWTKeyManager) RotateKeys() (*rsa.PrivateKey, *rsa.PublicKey, error) {
	log.Printf(" Starting JWT key rotation (key_size=%d)...", km.keySize)

	if km.keysExist() {
		timestamp := time.Now().Format("20060102-150405")
		archiveDir := filepath.Join(km.keyDir, "archive")

		if err := os.MkdirAll(archiveDir, 0700); err != nil {
			return nil, nil, fmt.Errorf("failed to create archive directory: %w", err)
		}

		archivePrivatePath := filepath.Join(archiveDir, fmt.Sprintf("private-%s.pem", timestamp))
		if err := os.Rename(km.privateKeyPath, archivePrivatePath); err != nil {
			log.Printf("  Failed to archive private key: %v", err)
		}

		archivePublicPath := filepath.Join(archiveDir, fmt.Sprintf("public-%s.pem", timestamp))
		if err := os.Rename(km.publicKeyPath, archivePublicPath); err != nil {
			log.Printf("  Failed to archive public key: %v", err)
		}

		log.Printf(" Old keys archived (private=%s, public=%s)",
			archivePrivatePath, archivePublicPath)
	}

	privateKey, publicKey, err := km.generateAndSaveKeys()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate new keys during rotation: %w", err)
	}

	log.Printf(" JWT key rotation completed successfully (key_size=%d, private=%s, public=%s)",
		km.keySize, km.privateKeyPath, km.publicKeyPath)

	return privateKey, publicKey, nil
}

func (km *JWTKeyManager) GetKeyInfo() (*JWTKeyMetadata, error) {
	if !km.keysExist() {
		return nil, fmt.Errorf("keys do not exist")
	}

	fileInfo, err := os.Stat(km.privateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get key file info: %w", err)
	}

	privateKey, _, err := km.loadKeysFromFile()
	if err != nil {
		return nil, fmt.Errorf("failed to load keys: %w", err)
	}

	createdAt := fileInfo.ModTime()
	rotateAfter := createdAt.Add(30 * 24 * time.Hour)

	metadata := &JWTKeyMetadata{
		Version:      1,
		KeySize:      privateKey.N.BitLen(),
		CreatedAt:    createdAt,
		RotateAfter:  rotateAfter,
		Algorithm:    "RS256",
		IsProduction: os.Getenv("APP_ENV") == "production",
	}

	return metadata, nil
}

func (km *JWTKeyManager) ShouldRotate(rotationDays int) (bool, error) {
	metadata, err := km.GetKeyInfo()
	if err != nil {
		return false, err
	}

	age := time.Since(metadata.CreatedAt)
	maxAge := time.Duration(rotationDays) * 24 * time.Hour

	return age >= maxAge, nil
}

func (km *JWTKeyManager) ValidateKeys() error {
	if !km.keysExist() {
		return fmt.Errorf("keys do not exist")
	}

	privateKey, publicKey, err := km.loadKeysFromFile()
	if err != nil {
		return fmt.Errorf("failed to load keys: %w", err)
	}

	keySize := privateKey.N.BitLen()
	if keySize < 2048 {
		return fmt.Errorf("key size %d is too small (minimum 2048 bits)", keySize)
	}

	if publicKey.N.Cmp(privateKey.N) != 0 || publicKey.E != privateKey.E {
		return fmt.Errorf("public key does not match private key")
	}

	log.Printf(" JWT keys validated successfully (key_size=%d)", keySize)

	return nil
}
