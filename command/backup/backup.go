package backup

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/0xPolygon/polygon-edge/archive"
	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
)

type BackupCommand struct {
	helper.Base
	Formatter *helper.FormatterFlag
	GRPC      *helper.GRPCFlag
}

// DefineFlags defines the command flags
func (c *BackupCommand) DefineFlags() {
	c.Base.DefineFlags(c.Formatter, c.GRPC)

	c.FlagMap["out"] = helper.FlagDescriptor{
		Description: "The path of backup data to save",
		Arguments: []string{
			"OUT",
		},
		ArgumentsOptional: false,
	}

	c.FlagMap["from"] = helper.FlagDescriptor{
		Description: "Beginning height of chain in backup",
		Arguments: []string{
			"FROM",
		},
		ArgumentsOptional: true,
	}

	c.FlagMap["to"] = helper.FlagDescriptor{
		Description: "End height of the chain in backup",
		Arguments: []string{
			"TO",
		},
		ArgumentsOptional: true,
	}
}

// GetHelperText returns a simple description of the command
func (c *BackupCommand) GetHelperText() string {
	return "Create blockchain backup data by fetching from running node"
}

func (c *BackupCommand) GetBaseCommand() string {
	return "backup"
}

// Help implements the cli.Command interface
func (c *BackupCommand) Help() string {
	c.DefineFlags()

	return helper.GenerateHelp(c.Synopsis(), helper.GenerateUsage(c.GetBaseCommand(), c.FlagMap), c.FlagMap)
}

// Synopsis implements the cli.Command interface
func (c *BackupCommand) Synopsis() string {
	return c.GetHelperText()
}

// Run implements the cli.Command interface
func (c *BackupCommand) Run(args []string) int {
	flags := c.Base.NewFlagSet(c.GetBaseCommand(), c.Formatter, c.GRPC)

	var out, rawFrom, rawTo string

	flags.StringVar(&out, "out", "", "")
	flags.StringVar(&rawFrom, "from", "0", "")
	flags.StringVar(&rawTo, "to", "", "")

	if err := flags.Parse(args); err != nil {
		c.Formatter.OutputError(err)

		return 1
	}

	var (
		from uint64
		to   *uint64
		err  error
	)

	if out == "" {
		c.Formatter.OutputError(errors.New("the path of backup file is required"))

		return 1
	}

	if from, err = types.ParseUint64orHex(&rawFrom); err != nil {
		c.Formatter.OutputError(fmt.Errorf("failed to decode from: %w", err))

		return 1
	}

	if rawTo != "" {
		var parsedTo uint64

		if parsedTo, err = types.ParseUint64orHex(&rawTo); err != nil {
			c.Formatter.OutputError(fmt.Errorf("failed to decode to: %w", err))

			return 1
		} else if from > parsedTo {
			c.Formatter.OutputError(errors.New("to must be greater than or equal to from"))

			return 1
		}

		to = &parsedTo
	}

	conn, err := c.GRPC.Conn()
	if err != nil {
		c.Formatter.OutputError(err)

		return 1
	}

	logger := hclog.New(&hclog.LoggerOptions{
		Name:  "backup",
		Level: hclog.LevelFromString("INFO"),
	})

	// resFrom and resTo represents the range of blocks that can be included in the file
	resFrom, resTo, err := archive.CreateBackup(conn, logger, from, to, out)
	if err != nil {
		c.Formatter.OutputError(err)

		return 1
	}

	res := &BackupResult{
		From: resFrom,
		To:   resTo,
		Out:  out,
	}
	c.Formatter.OutputResult(res)

	return 0
}

type BackupResult struct {
	From uint64 `json:"from"`
	To   uint64 `json:"to"`
	Out  string `json:"out"`
}

func (r *BackupResult) Output() string {
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
