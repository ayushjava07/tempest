package stream

import (
	"context"
	"errors"
	"hash/fnv"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrTopicNotFound      = errors.New("stream: topic not found")
	ErrPartitionNotFound  = errors.New("stream: partition index out of range")
	ErrOffsetOutOfRange   = errors.New("stream: offset out of range")
	ErrNoConsumersInGroup = errors.New("stream: no active consumers in consumer group")
)

// Record represents an individual sequenced message within a partition.
type Record struct {
	Topic     string
	Partition int
	Offset    uint64
	Key       string
	Value     []byte
	Headers   map[string]string
	Timestamp time.Time
}

// Partition manages an append-only sequence of records.
type Partition struct {
	mu      sync.RWMutex
	id      int
	records []Record
	nextOff atomic.Uint64
}

func NewPartition(id int) *Partition {
	return &Partition{
		id:      id,
		records: make([]Record, 0, 128),
	}
}

func (p *Partition) Append(key string, val []byte, headers map[string]string) Record {
	p.mu.Lock()
	defer p.mu.Unlock()

	off := p.nextOff.Add(1) - 1
	rec := Record{
		Partition: p.id,
		Offset:    off,
		Key:       key,
		Value:     append([]byte(nil), val...),
		Headers:   headers,
		Timestamp: time.Now().UTC(),
	}
	p.records = append(p.records, rec)
	return rec
}

func (p *Partition) ReadFrom(offset uint64, maxRecords int) ([]Record, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if offset > p.nextOff.Load() {
		return nil, ErrOffsetOutOfRange
	}

	var out []Record
	for _, r := range p.records {
		if r.Offset >= offset {
			out = append(out, r)
			if maxRecords > 0 && len(out) >= maxRecords {
				break
			}
		}
	}
	return out, nil
}

func (p *Partition) HighWaterMark() uint64 {
	return p.nextOff.Load()
}

// Topic holds multiple partitions for horizontal throughput scaling.
type Topic struct {
	Name       string
	Partitions []*Partition
}

func NewTopic(name string, numPartitions int) *Topic {
	if numPartitions <= 0 {
		numPartitions = 4
	}
	t := &Topic{
		Name:       name,
		Partitions: make([]*Partition, numPartitions),
	}
	for i := 0; i < numPartitions; i++ {
		t.Partitions[i] = NewPartition(i)
	}
	return t
}

func (t *Topic) PartitionForKey(key string) int {
	if key == "" {
		return 0
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return int(h.Sum32() % uint32(len(t.Partitions)))
}

func (t *Topic) Publish(key string, val []byte, headers map[string]string) Record {
	partID := t.PartitionForKey(key)
	rec := t.Partitions[partID].Append(key, val, headers)
	rec.Topic = t.Name
	return rec
}

// ConsumerGroup coordinates balanced partition assignment among consumer workers.
type ConsumerGroup struct {
	mu          sync.RWMutex
	name        string
	topic       *Topic
	consumers   map[string]bool  // consumerID -> active
	assignments map[string][]int // consumerID -> assigned partition IDs
	offsets     map[int]uint64   // partitionID -> committed offset
}

func NewConsumerGroup(name string, topic *Topic) *ConsumerGroup {
	return &ConsumerGroup{
		name:        name,
		topic:       topic,
		consumers:   make(map[string]bool),
		assignments: make(map[string][]int),
		offsets:     make(map[int]uint64),
	}
}

// Join registers a consumer into the group and triggers partition rebalancing.
func (g *ConsumerGroup) Join(consumerID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.consumers[consumerID] = true
	g.rebalanceLocked()
}

// Leave removes a consumer and triggers partition rebalancing.
func (g *ConsumerGroup) Leave(consumerID string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.consumers, consumerID)
	delete(g.assignments, consumerID)
	g.rebalanceLocked()
}

func (g *ConsumerGroup) rebalanceLocked() {
	g.assignments = make(map[string][]int)
	activeList := make([]string, 0, len(g.consumers))
	for c := range g.consumers {
		activeList = append(activeList, c)
	}

	if len(activeList) == 0 {
		return
	}

	// Round-robin partition distribution
	for partID := 0; partID < len(g.topic.Partitions); partID++ {
		consumer := activeList[partID%len(activeList)]
		g.assignments[consumer] = append(g.assignments[consumer], partID)
	}
}

// GetAssignments returns partitions currently owned by consumerID.
func (g *ConsumerGroup) GetAssignments(consumerID string) []int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	parts := g.assignments[consumerID]
	return append([]int(nil), parts...)
}

// CommitOffset saves consumer progress for a partition.
func (g *ConsumerGroup) CommitOffset(partitionID int, offset uint64) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.offsets[partitionID] = offset
}

// FetchOffset retrieves the last committed offset for a partition.
func (g *ConsumerGroup) FetchOffset(partitionID int) uint64 {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.offsets[partitionID]
}

// FetchMessages polls new uncommitted records for the specified consumer.
func (g *ConsumerGroup) FetchMessages(ctx context.Context, consumerID string, maxPerPart int) ([]Record, error) {
	g.mu.RLock()
	assigned, ok := g.assignments[consumerID]
	g.mu.RUnlock()

	if !ok || len(assigned) == 0 {
		return nil, nil
	}

	var all []Record
	for _, partID := range assigned {
		lastOff := g.FetchOffset(partID)
		recs, err := g.topic.Partitions[partID].ReadFrom(lastOff, maxPerPart)
		if err == nil && len(recs) > 0 {
			all = append(all, recs...)
		}
	}
	return all, nil
}
