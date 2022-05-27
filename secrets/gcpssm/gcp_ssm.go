package gcpssm

import (
	"context"
	"fmt"
	"github.com/0xPolygon/polygon-edge/secrets"
	"github.com/hashicorp/go-hclog"
	"os"

	sm "cloud.google.com/go/secretmanager/apiv1"
	smpb "google.golang.org/genproto/googleapis/cloud/secretmanager/v1"
)

type GCPSecretsManager struct {
	// project id in which to store the secrets
	projectID string
	// gcp secrets manager client
	client *sm.Client
	// credential file path
	credFilePath string
	// logger instance
	logger hclog.Logger
	// context used in API calls
	context context.Context
	// node name is used to create unique secret id
	nodeName string
}

type (
	configExtraParamFields string
	envVarsName            string
	errorMessages          error
)

const (
	projectID configExtraParamFields = "project-id"
	//nolint:gosec
	gcpSSMCredFile configExtraParamFields = "gcp-ssm-cred"
	//nolint:gosec
	secretsManagerCredentials envVarsName = "GOOGLE_APPLICATION_CREDENTIALS"
)

var (
	errNoProjectID   errorMessages = fmt.Errorf("no %s variable specified", projectID)
	errNoCredsFile   errorMessages = fmt.Errorf("no %s variable specified", gcpSSMCredFile)
	errParamsEmpty   errorMessages = fmt.Errorf("name, %s or %s, can not be an empty string", projectID, gcpSSMCredFile)
	errCantSetEnvVar errorMessages = fmt.Errorf("could not set %s environment variable", secretsManagerCredentials)
)

func SecretsManagerFactory(
	config *secrets.SecretsManagerConfig,
	params *secrets.SecretsManagerParams,
) (secrets.SecretsManager, error) {
	// Check if project id is defined
	if _, ok := config.Extra[string(projectID)]; !ok {
		return nil, errNoProjectID
	}
	// Check if gcp-ssm-cred is defined
	if _, ok := config.Extra[string(gcpSSMCredFile)]; !ok {
		return nil, errNoCredsFile
	}
	// Check if the variables are present
	if config.Name == "" || config.Extra[string(projectID)] == "" || config.Extra[string(gcpSSMCredFile)] == "" {
		return nil, errParamsEmpty
	}

	gcpSsmManager := &GCPSecretsManager{
		projectID:    fmt.Sprintf("%s", config.Extra[string(projectID)]),
		credFilePath: fmt.Sprintf("%s", config.Extra[string(gcpSSMCredFile)]),
		nodeName:     config.Name,
		logger:       params.Logger.Named(string(secrets.GCPSSM)),
	}

	if err := gcpSsmManager.Setup(); err != nil {
		return nil, err
	}

	return gcpSsmManager, nil
}

// Setup performs secret manager specific setup
func (gm *GCPSecretsManager) Setup() error {
	var clientErr error
	// Set environment variable that specifies credentials.json file path
	if err := os.Setenv(
		string(secretsManagerCredentials),
		gm.credFilePath,
	); err != nil {
		return errCantSetEnvVar
	}

	gm.context = context.Background()

	gm.client, clientErr = sm.NewClient(gm.context)
	if clientErr != nil {
		return fmt.Errorf("could not initialize new GCP secrets manager client %w", clientErr)
	}

	return nil
}

// GetSecret gets the secret by name
func (gm *GCPSecretsManager) GetSecret(name string) ([]byte, error) {
	// create get secret request
	getSecretReq := &smpb.AccessSecretVersionRequest{
		Name: gm.getFullyQualifiedSecretName(name),
	}

	// send the request
	result, err := gm.client.AccessSecretVersion(gm.context, getSecretReq)
	if err != nil {
		return nil, fmt.Errorf("could not fetch secret from GCP secret manager: %w", err)
	}

	return result.Payload.Data, nil
}

// SetSecret sets the secret to a provided value
func (gm *GCPSecretsManager) SetSecret(name string, value []byte) error {
	// create the request to create the secret placeholder.
	createSecretReq := &smpb.CreateSecretRequest{
		Parent:   fmt.Sprintf("projects/%s", gm.projectID),
		SecretId: gm.getSecretID(name),
		Secret: &smpb.Secret{
			Replication: &smpb.Replication{
				Replication: &smpb.Replication_Automatic_{
					Automatic: &smpb.Replication_Automatic{},
				},
			},
		},
	}

	// create secret placeholder
	secret, err := gm.client.CreateSecret(gm.context, createSecretReq)
	if err != nil {
		return fmt.Errorf("could not set secret, %w", err)
	}

	// create request to store secret data
	req := &smpb.AddSecretVersionRequest{
		Parent: secret.Name,
		Payload: &smpb.SecretPayload{
			Data: value,
		},
	}

	// store secret
	_, err = gm.client.AddSecretVersion(gm.context, req)
	if err != nil {
		return fmt.Errorf("could not store secret, %w", err)
	}

	return nil
}

// HasSecret checks if the secret is present
func (gm *GCPSecretsManager) HasSecret(name string) bool {
	_, err := gm.GetSecret(name)

	// if there is no error fetching secret return true
	return err == nil
}

// RemoveSecret removes the secret from storage used only for tests
func (gm *GCPSecretsManager) RemoveSecret(name string) error {
	// create delete secret request
	req := &smpb.DeleteSecretRequest{
		Name: gm.getFullyQualifiedSecretName(name),
	}

	// delete secret
	if err := gm.client.DeleteSecret(gm.context, req); err != nil {
		return fmt.Errorf("could not delete secret %s from GCP secret manager: %w",
			gm.getFullyQualifiedSecretName(name), err)
	}

	return nil
}

// getSecretID is used to format secret id with nodeName_secretName
func (gm *GCPSecretsManager) getSecretID(secretName string) string {
	return fmt.Sprintf("%s_%s", gm.nodeName, secretName)
}

// getFullyQualifiedSecretName returns the full path of the secret in the store manager
func (gm *GCPSecretsManager) getFullyQualifiedSecretName(secretName string) string {
	return fmt.Sprintf("projects/%s/secrets/%s/versions/1", gm.projectID, gm.getSecretID(secretName))
}
