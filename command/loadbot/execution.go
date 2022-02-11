package loadbot

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"
	"time"

	"github.com/0xPolygon/polygon-edge/command/loadbot/generator"
	txpoolOp "github.com/0xPolygon/polygon-edge/txpool/proto"
	"github.com/golang/protobuf/ptypes/any"

	"github.com/0xPolygon/polygon-edge/helper/tests"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/umbracle/go-web3"
	"github.com/umbracle/go-web3/jsonrpc"
)

const (
	maxReceiptWait = 5 * time.Minute
	minReceiptWait = 30 * time.Second

	defaultFastestTurnAround = time.Hour * 24
	defaultSlowestTurnAround = time.Duration(0)

	defaultGasLimit = 5242880 // 0x500000
)

type Mode string

const (
	transfer Mode = "transfer"
	deploy   Mode = "deploy"
	erc20    Mode = "erc20"
)

type Account struct {
	Address    types.Address
	PrivateKey *ecdsa.PrivateKey
}

type Configuration struct {
	TPS              uint64
	Sender           types.Address
	Receiver         types.Address
	Value            *big.Int
	Count            uint64
	JSONRPC          string
	GRPC             string
	MaxConns         int
	GeneratorMode    Mode
	ChainID          uint64
	GasPrice         *big.Int
	GasLimit         *big.Int
	ContractArtifact *generator.ContractArtifact
	ConstructorArgs  []byte // smart contract constructor args
}

type metadata struct {
	// turn around time for the transaction
	turnAroundTime time.Duration

	// block where it was sealed
	blockNumber uint64
}

type ExecDuration struct {
	// turnAroundMap maps the transaction hash -> turn around time for passing transactions
	turnAroundMap     sync.Map
	turnAroundMapSize uint64

	// blockTransactions maps how many transactions went into a block
	blockTransactions map[uint64]uint64

	// Arrival Time - Time at which the transaction is added
	// Completion Time -Time at which the transaction is sealed
	// Turn around time - Completion Time – Arrival Time

	// AverageTurnAround is the average turn around time for all passing transactions
	AverageTurnAround time.Duration

	// FastestTurnAround is the fastest turn around time recorded for a transaction
	FastestTurnAround time.Duration

	// SlowestTurnAround is the slowest turn around time recorded for a transaction
	SlowestTurnAround time.Duration

	// TotalExecTime is the total execution time for a single loadbot run
	TotalExecTime time.Duration

	// gas per block used
	GasUsed map[uint64]uint64
}

// calcTurnAroundMetrics updates the turn around metrics based on the turnAroundMap
func (ed *ExecDuration) calcTurnAroundMetrics() {
	// Set the initial values
	fastestTurnAround := defaultFastestTurnAround
	slowestTurnAround := defaultSlowestTurnAround
	totalPassing := atomic.LoadUint64(&ed.turnAroundMapSize)

	var (
		zeroTime  time.Time // Zero time
		totalTime time.Time // Zero time used for tracking
	)

	if totalPassing == 0 {
		// No data to show, use zero data
		zeroDuration := time.Duration(0)
		ed.SlowestTurnAround = zeroDuration
		ed.FastestTurnAround = zeroDuration
		ed.AverageTurnAround = zeroDuration

		return
	}

	ed.turnAroundMap.Range(func(_, value interface{}) bool {
		data, ok := value.(*metadata)
		if !ok {
			return false
		}

		turnAroundTime := data.turnAroundTime

		// Update the duration metrics
		if turnAroundTime < fastestTurnAround {
			fastestTurnAround = turnAroundTime
		}

		if turnAroundTime > slowestTurnAround {
			slowestTurnAround = turnAroundTime
		}

		totalTime = totalTime.Add(turnAroundTime)

		ed.blockTransactions[data.blockNumber]++

		return true
	})

	averageDuration := (totalTime.Sub(zeroTime)) / time.Duration(totalPassing)

	ed.SlowestTurnAround = slowestTurnAround
	ed.FastestTurnAround = fastestTurnAround
	ed.AverageTurnAround = averageDuration
}

// reportExecTime reports the turn around time for a transaction
// for a single loadbot run
func (ed *ExecDuration) reportTurnAroundTime(
	txHash web3.Hash,
	data *metadata,
) {
	ed.turnAroundMap.Store(txHash, data)
	atomic.AddUint64(&ed.turnAroundMapSize, 1)
}

type Metrics struct {
	TotalTransactionsSentCount uint64
	FailedTransactionsCount    uint64
	TransactionDuration        ExecDuration

	// contracts
	FailedContractTransactionsCount uint64
	ContractDeploymentDuration      ExecDuration
	ContractAddress                 web3.Address

	CumulativeGasUsed uint64
}

