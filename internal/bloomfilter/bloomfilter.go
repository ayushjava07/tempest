package bloomfilter

import (
	"hash"
	"hash/fnv"
	"sync"
)

type BloomFilter struct {
	mu     sync.RWMutex
	bits   []bool
	size   uint
	hashes []hash.Hash64
	k      int
	count  uint
}

func New(size uint, numHashes int) *BloomFilter {
	if size == 0 {
		size = 1024
	}
	if numHashes <= 0 {
		numHashes = 3
	}
	bf := &BloomFilter{
		bits: make([]bool, size),
		size: size,
		k:    numHashes,
	}
	for i := 0; i < numHashes; i++ {
		bf.hashes = append(bf.hashes, fnv.New64a())
	}
	return bf
}

func (bf *BloomFilter) hash(data []byte, seed uint) uint {
	h := fnv.New64a()
	_, _ = h.Write(data)
	_, _ = h.Write([]byte{byte(seed)})
	return uint(h.Sum64()) % bf.size
}

func (bf *BloomFilter) Add(data []byte) {
	bf.mu.Lock()
	defer bf.mu.Unlock()
	bf.count++
	for i := 0; i < bf.k; i++ {
		idx := bf.hash(data, uint(i))
		bf.bits[idx] = true
	}
}

func (bf *BloomFilter) Added() uint {
	bf.mu.RLock()
	defer bf.mu.RUnlock()
	return bf.count
}

func (bf *BloomFilter) Contains(data []byte) bool {
	bf.mu.RLock()
	defer bf.mu.RUnlock()
	for i := 0; i < bf.k; i++ {
		idx := bf.hash(data, uint(i))
		if !bf.bits[idx] {
			return false
		}
	}
	return true
}

func (bf *BloomFilter) Count() uint {
	bf.mu.RLock()
	defer bf.mu.RUnlock()
	return bf.count
}

func (bf *BloomFilter) Size() uint {
	return bf.size
}

func (bf *BloomFilter) FalsePositiveRate() float64 {
	bf.mu.RLock()
	defer bf.mu.RUnlock()
	set := float64(bf.count)
	return pow(1-pow(1-1/float64(bf.size), set), float64(bf.k))
}

func (bf *BloomFilter) Reset() {
	bf.mu.Lock()
	defer bf.mu.Unlock()
	bf.bits = make([]bool, bf.size)
	bf.count = 0
}

func (bf *BloomFilter) FillRatio() float64 {
	bf.mu.RLock()
	defer bf.mu.RUnlock()
	return float64(bf.count) / float64(bf.size)
}

func pow(base, exp float64) float64 {
	result := 1.0
	for i := 0; i < int(exp); i++ {
		result *= base
	}
	return result
}
