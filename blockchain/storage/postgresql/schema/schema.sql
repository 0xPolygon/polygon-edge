
CREATE TABLE headers (
    hash                char(66) PRIMARY KEY,
    parent_hash         char(66),
    sha3_uncles         char(66),
    miner               char(42),
    state_root          char(66),
    transactions_root   char(66),
    receipts_root       char(66),
    logs_bloom          char(514),
    difficulty          numeric,
    number              int,
    gas_limit           int,
    gas_used            int,
    timestamp           int,
    extradata           text,
    mixhash             char(66),
    nonce               char(18)
);

CREATE TABLE uncles (
    uncle   char(66),
    hash    char(66)
);

CREATE TABLE header (
    hash    char(66) REFERENCES headers(hash),
    number  int,
    forks   text
);

CREATE TABLE transactions (
    txhash      char(66) PRIMARY KEY,
    hash        char(66),
    nonce       int,
    gas_price   text,
    gas         int,
    dst         char(42),
    value       text,
    input       text,
    v           smallint,
    r           text,
    s           text
);

/*
TODO, Add 'log' table. It is necessary to include txindex
and other contextual information in the receipt type.
*/

CREATE TABLE receipts (
    hash                char(66) REFERENCES headers(hash),
    txhash              char(66) REFERENCES transactions(txhash),
    root                text,
    cumulative_gas_used int,
    gas_used            int,
    bloom               char(514),
    contract_address    char(42)
);

CREATE TABLE difficulty (
    hash        char(66) REFERENCES headers(hash),
    difficulty  numeric
);

CREATE TABLE canonical (
    hash    char(66) REFERENCES headers(hash),
    number  int UNIQUE
);

CREATE TABLE snapshot (
    hash  char(66) REFERENCES headers(hash),
    blob  bytea
);

CREATE TABLE tx_lookup (
    hash    char(66) REFERENCES headers(hash),
    number  numeric
);
