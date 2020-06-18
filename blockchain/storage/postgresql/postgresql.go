package postgresql

import (
	"database/sql"
	"fmt"
	"math/big"
	"strings"

	"github.com/0xPolygon/minimal/blockchain/storage"
	"github.com/0xPolygon/minimal/helper/hex"
	"github.com/0xPolygon/minimal/types"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"

	"github.com/hashicorp/go-hclog"
)

// Backend is the postgresql backend
type Backend struct {
	db *sqlx.DB
}

var _ storage.Storage = &Backend{}

// Factory creates a PostgreSQL storage
func Factory(config map[string]interface{}, logger hclog.Logger) (*Backend, error) {
	endpoint, ok := config["endpoint"]
	if !ok {
		return nil, fmt.Errorf("endpoint not found")
	}
	endpointStr, ok := endpoint.(string)
	if !ok {
		return nil, fmt.Errorf("endpoint is not a string")
	}
	db, err := sqlx.Connect("postgres", endpointStr)
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		return nil, err
	}

	// Check if the header table is populated
	var count uint
	if err := db.Get(&count, "SELECT count(*) FROM header"); err != nil {
		return nil, err
	}
	if count > 1 {
		return nil, fmt.Errorf("more than one entry in header table")
	}
	if count == 0 {
		// Insert an empty entry in the header
		if _, err := db.Exec("INSERT INTO header (forks) VALUES ('')"); err != nil {
			return nil, err
		}
	}

	b := &Backend{
		db: db,
	}
	return b, nil
}

// Close implements the storage interface
func (b *Backend) Close() error {
	return b.db.Close()
}

