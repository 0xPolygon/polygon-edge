package secrets

import (
	"fmt"

	"github.com/0xPolygon/polygon-sdk/command/helper"
	"github.com/0xPolygon/polygon-sdk/secrets"
)

// SecretsGenerate is the command to generate a secrets manager configuration
type SecretsGenerate struct {
	helper.Base
}

const (
	defaultNodeName       = "polygon-sdk-node"
	defaultConfigFileName = "./secretsManagerConfig.json"
	defaultNamespace = "admin"
)

func (s *SecretsGenerate) DefineFlags() {
	s.Base.DefineFlags()

	s.FlagMap["dir"] = helper.FlagDescriptor{
		Description: fmt.Sprintf("Sets the directory for the secrets manager configuration file Default: %s", defaultConfigFileName),
		Arguments: []string{
			"DIRECTORY",
		},
		ArgumentsOptional: false,
		FlagOptional:      true,
	}

	s.FlagMap["type"] = helper.FlagDescriptor{
		Description: fmt.Sprintf("Specifies the type of the secrets manager. Default: %s", secrets.HashicorpVault),
		Arguments: []string{
			"TYPE",
		},
		ArgumentsOptional: false,
		FlagOptional:      false,
	}

	s.FlagMap["token"] = helper.FlagDescriptor{
		Description: "Specifies the access token for the service",
		Arguments: []string{
			"TOKEN",
		},
		ArgumentsOptional: false,
		FlagOptional:      false,
	}

	s.FlagMap["server-url"] = helper.FlagDescriptor{
		Description: "Specifies the server URL for the service",
		Arguments: []string{
			"SERVER_URL",
		},
		ArgumentsOptional: false,
		FlagOptional:      false,
	}

	s.FlagMap["namespace"] = helper.FlagDescriptor{
		Description: "Specifics the namespace for the service",
		Arguments: []string{
			"NAMESPACE",
		},
		ArgumentsOptional: false,
		FlagOptional: false,
	}

	s.FlagMap["name"] = helper.FlagDescriptor{
		Description: fmt.Sprintf("Specifies the name of the node for on-service record keeping. Default: %s", defaultNodeName),
		Arguments: []string{
			"NODE_NAME",
		},
		ArgumentsOptional: false,
		FlagOptional:      false,
	}
}

// GetHelperText returns a simple description of the command
func (s *SecretsGenerate) GetHelperText() string {
	return "Initializes the secrets manager configuration in the provided directory. Used for Hashicorp Vault"
}

// Help implements the cli.SecretsManagerGenerate interface
func (s *SecretsGenerate) Help() string {
	s.DefineFlags()

	return helper.GenerateHelp(s.Synopsis(), helper.GenerateUsage(s.GetBaseCommand(), s.FlagMap), s.FlagMap)
}

// Synopsis implements the cli.SecretsManagerGenerate interface
func (s *SecretsGenerate) Synopsis() string {
	return s.GetHelperText()
}

func (s *SecretsGenerate) GetBaseCommand() string {
	return "secrets generate"
}

// Run implements the cli.SecretsManagerGenerate interface
func (s *SecretsGenerate) Run(args []string) int {
	flags := s.Base.NewFlagSet(s.GetBaseCommand())

	var path string
	var token string
	var serverURL string
	var serviceType string
	var name string
	var namespace string

	flags.StringVar(&path, "dir", defaultConfigFileName, "")
	flags.StringVar(&token, "token", "", "")
	flags.StringVar(&serverURL, "server-url", "", "")
	flags.StringVar(&serviceType, "type", string(secrets.HashicorpVault), "")
	flags.StringVar(&name, "name", defaultNodeName, "")
	flags.StringVar(&namespace, "namespace", defaultNamespace, "")

	if err := flags.Parse(args); err != nil {
		s.UI.Error(err.Error())
		return 1
	}

	// Safety checks
	if path == "" {
		s.UI.Error("required argument (path) not passed in")
		return 1
	}

	if token == "" {
		s.UI.Error("required argument (token) not passed in")
		return 1
	}

	if serverURL == "" {
		s.UI.Error("required argument (serverURL) not passed in")
		return 1
	}

	if name == "" {
		s.UI.Error("required argument (name) not passed in")
		return 1
	}

	if !secrets.SupportedServiceManager(secrets.SecretsManagerType(serviceType)) {
		s.UI.Error("unsupported service manager type")
		return 1
	}

	// Generate the configuration
	config := &secrets.SecretsManagerConfig{
		Token:     token,
		ServerURL: serverURL,
		Type:      secrets.SecretsManagerType(serviceType),
		Name:      name,
		Namespace: namespace,
		Extra:     nil,
	}

	writeErr := config.WriteConfig(path)
	if writeErr != nil {
		s.UI.Error("unable to write configuration file")
		return 1
	}

	output := "\n[SECRETS GENERATE]\n"

	output += helper.FormatKV([]string{
		fmt.Sprintf("Service Type|%s", serviceType),
		fmt.Sprintf("Server URL|%s", serverURL),
		fmt.Sprintf("Access Token|%s", token),
		fmt.Sprintf("Node Name|%s", name),
		fmt.Sprintf("Namespace|%s", namespace),
	})

	output += "\n\nCONFIGURATION GENERATED"
	output += "\n"

	s.UI.Output(output)

	return 0
}
