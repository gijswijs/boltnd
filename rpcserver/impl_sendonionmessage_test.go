package rpcserver

import (
	"context"
	"errors"
	"testing"

	"github.com/gijswijs/boltnd/lnwire"
	"github.com/gijswijs/boltnd/offersrpc"
	"github.com/gijswijs/boltnd/onionmsg"
	"github.com/gijswijs/boltnd/testutils"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestRPCSendOnionMessage tests the rpc mechanics around sending an onion
// message. This function is primarily concerned with parsing, error handling
// and response creation, so the actual send message functionality is mocked.
func TestRPCSendOnionMessage(t *testing.T) {
	pubkey := testutils.GetPubkeys(t, 1)[0]
	pubkeyBytes := pubkey.SerializeCompressed()

	finalPayload := &lnwire.FinalHopPayload{
		TLVType: 100,
		Value:   []byte{9, 9, 9},
	}

	tests := []struct {
		name      string
		setupMock func(*mock.Mock)
		request   *offersrpc.SendOnionMessageRequest
		success   bool
		errCode   codes.Code
	}{
		{
			name: "invalid pubkey",
			request: &offersrpc.SendOnionMessageRequest{
				Pubkey:        []byte{1, 2, 3},
				DirectConnect: true,
			},
			success: false,
			errCode: codes.InvalidArgument,
		},
		{
			name: "send message failed",
			// Setup our mock to fail sending a message.
			setupMock: func(m *mock.Mock) {
				req := onionmsg.NewSendMessageRequest(
					pubkey, nil, nil, []*lnwire.FinalHopPayload{}, true,
				)

				mockSendMessage(m, req, errors.New("mock"))
			},
			request: &offersrpc.SendOnionMessageRequest{
				Pubkey:        pubkeyBytes,
				DirectConnect: true,
			},
			success: false,
			errCode: codes.Internal,
		},
		{
			name: "send message succeeds",
			// Setup our mock to successfully send the message.
			setupMock: func(m *mock.Mock) {
				req := onionmsg.NewSendMessageRequest(
					pubkey, nil, nil, []*lnwire.FinalHopPayload{}, false,
				)

				mockSendMessage(m, req, nil)
			},
			request: &offersrpc.SendOnionMessageRequest{
				Pubkey:        pubkeyBytes,
				DirectConnect: false,
			},
			success: true,
		},
		{
			name: "send message succeeds with final payload",
			setupMock: func(m *mock.Mock) {
				finalPayloads := []*lnwire.FinalHopPayload{
					finalPayload,
				}

				req := onionmsg.NewSendMessageRequest(
					pubkey, nil, nil, finalPayloads, true,
				)

				mockSendMessage(m, req, nil)
			},
			request: &offersrpc.SendOnionMessageRequest{
				Pubkey: pubkeyBytes,
				FinalPayloads: map[uint64][]byte{
					uint64(finalPayload.TLVType): finalPayload.Value,
				},
				DirectConnect: true,
			},
			success: true,
		},
		{
			name: "invalid final payload",
			// We expect our test to fail when parsing the request,
			// so don't need to prime our mock at all.
			setupMock: func(*mock.Mock) {},
			request: &offersrpc.SendOnionMessageRequest{
				Pubkey: pubkeyBytes,
				// Provide a final payload tlv type that is
				// below the allowed range for final payloads.
				FinalPayloads: map[uint64][]byte{
					2: []byte{1, 2},
				},
				DirectConnect: true,
			},
			success: false,
			errCode: codes.InvalidArgument,
		},
	}

	for _, testCase := range tests {
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			s := newServerTest(t)
			s.start()
			defer s.stop()

			// Prime mock if required.
			if testCase.setupMock != nil {
				testCase.setupMock(s.offerMock.Mock)
			}

			// Send the test's request to the server.
			_, err := s.server.SendOnionMessage(
				context.Background(), testCase.request,
			)
			require.Equal(t, testCase.success, err == nil)

			// If our test was a success, we don't need to test our
			// error further.
			if testCase.success {
				return
			}

			// If we expect a failure, assert that it has the error
			// code we want.
			require.NotNil(t, err, "expected failure")

			status, ok := status.FromError(err)
			require.True(t, ok, "expected coded error")
			require.Equal(t, status.Code(), testCase.errCode)
		})
	}
}
