package autonat

import (
	"context"
	"errors"
	"math/rand"
	"sync/atomic"
	"time"

	"github.com/libp2p/go-eventbus"
	"github.com/libp2p/go-libp2p-core/event"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"

	logging "github.com/ipfs/go-log"
	ma "github.com/multiformats/go-multiaddr"
	manet "github.com/multiformats/go-multiaddr-net"
)

var log = logging.Logger("autonat")

// AmbientAutoNAT is the implementation of ambient NAT autodiscovery
type AmbientAutoNAT struct {
	ctx  context.Context
	host host.Host

	*config

	inboundConn  chan network.Conn
	observations chan autoNATResult
	// status is an autoNATResult reflecting current status.
	status atomic.Value
	// Reflects the confidence on of the NATStatus being private, as a single
	// dialback may fail for reasons unrelated to NAT.
	// If it is <3, then multiple autoNAT peers may be contacted for dialback
	// If only a single autoNAT peer is known, then the confidence increases
	// for each failure until it reaches 3.
	confidence  int
	lastInbound time.Time
	lastProbe   time.Time

	subAddrUpdated event.Subscription
	service        *autoNATService

	emitReachabilityChanged event.Emitter
}

// StaticAutoNAT is a simple AutoNAT implementation when a single NAT status is desired.
type StaticAutoNAT struct {
	ctx          context.Context
	host         host.Host
	reachability network.Reachability
	service      *autoNATService
}

type autoNATResult struct {
	network.Reachability
	address ma.Multiaddr
}

// New creates a new NAT autodiscovery system attached to a host
func New(ctx context.Context, h host.Host, options ...Option) (AutoNAT, error) {
	var err error
	conf := new(config)
	conf.host = h
	conf.dialPolicy.host = h

	if err = defaults(conf); err != nil {
		return nil, err
	}
	if conf.addressFunc == nil {
		conf.addressFunc = h.Addrs
	}

	for _, o := range options {
		if err = o(conf); err != nil {
			return nil, err
		}
	}
	emitReachabilityChanged, _ := h.EventBus().Emitter(new(event.EvtLocalReachabilityChanged), eventbus.Stateful)

	var service *autoNATService
	if (!conf.forceReachability || conf.reachability == network.ReachabilityPublic) && conf.dialer != nil {
		service, err = newAutoNATService(ctx, conf)
		if err != nil {
			return nil, err
		}
		service.Enable()
	}

	if conf.forceReachability {
		emitReachabilityChanged.Emit(event.EvtLocalReachabilityChanged{Reachability: conf.reachability})

		return &StaticAutoNAT{
			ctx:          ctx,
			host:         h,
			reachability: conf.reachability,
			service:      service,
		}, nil
	}

	subAddrUpdated, _ := h.EventBus().Subscribe(new(event.EvtLocalAddressesUpdated))

	as := &AmbientAutoNAT{
		ctx:          ctx,
		host:         h,
		config:       conf,
		inboundConn:  make(chan network.Conn, 5),
		observations: make(chan autoNATResult, 1),

		subAddrUpdated: subAddrUpdated,

		emitReachabilityChanged: emitReachabilityChanged,
		service:                 service,
	}
	as.status.Store(autoNATResult{network.ReachabilityUnknown, nil})

	h.Network().Notify(as)
	go as.background()

	return as, nil
}

// Status returns the AutoNAT observed reachability status.
func (as *AmbientAutoNAT) Status() network.Reachability {
	s := as.status.Load().(autoNATResult)
	return s.Reachability
}

func (as *AmbientAutoNAT) emitStatus() {
	status := as.status.Load().(autoNATResult)
	as.emitReachabilityChanged.Emit(event.EvtLocalReachabilityChanged{Reachability: status.Reachability})
}

// PublicAddr returns the publicly connectable Multiaddr of this node if one is known.
func (as *AmbientAutoNAT) PublicAddr() (ma.Multiaddr, error) {
	s := as.status.Load().(autoNATResult)
	if s.Reachability != network.ReachabilityPublic {
		return nil, errors.New("NAT status is not public")
	}

	return s.address, nil
}

func ipInList(candidate ma.Multiaddr, list []ma.Multiaddr) bool {
	candidateIP, _ := manet.ToIP(candidate)
	for _, i := range list {
		if ip, err := manet.ToIP(i); err == nil && ip.Equal(candidateIP) {
			return true
		}
	}
	return false
}

func (as *AmbientAutoNAT) background() {
	// wait a bit for the node to come online and establish some connections
	// before starting autodetection
	delay := as.config.bootDelay

	var lastAddrUpdated time.Time
	addrUpdatedChan := as.subAddrUpdated.Out()
	defer as.subAddrUpdated.Close()
	defer as.emitReachabilityChanged.Close()

	timer := time.NewTimer(delay)
	defer timer.Stop()
	timerRunning := true

	for {
		select {
		// new connection occured.
		case conn := <-as.inboundConn:
			localAddrs := as.host.Addrs()
			ca := as.status.Load().(autoNATResult)
			if ca.address != nil {
				localAddrs = append(localAddrs, ca.address)
			}
			if !ipInList(conn.RemoteMultiaddr(), localAddrs) {
				as.lastInbound = time.Now()
			}

		case <-addrUpdatedChan:
			if !lastAddrUpdated.Add(time.Second).After(time.Now()) {
				lastAddrUpdated = time.Now()
				if as.confidence > 1 {
					as.confidence--
				}
			}

		// probe finished.
		case result, ok := <-as.observations:
			if !ok {
				return
			}
			as.recordObservation(result)
		case <-timer.C:
			timerRunning = false
		case <-as.ctx.Done():
			return
		}

		// Drain the timer channel if it hasn't fired in preparation for Resetting it.
		if timerRunning && !timer.Stop() {
			<-timer.C
		}
		timer.Reset(as.scheduleProbe())
		timerRunning = true
	}
}

