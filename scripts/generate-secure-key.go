//go:build tools
// +build tools

package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
)

func main() {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		log.Fatal("Failed to generate key:", err)
	}

	secureKey := hex.EncodeToString(key)

	fmt.Println("=== GENERATE SECURE ENCRYPTION KEY ===")
	fmt.Printf("Replace ENCRYPTION_KEY in your .env file with:\n")
	fmt.Printf("ENCRYPTION_KEY=%s\n", secureKey)
	fmt.Println("\n IMPORTANT: Keep this key secure and don't commit it to git!")
	fmt.Println("Use different keys for development and production!")
}
