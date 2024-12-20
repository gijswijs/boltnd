package onionmsg

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/gijswijs/boltnd/lnwire"
	"github.com/gijswijs/boltnd/testutils"
	"github.com/lightninglabs/lndclient"
	sphinx "github.com/lightningnetwork/lightning-onion"
	"github.com/lightningnetwork/lnd/routing/route"
	"github.com/lightningnetwork/lnd/tlv"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type sendMessageTest struct {
	name string

	// peer is the peer to send our message to.
	peer *btcec.PublicKey

	// directConnect indicates whether we should connect to the peer to
	// send the message.
	directConnect bool

	// peerLookups is the number of times that we lookup our peer after
	// connecting.
	peerLookups int

	// expectedErr is the error we expect.
	expectedErr error

	// setMock primes our lnd mock for the specific test case.
	setMock func(*mock.Mock)
}

// TestSendMessage tests sending of onion messages using lnd's custom message
// api.
func TestSendMessage(t *testing.T) {
	pubkeys := testutils.GetPubkeys(t, 3)

	pubkey := route.NewVertex(pubkeys[0])
	node1 := route.NewVertex(pubkeys[1])
	node2 := route.NewVertex(pubkeys[2])

	var (
		peerList = []lndclient.Peer{
			{
				Pubkey: pubkey,
			},
		}
		nodeAddr = "host:port"

		privateNodeInfo = &lndclient.NodeInfo{
			Node: &lndclient.Node{},
		}

		nodeInfo = &lndclient.NodeInfo{
			Node: &lndclient.Node{
				Addresses: []string{
					nodeAddr,
				},
			},
		}

		listPeersErr = errors.New("listpeers failed")
		getNodeErr   = errors.New("get node failed")
		connectErr   = errors.New("connect failed")
	)

	tests := []sendMessageTest{
		{
			name:          "success - peer already connected",
			peer:          pubkeys[0],
			directConnect: true,
			peerLookups:   5,
			expectedErr:   nil,
			setMock: func(m *mock.Mock) {
				// We are already connected to the peer.
				testutils.MockListPeers(m, peerList, nil)

				// Send the message to the peer.
				testutils.MockSendAnyCustomMessage(m, nil)
			},
		},
		{
			name:          "failure - list peers fails",
			peer:          pubkeys[0],
			directConnect: true,
			peerLookups:   5,
			expectedErr:   listPeersErr,
			setMock: func(m *mock.Mock) {
				testutils.MockListPeers(m, nil, listPeersErr)
			},
		},
		{
			name:          "failure - peer not found in graph",
			peer:          pubkeys[0],
			directConnect: true,
			peerLookups:   5,
			expectedErr:   getNodeErr,
			setMock: func(m *mock.Mock) {
				// We have no peers at present.
				testutils.MockListPeers(m, nil, nil)

				// Fail because we can't find the peer in the
				// graph.
				testutils.MockGetNodeInfo(
					m, pubkey, false, nil, getNodeErr,
				)
			},
		},
		{
			name:          "failure - peer has no addresses",
			peer:          pubkeys[0],
			directConnect: true,
			peerLookups:   5,
			expectedErr:   ErrNoAddresses,
			setMock: func(m *mock.Mock) {
				// We have no peers at present.
				testutils.MockListPeers(m, nil, nil)

				// Peer lookup succeeds, but there are no
				// addresses listed.
				testutils.MockGetNodeInfo(
					m, pubkey, false, privateNodeInfo, nil,
				)
			},
		},

		{
			name:          "failure - could not connect to peer",
			peer:          pubkeys[0],
			directConnect: true,
			expectedErr:   connectErr,
			setMock: func(m *mock.Mock) {
				// We have no peers at present.
				testutils.MockListPeers(m, nil, nil)

				// Find the peer in the graph.
				testutils.MockGetNodeInfo(
					m, pubkey, false, nodeInfo, nil,
				)

				// Try to connect to the address provided, fail.
				testutils.MockConnect(
					m, pubkey, nodeAddr, true, connectErr,
				)
			},
		},
		{
			name:          "success - peer immediately found",
			peer:          pubkeys[0],
			directConnect: true,
			peerLookups:   5,
			expectedErr:   nil,
			setMock: func(m *mock.Mock) {
				// We have no peers at present.
				testutils.MockListPeers(m, nil, nil)

				// Find the peer in the graph.
				testutils.MockGetNodeInfo(
					m, pubkey, false, nodeInfo, nil,
				)

				// Succeed in connecting to the address
				// provided.
				testutils.MockConnect(
					m, pubkey, nodeAddr, true, nil,
				)

				// After connecting, immediately return the
				// target peer from listpeers.
				testutils.MockListPeers(m, peerList, nil)

				// Send the message to the peer.
				testutils.MockSendAnyCustomMessage(m, nil)
			},
		},
		{
			name:          "success - peer found after retry",
			peer:          pubkeys[0],
			directConnect: true,
			peerLookups:   5,
			expectedErr:   nil,
			setMock: func(m *mock.Mock) {
				// We have no peers at present.
				testutils.MockListPeers(m, nil, nil)

				// Find the peer in the graph.
				testutils.MockGetNodeInfo(
					m, pubkey, false, nodeInfo, nil,
				)

				// Succeed in connecting to the address
				// provided.
				testutils.MockConnect(
					m, pubkey, nodeAddr, true, nil,
				)

				// In our first peer lookups, don't return the
				// peer (mocking the time connection / handshake
				// takes.
				testutils.MockListPeers(m, nil, nil)
				testutils.MockListPeers(m, nil, nil)

				// On our third attempt, we're connected to the
				// peer.
				testutils.MockListPeers(m, peerList, nil)

				// Send the message to the peer.
				testutils.MockSendAnyCustomMessage(m, nil)
			},
		},
		{
			name:          "failure - peer not found after retry",
			peer:          pubkeys[0],
			directConnect: true,
			peerLookups:   2,
			expectedErr:   ErrNoConnection,
			setMock: func(m *mock.Mock) {
				// We have no peers at present.
				testutils.MockListPeers(m, nil, nil)

				// Find the peer in the graph.
				testutils.MockGetNodeInfo(
					m, pubkey, false, nodeInfo, nil,
				)

				// Succeed in connecting to the address
				// provided.
				testutils.MockConnect(
					m, pubkey, nodeAddr, true, nil,
				)

				// The peer does not show up in our peer list
				// after 2 calls.
				testutils.MockListPeers(m, nil, nil)
				testutils.MockListPeers(m, nil, nil)
			},
		},
		{
			name:          "multi-hop no path",
			peer:          pubkeys[0],
			directConnect: false,
			expectedErr:   ErrNoPath,
			setMock: func(m *mock.Mock) {
				req := queryRoutesRequest(pubkeys[0])
				resp := &lndclient.QueryRoutesResponse{}
				testutils.MockQueryRoutes(
					m, req, resp, nil,
				)
			},
		},
		{
			name:          "multi-hop finds path",
			peer:          pubkeys[0],
			directConnect: false,
			expectedErr:   nil,
			setMock: func(m *mock.Mock) {
				req := queryRoutesRequest(pubkeys[0])
				resp := &lndclient.QueryRoutesResponse{
					Hops: []*lndclient.Hop{
						{
							PubKey: &node1,
						},
						{
							PubKey: &node2,
						},
					},
				}

				testutils.MockQueryRoutes(m, req, resp, nil)

				// Send the message to the peer.
				testutils.MockSendAnyCustomMessage(m, nil)
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			testSendMessage(t, testCase)
		})
	}
}

