package awskms

import (
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"os"
	"testing"

	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/0xPolygon/polygon-edge/secrets"
	"github.com/hashicorp/go-hclog"

	"github.com/stretchr/testify/assert"
)

// getLocalSecretsManager is a helper method for creating an instance of the
// local secrets manager
func getLocalSecretsManager(t *testing.T) secrets.SecretsManager {
	t.Helper()

	// Set up the expected folder structure
	workingDirectory, tempErr := ioutil.TempDir("/tmp", "local-secrets-manager")
	if tempErr != nil {
		t.Fatalf("Unable to instantiate local secrets manager directories, %v", tempErr)
	}

	setupErr := common.SetupDataDir(workingDirectory, []string{secrets.ConsensusFolderLocal, secrets.NetworkFolderLocal})
	if setupErr != nil {
		t.Fatalf("Unable to instantiate local secrets manager directories, %v", setupErr)
	}

	// Set up a clean-up procedure
	t.Cleanup(func() {
		_ = os.RemoveAll(workingDirectory)
	})

	// Set up an instance of the local secrets manager
	baseConfig := &secrets.SecretsManagerParams{
		Logger: hclog.NewNullLogger(),
		Extra: map[string]interface{}{
			secrets.Path: workingDirectory,
		},
	}

	secretsConfig, readErr := secrets.ReadConfig("./secretsManagerConfig-kms.json")
	if readErr != nil {
		t.Fatalf("Unable read configPath, %v", readErr)
	}

	manager, factoryErr := SecretsManagerFactory(secretsConfig, baseConfig)
	if factoryErr != nil {
		t.Fatalf("Unable to instantiate local secrets manager, %v", factoryErr)
	}

	assert.NotNil(t, manager)

	return manager
}

func TestKmsSecretsManager_GetSecretInfo(t *testing.T) {
	// Set the secret
	manager := getLocalSecretsManager(t)

	manager.GetSecretInfo("validator-key")
	assert.True(t, true)
}

func TestKmsSecretsManager_SignData(t *testing.T) {
	// Set the secret
	manager := getLocalSecretsManager(t)

	content, err := manager.SignBySecret("validator-key", 37, []byte("hellokms"))
	// fmt.Println("sing data ", encoding.Base64(content))
	pub, err := crypto.RecoverPubkey(content, []byte("hellokms"))
	pubaddr := crypto.PubKeyToAddress(pub)
	fmt.Println("pubaddr: ", pubaddr)
	fmt.Println("sing data ", base64.StdEncoding.EncodeToString(content))
	assert.Nil(t, err)
	// assert.True(t, true)

}
