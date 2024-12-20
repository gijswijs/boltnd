package rpcserver

import (
	"context"

	"github.com/gijswijs/boltnd/lnwire"
	"github.com/gijswijs/boltnd/offersrpc"
	"github.com/gijswijs/boltnd/onionmsg"
	"github.com/lightningnetwork/lnd/tlv"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// SubscribeOnionPayload subscribes to onion message payloads
func (s *Server) SubscribeOnionPayload(
	req *offersrpc.SubscribeOnionPayloadRequest,
	stream offersrpc.Offers_SubscribeOnionPayloadServer) error {

	log.Debugf("SubscribeOnionPayload: %+v", req)

	if err := s.waitForReady(stream.Context()); err != nil {
		return err
	}

	tlvType, err := parseSubscribeOnionPayloadRequest(req)
	if err != nil {
		return err
	}

	// Create a channel to receive incoming payloads on. Buffer it by 1
	// so that we never risk blocking the calling function.
	incomingMessages := make(chan onionPayloadResponse, 1)

	return handleSubscribeOnionPayload(
		stream.Context(), tlvType, incomingMessages, s.quit,
		s.onionMsgr, stream.Send,
	)
}

func parseSubscribeOnionPayloadRequest(
	req *offersrpc.SubscribeOnionPayloadRequest) (tlv.Type, error) {

	tlvType := tlv.Type(req.TlvType)
	if err := lnwire.ValidateFinalPayload(tlvType); err != nil {
		return 0, status.Errorf(
			codes.InvalidArgument, "tlv type: %v", err,
		)
	}

	return tlvType, nil
}

type onionPayloadResponse struct {
	payload   []byte
	replyPath *lnwire.ReplyPath
}

// handleSubscribeOnionPayload creates a subscription for onion message
// payloads with tlvs of the provided type.
func handleSubscribeOnionPayload(ctx context.Context, tlvType tlv.Type,
	incoming chan onionPayloadResponse, quit chan struct{},
	messenger onionmsg.OnionMessenger,
	send func(*offersrpc.SubscribeOnionPayloadResponse) error) error {

	// Create an onion message handler which will consume messages from
	// our incoming channel, dropping messages if our server is shut down
	// or the client cancels their context.
	handler := func(replyPath *lnwire.ReplyPath, _ []byte,
		payload []byte) error {

		select {
		// Pass message to our incoming channel.
		case incoming <- onionPayloadResponse{
			replyPath: replyPath,
			payload:   payload,
		}:
			return nil

		// Exit on client cancel.
		case <-ctx.Done():
			return ctx.Err()

		// Exit on server shutdown.
		case <-quit:
			return ErrShuttingDown
		}
	}

	// Register our handler with the messenger, and deregister it on
	// exit.
	if err := messenger.RegisterHandler(tlvType, handler); err != nil {
		return status.Errorf(
			codes.Unavailable, "could not register "+
				"subscription: %v", err,
		)
	}

	defer func() {
		if err := messenger.DeregisterHandler(tlvType); err != nil {
			log.Errorf("Deregister handler: %v failed: %v",
				tlvType, err)
		}
	}()

	// Consume incoming messages until the client cancels the subscription
	// or our stream fails.
	for {
		// Receive incoming messages.
		select {
		case msg := <-incoming:
			resp := &offersrpc.SubscribeOnionPayloadResponse{
				Value:     msg.payload,
				ReplyPath: composeReplyPath(msg.replyPath),
			}

			if err := send(resp); err != nil {
				return err
			}

		// Exit if the client cancels their context.
		case <-ctx.Done():
			return status.Errorf(
				codes.Canceled, "client cancel",
			)

		// Error out if the server is shutting down.
		case <-quit:
			return ErrShuttingDown
		}
	}
}