func testSendMessage(t *testing.T, testCase sendMessageTest) {
	// Create a mock and prime it for the calls we expect in this test.
	lnd := testutils.NewMockLnd()
	defer lnd.Mock.AssertExpectations(t)

	testCase.setMock(lnd.Mock)

	privkeys := testutils.GetPrivkeys(t, 1)
	nodeKey := privkeys[0]

	// Create a simple SingleKeyECDH impl here for testing.
	nodeKeyECDH := &sphinx.PrivKeyECDH{
		PrivKey: nodeKey,
	}

	// We don't expect the messenger's shutdown function to be used, so
	// we can provide nil (knowing that our tests will panic if it's used).
	messenger := NewOnionMessenger(
		lnd, nodeKeyECDH, nil,
	)

	// Overwrite our peer lookup defaults so that we don't have sleeps in
	// our tests.
	messenger.lookupPeerAttempts = testCase.peerLookups
	messenger.lookupPeerBackoff = 0

	ctxb := context.Background()
	req := NewSendMessageRequest(
		testCase.peer, nil, nil, nil, testCase.directConnect,
	)

	err := messenger.SendMessage(ctxb, req)

	// All of our errors are wrapped, so we can just check err.Is the
	// error we expect (also works for nil).
	require.True(t, errors.Is(err, testCase.expectedErr))
}

