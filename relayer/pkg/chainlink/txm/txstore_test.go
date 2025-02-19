package txm

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/NethermindEth/juno/core/felt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTxStore(t *testing.T) {
	t.Parallel()

	t.Run("happypath", func(t *testing.T) {
		t.Parallel()

		s := NewTxStore(&felt.Zero)
		assert.Equal(t, 0, s.InflightCount())
		require.NoError(t, s.Save(new(felt.Felt).SetUint64(0), "0x0"))
		assert.Equal(t, 1, s.InflightCount())
		assert.Equal(t, []string{"0x0"}, s.GetUnconfirmed())
		require.NoError(t, s.Confirm("0x0"))
		assert.Equal(t, 0, s.InflightCount())
		assert.Equal(t, []string{}, s.GetUnconfirmed())
	})

	t.Run("save", func(t *testing.T) {
		t.Parallel()

		// create
		s := NewTxStore(new(felt.Felt).SetUint64(0))

		// accepts tx in order
		require.NoError(t, s.Save(new(felt.Felt).SetUint64(0), "0x0"))
		assert.Equal(t, 1, s.InflightCount())
		assert.Equal(t, new(felt.Felt).SetUint64(1), &s.currentNonce)

		// accepts tx that skips a nonce
		require.NoError(t, s.Save(new(felt.Felt).SetUint64(2), "0x2"))
		assert.Equal(t, 2, s.InflightCount())
		assert.Equal(t, new(felt.Felt).SetUint64(1), &s.currentNonce)

		// accepts tx that fills in the missing nonce + fast forwards currentNonce
		require.NoError(t, s.Save(new(felt.Felt).SetUint64(1), "0x1"))
		assert.Equal(t, 3, s.InflightCount())
		assert.Equal(t, new(felt.Felt).SetUint64(3), &s.currentNonce)

		// skip a nonce for later tests
		require.NoError(t, s.Save(new(felt.Felt).SetUint64(4), "0x4"))
		assert.Equal(t, 4, s.InflightCount())
		assert.Equal(t, new(felt.Felt).SetUint64(3), &s.currentNonce)

		// rejects old nonce
		require.ErrorContains(t, s.Save(new(felt.Felt).SetUint64(0), "0xold"), "nonce too low: 0x0 < 0x3 (lowest)")
		assert.Equal(t, 4, s.InflightCount())

		// reject already in use nonce
		require.ErrorContains(t, s.Save(new(felt.Felt).SetUint64(4), "0xskip"), "nonce used: tried to use nonce (0x4) for tx (0xskip), already used by (0x4)")
		assert.Equal(t, 4, s.InflightCount())

		// reject already in use tx hash
		require.ErrorContains(t, s.Save(new(felt.Felt).SetUint64(5), "0x0"), "hash used: tried to use tx (0x0) for nonce (0x5), already used nonce (0x0)")
		assert.Equal(t, 4, s.InflightCount())

		// race save
		var err0 error
		var err1 error
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			err0 = s.Save(new(felt.Felt).SetUint64(10), "0x10")
			wg.Done()
		}()
		go func() {
			err1 = s.Save(new(felt.Felt).SetUint64(10), "0x10")
			wg.Done()
		}()
		wg.Wait()
		assert.True(t, !errors.Is(err0, err1) && (err0 != nil || err1 != nil))
	})

	t.Run("confirm", func(t *testing.T) {
		t.Parallel()

		// init store
		s := NewTxStore(new(felt.Felt).SetUint64(0))
		for i := 0; i < 5; i++ {
			require.NoError(t, s.Save(new(felt.Felt).SetUint64(uint64(i)), "0x"+fmt.Sprintf("%d", i)))
		}

		// confirm in order
		require.NoError(t, s.Confirm("0x0"))
		require.NoError(t, s.Confirm("0x1"))
		assert.Equal(t, 3, s.InflightCount())

		// confirm out of order
		require.NoError(t, s.Confirm("0x4"))
		require.NoError(t, s.Confirm("0x3"))
		require.NoError(t, s.Confirm("0x2"))
		assert.Equal(t, 0, s.InflightCount())

		// confirm unknown/duplicate
		require.ErrorContains(t, s.Confirm("0x2"), "tx hash does not exist - it may already be confirmed")
		require.ErrorContains(t, s.Confirm("0xNULL"), "tx hash does not exist - it may already be confirmed")

		// race confirm
		require.NoError(t, s.Save(new(felt.Felt).SetUint64(10), "0x10"))
		var err0 error
		var err1 error
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			err0 = s.Confirm("0x10")
			wg.Done()
		}()
		go func() {
			err1 = s.Confirm("0x10")
			wg.Done()
		}()
		wg.Wait()
		assert.True(t, !errors.Is(err0, err1) && (err0 != nil || err1 != nil))
	})
}

func TestChainTxStore(t *testing.T) {
	t.Parallel()

	c := NewChainTxStore()

	felt0 := new(felt.Felt).SetUint64(0)
	felt1 := new(felt.Felt).SetUint64(1)

	// automatically save the from address
	require.NoError(t, c.Save(felt0, new(felt.Felt).SetUint64(0), "0x0"))

	// reject saving for existing address and reused hash & nonce
	// error messages are tested within TestTxStore
	assert.Error(t, c.Save(felt0, new(felt.Felt).SetUint64(0), "0x1"))
	assert.Error(t, c.Save(felt0, new(felt.Felt).SetUint64(1), "0x0"))

	// inflight count
	count, exists := c.GetAllInflightCount()[felt0]
	require.True(t, exists)
	assert.Equal(t, 1, count)
	_, exists = c.GetAllInflightCount()[felt1]
	require.False(t, exists)

	// get unconfirmed
	list := c.GetAllUnconfirmed()
	assert.Equal(t, 1, len(list))
	hashes, ok := list[felt0]
	assert.True(t, ok)
	assert.Equal(t, []string{"0x0"}, hashes)

	// confirm
	assert.NoError(t, c.Confirm(felt0, "0x0"))
	assert.ErrorContains(t, c.Confirm(felt1, "0x0"), "from address does not exist")
	assert.Error(t, c.Confirm(felt0, "0x1"))
	list = c.GetAllUnconfirmed()
	assert.Equal(t, 1, len(list))
	assert.Equal(t, 0, len(list[felt0]))
	count, exists = c.GetAllInflightCount()[felt0]
	assert.True(t, exists)
	assert.Equal(t, 0, count)
}
