package e2e

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/crypto"
	"github.com/0xPolygon/polygon-edge/secrets"
	secretsHelper "github.com/0xPolygon/polygon-edge/secrets/helper"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/mitchellh/go-glint"
	gc "github.com/mitchellh/go-glint/components"
	"github.com/spf13/cobra"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/abi"
	"github.com/umbracle/ethgo/jsonrpc"
)

var params registerParams

func GetCommand() *cobra.Command {
	registerCmd := &cobra.Command{
		Use:     "register",
		Short:   "Registers a new validator",
		PreRunE: runPreRun,
		Run:     runCommand,
	}

	helper.RegisterGRPCAddressFlag(registerCmd)
	setFlags(registerCmd)

	return registerCmd
}

func setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(
		&params.testDir,
		registerFlag,
		"",
		"the directory name where new validator key is stored",
	)
}

func runPreRun(_ *cobra.Command, _ []string) error {
	return params.validateFlags()
}

func runCommand(cmd *cobra.Command, _ []string) {
	secretsManager, err := secretsHelper.SetupLocalSecretsManager("./test-chain-1")
	if err != nil {
		panic(err)
	}

	existingValidatorAccount, err := wallet.GenerateNewAccountFromSecret(secretsManager, secrets.ValidatorBLSKey)
	if err != nil {
		panic(err)
	}

	existingValidator := newTxnSender(existingValidatorAccount)

	secretsManager, err = secretsHelper.SetupLocalSecretsManager(fmt.Sprintf("./%s", params.testDir))
	if err != nil {
		panic(err)
	}

	newValidatorAccount, err := wallet.GenerateNewAccountFromSecret(secretsManager, secrets.ValidatorBLSKey)
	if err != nil {
		panic(err)
	}

	newValidatorSender := newTxnSender(newValidatorAccount)

	var validator *NewValidator
	steps := []*txnStep{
		{
			name: "whitelist",
			action: func() asyncTxn {
				return whitelist(existingValidator, types.Address(newValidatorAccount.Ecdsa.Address()))
			},
		},
		{
			name: "fund",
			action: func() asyncTxn {
				return fund(existingValidator, types.Address(newValidatorAccount.Ecdsa.Address()))
			},
		},
		{
			name: "register",
			action: func() asyncTxn {
				return registerValidator(newValidatorSender, newValidatorAccount)
			},
			postHook: func(receipt *ethgo.Receipt) {
				event, err := newValidatorEvent.ParseLog(receipt.Logs[0])
				if err != nil {
					panic(err)
				}
				validator = &NewValidator{
					Validator: event["validator"].(ethgo.Address),
				}
			},
		},
		{
			name: "stake",
			action: func() asyncTxn {
				return stake(newValidatorSender)
			},
		},
	}

	d := glint.New()
	go d.Render(context.Background())

	print := func(done bool) {
		comps := []glint.Component{}
		for _, step := range steps {
			var status glint.Component
			var opts []glint.StyleOption

			switch step.status {
			case txnStepQueued:
				status = glint.Text("-")

			case txnStepPending:
				status = gc.Spinner()
				opts = append(opts, glint.Color("yellow"))

			case txnStepCompleted:
				status = glint.Text("✓")
				opts = append(opts, glint.Color("green"))

			case txnStepFailed:
				status = glint.Text("✗")
				opts = append(opts, glint.Color("red"))
			}

			comps = append(comps, glint.Style(
				glint.Layout(
					status,
					glint.Layout(glint.Text(step.name+"...")).MarginLeft(1),
				).Row(),
				opts...,
			))
		}
		if done {
			if validator != nil {
				comps = append(comps, glint.Text("\nDone: "+validator.Validator.String()))
			} else {
				comps = append(comps, glint.Text("\nDone:"))
			}
		} else {
			comps = append(comps, glint.Text("\nWaiting..."))
		}

		d.Set(comps...)
	}

	for _, step := range steps {
		step.status = txnStepPending
		print(false)

		txn := step.action()
		receipt, err := txn.Wait()
		if err != nil {
			step.status = txnStepFailed
		} else {
			if receipt.Status == uint64(types.ReceiptFailed) {
				step.status = txnStepFailed
			} else {
				step.status = txnStepCompleted
			}
		}
		if step.status == txnStepFailed {
			break
		}
		if step.postHook != nil {
			step.postHook(receipt)
		}
	}
	print(true)

	d.RenderFrame()
	d.Pause()
}

const (
	defaultGasPrice = 1879048192 // 0x70000000
	defaultGasLimit = 5242880    // 0x500000
)

var (
	stakeManager      = types.Address{0x1}
	stakeFn, _        = abi.NewMethod("function stake()")
	whitelistFn, _    = abi.NewMethod("function addToWhitelist(address[])")
	registerFn, _     = abi.NewMethod("function register(uint256[2] signature, uint256[4] pubkey)")
	newValidatorEvent = abi.MustNewEvent(`event NewValidator(
		address indexed validator,
		uint256[4] blsKey
	)`)
)

