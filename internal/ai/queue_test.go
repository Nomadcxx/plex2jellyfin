package ai

import (
	"testing"
	"time"
)

func TestBackgroundQueue_Enqueue(t *testing.T) {
	queue := NewBackgroundQueue(10, 1, time.Millisecond)

	req := &EnhancementRequest{
		ID:        "test-1",
		Filename:  "test.mkv",
		UserTitle: "Test Movie",
		UserType:  "movie",
		Timestamp: time.Now(),
	}

	err := queue.Enqueue(req)
	if err != nil {
		t.Fatalf("expected no error enqueuing request, got %v", err)
	}

	item, exists := queue.Status("test-1")
	if !exists {
		t.Fatal("expected item to exist in queue")
	}

	if item.Status != StatusPending {
		t.Errorf("expected status %s, got %s", StatusPending, item.Status)
	}
}

func TestBackgroundQueue_EnqueueAfterShutdown(t *testing.T) {
	queue := NewBackgroundQueue(10, 1, time.Millisecond)
	queue.Stop()

	req := &EnhancementRequest{
		ID:        "test-1",
		Filename:  "test.mkv",
		UserTitle: "Test Movie",
		UserType:  "movie",
		Timestamp: time.Now(),
	}

	err := queue.Enqueue(req)
	if err != ErrQueueShutdown {
		t.Errorf("expected ErrQueueShutdown, got %v", err)
	}
}

func TestBackgroundQueue_EnqueueInvalidRequest(t *testing.T) {
	queue := NewBackgroundQueue(10, 1, time.Millisecond)

	req := &EnhancementRequest{
		ID:        "",
		Filename:  "test.mkv",
		UserTitle: "Test Movie",
		UserType:  "movie",
		Timestamp: time.Now(),
	}

	err := queue.Enqueue(req)
	if err != ErrInvalidRequest {
		t.Errorf("expected ErrInvalidRequest, got %v", err)
	}
}

func TestBackgroundQueue_EnqueueQueueFull(t *testing.T) {
	queue := NewBackgroundQueue(2, 1, time.Millisecond)

	for i := 0; i < 3; i++ {
		req := &EnhancementRequest{
			ID:        string(rune('a' + i)),
			Filename:  "test.mkv",
			UserTitle: "Test Movie",
			UserType:  "movie",
			Timestamp: time.Now(),
		}

		err := queue.Enqueue(req)
		if i < 2 && err != nil {
			t.Fatalf("expected no error for item %d, got %v", i, err)
		}
		if i >= 2 && err != ErrQueueFull {
			t.Errorf("expected ErrQueueFull for item %d, got %v", i, err)
		}
	}
}

func TestBackgroundQueue_Dequeue(t *testing.T) {
	queue := NewBackgroundQueue(10, 1, time.Millisecond)

	req := &EnhancementRequest{
		ID:        "test-1",
		Filename:  "test.mkv",
		UserTitle: "Test Movie",
		UserType:  "movie",
		Timestamp: time.Now(),
	}

	queue.Enqueue(req)

	item := queue.Dequeue()
	if item == nil {
		t.Fatal("expected item to be dequeued")
	}

	if item.Status != StatusProcessing {
		t.Errorf("expected status %s, got %s", StatusProcessing, item.Status)
	}

	if item.Attempts != 1 {
		t.Errorf("expected 1 attempt, got %d", item.Attempts)
	}
}

func TestBackgroundQueue_DequeueEmpty(t *testing.T) {
	queue := NewBackgroundQueue(10, 1, time.Millisecond)

	item := queue.Dequeue()
	if item != nil {
		t.Error("expected nil when dequeueing empty queue")
	}
}

func TestBackgroundQueue_Complete(t *testing.T) {
	queue := NewBackgroundQueue(10, 1, time.Millisecond)

	req := &EnhancementRequest{
		ID:        "test-1",
		Filename:  "test.mkv",
		UserTitle: "Test Movie",
		UserType:  "movie",
		Timestamp: time.Now(),
	}

	queue.Enqueue(req)
	item := queue.Dequeue()

	queue.Complete(item)

	item, exists := queue.Status("test-1")
	if !exists {
		t.Fatal("expected item to exist")
	}

	if item.Status != StatusCompleted {
		t.Errorf("expected status %s, got %s", StatusCompleted, item.Status)
	}
}

