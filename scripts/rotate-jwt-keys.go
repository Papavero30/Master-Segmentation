//go:build tools
// +build tools

package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/IntegratedBrainEnvironment/BrainNav_GO_BE/helpers/utils"
)

func main() {
	fmt.Println("=== JWT KEY ROTATION ===")
	fmt.Println("")

	// Configuration
	keyDir := "./certs/jwt"
	keySize := 2048

	// Check if directory exists
	if _, err := os.Stat(keyDir); os.IsNotExist(err) {
		log.Fatalf("❌ Key directory does not exist: %s", keyDir)
	}

	// Create key manager
	keyManager := utils.NewJWTKeyManager(keyDir, keySize)

	// Get current key info
	fmt.Println("📊 Current Key Information:")
	if metadata, err := keyManager.GetKeyInfo(); err == nil {
		fmt.Printf("   Created:     %s\n", metadata.CreatedAt.Format("2006-01-02 15:04:05"))
		fmt.Printf("   Rotate After: %s\n", metadata.RotateAfter.Format("2006-01-02 15:04:05"))
		fmt.Printf("   Key Size:    %d bits\n", metadata.KeySize)
		fmt.Printf("   Algorithm:   %s\n", metadata.Algorithm)
		fmt.Println("")
	}

	// Confirm rotation
	fmt.Println("⚠️  WARNING: Key rotation will:")
	fmt.Println("   1. Archive current keys to ./certs/jwt/archive/")
	fmt.Println("   2. Generate new RSA keypair")
	fmt.Println("   3. All active tokens will be INVALIDATED")
	fmt.Println("   4. All users must LOGIN AGAIN")
	fmt.Println("")
	fmt.Print("Do you want to continue? (yes/no): ")

	var confirm string
	fmt.Scanln(&confirm)

	if confirm != "yes" && confirm != "y" {
		fmt.Println("❌ Key rotation cancelled")
		return
	}

	fmt.Println("")
	fmt.Println("🔄 Starting key rotation...")

	// Rotate keys
	privateKey, publicKey, err := keyManager.RotateKeys()
	if err != nil {
		log.Fatalf("❌ Key rotation failed: %v", err)
	}

	fmt.Println("✅ Key rotation completed successfully!")
	fmt.Println("")
	fmt.Println("📁 New keys generated:")
	fmt.Printf("   Private Key: %s\n", filepath.Join(keyDir, "private.pem"))
	fmt.Printf("   Public Key:  %s\n", filepath.Join(keyDir, "public.pem"))
	fmt.Println("")
	fmt.Println("📁 Old keys archived to:")
	fmt.Printf("   Directory: %s\n", filepath.Join(keyDir, "archive"))
	fmt.Println("")
	fmt.Println("🔐 New Key Information:")
	fmt.Printf("   Key Size:    %d bits\n", privateKey.N.BitLen())
	fmt.Printf("   Public Exp:  %d\n", publicKey.E)
	fmt.Println("")
	fmt.Println("✅ Next steps:")
	fmt.Println("   1. Restart your server to use new keys")
	fmt.Println("   2. All users will need to login again")
	fmt.Println("   3. Monitor for any authentication issues")
	fmt.Println("   4. Backup new keys securely")
	fmt.Println("")
	fmt.Println("📅 Next rotation recommended: 30 days from now")
	fmt.Println("")
}