type asyncTxn interface {
	Wait() (*ethgo.Receipt, error)
}

type asyncTxnImpl struct {
	t    *txnSender
	hash ethgo.Hash
}

func (a *asyncTxnImpl) Wait() (*ethgo.Receipt, error) {
	return a.t.waitForReceipt(a.hash)
}

type txnStepStatus int

const (
	txnStepQueued txnStepStatus = iota
	txnStepPending
	txnStepCompleted
	txnStepFailed
)

type txnStep struct {
	name     string
	action   func() asyncTxn
	postHook func(receipt *ethgo.Receipt)
	status   txnStepStatus
}

type txnSender struct {
	client  *jsonrpc.Client
	account *wallet.Account
}

func (t *txnSender) sendTransaction(txn *types.Transaction) asyncTxn {
	if txn.GasPrice.Uint64() == 0 {
		txn.GasPrice = big.NewInt(defaultGasPrice)
	}
	if txn.Gas == 0 {
		txn.Gas = defaultGasLimit
	}
	if txn.Nonce == 0 {
		nonce, err := t.client.Eth().GetNonce(t.account.Ecdsa.Address(), ethgo.Latest)
		if err != nil {
			panic(err)
		}
		txn.Nonce = nonce
	}

	chainID, err := t.client.Eth().ChainID()
	if err != nil {
		panic(err)
	}

	privateKey, err := t.account.GetEcdsaPrivateKey()
	if err != nil {
		panic(err)
	}

	signer := crypto.NewEIP155Signer(chainID.Uint64())
	signedTxn, err := signer.SignTx(txn, privateKey)
	if err != nil {
		panic(err)
	}

	txnRaw := signedTxn.MarshalRLP()
	hash, err := t.client.Eth().SendRawTransaction(txnRaw)
	if err != nil {
		panic(err)
	}
	return &asyncTxnImpl{hash: hash, t: t}
}

func (t *txnSender) waitForReceipt(hash ethgo.Hash) (*ethgo.Receipt, error) {
	var count uint64
	for {
		receipt, err := t.client.Eth().GetTransactionReceipt(hash)
		if err != nil {
			if err.Error() != "not found" {
				return nil, err
			}
		}
		if receipt != nil {
			return receipt, nil
		}
		if count > 1200 {
			break
		}
		time.Sleep(100 * time.Millisecond)
		count++
	}
	return nil, fmt.Errorf("timeout")
}

func newDemoClient() *jsonrpc.Client {
	client, err := jsonrpc.NewClient("http://localhost:9545")
	if err != nil {
		panic("cannot connect with jsonrpc")
	}
	return client
}

func newTxnSender(sender *wallet.Account) *txnSender {
	s := &txnSender{
		account: sender,
		client:  newDemoClient(),
	}
	return s
}

func stake(sender *txnSender) asyncTxn {
	if stakeFn == nil {
		panic("failed to create method")
	}

	input, err := stakeFn.Encode([]interface{}{})
	if err != nil {
		panic(err)
	}

	receipt := sender.sendTransaction(&types.Transaction{
		To:    &stakeManager,
		Input: input,
		Value: big.NewInt(1000),
	})
	return receipt
}

func whitelist(sender *txnSender, addr types.Address) asyncTxn {
	input, err := whitelistFn.Encode([]interface{}{
		[]types.Address{addr},
	})
	if err != nil {
		panic(err)
	}

	receipt := sender.sendTransaction(&types.Transaction{
		To:    &stakeManager,
		Input: input,
	})
	return receipt
}

func fund(sender *txnSender, addr types.Address) asyncTxn {
	genesisAmount, _ := new(big.Int).SetString("1000000000000000000", 10)

	to := types.Address(addr)
	receipt := sender.sendTransaction(&types.Transaction{
		To:    &to,
		Value: genesisAmount,
	})
	return receipt
}

func registerValidator(sender *txnSender, account *wallet.Account) asyncTxn {
	if registerFn == nil {
		panic("failed to create method")
	}

	signature, err := account.Bls.Sign([]byte("Polybft validator"))
	if err != nil {
		panic(err)
	}

	sigMarshal, err := signature.ToBigInt()
	if err != nil {
		panic(err)
	}
	pubKeys, err := account.Bls.PublicKey().ToBigInt()
	if err != nil {
		panic(err)
	}
	input, err := registerFn.Encode([]interface{}{
		sigMarshal,
		pubKeys,
	})
	if err != nil {
		panic(err)
	}

	return sender.sendTransaction(&types.Transaction{
		To:    &stakeManager,
		Input: input,
	})
}

// NewValidator represents validator which is being registered to the chain
type NewValidator struct {
	Validator ethgo.Address
}
