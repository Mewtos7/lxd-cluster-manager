// Command gen-api-key generates a cryptographically random API key and prints
// its bcrypt hash. Copy the hash into the API_KEYS environment variable (or
// .env file); give the raw key to the client that will call the manager API.
//
// Usage:
//
//	go run ./tools/gen-api-key          # via go run
//	make gen-api-key                    # via Makefile shortcut
//
// Output example:
//
//	Raw key  : Tz3...8Qw=
//	Bcrypt   : $2a$12$...
//	API_KEYS : $2a$12$...
//
// The "Raw key" value is shown only once. Store it securely — it cannot be
// recovered from the hash.
package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"

	"golang.org/x/crypto/bcrypt"
)

func main() {
	// Generate 32 random bytes (256 bits) and encode as URL-safe base64.
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		fmt.Fprintf(os.Stderr, "gen-api-key: failed to read random bytes: %v\n", err)
		os.Exit(1)
	}
	key := base64.RawURLEncoding.EncodeToString(raw)

	// Hash with bcrypt at cost 12 (the ADR-003 recommended minimum for
	// production; lower the cost only in development when speed matters).
	hash, err := bcrypt.GenerateFromPassword([]byte(key), 12)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gen-api-key: bcrypt failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Raw key  : %s\n", key)
	fmt.Printf("Bcrypt   : %s\n", hash)
	fmt.Printf("\nAdd to API_KEYS in .env or export directly:\n")
	fmt.Printf("  export API_KEYS='%s'\n", hash)
}
