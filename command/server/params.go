package server

import (
	"errors"
	"fmt"
	"github.com/0xPolygon/polygon-edge/chain"
	"github.com/0xPolygon/polygon-edge/command/helper"
	"github.com/0xPolygon/polygon-edge/network"
	"github.com/0xPolygon/polygon-edge/secrets"
	"github.com/0xPolygon/polygon-edge/server"
	"github.com/0xPolygon/polygon-edge/types"
	"github.com/hashicorp/go-hclog"
	"github.com/multiformats/go-multiaddr"
	"net"
)

const (
	configFlag            = "config"
	genesisPathFlag       = "chain"
	dataDirFlag           = "data-dir"
	libp2pAddressFlag     = "libp2p"
	prometheusAddressFlag = "prometheus"
	natFlag               = "nat"
	dnsFlag               = "dns"
	sealFlag              = "seal"
	maxPeersFlag          = "max-peers"
	maxInboundPeersFlag   = "max-inbound-peers"
	maxOutboundPeersFlag  = "max-outbound-peers"
	priceLimitFlag        = "price-limit"
	maxSlotsFlag          = "max-slots"
	blockGasTargetFlag    = "block-gas-target"
	secretsConfigFlag     = "secrets-config"
	restoreFlag           = "restore"
	blockTimeFlag         = "block-time"
)

const (
	defaultPeersValue = -1
)

var (
	params = &serverParams{
		rawConfig: &Config{
			Telemetry: &Telemetry{},
			Network:   &Network{},
			TxPool:    &TxPool{},
		},
	}
)

var (
	errInvalidPeerParams = errors.New("both max-peers and max-inbound/outbound flags are set")
	errInvalidNATAddress = errors.New("could not parse NAT IP address")
)

type serverParams struct {
	rawConfig  *Config
	configPath string

	libp2pAddress     *net.TCPAddr
	prometheusAddress *net.TCPAddr
	natAddress        net.IP
	dnsAddress        multiaddr.Multiaddr
	grpcAddress       *net.TCPAddr
	jsonRPCAddress    *net.TCPAddr

	blockGasTarget uint64

	genesisConfig *chain.Chain
	secretsConfig *secrets.SecretsManagerConfig
}

func (p *serverParams) isMaxPeersSet() bool {
	return p.rawConfig.Network.MaxPeers != defaultPeersValue
}

func (p *serverParams) isPeerRangeSet() bool {
	return p.rawConfig.Network.MaxInboundPeers != defaultPeersValue ||
		p.rawConfig.Network.MaxOutboundPeers != defaultPeersValue
}

func (p *serverParams) validateFlags() error {
	// Validate the max peers configuration
	// TODO refactor
	if p.isMaxPeersSet() && p.isPeerRangeSet() {
		return errInvalidPeerParams
	}

	return nil
}

func (p *serverParams) initBlockGasTarget() error {
	var parseErr error

	if p.blockGasTarget, parseErr = types.ParseUint64orHex(
		&p.rawConfig.BlockGasTarget,
	); parseErr != nil {
		return parseErr
	}

	return nil
}

func (p *serverParams) isSecretsConfigPathSet() bool {
	return p.rawConfig.SecretsConfigPath == ""
}

func (p *serverParams) initSecretsConfig() error {
	if !p.isSecretsConfigPathSet() {
		return nil
	}

	var parseErr error

	if p.secretsConfig, parseErr = secrets.ReadConfig(
		p.rawConfig.SecretsConfigPath,
	); parseErr != nil {
		return fmt.Errorf("unable to read config file, %w", parseErr)
	}

	return nil
}

func (p *serverParams) initGenesisConfig() error {
	var parseErr error

	if p.genesisConfig, parseErr = chain.Import(
		p.rawConfig.GenesisPath,
	); parseErr != nil {
		return parseErr
	}

	return nil
}

func (p *serverParams) isPrometheusAddressSet() bool {
	return p.rawConfig.Telemetry.PrometheusAddr != ""
}

func (p *serverParams) initPrometheusAddress() error {
	if !p.isPrometheusAddressSet() {
		return nil
	}

	var parseErr error

	if p.prometheusAddress, parseErr = helper.ResolveAddr(
		p.rawConfig.Telemetry.PrometheusAddr,
	); parseErr != nil {
		return parseErr
	}

	return nil
}

