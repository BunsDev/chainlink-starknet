package txm

import (
	"fmt"
	"sync"

	"github.com/NethermindEth/juno/core/felt"
	"golang.org/x/exp/maps"
)

// TxStore tracks broadcast & unconfirmed txs
type TxStore struct {
	lock         sync.RWMutex
	nonceToHash  map[felt.Felt]string // map nonce to txhash
	hashToNonce  map[string]felt.Felt // map hash to nonce
	currentNonce felt.Felt            // minimum nonce
}

func NewTxStore(current *felt.Felt) *TxStore {
	return &TxStore{
		nonceToHash:  map[felt.Felt]string{},
		hashToNonce:  map[string]felt.Felt{},
		currentNonce: *current,
	}
}

// TODO: Save should make a copy otherwise wee're modiffying the same memory and could loop
func (s *TxStore) Save(nonce *felt.Felt, hash string) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	if s.currentNonce.Cmp(nonce) == 1 {
		return fmt.Errorf("nonce too low: %s < %s (lowest)", nonce, &s.currentNonce)
	}
	if h, exists := s.nonceToHash[*nonce]; exists {
		return fmt.Errorf("nonce used: tried to use nonce (%s) for tx (%s), already used by (%s)", nonce, hash, h)
	}
	if n, exists := s.hashToNonce[hash]; exists {
		return fmt.Errorf("hash used: tried to use tx (%s) for nonce (%s), already used nonce (%s)", hash, nonce, &n)
	}

	// store hash
	s.nonceToHash[*nonce] = hash
	s.hashToNonce[hash] = *nonce

	// find next unused nonce
	_, exists := s.nonceToHash[s.currentNonce]
	for exists {
		s.currentNonce = *new(felt.Felt).Add(&s.currentNonce, new(felt.Felt).SetUint64(1))
		_, exists = s.nonceToHash[s.currentNonce]
	}
	return nil
}

func (s *TxStore) Confirm(hash string) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	if nonce, exists := s.hashToNonce[hash]; exists {
		delete(s.hashToNonce, hash)
		delete(s.nonceToHash, nonce)
		return nil
	}
	return fmt.Errorf("tx hash does not exist - it may already be confirmed: %s", hash)
}

func (s *TxStore) GetUnconfirmed() []string {
	s.lock.RLock()
	defer s.lock.RUnlock()
	return maps.Values(s.nonceToHash)
}

func (s *TxStore) InflightCount() int {
	s.lock.RLock()
	defer s.lock.RUnlock()
	return len(s.nonceToHash)
}

type ChainTxStore struct {
	store map[*felt.Felt]*TxStore
	lock  sync.RWMutex
}

func NewChainTxStore() *ChainTxStore {
	return &ChainTxStore{
		store: map[*felt.Felt]*TxStore{},
	}
}

func (c *ChainTxStore) Save(from *felt.Felt, nonce *felt.Felt, hash string) error {
	// use write lock for methods that modify underlying data
	c.lock.Lock()
	defer c.lock.Unlock()
	if err := c.validate(from); err != nil {
		// if does not exist, create a new store for the address
		c.store[from] = NewTxStore(nonce)
	}
	return c.store[from].Save(nonce, hash)
}

func (c *ChainTxStore) Confirm(from *felt.Felt, hash string) error {
	// use write lock for methods that modify underlying data
	c.lock.Lock()
	defer c.lock.Unlock()

	if err := c.validate(from); err != nil {
		return err
	}
	return c.store[from].Confirm(hash)
}

func (c *ChainTxStore) GetAllInflightCount() map[*felt.Felt]int {
	// use read lock for methods that read underlying data
	c.lock.RLock()
	defer c.lock.RUnlock()

	list := map[*felt.Felt]int{}

	for i := range c.store {
		list[i] = c.store[i].InflightCount()
	}

	return list
}

func (c *ChainTxStore) GetAllUnconfirmed() map[*felt.Felt][]string {
	// use read lock for methods that read underlying data
	c.lock.RLock()
	defer c.lock.RUnlock()

	list := map[*felt.Felt][]string{}

	for i := range c.store {
		list[i] = c.store[i].GetUnconfirmed()
	}
	return list
}

func (c *ChainTxStore) validate(from *felt.Felt) error {
	if _, exists := c.store[from]; !exists {
		return fmt.Errorf("from address does not exist: %s", from)
	}
	return nil
}
