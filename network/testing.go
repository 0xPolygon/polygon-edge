package network

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/0xPolygon/polygon-sdk/chain"
	"github.com/0xPolygon/polygon-sdk/secrets"
	"github.com/0xPolygon/polygon-sdk/secrets/local"
	"github.com/hashicorp/go-hclog"
	"github.com/stretchr/testify/assert"
)

var initialPort = uint64(2000)

func CreateServer(t *testing.T, callback func(c *Config)) *Server {
	// create the server
	cfg := DefaultConfig()
	cfg.Addr.Port = int(atomic.AddUint64(&initialPort, 1))
	cfg.Chain = &chain.Chain{
		Params: &chain.Params{
			ChainID: 1,
		},
	}

	logger := hclog.NewNullLogger()

	if callback != nil {
		callback(cfg)
	}

	secretsManager, factoryErr := local.SecretsManagerFactory(
		nil,
		&secrets.SecretsManagerParams{
			Logger: logger,
			Extra: map[string]interface{}{
				secrets.Path: cfg.DataDir,
			},
		},
	)

	assert.NoError(t, factoryErr)

	cfg.SecretsManager = secretsManager

	srv, err := NewServer(logger, cfg)
	assert.NoError(t, err)
	assert.NoError(t, srv.Start())

	return srv
}

func MultiJoinSerial(t *testing.T, srvs []*Server) {
	dials := []*Server{}
	for i := 0; i < len(srvs)-1; i++ {
		srv, dst := srvs[i], srvs[i+1]
		dials = append(dials, srv, dst)
	}
	MultiJoin(t, dials...)
}

func MultiJoin(t *testing.T, srvs ...*Server) {
	if len(srvs)%2 != 0 {
		t.Fatal("not an even number")
	}

	doneCh := make(chan error)
	for i := 0; i < len(srvs); i += 2 {
		go func(i int) {
			src, dst := srvs[i], srvs[i+1]
			doneCh <- src.Join(dst.AddrInfo(), 10*time.Second)
		}(i)
	}

	for i := 0; i < len(srvs)/2; i++ {
		err := <-doneCh
		if err != nil {
			t.Fatal(err)
		}
	}
}