// handleOnionMesageMock is a mock that handled all mocked calls for testing
// onion messaging.
type handleOnionMesageMock struct {
	*mock.Mock
}

func (h *handleOnionMesageMock) processOnion(d []byte) (*btcec.PublicKey,
	*sphinx.ProcessedPacket, error) {

	args := h.Mock.MethodCalled("processOnion", d)

	return args.Get(0).(*btcec.PublicKey),
		args.Get(1).(*sphinx.ProcessedPacket), args.Error(2)
}

// mockProcessOnion primes the mock to handle a call to decode an onion message.
func mockProcessOnion(m *mock.Mock, blinding *btcec.PublicKey,
	packet *sphinx.ProcessedPacket, err error) {

	m.On(
		"processOnion", mock.Anything,
	).Once().Return(
		blinding, packet, err,
	)
}

func (h *handleOnionMesageMock) DecodePayload(o []byte) (
	*lnwire.OnionMessagePayload, error) {

	args := h.Mock.MethodCalled("DecodePayload", o)

	return args.Get(0).(*lnwire.OnionMessagePayload), args.Error(1)
}

// mockPayloadDecode primes the mock to handle a call to decode payload and
// return the payload and error provided. Note that this call does not assert
// that this function is called with a specific value.
func mockPayloadDecode(m *mock.Mock, payload *lnwire.OnionMessagePayload,
	err error) {

	m.On(
		"DecodePayload", mock.Anything,
	).Once().Return(
		payload, err,
	)
}

// DecryptBlob mocks decrypting of our onion message's encrypted blob.
func (h *handleOnionMesageMock) DecryptBlob(blindingPoint *btcec.PublicKey,
	payload *lnwire.OnionMessagePayload) (*lnwire.BlindedRouteData, error) {

	args := h.Mock.MethodCalled("decryptBlob", blindingPoint, payload)

	return args.Get(0).(*lnwire.BlindedRouteData), args.Error(1)
}

// mockDecryptBlob primes our mock for a call to decrypt blob.
func mockDecryptBlob(m *mock.Mock, blindingPoint *btcec.PublicKey,
	payload *lnwire.OnionMessagePayload, data *lnwire.BlindedRouteData,
	err error) {

	m.On(
		"decryptBlob", blindingPoint, payload,
	).Once().Return(
		data, err,
	)
}

// ForwardMessage mocks forwarding a message to the next node.
func (h *handleOnionMesageMock) ForwardMessage(data *lnwire.BlindedRouteData,
	blinding *btcec.PublicKey, packet *sphinx.OnionPacket) error {

	args := h.Mock.MethodCalled("forwardMessage", data, blinding, packet)

	return args.Error(0)
}

// mockForwardMessage primes the mock for a call to forward message.
func mockForwardMessage(m *mock.Mock, data *lnwire.BlindedRouteData,
	blinding *btcec.PublicKey, packet *sphinx.OnionPacket, err error) {

	m.On(
		"forwardMessage", data, blinding, packet,
	).Once().Return(
		err,
	)
}

