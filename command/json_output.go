package command

import (
	"encoding/json"
	"fmt"
	"os"
)

// cliOutput implements OutputFormatter interface by printing the output into std out in JSON format
type jsonOutput struct {
	commonOutputFormatter
}

// newJSONOutput is the constructor of jsonOutput
func newJSONOutput() *jsonOutput {
	return &jsonOutput{}
}

// WriteOutput implements OutputFormatter interface
func (jo *jsonOutput) WriteOutput() {
	if jo.errorOutput != nil {
		_, _ = fmt.Fprintln(os.Stderr, jo.getErrorOutput())

		return
	}

	_, _ = fmt.Fprintln(os.Stdout, jo.getCommandOutput())
}

// WriteCommandResult implements OutputFormatter interface
func (jo *jsonOutput) WriteCommandResult(result CommandResult) {
	_, _ = fmt.Fprintln(os.Stdout, marshalJSONToString(result))
}

// WriteOutput implements OutputFormatter plus io.Writer interfaces
func (jo *jsonOutput) Write(p []byte) (n int, err error) {
	return os.Stdout.Write(p)
}

func (jo *jsonOutput) getErrorOutput() string {
	if jo.errorOutput == nil {
		return ""
	}

	return marshalJSONToString(
		struct {
			Err string `json:"error"`
		}{
			Err: jo.errorOutput.Error(),
		},
	)
}

func (jo *jsonOutput) getCommandOutput() string {
	if jo.commandOutput == nil {
		return ""
	}

	return marshalJSONToString(jo.commandOutput)
}

func marshalJSONToString(input interface{}) string {
	bytes, err := json.Marshal(input)
	if err != nil {
		return err.Error()
	}

	return string(bytes)
}
