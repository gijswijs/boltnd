package itest

import (
	"testing"
	"time"

	"github.com/gijswijs/boltnd/offersrpc"
	"github.com/lightningnetwork/lnd/lntest"
	"github.com/lightningnetwork/lnd/lntest/node"
	"github.com/stretchr/testify/require"
)

const (
	onionMsgProtocolOverride = "--protocol.custom-message=513"
	defaultTimeout           = time.Second * 30
)

type bolt12TestSetup struct {
	aliceOffers offersrpc.OffersClient
	bobOffers   offersrpc.OffersClient
	cleanup     func()
}

// setupForBolt12 restarts our nodes with the appropriate overrides required to
// use bolt 12 functionality and returns connections to each node's offers
// subserver.
func setupForBolt12(ht *lntest.HarnessTest) *bolt12TestSetup {
	// Update both nodes extra args to allow external handling of onion
	// messages and restart them so that the args some into effect.
	ht.RestartNodeWithExtraArgs(ht.Alice, []string{
		onionMsgProtocolOverride,
	})

	ht.RestartNodeWithExtraArgs(ht.Bob, []string{
		onionMsgProtocolOverride,
	})

	// Next, connect to each node's offers subserver.
	aliceClient, aliceClean := bolt12Client(ht.T, ht.Alice)
	bobClient, bobClean := bolt12Client(ht.T, ht.Bob)

	return &bolt12TestSetup{
		aliceOffers: aliceClient,
		bobOffers:   bobClient,
		cleanup: func() {
			aliceClean()
			bobClean()
		},
	}
}

// bolt12Client returns an offersrpc client and cleanup for a node.
func bolt12Client(t *testing.T, node *node.HarnessNode) (
	offersrpc.OffersClient, func()) {

	conn, err := node.ConnectRPC()
	require.NoError(t, err, "%v grpc conn", node.Name())

	return offersrpc.NewOffersClient(conn), func() {
		if err := conn.Close(); err != nil {
			t.Logf("%v grpc conn close %v", node.Name(), err)
		}
	}
}