// OnionMessageHandler mocks a call to handle an onion message.
func (h *handleOnionMesageMock) OnionMessageHandler(path *lnwire.ReplyPath,
	encrypted []byte, payload []byte) error {

	args := h.Mock.MethodCalled(
		"OnionMessageHandler", path, encrypted, payload,
	)

	return args.Error(0)
}

// mockMessageHandled primes the mock to handle a call to an onion message
// handler with the payload provided. The mock will return the error supplied.
func mockMessageHandled(m *mock.Mock, path *lnwire.ReplyPath, data,
	payload []byte, err error) {

	m.On(
		"OnionMessageHandler", path, data, payload,
	).Once().Return(
		err,
	)
}

// TestHandleOnionMessage tests different handling cases for onion messages.
func TestHandleOnionMessage(t *testing.T) {
	pubkeys := testutils.GetPubkeys(t, 4)
	nodeKey := pubkeys[0]
	blinding := pubkeys[3]

	// Create a single valid message that we can use across test cases.
	onionMsg := &lnwire.OnionMessage{
		BlindingPoint: blinding,
		OnionBlob:     []byte{1, 2, 3},
	}

	msg, err := customOnionMessage(nodeKey, onionMsg)
	require.NoError(t, err, "custom message")

	mockErr := errors.New("mock err")

	// Setup some values to use for our mocked payload decoding.
	replyPath := &lnwire.ReplyPath{
		FirstNodeID:   pubkeys[0],
		BlindingPoint: pubkeys[1],
		Hops: []*lnwire.BlindedHop{
			{
				BlindedNodeID: pubkeys[2],
				EncryptedData: []byte{6, 5, 4},
			},
		},
	}

	// Create one payload with no extra data for the final hop.
	payloadNoFinalHops := &lnwire.OnionMessagePayload{
		ReplyPath:     replyPath,
		EncryptedData: []byte{9, 8, 7},
	}

	// Create another payload with extra data for the final hop that will
	// need to be handled.
	finalHopPayload := &lnwire.FinalHopPayload{
		TLVType: tlv.Type(101),
		Value:   []byte{1, 2, 3},
	}

	payloadWithFinal := &lnwire.OnionMessagePayload{
		ReplyPath:     replyPath,
		EncryptedData: []byte{3, 2, 1},
		FinalHopPayloads: []*lnwire.FinalHopPayload{
			finalHopPayload,
		},
	}

	// Create a payload which we don't have a handler for (the test only
	// registers a handler for payload 101).
	unhandledPayload := &lnwire.OnionMessagePayload{
		ReplyPath: replyPath,
		FinalHopPayloads: []*lnwire.FinalHopPayload{
			{
				TLVType: 103,
			},
		},
	}

	tests := []struct {
		name        string
		msg         lndclient.CustomMessage
		setupMock   func(*mock.Mock)
		expectedErr error
	}{
		// TODO: add coverage for decoding errors
		{
			name: "message for our node",
			msg:  *msg,
			setupMock: func(m *mock.Mock) {
				// Return a packet indicating that we're the
				// recipient.
				packet := &sphinx.ProcessedPacket{
					Action: sphinx.ExitNode,
				}

				mockProcessOnion(m, blinding, packet, nil)
				mockPayloadDecode(m, payloadNoFinalHops, nil)
			},
			expectedErr: nil,
		},
		{
			name: "message for forwarding - no next onion",
			msg:  *msg,
			setupMock: func(m *mock.Mock) {
				// Return a packet indicating that there are
				// more hops, but it does not have a next onion
				// packet to forward.
				packet := &sphinx.ProcessedPacket{
					Action: sphinx.MoreHops,
				}

				mockProcessOnion(m, blinding, packet, nil)
				mockPayloadDecode(m, payloadNoFinalHops, nil)
			},
			expectedErr: ErrNoForwardingOnion,
		},
		{
			name: "message for forwarding",
			msg:  *msg,
			setupMock: func(m *mock.Mock) {
				// Return a packet indicating that there are
				// more hops and a payload with no extra data.
				packet := &sphinx.ProcessedPacket{
					Action:     sphinx.MoreHops,
					NextPacket: &sphinx.OnionPacket{},
				}

				mockProcessOnion(m, blinding, packet, nil)
				mockPayloadDecode(m, payloadNoFinalHops, nil)

				data := &lnwire.BlindedRouteData{
					NextNodeID: pubkeys[0],
				}

				mockDecryptBlob(
					m, blinding,
					payloadNoFinalHops, data, nil,
				)

				// Fail our message forward.
				mockForwardMessage(
					m, data, blinding,
					&sphinx.OnionPacket{}, mockErr,
				)
			},
			expectedErr: mockErr,
		},
		{
			name: "message for forwarding with final payload",
			msg:  *msg,
			setupMock: func(m *mock.Mock) {
				// Return a packet indicating that there are
				// more hops which also (incorrectly) contains
				// tlvs in the final node range.
				packet := &sphinx.ProcessedPacket{
					Action: sphinx.MoreHops,
				}

				mockProcessOnion(m, blinding, packet, nil)
				mockPayloadDecode(m, payloadWithFinal, nil)
			},
			expectedErr: ErrFinalPayload,
		},
		{
			name: "invalid message",
			msg:  *msg,
			setupMock: func(m *mock.Mock) {
				// Return a packet indicating that there are
				// more hops.
				packet := &sphinx.ProcessedPacket{
					Action: sphinx.Failure,
				}
				mockProcessOnion(m, blinding, packet, nil)

				// We'll decode the payload before  we check
				// the next action for the packet (and fail).
				mockPayloadDecode(m, payloadWithFinal, nil)
			},
			expectedErr: ErrBadMessage,
		},
		{
			name: "processing failed",
			msg:  *msg,
			setupMock: func(m *mock.Mock) {
				// Fail onion processing.
				mockProcessOnion(
					m, blinding, &sphinx.ProcessedPacket{},
					mockErr,
				)
			},
			expectedErr: ErrBadOnionBlob,
		},
		{
			name: "final payload handled",
			msg:  *msg,
			setupMock: func(m *mock.Mock) {
				// Setup our mock to return a final payload
				// for our node.
				packet := &sphinx.ProcessedPacket{
					Action: sphinx.ExitNode,
				}
				mockProcessOnion(m, blinding, packet, nil)
				mockPayloadDecode(m, payloadWithFinal, nil)

				// Handle the final payload without error.
				mockMessageHandled(
					m,
					payloadWithFinal.ReplyPath,
					payloadWithFinal.EncryptedData,
					finalHopPayload.Value,
					nil,
				)
			},
		},
		{
			name: "final payload handler error",
			msg:  *msg,
			setupMock: func(m *mock.Mock) {
				// Setup our mock to return a final payload for
				// our node.
				packet := &sphinx.ProcessedPacket{
					Action: sphinx.ExitNode,
				}
				mockProcessOnion(m, blinding, packet, nil)
				mockPayloadDecode(m, payloadWithFinal, nil)

				// Fail handling of final payload.
				mockMessageHandled(
					m,
					payloadWithFinal.ReplyPath,
					payloadWithFinal.EncryptedData,
					finalHopPayload.Value,
					mockErr,
				)
			},
			expectedErr: mockErr,
		},
		{
			name: "final payload no handler",
			msg:  *msg,
			setupMock: func(m *mock.Mock) {
				// Setup our mock to return a payload with
				// a final payload that we don't have a
				// handler registered for.
				packet := &sphinx.ProcessedPacket{
					Action: sphinx.ExitNode,
				}
				mockProcessOnion(m, blinding, packet, nil)
				mockPayloadDecode(m, unhandledPayload, nil)
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			// Setup a mock and prime it with the test's
			// requirements.
			mock := &handleOnionMesageMock{
				Mock: &mock.Mock{},
			}

			testCase.setupMock(mock.Mock)
			defer mock.AssertExpectations(t)

			handlers := map[tlv.Type]OnionMessageHandler{
				finalHopPayload.TLVType: mock.OnionMessageHandler,
			}

			kit := &onionMessageKit{
				processOnion:    mock.processOnion,
				decodePayload:   mock.DecodePayload,
				decryptDataBlob: mock.DecryptBlob,
				forwardMessage:  mock.ForwardMessage,
				handlers:        handlers,
			}

			err := handleOnionMessage(testCase.msg, kit)
			require.True(t, errors.Is(err, testCase.expectedErr))
		})
	}
}

