package secretsManager

import (
	"flag"
	"fmt"

	"github.com/0xPolygon/polygon-sdk/command/helper"
	"github.com/0xPolygon/polygon-sdk/secrets"
)

// SecretsManagerGenerate is the command to generate a secrets manager configuration
type SecretsManagerGenerate struct {
	helper.Meta
}

const (
	defaultNodeName       = "polygon-sdk-node"
	defaultConfigFileName = "./secretsManagerConfig.json"
)

func (i *SecretsManagerGenerate) DefineFlags() {
	if i.FlagMap == nil {
		// Flag map not initialized
		i.FlagMap = make(map[string]helper.FlagDescriptor)
	}

	i.FlagMap["dir"] = helper.FlagDescriptor{
		Description: fmt.Sprintf("Sets the directory for the secrets manager configuration file Default: %s", defaultConfigFileName),
		Arguments: []string{
			"DIRECTORY",
		},
		ArgumentsOptional: false,
		FlagOptional:      true,
	}

	i.FlagMap["type"] = helper.FlagDescriptor{
		Description: fmt.Sprintf("Specifies the type of the secrets manager. Default: %s", secrets.HashicorpVault),
		Arguments: []string{
			"TYPE",
		},
		ArgumentsOptional: false,
		FlagOptional:      false,
	}

	i.FlagMap["token"] = helper.FlagDescriptor{
		Description: "Specifies the access token for the service",
		Arguments: []string{
			"TOKEN",
		},
		ArgumentsOptional: false,
		FlagOptional:      false,
	}

	i.FlagMap["server-url"] = helper.FlagDescriptor{
		Description: "Specifies the server URL for the service",
		Arguments: []string{
			"SERVER_URL",
		},
		ArgumentsOptional: false,
		FlagOptional:      false,
	}

	i.FlagMap["name"] = helper.FlagDescriptor{
		Description: fmt.Sprintf("Specifies the name of the node for on-service record keeping. Default %s", defaultNodeName),
		Arguments: []string{
			"SERVER_URL",
		},
		ArgumentsOptional: false,
		FlagOptional:      false,
	}
}

// GetHelperText returns a simple description of the command
func (p *SecretsManagerGenerate) GetHelperText() string {
	return "Initializes the secrets manager configuration in the provided directory. Used for Hashicorp Vault"
}

// Help implements the cli.SecretsManagerGenerate interface
func (p *SecretsManagerGenerate) Help() string {
	p.DefineFlags()

	return helper.GenerateHelp(p.Synopsis(), helper.GenerateUsage(p.GetBaseCommand(), p.FlagMap), p.FlagMap)
}

// Synopsis implements the cli.SecretsManagerGenerate interface
func (p *SecretsManagerGenerate) Synopsis() string {
	return p.GetHelperText()
}

func (p *SecretsManagerGenerate) GetBaseCommand() string {
	return "secrets-manager generate"
}

// Run implements the cli.SecretsManagerGenerate interface
func (p *SecretsManagerGenerate) Run(args []string) int {
	flags := flag.NewFlagSet(p.GetBaseCommand(), flag.ContinueOnError)
	var path string
	var token string
	var serverURL string
	var serviceType string
	var name string

	flags.StringVar(&path, "dir", defaultConfigFileName, "")
	flags.StringVar(&token, "token", "", "")
	flags.StringVar(&serverURL, "server-url", "", "")
	flags.StringVar(&serviceType, "type", string(secrets.HashicorpVault), "")
	flags.StringVar(&name, "name", defaultNodeName, "")

	if err := flags.Parse(args); err != nil {
		p.UI.Error(err.Error())
		return 1
	}

	// Safety checks
	if path == "" {
		p.UI.Error("required argument (path) not passed in")
		return 1
	}

	if token == "" {
		p.UI.Error("required argument (token) not passed in")
		return 1
	}

	if serverURL == "" {
		p.UI.Error("required argument (serverURL) not passed in")
		return 1
	}

	if !secrets.SupportedServiceManager(secrets.SecretsManagerType(serviceType)) {
		p.UI.Error("unsupported service manager type")
		return 1
	}

	// Generate the configuration
	config := &secrets.SecretsManagerConfig{
		Token:     token,
		ServerURL: serverURL,
		Type:      secrets.SecretsManagerType(serviceType),
		Name:      name,
		Extra:     nil,
	}

	writeErr := config.WriteConfig(path)
	if writeErr != nil {
		p.UI.Error("unable to write configuration file")
		return 1
	}

	output := "\n[SECRETS MANAGER GENERATE]\n"

	output += helper.FormatKV([]string{
		fmt.Sprintf("Service Type|%s", serviceType),
		fmt.Sprintf("Server URL|%s", serverURL),
		fmt.Sprintf("Access Token|%s", token),
		fmt.Sprintf("Node Name|%s", name),
	})

	output += "\n\nCONFIGURATION GENERATED"
	output += "\n"

	p.UI.Output(output)

	return 0
}
