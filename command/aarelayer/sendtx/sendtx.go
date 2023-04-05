package sendtx

import (
	"bytes"
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
	"github.com/umbracle/ethgo"
)

const (
	waitForReceiptTime    = time.Second * 10
	numWaitReceiptRetries = 50
)

var params aaSendTxParams

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
	cmd.SetOut(os.Stdout)

	secretsManager, err := polybftsecrets.GetSecretsManager(params.accountDir, params.configPath, true)
	if err != nil {
		return err
	}

	account, err := wallet.NewAccountFromSecret(secretsManager)
	if err != nil {
		return err
	}

	client := &http.Client{}

	uuid, err := sendAATx(client, account.Ecdsa)
	if err != nil {
		return err
	}

	cmd.Printf("Transaction has been successfully sent: %s\n", uuid)

	if params.waitForReceipt {
		cmd.Println("waiting for receipt...")

		receipt, err := waitForRecipt(cmd, client, uuid)
		if err != nil {
			return err
		}

		if receipt.Error != nil {
			return fmt.Errorf("transaction %s failed with an error: %s", uuid, *receipt.Error)
		} else if receipt.Status == service.StatusFailed {
			return fmt.Errorf("transaction %s failed", uuid)
		}

		cmd.Printf("Transaction has been included in block: %d, %s\n",
			receipt.Mined.BlockNumber, receipt.Mined.BlockHash)
	}

	return nil
}

func sendAATx(client *http.Client, key ethgo.Key) (string, error) {
	tx, err := params.createAATransaction(key)
	if err != nil {
		return "", err
	}

	var (
		responseObj map[string]string
		buf         bytes.Buffer
	)

	if err := json.NewEncoder(&buf).Encode(tx); err != nil {
		return "", err
	}

	req, err := http.NewRequest(
		"POST",
		fmt.Sprintf("http://%s/v1/sendTransaction", params.addr),
		&buf)
	if err != nil {
		return "", err
	}

	if err := executeRequest(client, req, &responseObj); err != nil {
		return "", err
	}

	return responseObj["uuid"], nil
}

func waitForRecipt(cmd *cobra.Command, client *http.Client, uuid string) (*service.AAReceipt, error) {
	req, err := http.NewRequest(
		"GET",
		fmt.Sprintf("http://%s/v1/getTransactionReceipt/%s", params.addr, uuid),
		nil)
	if err != nil {
		return nil, err
	}

	responseObj := &service.AAReceipt{}
	stopCh := common.GetTerminationSignalCh()

	ticker := time.NewTicker(waitForReceiptTime)
	defer ticker.Stop()

	for i := 0; i < numWaitReceiptRetries; i++ {
		select {
		case <-stopCh:
			return nil, fmt.Errorf("program has been terminated while waiting for receipt: %s", uuid)
		case <-ticker.C:
			if err := executeRequest(client, req, &responseObj); err != nil {
				return nil, err
			}

			if responseObj.Status == service.StatusFailed || responseObj.Status == service.StatusCompleted {
				return responseObj, nil
			}
		}
	}

	return nil, fmt.Errorf("timeout while waiting for receipt: %s", uuid)
}

func executeRequest(client *http.Client, request *http.Request, responseObject interface{}) error {
	res, err := client.Do(request)
	if err != nil {
		return err
	}

	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("http request failed: response status = %d", res.StatusCode)
	}

	bytes, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}

	return json.Unmarshal(bytes, responseObject)
}
