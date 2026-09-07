package stream

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestTopic_PartitionHashing(t *testing.T) {
	topic := NewTopic("events", 4)

	rec1 := topic.Publish("user-1", []byte("payload-1"), nil)
	rec2 := topic.Publish("user-1", []byte("payload-2"), nil)

	// Same key must always map to the exact same partition
	if rec1.Partition != rec2.Partition {
		t.Errorf("same key mapped to different partitions: %d != %d", rec1.Partition, rec2.Partition)
	}

	if rec2.Offset <= rec1.Offset {
		t.Errorf("offsets must increase monotonically: %d <= %d", rec2.Offset, rec1.Offset)
	}
}

func TestConsumerGroup_Rebalance(t *testing.T) {
	topic := NewTopic("tasks", 4)
	group := NewConsumerGroup("workers", topic)

	// Consumer 1 joins: should own all 4 partitions
	group.Join("c1")
	assigned := group.GetAssignments("c1")
	if len(assigned) != 4 {
		t.Fatalf("expected 4 partitions assigned to c1, got %d", len(assigned))
	}

	// Consumer 2 joins: each should now own 2 partitions
	group.Join("c2")
	a1 := group.GetAssignments("c1")
	a2 := group.GetAssignments("c2")
	if len(a1) != 2 || len(a2) != 2 {
		t.Errorf("expected 2 partitions each, got c1=%d, c2=%d", len(a1), len(a2))
	}

	// Consumer 1 leaves: c2 should inherit all 4
	group.Leave("c1")
	a2After := group.GetAssignments("c2")
	if len(a2After) != 4 {
		t.Errorf("expected c2 to inherit all 4 partitions after c1 leaves, got %d", len(a2After))
	}
}

func TestConsumerGroup_CommitAndResume(t *testing.T) {
	topic := NewTopic("logs", 2)
	group := NewConsumerGroup("audit", topic)
	group.Join("auditor")

	ctx := context.Background()

	// Publish 5 records to partition 0
	for i := 0; i < 5; i++ {
		_ = topic.Partitions[0].Append("k", []byte(fmt.Sprintf("msg-%d", i)), nil)
	}

	// Read first batch of 3
	recs, err := group.FetchMessages(ctx, "auditor", 3)
	if err != nil {
		t.Fatalf("FetchMessages failed: %v", err)
	}
	if len(recs) != 3 {
		t.Fatalf("expected 3 records, got %d", len(recs))
	}
	if !bytes.Equal(recs[0].Value, []byte("msg-0")) {
		t.Errorf("unexpected first message: %s", string(recs[0].Value))
	}

	// Commit offset 3
	group.CommitOffset(0, 3)

	// Next fetch should resume from offset 3
	recs2, err := group.FetchMessages(ctx, "auditor", 5)
	if err != nil {
		t.Fatalf("FetchMessages 2 failed: %v", err)
	}
	if len(recs2) != 2 {
		t.Fatalf("expected 2 remaining records, got %d", len(recs2))
	}
	if !bytes.Equal(recs2[0].Value, []byte("msg-3")) {
		t.Errorf("expected offset 3 (msg-3), got %s", string(recs2[0].Value))
	}
}

func TestStream_Concurrency(t *testing.T) {
	topic := NewTopic("concurrent-stream", 4)
	concurrency := 10
	recsPerWorker := 50

	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < recsPerWorker; j++ {
				key := fmt.Sprintf("key-%d", (id+j)%4)
				_ = topic.Publish(key, []byte("data"), nil)
			}
		}(i)
	}
	wg.Wait()

	total := 0
	for _, p := range topic.Partitions {
		total += int(p.HighWaterMark())
	}
	if total != concurrency*recsPerWorker {
		t.Errorf("expected %d total records, got %d", concurrency*recsPerWorker, total)
	}
}
