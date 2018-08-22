package member

import (
	"sync"

	"github.com/hyperledger/fabric/msp"
	"github.com/pkg/errors"
	"github.com/s7techlab/hlf-sdk-go/api"
	"github.com/s7techlab/hlf-sdk-go/api/config"
	"github.com/s7techlab/hlf-sdk-go/crypto"
	"github.com/s7techlab/hlf-sdk-go/discovery"
	"github.com/s7techlab/hlf-sdk-go/member/chaincode/system"
	"github.com/s7techlab/hlf-sdk-go/member/channel"
	"github.com/s7techlab/hlf-sdk-go/orderer"
	"github.com/s7techlab/hlf-sdk-go/peer"
	"github.com/s7techlab/hlf-sdk-go/peer/deliver"
)

type Core struct {
	mspId             string
	identity          msp.SigningIdentity
	localPeer         api.Peer
	localPeerDeliver  api.DeliverClient
	orderer           api.Orderer
	discoveryProvider api.DiscoveryProvider
	channels          map[string]api.Channel
	channelMx         sync.Mutex
	cs                api.CryptoSuite
	options           *coreOptions
}

func (c *Core) System() api.SystemCC {
	return system.NewSCC(c.localPeer, c.identity)
}

func (c *Core) CurrentIdentity() msp.SigningIdentity {
	return c.identity
}

func (c *Core) CryptoSuite() api.CryptoSuite {
	return c.cs
}

func (c *Core) Channel(name string) api.Channel {
	c.channelMx.Lock()
	defer c.channelMx.Unlock()
	if ch, ok := c.channels[name]; ok {
		return ch
	} else {
		ch = channel.NewCore(name, c.localPeer, c.orderer, c.discoveryProvider, c.identity, c.localPeerDeliver)
		c.channels[name] = ch
		return ch
	}
}

func NewCore(mspId string, configPath string, identity api.Identity, opts ...CoreOpt) (*Core, error) {
	conf, err := config.NewYamlConfig(configPath)
	if err != nil {
		return nil, errors.Wrap(err, `failed to initialize config`)
	}

	core := Core{
		mspId:    mspId,
		channels: make(map[string]api.Channel),
		options:  new(coreOptions),
	}

	for _, option := range opts {
		if err = option(core.options); err != nil {
			return nil, errors.Wrap(err, `failed to apply option`)
		}
	}

	if dp, err := discovery.GetProvider(conf.Discovery.Type); err != nil {
		return nil, errors.Wrap(err, `failed to get discovery provider`)
	} else if core.discoveryProvider, err = dp.Initialize(conf.Discovery.Options); err != nil {
		return nil, errors.Wrap(err, `failed to initialize discovery provider`)
	}

	if core.cs, err = crypto.GetSuite(conf.Crypto.Type, conf.Crypto.Options); err != nil {
		return nil, errors.Wrap(err, `failed to initialize crypto suite`)
	}

	core.identity = identity.GetSigningIdentity(core.cs)

	if core.options.peer == nil {
		if core.localPeer, err = peer.New(conf.LocalPeer); err != nil {
			return nil, errors.Wrap(err, `failed to initialize local peer`)
		}
	} else {
		core.localPeer = core.options.peer
		core.localPeerDeliver = deliver.NewFromGRPC(core.localPeer.Conn(), core.identity)
	}

	if core.options.orderer == nil {
		if core.orderer, err = orderer.New(conf.Orderer); err != nil {
			return nil, errors.Wrap(err, `failed to initialize orderer`)
		}
	} else {
		core.orderer = core.options.orderer
	}

	if core.localPeerDeliver == nil {
		if core.localPeerDeliver, err = deliver.NewDeliverClient(conf.LocalPeer, core.identity); err != nil {
			return nil, errors.Wrap(err, `failed to initialize event hub`)
		}
	}

	return &core, nil
}
