package onionmsg

import (
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	sphinx "github.com/lightningnetwork/lightning-onion"
)

// encodeBlindedPayload is the function signature used to encode a TLV stream
// of blinded route data for onion messages.
type encodeBlindedPayload func(*btcec.PublicKey) ([]byte, error)

// createPathToBlind takes a set of public keys and creates a set of hops in
// a blinded route. The first node in the route is considered to be the
// introduction node N(0), and all nodes after it are denoted N(1), N(2), etc.
//
// Given a path N(0), N(1), N(2), ... , N(k), the blinded route will have
// the following entries.
// [0] NodePub: N(0)
//     Payload: TLV( next_node_id : N(1) )
// [1] NodePub: N(1)
//     Payload: TLV( next_node_id: N(2) )
// ...
// [k] NodePub: N(k)
//
// An encodePayload function is passed in as a parameter for easy mocking in
// tests.
//
// Note that this function currently sends empty onion messages to peers (no
// TLVs in the final hop).
func createPathToBlind(path []*btcec.PublicKey,
	encodePayload encodeBlindedPayload) ([]*sphinx.BlindedPathHop, error) {

	hopCount := len(path)

	// Create a set of blinded hops for our path.
	hopsToBlind := make([]*sphinx.BlindedPathHop, len(path))

	// Create our first hop, which it the introduction node.
	hopsToBlind[0] = &sphinx.BlindedPathHop{
		NodePub: path[0],
	}

	// Run through all paths and add the cleartext node ID to the
	// previous hop's payload. We need each hop to have the next node's ID
	// in its payload so that it can unblind the route.
	for i := 1; i < hopCount; i++ {
		// Add this node's cleartext pubkey to the previous node's
		// payload.
		var err error
		hopsToBlind[i-1].Payload, err = encodePayload(path[i])
		if err != nil {
			return nil, fmt.Errorf("intermediate node: %v "+
				"encoding failed: %w", i, err)
		}

		// Add our hop to the set of blinded hops.
		hopsToBlind[i] = &sphinx.BlindedPathHop{
			NodePub: path[i],
		}
	}

	return hopsToBlind, nil
}

// blindedToSphinx converts the blinded path provided to a sphinx path that can
// be wrapped up in an onion.
func blindedToSphinx(blindedRoute *sphinx.BlindedPath) (*sphinx.PaymentPath,
	error) {

	var sphinxPath sphinx.PaymentPath

	// We fill our first hop in with the introduction point for our route
	// and its encrypted data. We specifically separate this hop out because
	// we do not want to use the blinded node ID for the first hop.
	sphinxPath[0] = sphinx.OnionHop{
		NodePub: *blindedRoute.IntroductionPoint,
		HopPayload: sphinx.HopPayload{
			Type:    sphinx.PayloadTLV,
			Payload: blindedRoute.EncryptedData[0],
		},
	}

	// For all remaining hops, we'll fill in the blinded node id and
	// encrypted data.
	for i := 1; i < len(blindedRoute.EncryptedData); i++ {
		sphinxPath[i] = sphinx.OnionHop{
			NodePub: *blindedRoute.BlindedHops[i],
			HopPayload: sphinx.HopPayload{
				Type:    sphinx.PayloadTLV,
				Payload: blindedRoute.EncryptedData[i],
			},
		}
	}

	return &sphinxPath, nil
}
