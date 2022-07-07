package testutils

import (
	"encoding/hex"
	"testing"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/stretchr/testify/require"
)

// GetPubkeys provides n public keys for testing.
func GetPubkeys(t *testing.T, n int) []*btcec.PublicKey {
	t.Helper()

	if len(pubkeyStrs) < n {
		t.Fatalf("testing package only has: %v pubkeys, %v requested",
			len(pubkeyStrs), n)
	}

	pubkeys := make([]*btcec.PublicKey, n)

	for i, pkStr := range pubkeyStrs[0:n] {
		pkBytes, err := hex.DecodeString(pkStr)
		require.NoError(t, err, "pubkey decode string")

		pubkeys[i], err = btcec.ParsePubKey(pkBytes)
		require.NoError(t, err, "parse pubkey")
	}

	return pubkeys
}