func (p *serverParams) initLibp2pAddress() error {
	var parseErr error

	if p.libp2pAddress, parseErr = helper.ResolveAddr(
		p.rawConfig.Network.Libp2pAddr,
	); parseErr != nil {
		return parseErr
	}

	return nil
}

func (p *serverParams) isNATAddressSet() bool {
	return p.rawConfig.Network.NatAddr != ""
}

func (p *serverParams) initNATAddress() error {
	if !p.isNATAddressSet() {
		return nil
	}

	if p.natAddress = net.ParseIP(
		p.rawConfig.Network.NatAddr,
	); p.natAddress == nil {
		return errInvalidNATAddress
	}

	return nil
}

func (p *serverParams) isDNSAddressSet() bool {
	return p.rawConfig.Network.DNSAddr != ""
}

func (p *serverParams) initDNSAddress() error {
	if !p.isDNSAddressSet() {
		return nil
	}

	var parseErr error

	if p.dnsAddress, parseErr = helper.MultiAddrFromDNS(
		p.rawConfig.Network.DNSAddr, p.libp2pAddress.Port,
	); parseErr != nil {
		return parseErr
	}

	return nil
}

func (p *serverParams) initJSONRPCAddress() error {
	var parseErr error

	if p.jsonRPCAddress, parseErr = helper.ResolveAddr(
		p.rawConfig.JSONRPCAddr,
	); parseErr != nil {
		return parseErr
	}

	return nil
}

func (p *serverParams) initGRPCAddress() error {
	var parseErr error

	if p.grpcAddress, parseErr = helper.ResolveAddr(
		p.rawConfig.GRPCAddr,
	); parseErr != nil {
		return parseErr
	}

	return nil
}

func (p *serverParams) setRawGRPCAddress(grpcAddress string) {
	p.rawConfig.GRPCAddr = grpcAddress
}

func (p *serverParams) setRawJSONRPCAddress(jsonRPCAddress string) {
	p.rawConfig.JSONRPCAddr = jsonRPCAddress
}

func (p *serverParams) initAddresses() error {
	if err := p.initPrometheusAddress(); err != nil {
		return err
	}

	if err := p.initLibp2pAddress(); err != nil {
		return err
	}

	if err := p.initNATAddress(); err != nil {
		return err
	}

	if err := p.initDNSAddress(); err != nil {
		return err
	}

	if err := p.initJSONRPCAddress(); err != nil {
		return err
	}

	return p.initGRPCAddress()
}

func (p *serverParams) initRawParams() error {
	if err := p.initBlockGasTarget(); err != nil {
		return err
	}

	if err := p.initSecretsConfig(); err != nil {
		return err
	}

	if err := p.initGenesisConfig(); err != nil {
		return err
	}

	return p.initAddresses()
}

func (p *serverParams) initConfigFromFile() error {
	var parseErr error

	if p.rawConfig, parseErr = readConfigFile(p.configPath); parseErr != nil {
		return parseErr
	}

	return nil
}

func (p *serverParams) generateConfig() *server.Config {
	return &server.Config{
		Chain:       p.genesisConfig,
		JSONRPCAddr: p.jsonRPCAddress,
		GRPCAddr:    p.grpcAddress,
		LibP2PAddr:  p.libp2pAddress,
		Telemetry: &server.Telemetry{
			PrometheusAddr: p.prometheusAddress,
		},
		Network: &network.Config{
			NoDiscover:       p.rawConfig.Network.NoDiscover,
			Addr:             p.libp2pAddress,
			NatAddr:          p.natAddress,
			DNS:              p.dnsAddress,
			DataDir:          p.rawConfig.DataDir,
			MaxPeers:         p.rawConfig.Network.MaxPeers,
			MaxInboundPeers:  p.rawConfig.Network.MaxInboundPeers,
			MaxOutboundPeers: p.rawConfig.Network.MaxOutboundPeers,
			Chain:            p.genesisConfig,
		},
		DataDir:        p.rawConfig.DataDir,
		Seal:           p.rawConfig.ShouldSeal,
		PriceLimit:     p.rawConfig.TxPool.PriceLimit,
		MaxSlots:       p.rawConfig.TxPool.MaxSlots,
		SecretsManager: p.secretsConfig,
		RestoreFile:    &p.rawConfig.RestoreFile,
		BlockTime:      p.rawConfig.BlockTime,
		LogLevel:       hclog.LevelFromString(p.rawConfig.LogLevel),
	}
}
