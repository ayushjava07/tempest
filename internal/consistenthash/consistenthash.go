package consistenthash

import (
	"hash/crc32"
	"sort"
	"sync"
)

type HashRing struct {
	mu         sync.RWMutex
	ring       map[uint32]string
	sortedKeys []uint32
	nodes      map[string]bool
	replicas   int
}

func New(replicas int) *HashRing {
	if replicas <= 0 {
		replicas = 150
	}
	return &HashRing{
		ring:     make(map[uint32]string),
		nodes:    make(map[string]bool),
		replicas: replicas,
	}
}

func (hr *HashRing) hash(key string) uint32 {
	return crc32.ChecksumIEEE([]byte(key))
}

func (hr *HashRing) Add(node string) {
	hr.mu.Lock()
	defer hr.mu.Unlock()
	if hr.nodes[node] {
		return
	}
	hr.nodes[node] = true
	for i := 0; i < hr.replicas; i++ {
		key := hr.hash(node + "-" + string(rune(i)))
		hr.ring[key] = node
		hr.sortedKeys = append(hr.sortedKeys, key)
	}
	sort.Slice(hr.sortedKeys, func(i, j int) bool {
		return hr.sortedKeys[i] < hr.sortedKeys[j]
	})
}

func (hr *HashRing) Remove(node string) {
	hr.mu.Lock()
	defer hr.mu.Unlock()
	if !hr.nodes[node] {
		return
	}
	delete(hr.nodes, node)
	for i := 0; i < hr.replicas; i++ {
		key := hr.hash(node + "-" + string(rune(i)))
		delete(hr.ring, key)
		for j, k := range hr.sortedKeys {
			if k == key {
				hr.sortedKeys = append(hr.sortedKeys[:j], hr.sortedKeys[j+1:]...)
				break
			}
		}
	}
}

func (hr *HashRing) Get(key string) string {
	hr.mu.RLock()
	defer hr.mu.RUnlock()
	if len(hr.sortedKeys) == 0 {
		return ""
	}
	hash := hr.hash(key)
	idx := sort.Search(len(hr.sortedKeys), func(i int) bool {
		return hr.sortedKeys[i] >= hash
	})
	if idx >= len(hr.sortedKeys) {
		idx = 0
	}
	return hr.ring[hr.sortedKeys[idx]]
}

func (hr *HashRing) Len() int {
	hr.mu.RLock()
	defer hr.mu.RUnlock()
	return len(hr.nodes)
}

func (hr *HashRing) Nodes() []string {
	hr.mu.RLock()
	defer hr.mu.RUnlock()
	nodes := make([]string, 0, len(hr.nodes))
	for n := range hr.nodes {
		nodes = append(nodes, n)
	}
	return nodes
}

func (hr *HashRing) Distribution(keys []string) map[string]int {
	hr.mu.RLock()
	defer hr.mu.RUnlock()
	dist := make(map[string]int)
	for _, k := range keys {
		hash := hr.hash(k)
		idx := sort.Search(len(hr.sortedKeys), func(i int) bool {
			return hr.sortedKeys[i] >= hash
		})
		if idx >= len(hr.sortedKeys) {
			idx = 0
		}
		dist[hr.ring[hr.sortedKeys[idx]]]++
	}
	return dist
}
