package e2e

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/contractsapi"
	"github.com/0xPolygon/polygon-edge/consensus/polybft/wallet"
	"github.com/0xPolygon/polygon-edge/contracts"
	"github.com/0xPolygon/polygon-edge/crypto"
	secretsHelper "github.com/0xPolygon/polygon-edge/secrets/helper"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/mitchellh/go-glint"
	gc "github.com/mitchellh/go-glint/components"
	"github.com/spf13/cobra"
	"github.com/umbracle/ethgo"
	"github.com/umbracle/ethgo/jsonrpc"
)

const (
	defaultBalance = "0xD3C21BCECCEDA1000000" // 1e24
	defaultStake   = "0x56BC75E2D63100000"    // 1e20
)

var params registerParams

func GetCommand() *cobra.Command {
	registerCmd := &cobra.Command{
		Use:     "register-validator",
		Short:   "Registers a new validator",
		PreRunE: runPreRun,
		RunE:    runCommand,
	}

	setFlags(registerCmd)

	return registerCmd
}

func setFlags(cmd *cobra.Command) {
	cmd.Flags().StringVar(
		&params.newValidatorDataDir,
		dataDirFlag,
		"",
		"the directory path where new validator key is stored",
	)
	cmd.Flags().StringVar(
		&params.registratorValidatorDataDir,
		registratorDataDirFlag,
		"",
		"the directory path where registrator validator key is stored",
	)

	cmd.Flags().StringVar(
		&params.balance,
		balanceFlag,
		defaultBalance,
		"balance which is going to be funded to the new validator account",
	)

	cmd.Flags().StringVar(
		&params.stake,
		stakeFlag,
		defaultStake,
		"stake represents amount which is going to be staked by the new validator account",
	)

	helper.RegisterJSONRPCFlag(cmd)
}

func runPreRun(cmd *cobra.Command, _ []string) error {
	params.jsonRPCAddr = helper.GetJSONRPCAddress(cmd)

	return params.validateFlags()
}

