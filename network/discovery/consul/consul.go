package consul

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/mitchellh/mapstructure"
	"github.com/umbracle/minimal/helper/enode"

	consul "github.com/hashicorp/consul/api"
	"github.com/umbracle/minimal/network/discovery"
)

type config struct {
	Address     string `mapstructure:"endpoint"`
	NodeName    string `mapstructure:"node_name"`
	ServiceName string `mapstructure:"service_name"`
}

// Backend is the consul discovery backend
type Backend struct {
	logger  *log.Logger
	config  config
	client  *consul.Client
	eventCh chan string
	address *net.TCPAddr
	key     *ecdsa.PrivateKey
	enode   *enode.Enode
}

func (b *Backend) Close() error {
	return nil
}

func (b *Backend) Deliver() chan string {
	return b.eventCh
}

func (b *Backend) Schedule() {
	addr := b.address.IP.String()
	port := b.address.Port

	service := &consul.AgentServiceRegistration{
		ID:      b.config.NodeName,
		Name:    b.config.ServiceName,
		Tags:    []string{"minimal"},
		Address: addr,
		Port:    port,
		Meta: map[string]string{
			"enode": b.enode.String(),
		},
	}

	if err := b.client.Agent().ServiceRegister(service); err != nil {
		b.logger.Printf("Failed to register service: %v\n", err)
	}

	go func() {
		for {
			if err := b.findNodes(); err != nil {
				b.logger.Printf("Failed to find nodes: %v", err)
			}
			time.Sleep(10 * time.Second)
		}
	}()
}

func (b *Backend) findNodes() error {
	catalog := b.client.Catalog()

	dcs, err := catalog.Datacenters()
	if err != nil {
		return fmt.Errorf("failed to get the datacenters: %v", err)
	}

	serverServices := []string{}
	for _, dc := range dcs {
		opts := &consul.QueryOptions{
			Datacenter: dc,
		}

		services, _, err := catalog.Service(b.config.ServiceName, "minimal", opts)
		if err != nil {
			return fmt.Errorf("failed to get the services: %v", err)
		}

		for _, service := range services {
			addr := service.ServiceAddress
			port := service.ServicePort

			if addr == "" {
				addr = service.Address
			}
			if b.address.IP.String() == addr && int(b.address.Port) == port {
				continue
			}

			if enode, ok := service.ServiceMeta["enode"]; ok {
				serverServices = append(serverServices, enode)
			}
		}
	}

	for _, s := range serverServices {
		select {
		case b.eventCh <- s:
		default:
		}
	}
	return nil
}

func Factory(ctx context.Context, conf *discovery.BackendConfig) (discovery.Backend, error) {
	var c config
	if err := mapstructure.Decode(conf.Config, &c); err != nil {
		return nil, err
	}

	if c.Address == "" {
		c.Address = "127.0.0.1:8500"
	}
	if c.NodeName == "" {
		c.NodeName = "minimal"
	}
	if c.ServiceName == "" {
		c.ServiceName = "minimal"
	}

	consulConfig := consul.DefaultConfig()
	consulConfig.Address = c.Address

	client, err := consul.NewClient(consulConfig)
	if err != nil {
		return nil, fmt.Errorf("Failed to setup consul: %v", err)
	}

	tcpAddress := &net.TCPAddr{
		IP:   conf.Enode.IP,
		Port: int(conf.Enode.TCP),
	}

	b := &Backend{
		logger:  conf.Logger,
		config:  c,
		client:  client,
		eventCh: make(chan string, 10),
		address: tcpAddress,
		key:     conf.Key,
		enode:   conf.Enode,
	}

	return b, nil
}
