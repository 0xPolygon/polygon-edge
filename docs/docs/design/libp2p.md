## Overview

PolyBFT uses a decentralized networking layer based on the libp2p protocol. The protocol provides peer-to-peer networking primitives such as peer discovery, connection management, and secure messaging. The network relies on a secure Identity Service to manage peer connectivity and handshaking, ensuring only valid peers can join the network.

## Identity

The Identity Service validates incoming connections and manages peer handshaking. It maintains a list of pending peer connections and uses a networkingServer interface to communicate with the underlying networking layer.

## Peer discovery

PolyBFT uses libp2p's distributed hash table (DHT) based on the Kademlia algorithm for peer discovery. The DHT stores information about other peers in the network, such as their addresses and availability. When a new node joins the network, it uses the DHT to find other peers that are currently online. The process of using the DHT to discover peers and then sending out connection requests is repeated periodically to maintain a sufficient number of connections in the network.

## Peer routing

Bootnodes act as rendezvous servers that help new nodes discover and connect to the network. The polygon-edge command allows you to specify one or more bootnodes while creating the genesis file. Bootnodes are defined using libp2p multiaddrs, which contain information about the protocol, network address, and node port number.

```bash
--bootnode /ip4/127.0.0.1/tcp/30301/p2p/16Uiu2HAmJxxH1tScDX2rLGSU9exnuvZKNM9SoK3v315azp68DLPW \
--bootnode /ip4/127.0.0.1/tcp/30302/p2p/16Uiu2HAmS9Nq4QAaEiogE4ieJFUYsoH28magT7wSvJPpfUGBj3Hq \
```

## Gossipsub

Gossipsub is a decentralized, peer-to-peer messaging protocol used in Polygon Edge to broadcast messages efficiently across the network. It's used in various network components, including the TxPool, where it's used to broadcast new transactions and relay transaction data between nodes. Gossipsub allows for efficient and reliable message propagation while minimizing the bandwidth requirements of the network.

To learn more about libp2p networking, check out the [<ins>official libp2p documentation</ins>](https://docs.libp2p.io/).
