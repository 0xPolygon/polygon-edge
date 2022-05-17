package ibft

// const (
// 	BenchNumValidators = 100
// )

// var (
// 	benchKeys    = []*ecdsa.PrivateKey{}
// 	benchBLSKeys = []*bls_sig.SecretKey{}
// 	benchAddrs   = []types.Address{}

// 	benchHeaders []*types.Header
// 	genesis      = types.StringToHash("0")

// 	numHeaders = []int{
// 		10,
// 		100,
// 		1000,
// 		10000,
// 	}
// )

// func obtainKeys(n int) error {
// 	r := make([]byte, 32)
// 	for len(benchKeys) < n {
// 		key, err := crypto.GenerateKey()
// 		if err != nil {
// 			return err
// 		}

// 		benchKeys = append(benchKeys, key)

// 		rand.Read(r)
// 		bls := bls_sig.NewSigBasicVt()
// 		_, sk, _ := bls.KeygenWithSeed(r)
// 		benchBLSKeys = append(benchBLSKeys, sk)

// 		benchAddrs = append(benchAddrs, crypto.PubKeyToAddress(&key.PublicKey))
// 	}

// 	return nil
// }

// func createHeaders(n int, bls bool) ([]*types.Header, error) {
// 	headers := make([]*types.Header, n)

// 	for i := 0; i < n; i++ {
// 		parentHeader := &types.Header{
// 			Hash:   genesis,
// 			Number: 0,
// 		}

// 		if i > 0 {
// 			parentHeader = headers[i-1]
// 		}

// 		header := &types.Header{
// 			ParentHash: parentHeader.Hash,
// 			Number:     parentHeader.Number + 1,
// 		}

// 		var seal Sealer
// 		if bls {
// 			seal = new(BLSSeal)
// 		} else {
// 			seal = new(SerializedSeal)
// 		}

// 		putIbftExtra(header, &IstanbulExtra{
// 			Validators:          benchAddrs,
// 			Seal:                []byte{},
// 			CommittedSeal:       seal,
// 			ParentCommittedSeal: seal,
// 		})

// 		headers[i] = header
// 	}

// 	return headers, nil
// }

// func initBench(N int, bls bool) (err error) {
// 	if err = obtainKeys(BenchNumValidators); err != nil {
// 		return
// 	}

// 	if benchHeaders, err = createHeaders(N, bls); err != nil {
// 		return
// 	}

// 	return nil
// }

// func BenchmarkSerializedSignature(b *testing.B) {
// 	b.Skip()

// 	for _, size := range numHeaders {
// 		size := size
// 		if err := initBench(size, false); err != nil {
// 			b.Fatal(err)

// 			return
// 		}

// 		b.Run(fmt.Sprintf("%d_Headers_%d_Validators", size, len(benchKeys)), func(b *testing.B) {
// 			b.ResetTimer()
// 			benchSerializedSignature(b, size)
// 		})
// 	}
// }

// func BenchmarkBLSSignature(b *testing.B) {
// 	for _, size := range numHeaders {
// 		size := size
// 		if err := initBench(size, true); err != nil {
// 			b.Fatal(err)

// 			return
// 		}

// 		b.Run(fmt.Sprintf("%d_Headers_%d_Validators", size, len(benchKeys)), func(b *testing.B) {
// 			b.ResetTimer()
// 			benchBLSSignature(b, size)
// 		})
// 	}
// }

// func benchSerializedSignature(b *testing.B, numHeader int) {
// 	b.Helper()

// 	var err error

// 	for i := 0; i < numHeader; i++ {
// 		header := benchHeaders[i]
// 		seals := make([][]byte, len(benchKeys))

// 		if i == 0 {
// 			seal := new(SerializedSeal)
// 			initIbftExtra(header, benchAddrs, seal, false)
// 		} else {
// 			parentExtra, err := getIbftExtra(benchHeaders[i-1])
// 			if err != nil {
// 				b.Fatal(err)

// 				return
// 			}

// 			initIbftExtra(header, benchAddrs, parentExtra.CommittedSeal, false)
// 		}

// 		for j, k := range benchKeys {
// 			if seals[j], err = createCommittedSeal(k, benchHeaders[i]); err != nil {
// 				b.Fatal(err)

// 				return
// 			}
// 		}

// 		header, err := writeCommittedSeals(header, seals, false, nil)
// 		if err != nil {
// 			b.Fatal(err)

// 			return
// 		}

// 		benchHeaders[i] = header
// 	}
// }

// func benchBLSSignature(b *testing.B, numHeader int) {
// 	b.Helper()

// 	for i := 0; i < numHeader; i++ {
// 		header := benchHeaders[i]

// 		seals := make([]*bls_sig.Signature, len(benchBLSKeys))

// 		if i == 0 {
// 			seal := new(BLSSeal)
// 			initIbftExtra(header, benchAddrs, seal, false)
// 		} else {
// 			parentExtra, err := getIbftExtra(benchHeaders[i-1])
// 			if err != nil {
// 				b.Fatal(err)

// 				return
// 			}

// 			initIbftExtra(header, benchAddrs, parentExtra.CommittedSeal, false)
// 		}

// 		bitMap := new(big.Int)
// 		for j, k := range benchBLSKeys {
// 			seal, err := createBLSSeal(k, benchHeaders[i])
// 			if err != nil {
// 				b.Fatal(err)

// 				return
// 			}

// 			seals[j] = seal
// 			bitMap.SetBit(bitMap, j, 1)
// 		}

// 		pubKeys := make([]*bls_sig.PublicKey, len(benchBLSKeys))
// 		for i, sk := range benchBLSKeys {
// 			pubKeys[i], _ = sk.GetPublicKey()
// 		}

// 		blsPop := bls_sig.NewSigPop()
// 		aSig, _ := blsPop.AggregateSignatures(seals...)
// 		sigBytes, _ := aSig.MarshalBinary()

// 		seal := &BLSSeal{
// 			Bitmap:    bitMap,
// 			Signature: sigBytes,
// 		}

// 		if err := packCommittedSealIntoIbftExtra(header, seal); err != nil {
// 			b.Fatal(err)

// 			return
// 		}

// 		benchHeaders[i] = header
// 	}
// }

// func createBLSSeal(blsKey *bls_sig.SecretKey, header *types.Header) (*bls_sig.Signature, error) {
// 	hash, err := calculateHeaderHash(header)
// 	if err != nil {
// 		return nil, err
// 	}

// 	// if we are singing the committed seals we need to do something more
// 	msg := crypto.Keccak256(commitMsg(hash))

// 	bls1 := bls_sig.NewSigPop()
// 	return bls1.Sign(blsKey, msg)
// }