// receiveMessageHandler is the function signature for handlers that drive
// tests for our receive message loop.
type receiveMessageHandler func(*testing.T, chan<- lndclient.CustomMessage,
	chan<- error)

// sendMsg is a helped that sends a custom message into the channel provided,
// failing the test if it is not delivered on time.
func sendMsg(t *testing.T, msgChan chan<- lndclient.CustomMessage,
	msg lndclient.CustomMessage) {

	select {
	case msgChan <- msg:
	case <-time.After(defaultTimeout):
		t.Fatalf("could not send message: %v", msg)
	}
}

// sendErr is a helper that sends an error into the channel provided, failing
// the test if it is not delivered in time.
func sendErr(t *testing.T, errChan chan<- error, err error) {
	select {
	case errChan <- err:
	case <-time.After(defaultTimeout):
		t.Fatalf("could not send error: %v", err)
	}
}

// TestReceiveOnionMessages tests the messenger's receive loop for messages.
func TestReceiveOnionMessages(t *testing.T) {
	privkeys := testutils.GetPrivkeys(t, 2)

	// Create an onion message that is *to our node* that we can use
	// across tests. The message itself can be junk, because we're not
	// reading it in this test.
	nodePubkey := privkeys[0].PubKey()
	onionMsg := lnwire.NewOnionMessage(nodePubkey, []byte{1, 2, 3})

	msg, err := customOnionMessage(
		nodePubkey, onionMsg,
	)
	require.NoError(t, err, "custom message")

	mockErr := errors.New("mock")

	tests := []struct {
		name          string
		handler       receiveMessageHandler
		expectedError error
	}{
		{
			name: "message sent",
			handler: func(t *testing.T,
				msgChan chan<- lndclient.CustomMessage,
				errChan chan<- error) {

				sendMsg(t, msgChan, *msg)
			},
		}, {
			name: "non-onion message",
			handler: func(t *testing.T,
				msgChan chan<- lndclient.CustomMessage,
				errChan chan<- error) {

				msg := lndclient.CustomMessage{
					MsgType: 1001,
				}

				sendMsg(t, msgChan, msg)
			},
			expectedError: nil,
		},
		{
			name: "lnd shutdown - messages",
			handler: func(t *testing.T,
				msgChan chan<- lndclient.CustomMessage,
				errChan chan<- error) {

				close(msgChan)
			},
			expectedError: ErrLNDShutdown,
		},
		{
			name: "lnd shutdown - errors",
			handler: func(t *testing.T,
				msgChan chan<- lndclient.CustomMessage,
				errChan chan<- error) {

				close(errChan)
			},
			expectedError: ErrLNDShutdown,
		},
		{
			name: "subscription error",
			handler: func(t *testing.T,
				msgChan chan<- lndclient.CustomMessage,
				errChan chan<- error) {

				sendErr(t, errChan, mockErr)
			},
			expectedError: mockErr,
		},
	}

	for _, testCase := range tests {
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			testReceiveOnionMessages(
				t, privkeys[0], testCase.handler,
				testCase.expectedError,
			)
		})
	}
}

