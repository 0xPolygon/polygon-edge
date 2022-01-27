module github.com/libp2p/go-libp2p-peerstore

go 1.16

retract v0.2.9 // Contains backwards-incompatible changes. Use v0.3.0 instead.

require (
	github.com/gogo/protobuf v1.3.2
	github.com/hashicorp/golang-lru v0.5.4
	github.com/ipfs/go-datastore v0.5.0
	github.com/ipfs/go-ds-badger v0.3.0
	github.com/ipfs/go-ds-leveldb v0.5.0
	github.com/ipfs/go-log/v2 v2.3.0
	github.com/libp2p/go-buffer-pool v0.0.2
	github.com/libp2p/go-libp2p-core v0.12.0
	github.com/multiformats/go-base32 v0.0.3
	github.com/multiformats/go-multiaddr v0.3.3
	github.com/multiformats/go-multiaddr-fmt v0.1.0
	github.com/stretchr/testify v1.7.0
	go.uber.org/goleak v1.1.10
	golang.org/x/tools v0.1.1 // indirect
)
