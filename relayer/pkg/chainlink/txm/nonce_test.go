package txm_test

import (
	"fmt"
	"math/big"
	"testing"

	"github.com/NethermindEth/juno/core/felt"
	starknetutils "github.com/NethermindEth/starknet.go/utils"

	"github.com/smartcontractkit/chainlink-common/pkg/logger"
	"github.com/smartcontractkit/chainlink-common/pkg/utils/tests"
	"github.com/smartcontractkit/chainlink-starknet/relayer/pkg/chainlink/txm"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/smartcontractkit/chainlink-starknet/relayer/pkg/chainlink/txm/mocks"
)

func newTestNonceManager(t *testing.T, chainID string, initNonce *felt.Felt) (txm.NonceManager, *felt.Felt, func()) {
	// setup
	c := mocks.NewNonceManagerClient(t)
	lggr := logger.Test(t)
	nm := txm.NewNonceManager(lggr)

	// mock returns
	keyHash, err := starknetutils.HexToFelt("0x0")
	require.NoError(t, err)
	c.On("AccountNonce", mock.Anything, mock.Anything).Return(initNonce, nil).Once()

	require.NoError(t, nm.Start(tests.Context(t)))
	require.NoError(t, nm.Register(tests.Context(t), keyHash, keyHash, chainID, c))

	return nm, keyHash, func() { require.NoError(t, nm.Close()) }
}

func TestNonceManager_NextSequence(t *testing.T) {
	t.Parallel()

	chainId := "test_nextSequence"
	initNonce := new(felt.Felt).SetUint64(10)
	nm, k, stop := newTestNonceManager(t, chainId, initNonce)
	defer stop()

	// get with proper inputs
	nonce, err := nm.NextSequence(k, chainId)
	require.NoError(t, err)
	assert.Equal(t, initNonce, nonce)

	// should fail with invalid chain id
	_, err = nm.NextSequence(k, "invalid_chainId")
	require.Error(t, err)
	assert.Contains(t, err.Error(), fmt.Sprintf("nonce does not exist for key: %s and chain: %s", k.String(), "invalid_chainId"))

	// should fail with invalid address
	randAddr1 := starknetutils.BigIntToFelt(big.NewInt(1))
	_, err = nm.NextSequence(randAddr1, chainId)
	require.Error(t, err)
	assert.Contains(t, err.Error(), fmt.Sprintf("nonce tracking does not exist for key: %s", randAddr1.String()))
}

func TestNonceManager_IncrementNextSequence(t *testing.T) {
	t.Parallel()

	chainId := "test_nextSequence"
	initNonce := new(felt.Felt).SetUint64(10)
	nm, k, stop := newTestNonceManager(t, chainId, initNonce)
	defer stop()

	one := new(felt.Felt).SetUint64(1)
	initMinusOne := new(felt.Felt).Sub(initNonce, one)
	initPlusOne := new(felt.Felt).Add(initNonce, one)

	// should fail if nonce is lower then expected
	err := nm.IncrementNextSequence(k, chainId, initMinusOne)
	require.Error(t, err)
	assert.Contains(t, err.Error(), fmt.Sprintf("mismatched nonce for %s: %s (expected) != %s (got)", k, initNonce, initMinusOne))

	// increment with proper inputs
	err = nm.IncrementNextSequence(k, chainId, initNonce)
	require.NoError(t, err)
	next, err := nm.NextSequence(k, chainId)
	require.NoError(t, err)
	assert.Equal(t, initPlusOne, next)

	// should fail with invalid chain id
	err = nm.IncrementNextSequence(k, "invalid_chainId", initPlusOne)
	require.Error(t, err)
	assert.Contains(t, err.Error(), fmt.Sprintf("nonce does not exist for key: %s and chain: %s", k.String(), "invalid_chainId"))

	// should fail with invalid address
	randAddr1 := starknetutils.BigIntToFelt(big.NewInt(1))
	err = nm.IncrementNextSequence(randAddr1, chainId, initPlusOne)
	require.Error(t, err)
	assert.Contains(t, err.Error(), fmt.Sprintf("nonce tracking does not exist for key: %s", randAddr1.String()))

	// verify it didnt get changed by any erroring calls
	next, err = nm.NextSequence(k, chainId)
	require.NoError(t, err)
	assert.Equal(t, initPlusOne, next)
}
