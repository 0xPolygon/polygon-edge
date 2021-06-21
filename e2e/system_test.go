package e2e

//func TestSystem_StakeAmount(t *testing.T) {
//	validAccounts := []struct {
//		address types.Address
//		balance *big.Int
//	}{
//		// Valid account #1
//		{
//			types.StringToAddress("10000"),
//			framework.EthToWei(50), // 50 ETH
//		},
//		// Empty account
//		{
//			types.StringToAddress("10001"),
//			big.NewInt(0),
//		},
//	}
//
//	stakingAddress := types.StringToAddress("1001")
//
//	testTable := []struct {
//		name          string
//		staker        types.Address
//		stakeAmount   *big.Int
//		shouldSucceed bool
//	}{
//		{
//			"Valid stake",
//			validAccounts[0].address,
//			framework.EthToWei(10),
//			true,
//		},
//		{
//			"Invalid stake",
//			validAccounts[1].address,
//			framework.EthToWei(100),
//			false,
//		},
//	}
//
//	srvs := framework.NewTestServers(t, 1, func(config *framework.TestServerConfig) {
//		config.SetConsensus(framework.ConsensusDev)
//		config.SetSeal(true)
//		for _, acc := range validAccounts {
//			config.Premine(acc.address, acc.balance)
//		}
//	})
//	srv := srvs[0]
//
//	rpcClient := srv.JSONRPC()
//	for _, testCase := range testTable {
//		t.Run(testCase.name, func(t *testing.T) {
//			// Fetch the staker balance before sending
//			balanceStaker, err := rpcClient.Eth().GetBalance(
//				web3.Address(testCase.staker),
//				web3.Latest,
//			)
//			assert.NoError(t, err)
//
//			balanceStakerStaked, err := rpcClient.Eth().GetStakedBalance(
//				web3.Address(testCase.staker),
//				web3.Latest,
//			)
//
//			assert.NoError(t, err)
//
//			// Set the preSend balances
//			previousStakerBalance := balanceStaker
//			previousStakerStakedBalance := balanceStakerStaked
//
//			// Create the transaction
//			toAddr := web3.Address(stakingAddress)
//			txnObject := &web3.Transaction{
//				From:     web3.Address(testCase.staker),
//				To:       &toAddr,
//				GasPrice: uint64(1048576),
//				Gas:      1000000,
//				Value:    testCase.stakeAmount,
//			}
//
//			fee := big.NewInt(0)
//
//			// Do the transfer
//			txnHash, err := rpcClient.Eth().SendTransaction(txnObject)
//			assert.NoError(t, err)
//			assert.IsTypef(t, web3.Hash{}, txnHash, "Return type mismatch")
//
//			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
//			defer cancel()
//			receipt, err := srv.WaitForReceipt(ctx, txnHash)
//			assert.NoError(t, err)
//			assert.NotNil(t, receipt)
//
//			if testCase.shouldSucceed {
//				fee = new(big.Int).Mul(
//					big.NewInt(int64(receipt.GasUsed)),
//					big.NewInt(int64(txnObject.GasPrice)),
//				)
//			}
//
//			// Fetch the balance after sending
//			balanceStaker, err = rpcClient.Eth().GetBalance(
//				web3.Address(testCase.staker),
//				web3.Latest,
//			)
//			assert.NoError(t, err)
//
//			balanceStakerStaked, err = rpcClient.Eth().GetStakedBalance(
//				web3.Address(testCase.staker),
//				web3.Latest,
//			)
//			assert.NoError(t, err)
//
//			expectedStakerBalance := previousStakerBalance
//			if testCase.shouldSucceed {
//				expectedStakerBalance = previousStakerBalance.Sub(
//					previousStakerBalance,
//					new(big.Int).Add(testCase.stakeAmount, fee),
//				)
//			}
//
//			expectedStakerStakedBalance := previousStakerStakedBalance
//			if testCase.shouldSucceed {
//				expectedStakerStakedBalance = previousStakerStakedBalance.Add(
//					previousStakerBalance,
//					testCase.stakeAmount,
//				)
//			}
//
//			// Check the balances
//			assert.Equalf(t,
//				expectedStakerBalance,
//				balanceStaker,
//				"Staker balance incorrect")
//
//			assert.Equalf(t,
//				expectedStakerStakedBalance,
//				balanceStakerStaked,
//				"Staker's staked balance incorrect")
//		})
//	}
//}
