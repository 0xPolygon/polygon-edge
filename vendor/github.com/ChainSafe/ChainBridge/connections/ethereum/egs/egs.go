// Copyright 2020 ChainSafe Systems
// SPDX-License-Identifier: LGPL-3.0-only

package egs

import (
	"context"
	"encoding/json"
	"errors"
	"io/ioutil"
	"math/big"
	"net/http"
	"time"
)

const (
	// Retries is the amount of times to reattempt fetching price data before giving up
	Retries = 5
	// Timeout is the duration in seconds for http timeouts
	Timeout = 10
	Fast    = "fast"
	Fastest = "fastest"
	Average = "average"
)

type gasPriceResponse struct {
	Fast          float32     `json:"fast"`
	Fastest       float32     `json:"fastest"`
	SafeLow       float32     `json:"safeLow"`
	Average       float32     `json:"average"`
	BlockTime     float32     `json:"block_time"`
	BlockNum      int64       `json:"blockNum"`
	Speed         float32     `json:"speed"`
	SafeLowWait   float32     `json:"safeLowWait"`
	AvgWait       float32     `json:"avgWait"`
	FastWait      float32     `json:"fastWait"`
	FastestWait   float32     `json:"fastestWait"`
	GasPriceRange interface{} `json:"gasPriceRange"`
}

// FetchGasPrice will query EGS for the current gas prices and return the price for the specified speed.
func FetchGasPrice(apiKey, speed string) (*big.Int, error) {
	var gsnURL = "https://ethgasstation.info/api/ethgasAPI.json?api-key=" + apiKey

	for i := 0; i < Retries; i++ {
		res, err := queryAPI(gsnURL)
		if err != nil {
			return nil, err
		}

		if res != nil {
			return parsePrice(res, speed), nil
		}

		time.Sleep(1 * time.Second)
	}

	return nil, errors.New("failed to fetch GSN gas price - retries exceeded")
}

func queryAPI(url string) (*gasPriceResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	defer func() { _ = res.Body.Close() }()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	var data gasPriceResponse
	err = json.Unmarshal(body, &data)
	if err != nil {
		return nil, err
	}

	return &data, nil
}

func parsePrice(result *gasPriceResponse, speed string) *big.Int {
	var res *big.Int
	switch speed {
	case Fastest:
		res = big.NewInt(int64(result.Fastest))
	case Fast:
		res = big.NewInt(int64(result.Fast))
	case Average:
		res = big.NewInt(int64(result.Average))
	default:
		res = big.NewInt(int64(result.Fast))
	}
	// Make sure we get at least 10 gwei to avoid prices of 0
	if res.Cmp(big.NewInt(1)) == -1 {
		res = big.NewInt(1)
	}

	base := big.NewInt(8) // we are using 8 here but not 9 bcs ethgas station returns values in Gwei * 10
	return res.Mul(res, big.NewInt(0).Exp(big.NewInt(10), base, nil))
}
