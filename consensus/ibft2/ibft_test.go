package ibft2

import (
	"io/ioutil"
	"testing"
	"time"

	"github.com/0xPolygon/minimal/consensus"
	"github.com/0xPolygon/minimal/consensus/ibft2/proto"
	"github.com/0xPolygon/minimal/types"
	"github.com/hashicorp/go-hclog"
)

func makeServers(t *testing.T, num int) []*Ibft2 {
	transport := &mockNetwork{}

	servers := []*Ibft2{}
	for i := 0; i < num; i++ {
		tmpDir, err := ioutil.TempDir("/tmp", "ibft-")
		if err != nil {
			t.Fatal(err)
		}

		p := &Ibft2{
			logger: hclog.NewNullLogger(),
			index:  uint64(i),
			config: &consensus.Config{
				Path: tmpDir,
			},
			transportFactory: transport.newTransport,
			preprepareCh:     make(chan *proto.MessageReq, 10),
			commitCh:         make(chan *proto.MessageReq, 10),
			prepareCh:        make(chan *proto.MessageReq, 10),
		}
		p.createKey()
		servers = append(servers, p)
	}

	// get the validators
	validators := []types.Address{}
	for _, srv := range servers {
		validators = append(validators, srv.validatorKeyAddr)
	}

	// add the validators to the servers
	for _, srv := range servers {
		srv.validators = validators
	}

	// start the seal
	for _, srv := range servers {
		srv.StartSeal()
	}
	return servers
}

func TestStates(t *testing.T) {
	makeServers(t, 3)

	time.Sleep(3 * time.Second)
}
