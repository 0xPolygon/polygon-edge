package backup

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/0xPolygon/polygon-sdk/command/helper"
	"github.com/0xPolygon/polygon-sdk/helper/common"
	"github.com/0xPolygon/polygon-sdk/server/proto"
	"github.com/0xPolygon/polygon-sdk/types"
)

type BackupCommand struct {
	helper.Base
	Formatter *helper.FormatterFlag
	GRPC      *helper.GRPCFlag
}

// DefineFlags defines the command flags
func (c *BackupCommand) DefineFlags() {
	c.Base.DefineFlags(c.Formatter, c.GRPC)

	// TODO: description
	c.FlagMap["out"] = helper.FlagDescriptor{
		Description: "Filepath the path to save the backup",
		Arguments: []string{
			"OUT",
		},
		ArgumentsOptional: false,
	}

	c.FlagMap["from"] = helper.FlagDescriptor{
		Description: "Begining height of chain to save data",
		Arguments: []string{
			"FROM",
		},
		ArgumentsOptional: true,
	}

	c.FlagMap["to"] = helper.FlagDescriptor{
		Description: "End height of the chain in data",
		Arguments: []string{
			"TO",
		},
		ArgumentsOptional: true,
	}
}

// GetHelperText returns a simple description of the command
func (c *BackupCommand) GetHelperText() string {
	return "Fetch blockchain data from node and save to a file"
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

	var from uint64
	var to uint64
	var err error

	if out == "" {
		c.Formatter.OutputError(errors.New("out is required"))
		return 1
	}

	if from, err = types.ParseUint64orHex(&rawFrom); err != nil {
		c.Formatter.OutputError(fmt.Errorf("Failed to decode from: %w", err))
		return 1
	}

	if rawTo != "" {
		var parsedTo uint64
		if parsedTo, err = types.ParseUint64orHex(&rawTo); err != nil {
			c.Formatter.OutputError(fmt.Errorf("Failed to decode to: %w", err))
			return 1
		} else if from > parsedTo {
			c.Formatter.OutputError(errors.New("to must be greater than or equal to from"))
			return 1
		}
		to = parsedTo
	}

	res, err := c.fetchAndSaveBackup(out, from, to)
	if err != nil {
		c.Formatter.OutputError(err)
		return 1
	}
	c.Formatter.OutputResult(res)

	return 0
}

func (c *BackupCommand) fetchAndSaveBackup(outPath string, from, to uint64) (*BackupResult, error) {
	// always create new file, throw error if the file exists
	fs, err := os.OpenFile(outPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return nil, err
	}

	conn, err := c.GRPC.Conn()
	if err != nil {
		return nil, err
	}

	clt := proto.NewSystemClient(conn)
	ctx, cancelFn := context.WithCancel(context.Background())
	defer cancelFn()

	stream, err := clt.Export(ctx, &proto.ExportRequest{
		From: from,
		To:   to,
	})
	if err != nil {
		return nil, err
	}

	resCh, errCh := processExportStream(stream, fs)

	var res *BackupResult
	select {
	case res = <-resCh:
	case err = <-errCh:
	}

	fs.Close()
	if err != nil {
		os.Remove(outPath)
	}

	return res, err
}

func processExportStream(stream proto.System_ExportClient, fs *os.File) (<-chan *BackupResult, <-chan error) {
	resCh := make(chan *BackupResult, 1)
	errCh := make(chan error, 1)

	signalCh := common.GetTerminationSignalCh()

	go func() {
		defer close(resCh)
		defer close(errCh)

		var from, to *uint64
		returnResult := func() {
			if from == nil || to == nil {
				errCh <- errors.New("couldn't fetch any block")
			} else {
				resCh <- &BackupResult{
					From: *from,
					To:   *to,
					Out:  fs.Name(),
				}
			}
		}

		for {
			evnt, err := stream.Recv()
			if err == io.EOF {
				returnResult()
				return
			}
			if err != nil {
				errCh <- err
				return
			}

			// write data
			if _, err := fs.Write(evnt.Data); err != nil {
				errCh <- err
			}

			// update result
			if from == nil {
				from = &evnt.From
			}
			to = &evnt.To

			// finish in the middle if received termination signal
			select {
			case <-signalCh:
				returnResult()
				return
			default:
			}
		}
	}()

	return resCh, errCh
}

type BackupResult struct {
	From uint64 `json:"from"`
	To   uint64 `json:"to"`
	Out  string `json:"out"`
}

func (r *BackupResult) Output() string {
	var buffer bytes.Buffer

	buffer.WriteString("\n[BACKUP]\n")
	buffer.WriteString("Successfully saved backup:\n")
	buffer.WriteString(helper.FormatKV([]string{
		fmt.Sprintf("File|%s", r.Out),
		fmt.Sprintf("From|%d", r.From),
		fmt.Sprintf("To|%d", r.To),
	}))

	return buffer.String()
}