func TestBackgroundQueue_Fail(t *testing.T) {
	queue := NewBackgroundQueue(10, 1, time.Millisecond)

	req := &EnhancementRequest{
		ID:        "test-1",
		Filename:  "test.mkv",
		UserTitle: "Test Movie",
		UserType:  "movie",
		Timestamp: time.Now(),
	}

	queue.Enqueue(req)
	item := queue.Dequeue()

	queue.Fail(item, &testError{msg: "test error"})

	item, exists := queue.Status("test-1")
	if !exists {
		t.Fatal("expected item to exist")
	}

	if item.Status != StatusFailed {
		t.Errorf("expected status %s, got %s", StatusFailed, item.Status)
	}

	if item.Error != "test error" {
		t.Errorf("expected error 'test error', got '%s'", item.Error)
	}
}

func TestBackgroundQueue_QueueStats(t *testing.T) {
	queue := NewBackgroundQueue(10, 1, time.Millisecond)

	req1 := &EnhancementRequest{
		ID:        "test-1",
		Filename:  "test1.mkv",
		UserTitle: "Test Movie 1",
		UserType:  "movie",
		Timestamp: time.Now(),
	}

	req2 := &EnhancementRequest{
		ID:        "test-2",
		Filename:  "test2.mkv",
		UserTitle: "Test Movie 2",
		UserType:  "movie",
		Timestamp: time.Now(),
	}

	req3 := &EnhancementRequest{
		ID:        "test-3",
		Filename:  "test3.mkv",
		UserTitle: "Test Movie 3",
		UserType:  "movie",
		Timestamp: time.Now(),
	}

	queue.Enqueue(req1)
	queue.Enqueue(req2)
	queue.Enqueue(req3)

	item1 := queue.Dequeue()
	item2 := queue.Dequeue()
	_ = queue.Dequeue()

	queue.Complete(item1)
	queue.Fail(item2, &testError{msg: "failed"})

	stats := queue.QueueStats()

	if stats.Pending != 0 {
		t.Errorf("expected 0 pending, got %d", stats.Pending)
	}
	if stats.Processing != 1 {
		t.Errorf("expected 1 processing, got %d", stats.Processing)
	}
	if stats.Completed != 1 {
		t.Errorf("expected 1 completed, got %d", stats.Completed)
	}
	if stats.Failed != 1 {
		t.Errorf("expected 1 failed, got %d", stats.Failed)
	}
	if stats.Total != 3 {
		t.Errorf("expected 3 total, got %d", stats.Total)
	}
}

func TestBackgroundQueue_Stop(t *testing.T) {
	queue := NewBackgroundQueue(10, 1, time.Millisecond)

	req := &EnhancementRequest{
		ID:        "test-1",
		Filename:  "test.mkv",
		UserTitle: "Test Movie",
		UserType:  "movie",
		Timestamp: time.Now(),
	}

	queue.Enqueue(req)
	queue.Stop()

	if !queue.IsStopped() {
		t.Error("expected queue to be stopped")
	}

	err := queue.Enqueue(req)
	if err != ErrQueueShutdown {
		t.Errorf("expected ErrQueueShutdown after stop, got %v", err)
	}
}

func TestBackgroundQueue_Output(t *testing.T) {
	queue := NewBackgroundQueue(10, 1, time.Millisecond)

	req := &EnhancementRequest{
		ID:        "test-1",
		Filename:  "test.mkv",
		UserTitle: "Test Movie",
		UserType:  "movie",
		Timestamp: time.Now(),
	}

	queue.Enqueue(req)
	item := queue.Dequeue()

	queue.Complete(item)

	select {
	case outputItem := <-queue.Output():
		if outputItem.Status != StatusCompleted {
			t.Errorf("expected status %s in output channel, got %s", StatusCompleted, outputItem.Status)
		}
	default:
		t.Error("expected item in output channel")
	}
}
