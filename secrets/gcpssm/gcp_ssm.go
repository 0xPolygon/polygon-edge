package gcpssm

import (
	"context"
	"errors"
	"fmt"
	"github.com/0xPolygon/polygon-edge/secrets"
	"github.com/hashicorp/go-hclog"
	"os"

	sm "cloud.google.com/go/secretmanager/apiv1"
	smpb "google.golang.org/genproto/googleapis/cloud/secretmanager/v1"
)

type GcpSsmManager struct {
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

func SecretsManagerFactory(
	config *secrets.SecretsManagerConfig,
	params *secrets.SecretsManagerParams,
) (secrets.SecretsManager, error) {
	// Check if project id is defined
	if _, ok := config.Extra["project-id"]; !ok {
		return nil, errors.New("no project-id variable specified")
	}
	// Check if gcp-ssm-cred is defined
	if _, ok := config.Extra["gcp-ssm-cred"]; !ok {
		return nil, errors.New("no gcp-ssm-cred variable specified")
	}
	// Check if the variables are present
	if config.Name == "" || config.Extra["project-id"] == "" || config.Extra["gcp-ssm-cred"] == "" {
		return nil, errors.New("name, project-id or gcp-ssm-cred can't be empty string")
	}

	gcpSsmManager := &GcpSsmManager{
		projectID:    fmt.Sprintf("%s", config.Extra["project-id"]),
		credFilePath: fmt.Sprintf("%s", config.Extra["gcp-ssm-cred"]),
		nodeName:     config.Name,
		logger:       params.Logger.Named(string(secrets.GCPSSM)),
	}

	if err := gcpSsmManager.Setup(); err != nil {
		return nil, err
	}

	return gcpSsmManager, nil
}

// Setup performs secret manager-specific setup
func (gm *GcpSsmManager) Setup() error {
	var clientErr error
	// Set environment variable that specifies credentials.json file path
	if err := os.Setenv(
		"GOOGLE_APPLICATION_CREDENTIALS",
		gm.credFilePath,
	); err != nil {
		return errors.New("could not set GOOGLE_APPLICATION_CREDENTIALS environment variable")
	}

	gm.context = context.Background()

	gm.client, clientErr = sm.NewClient(gm.context)
	if clientErr != nil {
		return fmt.Errorf("could not initialize new GCP secrets manager client %w", clientErr)
	}

	return nil
}

// GetSecret gets the secret by name
func (gm *GcpSsmManager) GetSecret(name string) ([]byte, error) {
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
func (gm *GcpSsmManager) SetSecret(name string, value []byte) error {
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
func (gm *GcpSsmManager) HasSecret(name string) bool {
	_, err := gm.GetSecret(name)

	// if there is no error fetching secret return true
	return err == nil
}

// RemoveSecret removes the secret from storage used only for tests
func (gm *GcpSsmManager) RemoveSecret(name string) error {
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
func (gm *GcpSsmManager) getSecretID(secretName string) string {
	return fmt.Sprintf("%s_%s", gm.nodeName, secretName)
}

// getFullyQualifiedSecretName returns the full path of the secret in the store manager
func (gm *GcpSsmManager) getFullyQualifiedSecretName(secretName string) string {
	return fmt.Sprintf("projects/%s/secrets/%s/versions/1", gm.projectID, gm.getSecretID(secretName))
}
