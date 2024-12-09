package itest

import (
	"context"
	"sync"

	"github.com/gijswijs/boltnd/offersrpc"
	"github.com/lightningnetwork/lnd/lntest"
	"github.com/stretchr/testify/require"
)

// OnionMsgForwardTestCase tests forwarding of onion messages.
func OnionMsgForwardTestCase(ht *lntest.HarnessTest) {
	offersTest := setupForBolt12(ht)
	defer offersTest.cleanup()

	// Spin up a third node immediately because we will need a three-hop
	// network for this test.
	carol := ht.NewNode("carol", []string{onionMsgProtocolOverride})
	carolB12, cleanup := bolt12Client(ht.T, carol)
	defer cleanup()

	// Connect nodes before channel opening so that they can share gossip.
	ht.ConnectNodesPerm(ht.Alice, ht.Bob)
	ht.ConnectNodesPerm(ht.Bob, carol)

	// Open channels: Alice --- Bob --- Carol and wait for each node to
	// sync the network graph.
																									AliceBobChanPoint := openChannelAndAnnounce(ht, ht.Alice, ht.Bob, carol)
	BobCarolChanPoint := openChannelAndAnnounce(ht, ht.Bob, carol, ht.Alice)

	var (
		ctxb = context.Background()
		wg   sync.WaitGroup
	)

	// Create a context with no timeout that will cancel at the end of our
	// test and wait for any goroutines that have been spun up.
	ctxc, cancel := context.WithCancel(ctxb)
	defer func() {
		cancel()
		wg.Wait()
	}()

	// Setup a subscription to a specific TLV payload.
	var tlvType uint64 = 101
	subReq := &offersrpc.SubscribeOnionPayloadRequest{
		TlvType: tlvType,
	}

	// We expect failed subscriptions to exit quickly, so we use a timeout
	// so that our receives won't block indefinitely.
	client, err := carolB12.SubscribeOnionPayload(
		ctxc, subReq,
	)
	require.NoError(ht.T, err)

	// Setup closures to receive from Carol's subscription.
	var (
		errChan = make(chan error, 1)
		msgChan = make(
			chan *offersrpc.SubscribeOnionPayloadResponse, 1,
		)
	)
	consumeMessage := consumeOnionMessage(&wg, msgChan, errChan)
	receiveMessage := readOnionMessage(msgChan, errChan)

	// Now send an onion message from Alice to Carol without using direct
	// connect. This should prompt Alice to send a multi-hop onion message,
	// which is forwarded by Bob and received by Carol.
	tlvPayload := []byte{1, 2, 3}

	ctxt, cancel := context.WithTimeout(ctxc, defaultTimeout)
	req := &offersrpc.SendOnionMessageRequest{
		Pubkey: carol.PubKey[:],
		FinalPayloads: map[uint64][]byte{
			tlvType: tlvPayload,
		},
	}

	_, err = offersTest.aliceOffers.SendOnionMessage(ctxt, req)
	require.NoError(ht.T, err, "alice -> carol message")
	cancel()

	// Read the message from our subscription and assert that we have the
	// payload we expect.
	consumeMessage(client)
	onionMsg, err := receiveMessage()
	require.NoError(ht.T, err)
	require.Equal(ht.T, tlvPayload, onionMsg.Value)

	ht.CloseChannel(ht.Alice, AliceBobChanPoint)
	ht.CloseChannel(ht.Bob, BobCarolChanPoint)
}
