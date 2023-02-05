package command

import "bytes"

// Results implements CommandResult interface by aggregating multiple commands outputs into one
type Results []CommandResult

func (r Results) GetOutput() string {
	var buffer bytes.Buffer

	for _, res := range r {
		buffer.WriteString(res.GetOutput())
	}

	return buffer.String()
}
