package hashicorpvault

import (
	"fmt"
	"testing"

	"github.com/0xPolygon/polygon-sdk/crypto"
	vault "github.com/hashicorp/vault/api"
)

func readSecret(client *vault.Client, path string, key string, t *testing.T) {
	secret, err := client.Logical().Read(path)
	if err != nil {
		t.Fatalf("unable to read secret: %v", err)
	}
	data, ok := secret.Data["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("data type assertion failed: %T %#v", secret.Data["data"], secret.Data["data"])
	}
	// data map can contain more than one key-value pair, in this case we're just grabbing one of them
	value, ok := data[key].(string)
	if !ok {
		t.Fatalf("value type assertion failed: %T %#v", data[key], data[key])
	}

	fmt.Printf("Value is %s", value)
}

//secret/data/validator-1
func writeSecret(client *vault.Client, path string, key string, value interface{}, t *testing.T) {
	data := make(map[string]interface{})

	data[key] = value
	_, err := client.Logical().Write(path, map[string]interface{}{
		"data": data,
	})
	if err != nil {
		t.Fatalf("unable to store secret: %v", err)
	}
}

func TestVaultConnection(t *testing.T) {
	config := vault.DefaultConfig() // modify for more granular configuration
	config.Address = "http://127.0.0.1:8200"
	client, err := vault.NewClient(config)
	if err != nil {
		t.Fatalf("unable to initialize Vault client: %v", err)
	}

	// WARNING: Storing any long-lived token with secret access in an environment variable poses a security risk.
	// Additionally, root tokens should never be used in production or against Vault installations containing real secrets.
	// See the files starting in auth-* for examples of how to securely log in to Vault using various auth methods.

	_, privKeyEncoded, _ := crypto.GenerateAndEncodePrivateKey()

	client.SetToken("s.e7Mm84sMpwug5TXoIr2tGIiB")

	readSecret(client, "secret/data/hello", "foo", t)

	keyPath := "secret/data/validator-1"
	keyKey := "validator-key"

	writeSecret(client, keyPath, keyKey, privKeyEncoded, t)
	readSecret(client, keyPath, keyKey, t)
}