func testReceiveOnionMessages(t *testing.T, privkey *btcec.PrivateKey,
	handler receiveMessageHandler, expectedErr error) {

	// Create a simple node key ecdh impl for our messenger.
	nodeKeyECDH := &sphinx.PrivKeyECDH{
		PrivKey: privkey,
	}

	// Setup a mocked lnd and prime it to have SubscribeCustomMessages
	// called.
	lnd := testutils.NewMockLnd()
	defer lnd.Mock.AssertExpectations(t)

	// Create channels to deliver messages and fail if they block for too
	// long.
	var (
		msgChan = make(chan lndclient.CustomMessage)
		errChan = make(chan error)

		shutdownChan    = make(chan error)
		requestShutdown = func(err error) {
			select {
			case shutdownChan <- err:
			case <-time.After(defaultTimeout):
				t.Fatalf("did not shutdown with: %v", err)
			}
		}
	)

	// Set up our mock to return our message channels when we subscribe to
	// custom lnd messages.
	// Note: might be wrong types?
	testutils.MockSubscribeCustomMessages(
		lnd.Mock, msgChan, errChan, nil,
	)

	messenger := NewOnionMessenger(
		lnd, nodeKeyECDH,
		requestShutdown,
	)
	err := messenger.Start()
	require.NoError(t, err, "start messenger")

	// Shutdown our messenger at the end of the test.
	defer func() {
		err := messenger.Stop()
		require.NoError(t, err, "stop messenger")
	}()

	// Run the specific test's handler.
	handler(t, msgChan, errChan)

	// If we expect to exit with an error, expect it to be surfaced through
	// requesting a graceful shutdown.
	if expectedErr != nil {
		select {
		case err := <-shutdownChan:
			require.True(t, errors.Is(err, expectedErr), "shutdown")

		case <-time.After(defaultTimeout):
			t.Fatal("no shutdown error recieved")
		}
	}
}