// scheduleProbe calculates when the next probe should be scheduled for,
// and launches it if that time is now.
func (as *AmbientAutoNAT) scheduleProbe() time.Duration {
	// Our baseline is a probe every 'AutoNATRefreshInterval'
	// This is modulated by:
	// * recent inbound connections make us willing to wait up to 2x longer between probes.
	// * low confidence makes us speed up between probes.
	fixedNow := time.Now()
	currentStatus := as.status.Load().(autoNATResult)

	nextProbe := fixedNow
	if !as.lastProbe.IsZero() {
		untilNext := as.config.refreshInterval
		if currentStatus.Reachability == network.ReachabilityUnknown {
			untilNext = as.config.retryInterval
		} else if as.confidence < 3 {
			untilNext = as.config.retryInterval
		} else if currentStatus.Reachability == network.ReachabilityPublic && as.lastInbound.After(as.lastProbe) {
			untilNext *= 2
		}
		nextProbe = as.lastProbe.Add(untilNext)
	}
	if fixedNow.After(nextProbe) || fixedNow == nextProbe {
		go as.probeNextPeer()
		return as.config.retryInterval
	}
	return nextProbe.Sub(fixedNow)
}

// Update the current status based on an observed result.
func (as *AmbientAutoNAT) recordObservation(observation autoNATResult) {
	currentStatus := as.status.Load().(autoNATResult)
	if observation.Reachability == network.ReachabilityPublic {
		log.Debugf("NAT status is public")
		changed := false
		if currentStatus.Reachability != network.ReachabilityPublic {
			// we are flipping our NATStatus, so confidence drops to 0
			as.confidence = 0
			if as.service != nil {
				as.service.Enable()
			}
			changed = true
		} else if as.confidence < 3 {
			as.confidence++
		}
		if observation.address != nil {
			if !changed && currentStatus.address != nil && !observation.address.Equal(currentStatus.address) {
				as.confidence--
			}
			if currentStatus.address == nil || !observation.address.Equal(currentStatus.address) {
				changed = true
			}
			as.status.Store(observation)
		}
		if observation.address != nil && changed {
			as.emitStatus()
		}
	} else if observation.Reachability == network.ReachabilityPrivate {
		log.Debugf("NAT status is private")
		if currentStatus.Reachability == network.ReachabilityPublic {
			if as.confidence > 0 {
				as.confidence--
			} else {
				// we are flipping our NATStatus, so confidence drops to 0
				as.confidence = 0
				as.status.Store(observation)
				if as.service != nil {
					as.service.Disable()
				}
				as.emitStatus()
			}
		} else if as.confidence < 3 {
			as.confidence++
			as.status.Store(observation)
			if currentStatus.Reachability != network.ReachabilityPrivate {
				as.emitStatus()
			}
		}
	} else if as.confidence > 0 {
		// don't just flip to unknown, reduce confidence first
		as.confidence--
	} else {
		log.Debugf("NAT status is unknown")
		as.status.Store(autoNATResult{network.ReachabilityUnknown, nil})
		if currentStatus.Reachability != network.ReachabilityUnknown {
			if as.service != nil {
				as.service.Enable()
			}
			as.emitStatus()
		}
	}
}

func (as *AmbientAutoNAT) probe(pi *peer.AddrInfo) {
	cli := NewAutoNATClient(as.host, as.config.addressFunc)
	ctx, cancel := context.WithTimeout(as.ctx, as.config.requestTimeout)
	defer cancel()

	a, err := cli.DialBack(ctx, pi.ID)

	switch {
	case err == nil:
		log.Debugf("Dialback through %s successful; public address is %s", pi.ID.Pretty(), a.String())
		as.observations <- autoNATResult{network.ReachabilityPublic, a}
	case IsDialError(err):
		log.Debugf("Dialback through %s failed", pi.ID.Pretty())
		as.observations <- autoNATResult{network.ReachabilityPrivate, nil}
	default:
		as.observations <- autoNATResult{network.ReachabilityUnknown, nil}
	}
}

func (as *AmbientAutoNAT) probeNextPeer() {
	peers := as.host.Network().Peers()
	if len(peers) == 0 {
		return
	}

	addrs := make([]peer.AddrInfo, 0, len(peers))

	for _, p := range peers {
		info := as.host.Peerstore().PeerInfo(p)
		// Exclude peers which don't support the autonat protocol.
		if proto, err := as.host.Peerstore().SupportsProtocols(p, AutoNATProto); len(proto) == 0 || err != nil {
			continue
		}

		if !as.config.dialPolicy.skipPeer(info.Addrs) {
			addrs = append(addrs, info)
		}
		addrs = append(addrs, info)
	}
	// TODO: track and exclude recently probed peers.

	if len(addrs) == 0 {
		return
	}

	shufflePeers(addrs)

	as.lastProbe = time.Now()
	as.probe(&addrs[0])
}

func shufflePeers(peers []peer.AddrInfo) {
	for i := range peers {
		j := rand.Intn(i + 1)
		peers[i], peers[j] = peers[j], peers[i]
	}
}

// Status returns the AutoNAT observed reachability status.
func (s *StaticAutoNAT) Status() network.Reachability {
	return s.reachability
}

// PublicAddr returns the publicly connectable Multiaddr of this node if one is known.
func (s *StaticAutoNAT) PublicAddr() (ma.Multiaddr, error) {
	if s.reachability != network.ReachabilityPublic {
		return nil, errors.New("NAT status is not public")
	}
	addrs := s.host.Addrs()
	if len(addrs) > 0 {
		return s.host.Addrs()[0], nil
	}
	return nil, errors.New("No available address")
}
