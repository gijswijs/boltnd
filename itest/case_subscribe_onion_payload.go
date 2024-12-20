package itest

import (
	"context"
	"sync"

	"github.com/gijswijs/boltnd/offersrpc"
	"github.com/gijswijs/boltnd/testutils"
	"github.com/lightningnetwork/lnd/lntest"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// SubscribeOnionPayload tests subscriptions to specific tlv fields in our
// onion payload.
func SubscribeOnionPayload(ht *lntest.HarnessTest) {
	offersTest := setupForBolt12(ht)
	defer offersTest.cleanup()

	var (
		ctxb = context.Background()
		wg   sync.WaitGroup
	)

	// Connect Alice and Bob so that they can exchange onion messages.
	ht.ConnectNodes(ht.Alice, ht.Bob)

	// Create ctx with cancelation (but no timeout) to use for
	// subscriptions.
	ctxc, cancel := context.WithCancel(ctxb)
	defer func() {
		cancel()
		wg.Wait()
	}()

	// We don't want our test to block if we don't receive, so we buffer
	// channels and spin up a goroutine to wait for Bob's message
	// subscription.
	var (
		errChan = make(chan error, 1)
		msgChan = make(
			chan *offersrpc.SubscribeOnionPayloadResponse, 1,
		)
	)

	// Setup a closure that can be used to consume messages async.
	consumeMessage := consumeOnionMessage(&wg, msgChan, errChan)

	// Setup a closure that will read our received message or error if
	// nothing is received by a timeout.
	receiveMessage := readOnionMessage(msgChan, errChan)

	// First, start with a request that is not in the correct range.
	sub1Req := &offersrpc.SubscribeOnionPayloadRequest{
		TlvType: 2,
	}

	// We expect failed subscriptions to exit quickly, so we use a timeout
	// so that our receives won't block indefinitely.
	failedClient, err := offersTest.bobOffers.SubscribeOnionPayload(
		ctxc, sub1Req,
	)
	require.NoError(ht.T, err, "bad tlv subscription")

	// We expect to immediately receive an error from our subscription,
	// because we've requested a bad tlv value.
	consumeMessage(failedClient)
	_, err = receiveMessage()

	// Assert that we get an invalid argument error when we try to register
	// outside of the allowed range.
	require.NotNil(ht.T, err, "bad tlv error")
	errStat, ok := status.FromError(err)
	require.True(ht.T, ok, "expect coded error: %v", err)
	require.Equal(ht.T, codes.InvalidArgument, errStat.Code())

	// Update to a value inside of our range, and assert that we can
	// subscribe.
	sub1Req.TlvType = 101

	client1, err := offersTest.bobOffers.SubscribeOnionPayload(
		ctxc, sub1Req,
	)
	require.NoError(ht.T, err, "subscribe type=101")

	// First, send an onion message from Alice to Bob that *does not*
	// include the type that we're subscribed to.
	ctxt, cancel := context.WithTimeout(ctxb, defaultTimeout)
	defer cancel()
	req := &offersrpc.SendOnionMessageRequest{
		Pubkey: ht.Bob.PubKey[:],
		FinalPayloads: map[uint64][]byte{
			103: []byte{1, 2, 3},
		},
		DirectConnect: true,
	}

	_, err = offersTest.aliceOffers.SendOnionMessage(ctxt, req)
	require.NoError(ht.T, err, "payload 103")

	// Next, we send a message that does use the type that we're subscribed
	// to.
	payload1 := []byte{9, 8, 7}
	req.FinalPayloads[sub1Req.TlvType] = payload1

	// We'll also include a reply path to test coverage of reply paths in
	// subscriptions.
	pubkeys := testutils.GetPubkeys(ht.T, 3)

	req.ReplyPath = &offersrpc.BlindedPath{
		IntroductionNode: pubkeys[0].SerializeCompressed(),
		BlindingPoint:    pubkeys[1].SerializeCompressed(),
		Hops: []*offersrpc.BlindedHop{
			{
				BlindedNodeId: pubkeys[2].SerializeCompressed(),
				EncryptedData: []byte{1, 2, 3},
			},
		},
	}

	_, err = offersTest.aliceOffers.SendOnionMessage(ctxt, req)
	require.NoError(ht.T, err, "send payload")

	consumeMessage(client1)
	payloadReceived, err := receiveMessage()
	require.NoError(ht.T, err)

	require.Equal(ht.T, payload1, payloadReceived.Value)
	assertBlindedPathEqual(ht.T, req.ReplyPath, payloadReceived.ReplyPath)

	// Finally, we test a case where a single onion message contains two
	// payloads, both belonging to our subscriptions.
	sub2Req := &offersrpc.SubscribeOnionPayloadRequest{
		TlvType: 105,
	}

	client2, err := offersTest.bobOffers.SubscribeOnionPayload(
		ctxc, sub2Req,
	)
	require.NoError(ht.T, err, "subscribe type=105")

	// Send a message from Alice to Bob that has both subscribed payloads
	// in it.
	payload2 := []byte{6, 5, 4}
	req.FinalPayloads = map[uint64][]byte{
		sub1Req.TlvType: payload1,
		sub2Req.TlvType: payload2,
	}

	_, err = offersTest.aliceOffers.SendOnionMessage(ctxt, req)
	require.NoError(ht.T, err, "send 2 payloads")

	// Assert that both clients received the correct payload.
	consumeMessage(client1)
	payloadReceived, err = receiveMessage()
	require.NoError(ht.T, err)
	require.Equal(ht.T, payload1, payloadReceived.Value)
	assertBlindedPathEqual(ht.T, req.ReplyPath, payloadReceived.ReplyPath)

	consumeMessage(client2)
	payloadReceived, err = receiveMessage()
	require.NoError(ht.T, err)
	require.Equal(ht.T, payload2, payloadReceived.Value)
	assertBlindedPathEqual(ht.T, req.ReplyPath, payloadReceived.ReplyPath)
}
