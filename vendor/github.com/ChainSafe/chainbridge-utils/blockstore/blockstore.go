// Copyright 2020 ChainSafe Systems
// SPDX-License-Identifier: LGPL-3.0-only

package blockstore

import (
	"fmt"
	"io/ioutil"
	"math/big"
	"os"
	"path/filepath"

	"github.com/ChainSafe/chainbridge-utils/msg"
)

const PathPostfix = ".chainbridge/blockstore"

type Blockstorer interface {
	StoreBlock(*big.Int) error
}

var _ Blockstorer = &EmptyStore{}
var _ Blockstorer = &Blockstore{}

// Dummy store for testing only
type EmptyStore struct{}

func (s *EmptyStore) StoreBlock(_ *big.Int) error { return nil }

// Blockstore implements Blockstorer.
type Blockstore struct {
	path     string // Path excluding filename
	fullPath string
	chain    msg.ChainId
	relayer  string
}

func NewBlockstore(path string, chain msg.ChainId, relayer string) (*Blockstore, error) {
	fileName := getFileName(chain, relayer)
	if path == "" {
		def, err := getDefaultPath()
		if err != nil {
			return nil, err
		}
		path = def
	}

	return &Blockstore{
		path:     path,
		fullPath: filepath.Join(path, fileName),
		chain:    chain,
		relayer:  relayer,
	}, nil
}

// StoreBlock writes the block number to disk.
func (b *Blockstore) StoreBlock(block *big.Int) error {
	// Create dir if it does not exist
	if _, err := os.Stat(b.path); os.IsNotExist(err) {
		errr := os.MkdirAll(b.path, os.ModePerm)
		if errr != nil {
			return errr
		}
	}

	// Write bytes to file
	data := []byte(block.String())
	err := ioutil.WriteFile(b.fullPath, data, 0600)
	if err != nil {
		return err
	}
	return nil
}

// TryLoadLatestBlock will attempt to load the latest block for the chain/relayer pair, returning 0 if not found.
// Passing an empty string for path will cause it to use the home directory.
func (b *Blockstore) TryLoadLatestBlock() (*big.Int, error) {
	// If it exists, load and return
	exists, err := fileExists(b.fullPath)
	if err != nil {
		return nil, err
	}
	if exists {
		dat, err := ioutil.ReadFile(b.fullPath)
		if err != nil {
			return nil, err
		}
		block, _ := big.NewInt(0).SetString(string(dat), 10)
		return block, nil
	}
	// Otherwise just return 0
	return big.NewInt(0), nil
}

func getFileName(chain msg.ChainId, relayer string) string {
	return fmt.Sprintf("%s-%d.block", relayer, chain)
}

// getHomePath returns the home directory joined with PathPostfix
func getDefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(home, PathPostfix), nil
}

func fileExists(fileName string) (bool, error) {
	_, err := os.Stat(fileName)
	if os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	return true, nil
}