func runCommand(cmd *cobra.Command, _ []string) error {
	secretsManager, err := secretsHelper.SetupLocalSecretsManager(params.registratorValidatorDataDir)
	if err != nil {
		return err
	}

	existingValidatorAccount, err := wallet.NewAccountFromSecret(secretsManager)
	if err != nil {
		return err
	}

	existingValidatorSender, err := newTxnSender(existingValidatorAccount)
	if err != nil {
		return err
	}

	secretsManager, err = secretsHelper.SetupLocalSecretsManager(params.newValidatorDataDir)
	if err != nil {
		return err
	}

	newValidatorAccount, err := wallet.NewAccountFromSecret(secretsManager)
	if err != nil {
		return err
	}

	newValidatorSender, err := newTxnSender(newValidatorAccount)
	if err != nil {
		return err
	}

	var validator *NewValidator

	steps := []*txnStep{
		{
			name: "whitelist",
			action: func() asyncTxn {
				return whitelist(existingValidatorSender, types.Address(newValidatorAccount.Ecdsa.Address()))
			},
		},
		{
			name: "fund",
			action: func() asyncTxn {
				return fund(existingValidatorSender, types.Address(newValidatorAccount.Ecdsa.Address()))
			},
		},
		{
			name: "register",
			action: func() asyncTxn {
				return registerValidator(newValidatorSender, newValidatorAccount)
			},
			postHook: func(receipt *ethgo.Receipt) error {
				if receipt.Status != uint64(types.ReceiptSuccess) {
					return errors.New("register validator transaction failed")
				}

				for _, log := range receipt.Logs {
					if newValidatorEvent.Match(log) {
						event, err := newValidatorEvent.ParseLog(log)
						if err != nil {
							return err
						}

						validatorAddr, ok := event["validator"].(ethgo.Address)
						if !ok {
							return errors.New("type assertions failed for parameter validator")
						}

						validator = &NewValidator{
							Validator: validatorAddr,
						}

						return nil
					}
				}

				return errors.New("NewValidator event was not emitted")
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

	printStatus := func(done bool) {
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
			if step.err != nil {
				comps = append(comps, glint.Style(
					glint.Layout(
						status,
						glint.Layout(glint.Text("error: "+step.err.Error())).MarginLeft(5),
					).Row(),
					opts...,
				))
			}
		}

		if done {
			if validator != nil {
				comps = append(comps, glint.Text("\nDone: "+validator.Validator.String()+"\n"))
			} else {
				comps = append(comps, glint.Text("\nDone\n"))
			}
		} else {
			comps = append(comps, glint.Text("\nWaiting..."))
		}

		d.Set(comps...)
	}

	for _, step := range steps {
		step.status = txnStepPending

		printStatus(false)

		txn := step.action()
		receipt, err := txn.Wait()

		if err != nil {
			step.status = txnStepFailed
			step.err = err
		} else {
			if receipt.Status == uint64(types.ReceiptFailed) {
				step.status = txnStepFailed
			} else {
				step.status = txnStepCompleted
			}
		}

		if step.postHook != nil {
			err := step.postHook(receipt)
			if err != nil {
				step.status = txnStepFailed
				step.err = err
			}
		}

		if step.status == txnStepFailed {
			break
		}
	}

	printStatus(true)

	d.RenderFrame()
	d.Pause()

	return nil
}

const (
	defaultGasPrice = 1879048192 // 0x70000000
	defaultGasLimit = 5242880    // 0x500000
)

var (
	stakeManager      = contracts.ValidatorSetContract
	stakeFn           = contractsapi.ChildValidatorSet.Abi.Methods["stake"]
	whitelistFn       = contractsapi.ChildValidatorSet.Abi.Methods["addToWhitelist"]
	registerFn        = contractsapi.ChildValidatorSet.Abi.Methods["register"]
	newValidatorEvent = contractsapi.ChildValidatorSet.Abi.Events["NewValidator"]
)

type asyncTxn interface {
	Wait() (*ethgo.Receipt, error)
}

type asyncTxnImpl struct {
	t    *txnSender
	hash ethgo.Hash
	err  error
}

func (a *asyncTxnImpl) Wait() (*ethgo.Receipt, error) {
	// propagate error if there were any
	if a.err != nil {
		return nil, a.err
	}

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
	postHook func(receipt *ethgo.Receipt) error
	status   txnStepStatus
	err      error
}

type txnSender struct {
	client  *jsonrpc.Client
	account *wallet.Account
}

func (t *txnSender) sendTransaction(txn *types.Transaction) asyncTxn {
	if txn.GasPrice == nil {
		txn.GasPrice = big.NewInt(defaultGasPrice)
	}

	if txn.Gas == 0 {
		txn.Gas = defaultGasLimit
	}

	if txn.Nonce == 0 {
		nonce, err := t.client.Eth().GetNonce(t.account.Ecdsa.Address(), ethgo.Latest)
		if err != nil {
			return &asyncTxnImpl{err: err}
		}

		txn.Nonce = nonce
	}

	chainID, err := t.client.Eth().ChainID()
	if err != nil {
		return &asyncTxnImpl{err: err}
	}

	privateKey, err := t.account.GetEcdsaPrivateKey()
	if err != nil {
		return &asyncTxnImpl{err: err}
	}

	signer := crypto.NewEIP155Signer(chainID.Uint64())
	signedTxn, err := signer.SignTx(txn, privateKey)

	if err != nil {
		return &asyncTxnImpl{err: err}
	}

	txnRaw := signedTxn.MarshalRLP()
	hash, err := t.client.Eth().SendRawTransaction(txnRaw)

	if err != nil {
		return &asyncTxnImpl{err: err}
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

		time.Sleep(1000 * time.Millisecond)
		count++
	}

	return nil, fmt.Errorf("timeout")
}

func newDemoClient() (*jsonrpc.Client, error) {
	client, err := jsonrpc.NewClient(params.jsonRPCAddr)
	if err != nil {
		return nil, fmt.Errorf("cannot connect with jsonrpc: %w", err)
	}

	return client, err
}

func newTxnSender(sender *wallet.Account) (*txnSender, error) {
	client, err := newDemoClient()
	if err != nil {
		return nil, err
	}

	return &txnSender{
		account: sender,
		client:  client,
	}, nil
}

func stake(sender *txnSender) asyncTxn {
	if stakeFn == nil {
		return &asyncTxnImpl{err: errors.New("failed to create stake ABI function")}
	}

	input, err := stakeFn.Encode([]interface{}{})
	if err != nil {
		return &asyncTxnImpl{err: err}
	}

	stake, err := types.ParseUint256orHex(&params.stake)
	if err != nil {
		return &asyncTxnImpl{err: err}
	}

	receipt := sender.sendTransaction(&types.Transaction{
		To:    &stakeManager,
		Input: input,
		Value: stake,
	})

	return receipt
}

func whitelist(sender *txnSender, addr types.Address) asyncTxn {
	if whitelistFn == nil {
		return &asyncTxnImpl{err: errors.New("failed to create whitelist ABI function")}
	}

	input, err := whitelistFn.Encode([]interface{}{
		[]types.Address{addr},
	})
	if err != nil {
		return &asyncTxnImpl{err: err}
	}

	receipt := sender.sendTransaction(&types.Transaction{
		To:    &stakeManager,
		Input: input,
	})

	return receipt
}

func fund(sender *txnSender, addr types.Address) asyncTxn {
	balance, err := types.ParseUint256orHex(&params.balance)
	if err != nil {
		return &asyncTxnImpl{err: err}
	}

	txn := &types.Transaction{
		To:    &addr,
		Value: balance,
	}

	return sender.sendTransaction(txn)
}

func registerValidator(sender *txnSender, account *wallet.Account) asyncTxn {
	if registerFn == nil {
		return &asyncTxnImpl{err: errors.New("failed to create register ABI function")}
	}

	signature, err := account.Bls.Sign([]byte(contracts.PolyBFTRegisterMessage))
	if err != nil {
		return &asyncTxnImpl{err: err}
	}

	sigMarshal, err := signature.ToBigInt()
	if err != nil {
		return &asyncTxnImpl{err: err}
	}

	input, err := registerFn.Encode([]interface{}{
		sigMarshal,
		account.Bls.PublicKey().ToBigInt(),
	})
	if err != nil {
		return &asyncTxnImpl{err: err}
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