type Loadbot struct {
	cfg       *Configuration
	metrics   *Metrics
	generator generator.TransactionGenerator
}

// calcMaxTimeout calculates the max timeout for transactions receipts
// based on the transaction count and tps params
func calcMaxTimeout(count, tps uint64) time.Duration {
	waitTime := minReceiptWait
	// The receipt timeout should be at max maxReceiptWait
	// or minReceiptWait + tps / count * 100
	// This way the wait time scales linearly for more stressful situations
	waitFactor := time.Duration(float64(tps)/float64(count)*100) * time.Second

	if waitTime+waitFactor > maxReceiptWait {
		return maxReceiptWait
	}

	return waitTime + waitFactor
}

func NewLoadBot(cfg *Configuration, metrics *Metrics) *Loadbot {
	return &Loadbot{
		cfg:     cfg,
		metrics: metrics,
	}
}

func getInitialSenderNonce(client *jsonrpc.Client, address types.Address) (uint64, error) {
	nonce, err := client.Eth().GetNonce(web3.Address(address), web3.Latest)
	if err != nil {
		return 0, fmt.Errorf("failed to query initial sender nonce: %w", err)
	}

	return nonce, nil
}

func getAverageGasPrice(client *jsonrpc.Client) (uint64, error) {
	gasPrice, err := client.Eth().GasPrice()
	if err != nil {
		return 0, fmt.Errorf("failed to query initial gas price: %w", err)
	}

	return gasPrice, nil
}

func estimateGas(client *jsonrpc.Client, txn *types.Transaction) (uint64, error) {
	gasEstimate, err := client.Eth().EstimateGas(&web3.CallMsg{
		From:     web3.Address(txn.From),
		To:       (*web3.Address)(txn.To),
		Data:     txn.Input,
		GasPrice: txn.GasPrice.Uint64(),
		Value:    txn.Value,
	})

	if err != nil {
		return 0, fmt.Errorf("failed to query gas estimate: %w", err)
	}

	if gasEstimate == 0 {
		gasEstimate = defaultGasLimit
	}

	return gasEstimate, nil
}

func (l *Loadbot) executeTxn(
	client txpoolOp.TxnPoolOperatorClient,
	mode string,
	contractAddr *types.Address,
) (web3.Hash, error) {
	var (
		txn *types.Transaction
		err error
	)

	if mode == "erc20Transfer" {
		// convert web3 to types address
		txn, err = l.generator.GenerateTokenTransferTransaction(mode, contractAddr)
		if err != nil {
			return web3.Hash{}, err
		}
	} else {
		txn, err = l.generator.GenerateTransaction(mode)
		if err != nil {
			return web3.Hash{}, err
		}
	}

	addReq := &txpoolOp.AddTxnReq{
		Raw: &any.Any{
			Value: txn.MarshalRLP(),
		},
		From: types.ZeroAddress.String(),
	}

	addRes, addErr := client.AddTxn(context.Background(), addReq)
	if addErr != nil {
		return web3.Hash{}, fmt.Errorf("unable to add transaction, %w", addErr)
	}

	return web3.Hash(types.StringToHash(addRes.TxHash)), nil
}

