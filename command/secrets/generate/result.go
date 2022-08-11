package generate

import (
	"bytes"
	"fmt"

	"github.com/0xPolygon/polygon-edge/command/helper"
)

type SecretsGenerateResult struct {
	ServiceType string `json:"service_type"`
	ServerURL   string `json:"server_url"`
	AccessToken string `json:"access_token"`
	NodeName    string `json:"node_name"`
	Namespace   string `json:"namespace"`
	Extra       string `json:"extra"`
}

func (r *SecretsGenerateResult) GetOutput() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[SECRETS GENERATE]\n")
	buffer.WriteString(helper.FormatKV([]string{
		fmt.Sprintf("Service Type|%s", r.ServiceType),
		fmt.Sprintf("Server URL|%s", r.ServerURL),
		fmt.Sprintf("Access Token|%s", r.AccessToken),
		fmt.Sprintf("Node Name|%s", r.NodeName),
		fmt.Sprintf("Namespace|%s", r.Namespace),
		fmt.Sprintf("Extra|%s", r.Extra),
	}))
	buffer.WriteString("\n")

	return buffer.String()
}
