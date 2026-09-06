package broadcast

import (
	"testing"
	"time"
)

func TestBroadcast_PublishSubscribe(t *testing.T) {
	b := New[string]()
	_, ch := b.Subscribe(10)
	b.Publish("hello")
	select {
	case msg := <-ch:
		if msg != "hello" {
			t.Errorf("expected hello, got %s", msg)
		}
	case <-time.After(time.Second):
		t.Error("timeout waiting for message")
	}
}

func TestBroadcast_MultipleSubscribers(t *testing.T) {
	b := New[int]()
	_, ch1 := b.Subscribe(10)
	_, ch2 := b.Subscribe(10)
	b.Publish(42)
	select {
	case v := <-ch1:
		if v != 42 {
			t.Errorf("expected 42, got %d", v)
		}
	case <-time.After(time.Second):
		t.Error("timeout")
	}
	select {
	case v := <-ch2:
		if v != 42 {
			t.Errorf("expected 42, got %d", v)
		}
	case <-time.After(time.Second):
		t.Error("timeout")
	}
}

func TestBroadcast_Unsubscribe(t *testing.T) {
	b := New[string]()
	id, _ := b.Subscribe(10)
	b.Unsubscribe(id)
	if b.SubscriberCount() != 0 {
		t.Error("expected 0 subscribers")
	}
}

func TestBroadcast_SubscriberCount(t *testing.T) {
	b := New[int]()
	b.Subscribe(10)
	b.Subscribe(10)
	if b.SubscriberCount() != 2 {
		t.Errorf("expected 2, got %d", b.SubscriberCount())
	}
}

func TestBroadcast_Close(t *testing.T) {
	b := New[string]()
	b.Subscribe(10)
	b.Subscribe(10)
	b.Close()
	if b.SubscriberCount() != 0 {
		t.Error("expected 0 after close")
	}
}

func TestBroadcast_NoBlockOnFull(t *testing.T) {
	b := New[int]()
	_, ch := b.Subscribe(1)
	b.Publish(1)
	b.Publish(2)
	b.Publish(3)
	select {
	case <-ch:
	default:
		t.Error("expected at least one message")
	}
}

func TestBroadcast_UnsubscribeNonexistent(t *testing.T) {
	b := New[int]()
	b.Unsubscribe(999)
	if b.SubscriberCount() != 0 {
		t.Error("expected no change")
	}
}
