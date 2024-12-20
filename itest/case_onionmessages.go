package itest

import (
	"context"
	"sync"
	"time"

	"github.com/gijswijs/boltnd/lnwire"
	"github.com/gijswijs/boltnd/offersrpc"
	"github.com/lightningnetwork/lnd/lnrpc"
	"github.com/lightningnetwork/lnd/lntest"
	"github.com/stretchr/testify/require"
)

// OnionMessageTestCase tests the exchange of onion messages.
func OnionMessageTestCase(ht *lntest.HarnessTest) {
	offersTest := setupForBolt12(ht)
	defer offersTest.cleanup()

	var (
		ctxb = context.Background()
		wg   sync.WaitGroup
	)

	// Start with Alice and Bob connected. Alice won't be able to look up
	// Bob in the graph because he doesn't have any channels (and she has
	// no peers for gossip), so we first test the easy case where they're
	// already connected.
	ht.ConnectNodes(ht.Alice, ht.Bob)

	// Subscribe to custom messages that bob receives.
	bobMsg, cancel := ht.Bob.RPC.SubscribeCustomMessages()
	defer func() {
		cancel()
		wg.Wait()
	}()

	// Send an onion message from alice to bob, using our default timeout
	// to ensure that sending does not hang.
	ctxt, cancel := context.WithTimeout(ctxb, defaultTimeout)
	req := &offersrpc.SendOnionMessageRequest{
		Pubkey:        ht.Bob.PubKey[:],
		DirectConnect: true,
	}
	_, err := offersTest.aliceOffers.SendOnionMessage(ctxt, req)
	require.NoError(ht.T, err, "send onion message")
	cancel()

	// We don't want our test to block if we don't receive, so we buffer
	// channels and spin up a goroutine to wait for Bob's message
	// subscription.
	var (
		errChan = make(chan error, 1)
		msgChan = make(chan *lnrpc.CustomMessage, 1)
	)

	// Setup a closure that can be used to receive messages async.
	receiveMessage := func() {
		wg.Add(1)
		go func() {
			defer wg.Done()

			msg, err := bobMsg.Recv()
			if err != nil {
				errChan <- err
				return
			}

			msgChan <- msg
		}()
	}

	// Setup a closure that will consume our received message or fail if
	// nothing is received by a timeout.
	readMessage := func() {
		select {
		// If we receive a message as expected, assert that it is of
		// the correct type.
		case msg := <-msgChan:
			require.Equal(ht.T, uint32(lnwire.OnionMessageType),
				msg.Type)

		// If we received an error, something went wrong.
		case err := <-errChan:
			ht.T.Fatalf("message not received: %v", err)

		// In the case of a timeout, let our test exit. This will
		// cancel the receive goroutine (through context cancelation)
		// and wait for it to exit.
		case <-time.After(defaultTimeout):
			ht.T.Fatal("message not received within timeout")
		}
	}

	// Listen for a message and wait to receive it.
	receiveMessage()
	readMessage()

	// Now, we will spin up a new node, carol to test sending messages to
	// peers that we are not currently connected to.
	carol := ht.NewNode("carol", []string{onionMsgProtocolOverride})

	// Connect Alice and Carol so that Carol can sync the graph from Alice.
	ht.ConnectNodesPerm(ht.Alice, carol)

	// We're going to open a channel between Alice and Bob, so that they
	// become part of the public graph.
	AliceBobChanPoint := openChannelAndAnnounce(ht, ht.Alice, ht.Bob, carol)

	// We now have the following setup:
	//  Alice --- (channel) ---- Bob
	//    |
	// p2p conn
	//    |
	// Carol
	//
	// Carol should be able to send an onion message to Bob by looking
	// him up in the graph and sending to his public address.
	carolB12, cleanup := bolt12Client(ht.T, carol)
	defer cleanup()

	ctxt, cancel = context.WithTimeout(ctxb, defaultTimeout)
	req = &offersrpc.SendOnionMessageRequest{
		Pubkey:        ht.Bob.PubKey[:],
		DirectConnect: true,
	}
	_, err = carolB12.SendOnionMessage(ctxt, req)
	require.NoError(ht.T, err, "carol message")
	cancel()

	// Listen for a message from Carol -> Bob and wait to receive it.
	receiveMessage()
	readMessage()

	// Now that Alice has a channel open with Bob, she should be able to
	// send an onion message to him without using "direct connect".
	req.DirectConnect = false

	ctxt, cancel = context.WithTimeout(ctxb, defaultTimeout)
	_, err = offersTest.aliceOffers.SendOnionMessage(ctxt, req)
	require.NoError(ht.T, err, "alice -> bob no direct connect")
	cancel()

	receiveMessage()
	readMessage()

	// Now open a channel from Carol -> Alice so that we have the following
	// network structure:
	// Carol --- Alice ---- Bob
	AliceCarolChanPoint := openChannelAndAnnounce(ht, ht.Alice, carol, ht.Bob)

	// Generate a blinded path to Carol.
	ctxt, cancel = context.WithTimeout(ctxb, defaultTimeout)
	routeResp, err := carolB12.GenerateBlindedRoute(
		ctxt, &offersrpc.GenerateBlindedRouteRequest{},
	)
	require.NoError(ht.T, err, "carol blinded route")

	// Send an onion message from Carol -> Bob including a reply path
	// back to Carol.
	ctxt, cancel = context.WithTimeout(ctxb, defaultTimeout)
	req = &offersrpc.SendOnionMessageRequest{
		Pubkey:        ht.Bob.PubKey[:],
		ReplyPath:     routeResp.Route,
		DirectConnect: true,
	}

	_, err = carolB12.SendOnionMessage(ctxt, req)
	require.NoError(ht.T, err, "carol message")
	cancel()

	// Listen for a message from Carol -> Bob and wait to receive it.
	receiveMessage()
	readMessage()

	ht.CloseChannel(ht.Alice, AliceBobChanPoint)
	ht.CloseChannel(ht.Alice, AliceCarolChanPoint)
}