// WriteReceipts implements the storage interface
func (b *Backend) WriteReceipts(hash types.Hash, receipts []*types.Receipt) error {
	// TODO, it does not store logs
	query := "INSERT INTO receipts (hash, txhash, root, cumulative_gas_used, gas_used, bloom, contract_address) VALUES ($1, $2, $3, $4, $5, $6, $7)"

	tx, err := b.db.Begin()
	if err != nil {
		return err
	}
	for _, i := range receipts {
		if _, err := tx.Exec(query, hash.String(), i.TxHash, i.Root, i.CumulativeGasUsed, i.GasUsed, i.LogsBloom, i.ContractAddress); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

// ReadReceipts implements the storage interface
func (b *Backend) ReadReceipts(hash types.Hash) ([]*types.Receipt, bool) {
	query := "SELECT txhash, root, cumulative_gas_used, gas_used, bloom, contract_address FROM receipts WHERE hash=$1"

	var receipts []*types.Receipt
	if err := b.db.Select(&receipts, query, hash); err != nil {
		return nil, false
	}

	return receipts, true
}

func (b *Backend) WriteTransaction(hash types.Hash, t *types.Transaction) error {
	tx, err := b.db.Begin()
	if err != nil {
		return err
	}
	if err := b.writeTransactionImpl(tx, hash, t); err != nil {
		return err
	}
	return tx.Commit()
}

func (b *Backend) writeTransactionImpl(tx *sql.Tx, hash types.Hash, t *types.Transaction) error {
	query := "INSERT INTO transactions (hash, txhash, nonce, gas_price, gas, dst, value, input, v, r, s) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)"

	if _, err := b.db.Exec(query, hash, t.Hash, t.Nonce, t.GasPrice, t.Gas, t.To, t.Value.String(), t.Input.String(), int(t.V), t.R.String(), t.S.String()); err != nil {
		return err
	}
	return nil
}

// ReadTransaction read a transaction by hash
func (b *Backend) ReadTransaction(hash types.Hash) (*types.Transaction, bool) {
	query := "SELECT nonce, gas_price, gas, value, dst, input, v, r, s FROM transactions WHERE txhash=$1"

	txn := types.Transaction{}
	if err := b.db.Get(&txn, query, hash.String()); err != nil {
		return nil, false
	}

	hh := txn.Hash
	if hh != hash {
		return nil, false
	}
	return &txn, true
}

// ReadHeader implements the storage backend
func (b *Backend) ReadHeader(hash types.Hash) (*types.Header, bool) {
	query := "SELECT parent_hash, sha3_uncles, miner, state_root, transactions_root, receipts_root, logs_bloom, difficulty, number, gas_limit, gas_used, timestamp, extradata, mixhash, nonce FROM headers where hash=$1"

	header := types.Header{}
	if err := b.db.Get(&header, query, hash.String()); err != nil {
		// TODO, return error
		return nil, false
	}

	hh := header.Hash
	if hh != hash {
		return nil, false
	}
	return &header, true
}

// WriteHeader implements the storage backend
func (b *Backend) WriteHeader(h *types.Header) error {
	tx, err := b.db.Begin()
	if err != nil {
		return err
	}
	if err := b.writeHeaderImpl(tx, h); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (b *Backend) writeHeaderImpl(tx *sql.Tx, h *types.Header) error {
	query := `INSERT INTO headers (hash, parent_hash, sha3_uncles, miner, state_root, transactions_root, receipts_root, logs_bloom, difficulty, number, gas_limit, gas_used, timestamp, extradata, mixhash, nonce) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)`

	if _, err := tx.Exec(query, h.Hash, h.ParentHash, h.Sha3Uncles, h.Miner, h.StateRoot, h.TxRoot, h.ReceiptsRoot, h.LogsBloom, h.Difficulty, h.Number, h.GasLimit, h.GasUsed, h.Timestamp, h.ExtraData, h.MixHash, hex.EncodeToHex(h.Nonce[:])); err != nil {
		return err
	}
	return nil
}

// ReadDiff implements the storage backend
func (b *Backend) ReadDiff(hash types.Hash) (*big.Int, bool) {
	query := "SELECT difficulty FROM difficulty WHERE hash=$1"

	var diff string
	if err := b.db.Get(&diff, query, hash.String()); err != nil {
		return nil, false
	}

	bb, _ := new(big.Int).SetString(diff, 10)
	return bb, true
}

// WriteDiff implements the storage backend
func (b *Backend) WriteDiff(hash types.Hash, diff *big.Int) error {
	query := "INSERT INTO difficulty (hash, difficulty) VALUES ($1, $2)"

	if _, err := b.db.Exec(query, hash, diff.String()); err != nil {
		return err
	}
	return nil
}

// ReadCanonicalHash implements the storage backend
func (b *Backend) ReadCanonicalHash(n uint64) (types.Hash, bool) {
	query := "SELECT hash FROM canonical WHERE number=$1"

	var hash types.Hash
	if err := b.db.Get(&hash, query, n); err != nil {
		return types.Hash{}, false
	}
	return hash, true
}

// WriteCanonicalHash implements the storage backend
func (b *Backend) WriteCanonicalHash(n uint64, hash types.Hash) error {
	tx, err := b.db.Begin()
	if err != nil {
		return err
	}
	if err := b.writeCanonicalHashImpl(tx, n, hash); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (b *Backend) writeCanonicalHashImpl(tx *sql.Tx, n uint64, hash types.Hash) error {
	query := "INSERT INTO canonical (hash, number) VALUES ($1, $2) ON CONFLICT (number) DO UPDATE SET hash = $1"

	if _, err := tx.Exec(query, hash.String(), n); err != nil {
		return err
	}
	return nil
}

// WriteForks implements the storage backend
func (b *Backend) WriteForks(forks []types.Hash) error {
	query := "UPDATE header SET forks=$1"

	str := ""
	for indx, h := range forks {
		str += h.String()
		if indx != len(forks)-1 {
			str += ","
		}
	}
	if _, err := b.db.Exec(query, str); err != nil {
		return err
	}
	return nil
}

// ReadForks implements the storage backend
func (b *Backend) ReadForks() []types.Hash {
	query := "SELECT forks FROM header"

	var str string
	if err := b.db.Get(&str, query); err != nil {
		return []types.Hash{}
	}

	res := []types.Hash{}
	for _, i := range strings.Split(str, ",") {
		res = append(res, types.StringToHash(i))
	}
	return res
}

// ReadHeadHash implements the storage backend
func (b *Backend) ReadHeadHash() (types.Hash, bool) {
	query := "SELECT hash FROM header"

	var head *string
	if err := b.db.Get(&head, query); err != nil {
		return types.Hash{}, false
	}
	if head == nil {
		return types.Hash{}, false
	}
	return types.StringToHash(*head), true
}

// ReadHeadNumber implements the storage backend
func (b *Backend) ReadHeadNumber() (uint64, bool) {
	query := "SELECT number FROM header"

	var head uint64
	if err := b.db.Get(&head, query); err != nil {
		return 0, false
	}
	return head, true
}

// WriteHeadHash implements the storage backend
func (b *Backend) WriteHeadHash(h types.Hash) error {
	query := "UPDATE header SET hash=$1"

	if _, err := b.db.Exec(query, h); err != nil {
		return err
	}
	return nil
}

// WriteHeadNumber implements the storage backend
func (b *Backend) WriteHeadNumber(n uint64) error {
	query := "UPDATE header SET number=$1"

	if _, err := b.db.Exec(query, n); err != nil {
		return err
	}
	return nil
}

// WriteBody implements the storage backend
func (b *Backend) WriteBody(hash types.Hash, body *types.Body) error {
	if len(body.Uncles) == 0 && len(body.Transactions) == 0 {
		return nil
	}

	tx, err := b.db.Begin()
	if err != nil {
		return err
	}

	// write the uncles
	for _, u := range body.Uncles {
		if err := b.writeHeaderImpl(tx, u); err != nil {
			return err
		}
		if _, err := tx.Exec("INSERT INTO uncles (hash, uncle) VALUES ($1, $2)", hash, u.Hash); err != nil {
			return err
		}
	}

	// write the transactions
	for _, t := range body.Transactions {
		if err := b.writeTransactionImpl(tx, hash, t); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

// ReadBody implements the storage backend
func (b *Backend) ReadBody(hash types.Hash) (*types.Body, bool) {
	queryHeaders := "SELECT parent_hash, sha3_uncles, miner, state_root, transactions_root, receipts_root, logs_bloom, difficulty, number, gas_limit, gas_used, timestamp, extradata, mixhash, nonce FROM headers INNER JOIN uncles ON (uncles.uncle = headers.hash) AND uncles.hash=$1"

	uncles := []*types.Header{}
	if err := b.db.Select(&uncles, queryHeaders, hash); err != nil {
		return nil, false
	}

	queryTxs := "SELECT nonce, gas_price, gas, value, dst, input, v, r, s FROM transactions WHERE hash=$1"

	transactions := []*types.Transaction{}
	if err := b.db.Select(&transactions, queryTxs, hash); err != nil {
		return nil, false
	}

	body := &types.Body{
		Uncles:       uncles,
		Transactions: transactions,
	}
	return body, true
}

// WriteSnapshot implements the storage backend
func (b *Backend) WriteSnapshot(hash types.Hash, blob []byte) error {
	_, err := b.db.Exec("INSERT INTO shapshot (hash, blob) VALUES ($1, $2)", hash, blob)
	if err != nil {
		return err
	}

	return nil
}

// ReadSnapshot implements the storage backend
func (b *Backend) ReadSnapshot(hash types.Hash) ([]byte, bool) {
	query := "SELECT blob FROM shapshot WHERE hash=$1"
	var snapshot struct {
		blob []byte
	}

	if err := b.db.Select(&snapshot, query, hash); err != nil {
		return nil, false
	}

	return snapshot.blob, true
}

// WriteCanonicalHeader implements the storage backend
func (b *Backend) WriteCanonicalHeader(h *types.Header, diff *big.Int) error {
	tx, err := b.db.Begin()
	if err != nil {
		return err
	}

	if err := b.writeHeaderImpl(tx, h); err != nil {
		return err
	}
	if err := b.writeCanonicalHashImpl(tx, h.Number, h.Hash); err != nil {
		return err
	}
	if _, err := tx.Exec("UPDATE header SET hash=$1, number=$2", h.Hash, h.Number); err != nil {
		return err
	}
	if _, err := tx.Exec("INSERT INTO difficulty (hash, difficulty) VALUES ($1, $2)", h.Hash, diff.String()); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}
