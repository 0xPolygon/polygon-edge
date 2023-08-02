package show

import (
	"bytes"
	"fmt"
)

type ShowResult struct {
	Whitelists Whitelists
}

func (r *ShowResult) GetOutput() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[WHITELISTS]\n\n")

	buffer.WriteString(fmt.Sprintf("Contract deployment whitelist : %s,\n", r.Whitelists.deployment))

	return buffer.String()
}
