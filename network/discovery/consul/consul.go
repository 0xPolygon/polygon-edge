package consul

import (
	"context"
	"fmt"
	"log"

	"github.com/mitchellh/mapstructure"

	consul "github.com/hashicorp/consul/api"
	"github.com/umbracle/minimal/network/discovery"
)

type config struct {
	Endpoint    string
	NodeName    string
	ServiceName string
}

// Backend is the consul discovery backend
type Backend struct {
	logger  *log.Logger
	config  config
	client  *consul.Client
	eventCh chan string
}

func (b *Backend) Close() error {
	return nil
}

func (b *Backend) Deliver() chan string {
	return b.eventCh
}

func (b *Backend) Schedule() {
	service := &consul.AgentServiceRegistration{
		ID:   b.config.NodeName,
		Name: b.config.ServiceName,
		Tags: []string{"minimal"},
	}

	if err := b.client.Agent().ServiceRegister(service); err != nil {
		b.logger.Printf("Failed to register service: %v\n", err)
	}

	// TODO, start listening
}

func Factory(ctx context.Context, conf *discovery.BackendConfig) (discovery.Backend, error) {

	var c config
	if err := mapstructure.Decode(conf.Config, &c); err != nil {
		return nil, err
	}

	if c.Endpoint == "" {
		c.Endpoint = "127.0.0.1:8500"
	}
	if c.NodeName == "" {
		c.NodeName = "minimal"
	}
	if c.ServiceName == "" {
		c.ServiceName = "minimal"
	}

	consulConfig := consul.DefaultConfig()
	consulConfig.Address = c.Endpoint

	client, err := consul.NewClient(consulConfig)
	if err != nil {
		return nil, fmt.Errorf("Failed to setup consul: %v", err)
	}

	b := &Backend{
		logger:  conf.Logger,
		config:  c,
		client:  client,
		eventCh: make(chan string, 10),
	}

	return b, nil
}
