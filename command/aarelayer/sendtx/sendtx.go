package sendtx

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/0xPolygon/polygon-edge/command/aarelayer/service"
	"github.com/0xPolygon/polygon-edge/command/polybftsecrets"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/helper/common"
	"github.com/spf13/cobra"
)

var params aarelayerSendTxParams

func GetCommand() *cobra.Command {
	configCmd := &cobra.Command{
		Use:     "sendtx",
		Short:   "sends account abstraction transaction to the relayer",
		PreRunE: runPreRun,
		RunE:    runCommand,
	}

	setFlags(configCmd)

	return configCmd
}

func runPreRun(_ *cobra.Command, _ []string) error {
	return params.validateFlags()
}

func runCommand(cmd *cobra.Command, _ []string) error {
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetOut(os.Stdin)

	secretsManager, err := polybftsecrets.GetSecretsManager(params.accountDir, params.configPath, true)
	if err != nil {
		return err
	}

	account, err := wallet.NewAccountFromSecret(secretsManager)
	if err != nil {
		return err
	}

	tx, err := params.createAATransaction(account.Ecdsa)
	if err != nil {
		return err
	}

	var (
		responseObj map[string]string
		buf         bytes.Buffer
	)

	if err := json.NewEncoder(&buf).Encode(tx); err != nil {
		return err
	}

	req, err := http.NewRequest("POST", fmt.Sprintf("http://%s/v1/sendTransaction", params.addr), &buf)
	if err != nil {
		return err
	}

	client := &http.Client{}

	res, err := client.Do(req)
	if err != nil {
		return err
	}

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("http response %d", res.StatusCode)
	}

	uuidBytes, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(uuidBytes, &responseObj); err != nil {
		return err
	}

	cmd.Printf("Transaction has been successfully sent: %s\n", responseObj["uuid"])

	if params.waitForReceipt {
		cmd.Println("waiting for receipt...")

		receipt, err := waitForRecipt(cmd, responseObj["uuid"])
		if err != nil {
			return err
		}

		if receipt.Error == nil {
			cmd.Printf("Transaction has been included in block: %s\n", receipt.Mined.BlockHash.String())
		} else {
			cmd.Println("Transaction failed to be included in block")
			cmd.Printf("Error: %s\n", *receipt.Error)
		}
	}

	return nil
}

func waitForRecipt(cmd *cobra.Command, uuid string) (*service.AAReceipt, error) {
	const numRetries = 50

	endpoint := fmt.Sprintf("http://%s/v1/getTransactionReceipt/%s", params.addr, uuid)

	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, err
	}

	responseObj := &service.AAReceipt{}
	client := &http.Client{}
	ctx, cancel := context.WithCancel(cmd.Context())
	stopCh := common.GetTerminationSignalCh()
	ticker := time.NewTicker(time.Second * 10)

	defer ticker.Stop()
	defer cancel()

	// just waits for os.Signal to cancel context
	go func() {
		select {
		case <-stopCh:
			cancel()
		case <-ctx.Done():
		}
	}()

	for i := 0; i < numRetries; i++ {
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("program has been terminated while waiting for receipt: %s", uuid)
		case <-ticker.C:
			res, err := client.Do(req)
			if err != nil {
				return nil, err
			}

			if res.StatusCode != http.StatusOK {
				return nil, fmt.Errorf("http response %d for uuid = %s", res.StatusCode, uuid)
			}

			uuidBytes, err := io.ReadAll(res.Body)
			if err != nil {
				return nil, err
			}

			if err := json.Unmarshal(uuidBytes, responseObj); err != nil {
				return nil, err
			}

			if responseObj.Status == service.StatusFailed || responseObj.Status == service.StatusCompleted {
				return responseObj, nil
			}
		}
	}

	return nil, fmt.Errorf("timeout while waiting for receipt: %s", uuid)
}