// TestHandleRegistration tests registration of handlers for tlv payloads.
func TestHandleRegistration(t *testing.T) {
	var (
		invalidTlv tlv.Type = 10
		validTlv   tlv.Type = 100

		handler = func(*lnwire.ReplyPath, []byte, []byte) error {
			return nil
		}

		nodeKeyECDH = &sphinx.PrivKeyECDH{
			PrivKey: testutils.GetPrivkeys(t, 1)[0],
		}
	)

	// Assert that our test tlv values have the validity we expect.
	require.Nil(t, lnwire.ValidateFinalPayload(validTlv))

	err := lnwire.ValidateFinalPayload(invalidTlv)
	require.True(t, errors.Is(err, lnwire.ErrNotFinalPayload))

	// Setups a mock lnd. We need this to subscribe to incoming messages,
	// even though we're not testing message handling in this test. W
	lnd := testutils.NewMockLnd()
	defer lnd.Mock.AssertExpectations(t)

	// Prime our mock for our startup call, using nil channels because we
	// won't actually deliver messages.
	testutils.MockSubscribeCustomMessages(
		lnd.Mock, nil, nil, nil,
	)

	// Create a messenger, but don't start it yet.
	messenger := NewOnionMessenger(
		lnd, nodeKeyECDH, nil,
	)

	// Assert the registration fails if we're not started.
	err = messenger.RegisterHandler(validTlv, handler)
	require.True(t, errors.Is(err, ErrNotStarted), "err: %v", err.Error())

	// Start our messenger. We'll shut it down manually later, so we don't
	// defer stop here.
	require.NoError(t, messenger.Start(), "start messenger")

	// Now that we're started, we should be able to register with no issue.
	err = messenger.RegisterHandler(validTlv, handler)
	require.NoError(t, err, "valid tlv register")

	// Try to re-register with the same type, we should fail.
	err = messenger.RegisterHandler(validTlv, handler)
	require.True(t, errors.Is(err, ErrHandlerRegistered))

	// Try to register a handler for an out-of-range tlv type, expect
	// failure.
	err = messenger.RegisterHandler(invalidTlv, handler)
	require.True(t, errors.Is(err, lnwire.ErrNotFinalPayload))

	// Try to de-register our existing handler, we should succeed.
	err = messenger.DeregisterHandler(validTlv)
	require.NoError(t, err)

	// Try to de-register a handler that's no longer registered, we should
	// get an error.
	err = messenger.DeregisterHandler(validTlv)
	require.True(t, errors.Is(err, ErrHandlerNotFound))

	// Shut down our messenger to test registration requests during
	// shutdown.
	require.NoError(t, messenger.Stop(), "stop messenger")

	err = messenger.RegisterHandler(validTlv, handler)
	require.True(t, errors.Is(err, ErrShuttingDown))
}

