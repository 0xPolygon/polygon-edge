package backup

import (
	"bytes"
	"fmt"

	"github.com/0xPolygon/polygon-edge/command/helper"
)

type BackupResult struct {
	From uint64 `json:"from"`
	To   uint64 `json:"to"`
	Out  string `json:"out"`
}

func (r *BackupResult) GetOutput() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[BACKUP]\n")
	buffer.WriteString("Exported backup file successfully:\n")
	buffer.WriteString(helper.FormatKV([]string{
		fmt.Sprintf("File|%s", r.Out),
		fmt.Sprintf("From|%d", r.From),
		fmt.Sprintf("To|%d", r.To),
	}))

	return buffer.String()
}
