package generate

import (
	"fmt"
	"strings"

	"github.com/0xPolygon/polygon-edge/command"
	"github.com/0xPolygon/polygon-edge/secrets"
)

var (
	params = &generateParams{}
)

const (
	dirFlag       = "dir"
	tokenFlag     = "token"
	serverURLFlag = "server-url"
	typeFlag      = "type"
	nameFlag      = "name"
	namespaceFlag = "namespace"
	extraFlag     = "extra"
)

const (
	defaultNodeName       = ""
	defaultConfigFileName = "./secretsManagerConfig.json"
	defaultNamespace      = "admin"
)

var (
	errUnsupportedType = fmt.Errorf(
		"unsupported service manager type; only %s, %s, %s and %s are supported for now",
		secrets.Local, secrets.HashicorpVault, secrets.AWSSSM, secrets.GCPSSM)
)

type generateParams struct {
	dir         string
	token       string
	serverURL   string
	serviceType string
	name        string
	namespace   string
	extra       string
}

func (p *generateParams) getRequiredFlags() []string {
	return []string{
		nameFlag,
	}
}

func (p *generateParams) writeSecretsConfig() error {
	secretsConfig, err := p.generateSecretsConfig()
	if err != nil {
		return err
	}

	writeErr := secretsConfig.WriteConfig(p.dir)
	if writeErr != nil {
		return fmt.Errorf("unable to write configuration file, %w", writeErr)
	}

	return nil
}

func (p *generateParams) generateSecretsConfig() (*secrets.SecretsManagerConfig, error) {
	if !secrets.SupportedServiceManager(secrets.SecretsManagerType(p.serviceType)) {
		return nil, errUnsupportedType
	}

	// Init the extra map
	extraMap := make(map[string]interface{})

	if p.extra != "" {
		entries := strings.Split(p.extra, ",")
		for _, e := range entries {
			parts := strings.Split(e, "=")
			extraMap[parts[0]] = parts[1]
		}
	}

	// Generate the configuration
	return &secrets.SecretsManagerConfig{
		Token:     p.token,
		ServerURL: p.serverURL,
		Type:      secrets.SecretsManagerType(p.serviceType),
		Name:      p.name,
		Namespace: p.namespace,
		Extra:     extraMap,
	}, nil
}

func (p *generateParams) getResult() command.CommandResult {
	return &SecretsGenerateResult{
		ServiceType: p.serviceType,
		ServerURL:   p.serverURL,
		AccessToken: p.token,
		NodeName:    p.name,
		Namespace:   p.namespace,
		Extra:       p.extra,
	}
}