// TestMultiHopPath tests selection of multi-hop onion message paths.
func TestMultiHopPath(t *testing.T) {
	var (
		pubkeys = testutils.GetPubkeys(t, 3)
		peer    = pubkeys[0]
		node1   = route.NewVertex(pubkeys[1])
		node2   = route.NewVertex(pubkeys[2])
		mockErr = errors.New("mock err")
	)
	tests := []struct {
		name            string
		peer            *btcec.PublicKey
		queryRoutesResp *lndclient.QueryRoutesResponse
		queryRoutesErr  error
		path            []*btcec.PublicKey
		err             error
	}{
		{
			name:            "no routes found",
			peer:            peer,
			queryRoutesResp: &lndclient.QueryRoutesResponse{},
			queryRoutesErr:  lndclient.ErrNoRouteFound,
			path:            nil,
			err:             nil,
		},
		{
			name:            "query routes fails",
			peer:            peer,
			queryRoutesResp: &lndclient.QueryRoutesResponse{},
			queryRoutesErr:  mockErr,
			path:            nil,
			err:             mockErr,
		},
		{
			name: "path found, pubkey missing",
			peer: peer,
			queryRoutesResp: &lndclient.QueryRoutesResponse{
				Hops: []*lndclient.Hop{
					{
						ChannelID: 1,
						PubKey:    &node1,
					},
					{
						ChannelID: 2,
						PubKey:    nil,
					},
				},
			},
			path: nil,
			err:  ErrNilPubkeyInRoute,
		},
		{
			name: "path found",
			peer: peer,
			queryRoutesResp: &lndclient.QueryRoutesResponse{
				Hops: []*lndclient.Hop{
					{
						ChannelID: 1,
						PubKey:    &node1,
					},
					{
						ChannelID: 2,
						PubKey:    &node2,
					},
				},
			},
			path: []*btcec.PublicKey{
				pubkeys[1],
				pubkeys[2],
			},
		},
	}

	for _, testCase := range tests {
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			lnd := testutils.NewMockLnd()
			defer lnd.Mock.AssertExpectations(t)

			// Setup our mock to return the response specified by
			// the test case.
			req := queryRoutesRequest(testCase.peer)
			testutils.MockQueryRoutes(
				lnd.Mock, req, testCase.queryRoutesResp,
				testCase.queryRoutesErr,
			)

			ctxb := context.Background()
			path, err := multiHopPath(ctxb, lnd, testCase.peer)
			require.True(t, errors.Is(err, testCase.err))
			require.Equal(t, testCase.path, path)
		})
	}
}

// TestValidateSendMessageRequest tests validation of send message requests.
func TestValidateSendMessageRequest(t *testing.T) {
	pubkeys := testutils.GetPubkeys(t, 1)

	tests := []struct {
		name string
		req  *SendMessageRequest
		err  error
	}{
		{
			name: "peer and blinded dest",
			req: &SendMessageRequest{
				Peer:               pubkeys[0],
				BlindedDestination: &lnwire.ReplyPath{},
			},
			err: ErrBothDest,
		},
		{
			name: "neither dest set",
			req:  &SendMessageRequest{},
			err:  ErrNoDest,
		},
		{
			name: "blinded dest with no hops",
			req: &SendMessageRequest{
				BlindedDestination: &lnwire.ReplyPath{},
			},
			err: ErrNoBlindedHops,
		},
		{
			name: "valid - cleartext peer",
			req: &SendMessageRequest{
				Peer: pubkeys[0],
			},
		},
		{
			name: "valid - blinded dest",
			req: &SendMessageRequest{
				BlindedDestination: &lnwire.ReplyPath{
					Hops: []*lnwire.BlindedHop{
						{},
					},
				},
			},
		},
	}

	for _, testCase := range tests {
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			err := testCase.req.Validate()
			require.True(t, errors.Is(err, testCase.err))
		})
	}
}
