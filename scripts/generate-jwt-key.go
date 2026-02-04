//go:build tools
// +build tools

package main

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

func main() {
	fmt.Println("=== GENERATE JWT RSA KEYPAIR ===")
	fmt.Println("")

	// Configuration
	keySize := 2048 // Can be changed to 4096 for production
	outputDir := "./certs/jwt"

	// Ask for key size
	fmt.Println("Select RSA key size:")
	fmt.Println("1. 2048-bit (Fast, Standard)")
	fmt.Println("2. 4096-bit (Slower, High Security)")
	fmt.Print("\nEnter choice (1 or 2) [default: 1]: ")

	var choice string
	fmt.Scanln(&choice)

	if choice == "2" {
		keySize = 4096
		fmt.Println("✅ Using RSA 4096-bit (production-grade)")
	} else {
		fmt.Println("✅ Using RSA 2048-bit (standard)")
	}

	fmt.Println("")
	fmt.Println("🔄 Generating RSA keypair...")

	// Generate RSA key pair
	privateKey, err := rsa.GenerateKey(rand.Reader, keySize)
	if err != nil {
		log.Fatalf("❌ Failed to generate RSA key: %v", err)
	}

	publicKey := &privateKey.PublicKey

	// Ensure output directory exists
	if err := os.MkdirAll(outputDir, 0700); err != nil {
		log.Fatalf("❌ Failed to create directory: %v", err)
	}

	// Save private key
	privateKeyPath := filepath.Join(outputDir, "private.pem")
	if err := savePrivateKey(privateKey, privateKeyPath); err != nil {
		log.Fatalf("❌ Failed to save private key: %v", err)
	}

	// Save public key
	publicKeyPath := filepath.Join(outputDir, "public.pem")
	if err := savePublicKey(publicKey, publicKeyPath); err != nil {
		log.Fatalf("❌ Failed to save public key: %v", err)
	}

	fmt.Println("✅ JWT RSA keypair generated successfully!")
	fmt.Println("")
	fmt.Println("📁 Files created:")
	fmt.Printf("   Private Key: %s\n", privateKeyPath)
	fmt.Printf("   Public Key:  %s\n", publicKeyPath)
	fmt.Println("")
	fmt.Println("� Key Information:")
	fmt.Printf("   Algorithm:   RS256 (RSA with SHA-256)\n")
	fmt.Printf("   Key Size:    %d bits\n", keySize)
	fmt.Printf("   Created:     %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Println("")
	fmt.Println("⚠️  IMPORTANT SECURITY NOTES:")
	fmt.Println("1. Keep private.pem SECRET and secure!")
	fmt.Println("2. Never commit private.pem to git!")
	fmt.Println("3. Set file permissions: chmod 600 private.pem")
	fmt.Println("4. Backup keys securely (encrypted)")
	fmt.Println("5. Rotate keys every 30 days (hospital compliance)")
	fmt.Println("")
	fmt.Println("📋 File Permissions (recommended):")
	fmt.Println("   private.pem: 600 (owner read/write only)")
	fmt.Println("   public.pem:  644 (owner read/write, others read)")
	fmt.Println("")
	fmt.Println("🔄 Key Rotation Policy:")
	fmt.Println("   • Development: 90 days")
	fmt.Println("   • Production:  30 days (hospital compliance)")
	fmt.Println("   • Run: go run scripts/rotate-jwt-keys.go")
	fmt.Println("")
	fmt.Println("✅ Next steps:")
	fmt.Println("   1. Backup these keys securely")
	fmt.Println("   2. Restart your server to use new keys")
	fmt.Println("   3. All users will need to login again (one time)")
	fmt.Println("")
}

func savePrivateKey(privateKey *rsa.PrivateKey, path string) error {
	// Convert to PKCS1 format
	privateKeyBytes := x509.MarshalPKCS1PrivateKey(privateKey)

	// Create PEM block
	privateKeyPEM := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privateKeyBytes,
	}

	// Write to file with restricted permissions
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to create private key file: %w", err)
	}
	defer file.Close()

	if err := pem.Encode(file, privateKeyPEM); err != nil {
		return fmt.Errorf("failed to encode private key PEM: %w", err)
	}

	return nil
}

func savePublicKey(publicKey *rsa.PublicKey, path string) error {
	// Convert to PKIX format
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return fmt.Errorf("failed to marshal public key: %w", err)
	}

	// Create PEM block
	publicKeyPEM := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	}

	// Write to file
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to create public key file: %w", err)
	}
	defer file.Close()

	if err := pem.Encode(file, publicKeyPEM); err != nil {
		return fmt.Errorf("failed to encode public key PEM: %w", err)
	}

	return nil
}
