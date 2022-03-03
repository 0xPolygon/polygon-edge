package command

import (
	"encoding/json"
	"fmt"
	"os"
)

type JSONOutput struct {
	commonOutputFormatter
}

func (jo *JSONOutput) WriteOutput() {
	if jo.errorOutput != nil {
		_, _ = fmt.Fprintln(os.Stderr, jo.getErrorOutput())

		return
	}

	_, _ = fmt.Fprintln(os.Stdout, jo.getCommandOutput())
}

func newJSONOutput() *JSONOutput {
	return &JSONOutput{}
}

func (jo *JSONOutput) getErrorOutput() string {
	return marshalJSONToString(
		struct {
			Err string `json:"error"`
		}{
			Err: jo.errorOutput.Error(),
		},
	)
}

func (jo *JSONOutput) getCommandOutput() string {
	return marshalJSONToString(jo.commandOutput)
}

func marshalJSONToString(input interface{}) string {
	bytes, err := json.Marshal(input)
	if err != nil {
		return err.Error()
	}

	return string(bytes)
}