func (l *Loadbot) Run() error {
	sender, err := extractSenderAccount(l.cfg.Sender)
	if err != nil {
		return fmt.Errorf("failed to extract sender account: %w", err)
	}

	jsonClient, err := createJSONRPCClient(l.cfg.JSONRPC, l.cfg.MaxConns)
	if err != nil {
		return fmt.Errorf("an error has occurred while creating JSON-RPC client: %w", err)
	}

	grpcClient, err := createGRPCClient(l.cfg.GRPC)
	if err != nil {
		return fmt.Errorf("an error has occurred while creating JSON-RPC client: %w", err)
	}

	defer func(client *jsonrpc.Client) {
		_ = client.Close()
	}(jsonClient)

	nonce, err := getInitialSenderNonce(jsonClient, sender.Address)
	if err != nil {
		return fmt.Errorf("unable to get initial sender nonce: %w", err)
	}

	gasPrice := l.cfg.GasPrice
	if gasPrice == nil {
		// No gas price specified, query the network for an estimation
		avgGasPrice, err := getAverageGasPrice(jsonClient)
		if err != nil {
			return fmt.Errorf("unable to get average gas price: %w", err)
		}

		gasPrice = new(big.Int).SetUint64(avgGasPrice)
	}

	// Set up the transaction generator
	generatorParams := &generator.GeneratorParams{
		Nonce:            nonce,
		ChainID:          l.cfg.ChainID,
		SenderAddress:    sender.Address,
		RecieverAddress:  l.cfg.Receiver,
		SenderKey:        sender.PrivateKey,
		GasPrice:         gasPrice,
		Value:            l.cfg.Value,
		ContractArtifact: l.cfg.ContractArtifact,
		ConstructorArgs:  l.cfg.ConstructorArgs,
	}

	var (
		txnGenerator generator.TransactionGenerator
		genErr       error
	)

	switch l.cfg.GeneratorMode {
	case transfer:
		txnGenerator, genErr = generator.NewTransferGenerator(generatorParams)
	default:
		txnGenerator, genErr = generator.NewDeployGenerator(generatorParams)
	}

	if genErr != nil {
		return fmt.Errorf("unable to start generator, %w", genErr)
	}

	l.generator = txnGenerator

	// Get the gas estimate
	exampleTxn, err := l.generator.GetExampleTransaction()
	if err != nil {
		return fmt.Errorf("unable to get example transaction, %w", err)
	}

	gasLimit := l.cfg.GasLimit
	if gasLimit == nil {
		// No gas limit specified, query the network for an estimation
		gasEstimate, estimateErr := estimateGas(jsonClient, exampleTxn)
		if estimateErr != nil {
			return fmt.Errorf("unable to get gas estimate, %w", err)
		}

		gasLimit = new(big.Int).SetUint64(gasEstimate)
	}

	l.generator.SetGasEstimate(gasLimit.Uint64())

	ticker := time.NewTicker(1 * time.Second / time.Duration(l.cfg.TPS))
	defer ticker.Stop()

	var wg sync.WaitGroup

	receiptTimeout := calcMaxTimeout(l.cfg.Count, l.cfg.TPS)

	startTime := time.Now()

	// deploy contracts
	l.deployContract(grpcClient, jsonClient, receiptTimeout)

	for i := uint64(0); i < l.cfg.Count; i++ {
		<-ticker.C

		l.metrics.TotalTransactionsSentCount += 1

		wg.Add(1)

		go func(index uint64) {
			defer wg.Done()

			var txHash web3.Hash
			// Start the performance timer
			start := time.Now()

			// run different transactions for different modes
			if l.cfg.GeneratorMode == erc20 {
				// Execute ERC20 Contract token transaction and report any errors
				contractAddr := types.Address(l.metrics.ContractAddress)

				txHash, err = l.executeTxn(grpcClient, "erc20Transfer", &contractAddr)
				if err != nil {
					l.generator.MarkFailedTxn(&generator.FailedTxnInfo{
						Index:  index,
						TxHash: txHash.String(),
						Error: &generator.TxnError{
							Error:     err,
							ErrorType: generator.AddErrorType,
						},
					})
					atomic.AddUint64(&l.metrics.FailedTransactionsCount, 1)

					return
				}
			} else {
				// Execute the transaction
				txHash, err = l.executeTxn(grpcClient, "transaction", &types.ZeroAddress)
				if err != nil {
					l.generator.MarkFailedTxn(&generator.FailedTxnInfo{
						Index:  index,
						TxHash: txHash.String(),
						Error: &generator.TxnError{
							Error:     err,
							ErrorType: generator.AddErrorType,
						},
					})
					atomic.AddUint64(&l.metrics.FailedTransactionsCount, 1)

					return
				}
			}

			ctx, cancel := context.WithTimeout(context.Background(), receiptTimeout)
			defer cancel()

			receipt, err := tests.WaitForReceipt(ctx, jsonClient.Eth(), txHash)

			if err != nil {
				l.generator.MarkFailedTxn(&generator.FailedTxnInfo{
					Index:  index,
					TxHash: txHash.String(),
					Error: &generator.TxnError{
						Error:     err,
						ErrorType: generator.ReceiptErrorType,
					},
				})
				atomic.AddUint64(&l.metrics.FailedTransactionsCount, 1)

				return
			}

			// Stop the performance timer
			end := time.Now()

			l.metrics.TransactionDuration.reportTurnAroundTime(
				txHash,
				&metadata{
					turnAroundTime: end.Sub(start),
					blockNumber:    receipt.BlockNumber,
				},
			)
		}(i)
	}

	wg.Wait()

	endTime := time.Now()

	// Calculate the turn around metrics now that the loadbot is done
	l.metrics.TransactionDuration.calcTurnAroundMetrics()
	l.metrics.TransactionDuration.TotalExecTime = endTime.Sub(startTime)

	return nil
}
