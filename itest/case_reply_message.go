package itest

import (
	"context"
	"sync"

	"github.com/gijswijs/boltnd/offersrpc"
	"github.com/lightningnetwork/lnd/lntest"
	"github.com/stretchr/testify/require"
)

// ReplyMessageTestCase tests sending of onion messages to reply paths.
func ReplyMessageTestCase(ht *lntest.HarnessTest) {
	offersTest := setupForBolt12(ht)
	defer offersTest.cleanup()

	ctxb := context.Background()

	// Setup our network with the following topology:
	// Alice -- Bob -- Carol -- Dave
	carol := ht.NewNode("carol", []string{onionMsgProtocolOverride})
	dave := ht.NewNode("dave", []string{onionMsgProtocolOverride})

	// We'll also need a bolt 12 client for dave, because he's going to be
	// receiving our onion messages.
	daveB12, cleanup := bolt12Client(ht.T, dave)
	defer cleanup()

	// First we make p2p connections so that all the nodes can gossip
	// channel information with each other, then we setup the channels
	// themselves.
	ht.ConnectNodesPerm(ht.Alice, ht.Bob)
	ht.ConnectNodesPerm(ht.Bob, carol)
	ht.ConnectNodesPerm(carol, dave)

	// Alice -> Bob
	AliceBobChanPoint := openChannelAndAnnounce(ht, ht.Alice, ht.Bob, carol, dave)

	// Bob -> Carol
	BobCarolChanPoint := openChannelAndAnnounce(ht, ht.Bob, carol, ht.Alice, dave)

	// Carol -> Dave
	fundNode(ctxb, ht, carol)
	CarolDaveChanPoint := openChannelAndAnnounce(ht, carol, dave, ht.Alice, ht.Bob)

	// Create a reply path to Dave's node.
	ctxt, cancel := context.WithTimeout(ctxb, defaultTimeout)
	replyPath, err := daveB12.GenerateBlindedRoute(
		ctxt, &offersrpc.GenerateBlindedRouteRequest{},
	)
	cancel()
	require.NoError(ht.T, err, "reply path")

	// Now subscribe to onion payloads received by dave. We don't add a
	// timeout on this subscription, but rather just cancel it at the end
	// of the test.
	ctxc, cancelSub := context.WithCancel(ctxb)
	defer cancelSub()

	subReq := &offersrpc.SubscribeOnionPayloadRequest{
		TlvType: 101,
	}
	client, err := daveB12.SubscribeOnionPayload(ctxc, subReq)
	require.NoError(ht.T, err, "subscription")

	var (
		errChan = make(chan error, 1)
		msgChan = make(
			chan *offersrpc.SubscribeOnionPayloadResponse, 1,
		)

		wg sync.WaitGroup
	)
	defer wg.Wait()

	// Setup a closure that can be used to consume messages async and one
	// that will read our received messages.
	consumeMessage := consumeOnionMessage(&wg, msgChan, errChan)
	receiveMessage := readOnionMessage(msgChan, errChan)

	// Send an onion message from Alice to Dave's reply path.
	ctxt, cancel = context.WithTimeout(ctxb, defaultTimeout)
	data := []byte{9, 8, 7}
	req := &offersrpc.SendOnionMessageRequest{
		BlindedDestination: replyPath.Route,
		FinalPayloads: map[uint64][]byte{
			subReq.TlvType: data,
		},
	}

	_, err = offersTest.aliceOffers.SendOnionMessage(ctxt, req)
	require.NoError(ht.T, err)
	cancel()

	// Read and receive the message from Dave's subscription and assert
	// that we get the payload we expect.
	consumeMessage(client)
	msg, err := receiveMessage()
	require.NoError(ht.T, err, "receive message")
	require.Equal(ht.T, data, msg.Value)

	ht.CloseChannel(ht.Alice, AliceBobChanPoint)
	ht.CloseChannel(ht.Bob, BobCarolChanPoint)
	ht.CloseChannel(carol, CarolDaveChanPoint)
}
