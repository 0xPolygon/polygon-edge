package polybft

type txpoolHook struct {
	txPool txPoolInterface
}

func newTxpoolHook(txPool txPoolInterface) *txpoolHook {
	h := &txpoolHook{
		txPool: txPool,
	}

	return h
}

func (t *txpoolHook) Name() string {
	return "txpool-hook"
}

func (t *txpoolHook) PostBlock(req *PostBlockRequest) error {
	t.txPool.ResetWithHeaders(req.Header)

	return nil
}
