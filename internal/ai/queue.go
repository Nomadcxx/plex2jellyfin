package ai

import (
	"sync"
	"time"
)

// EnhancementRequest represents a request to enhance AI detection with user feedback.
type EnhancementRequest struct {
	ID               string    `json:"id"`
	Filename         string    `json:"filename"`
	UserTitle        string    `json:"user_title"`
	UserType         string    `json:"user_type"`
	UserYear         *int      `json:"user_year,omitempty"`
	Timestamp        time.Time `json:"timestamp"`
	OriginalAIResult *Result   `json:"original_ai_result,omitempty"`
}

// EnhancementStatus represents the status of an enhancement request.
type EnhancementStatus string

const (
	StatusPending    EnhancementStatus = "pending"
	StatusProcessing EnhancementStatus = "processing"
	StatusCompleted  EnhancementStatus = "completed"
	StatusFailed     EnhancementStatus = "failed"
)

// QueueItem wraps an EnhancementRequest with queue metadata.
type QueueItem struct {
	Request  EnhancementRequest `json:"request"`
	Status   EnhancementStatus  `json:"status"`
	Attempts int                `json:"attempts"`
	AddedAt  time.Time          `json:"added_at"`
	Error    string             `json:"error,omitempty"`
}

// BackgroundQueue manages a queue of enhancement requests to be processed asynchronously.
type BackgroundQueue struct {
	items    map[string]*QueueItem
	itemLock sync.RWMutex

	inputChan  chan *QueueItem
	outputChan chan *QueueItem

	stopChan chan struct{}
	stopped  bool
	stopLock sync.RWMutex

	queueSize  int
	workers    int
	retryDelay time.Duration
}

// NewBackgroundQueue creates a new background processing queue.
func NewBackgroundQueue(queueSize, workers int, retryDelay time.Duration) *BackgroundQueue {
	if queueSize <= 0 {
		queueSize = 100
	}
	if workers <= 0 {
		workers = 1
	}
	if retryDelay <= 0 {
		retryDelay = 100 * time.Millisecond
	}

	q := &BackgroundQueue{
		items:      make(map[string]*QueueItem),
		inputChan:  make(chan *QueueItem, queueSize),
		outputChan: make(chan *QueueItem, queueSize),
		stopChan:   make(chan struct{}),
		queueSize:  queueSize,
		workers:    workers,
		retryDelay: retryDelay,
	}

	return q
}

// Enqueue adds an enhancement request to the queue.
func (q *BackgroundQueue) Enqueue(req *EnhancementRequest) error {
	q.stopLock.RLock()
	stopped := q.stopped
	q.stopLock.RUnlock()

	if stopped {
		return ErrQueueShutdown
	}

	if req.ID == "" {
		return ErrInvalidRequest
	}

	item := &QueueItem{
		Request:  *req,
		Status:   StatusPending,
		Attempts: 0,
		AddedAt:  time.Now(),
	}

	q.itemLock.Lock()
	q.items[req.ID] = item
	q.itemLock.Unlock()

	select {
	case q.inputChan <- item:
		return nil
	default:
		q.itemLock.Lock()
		delete(q.items, req.ID)
		q.itemLock.Unlock()
		return ErrQueueFull
	}
}

// Dequeue retrieves the next pending item from the queue.
func (q *BackgroundQueue) Dequeue() *QueueItem {
	select {
	case item := <-q.inputChan:
		q.itemLock.Lock()
		item.Status = StatusProcessing
		item.Attempts++
		q.itemLock.Unlock()
		return item
	default:
		return nil
	}
}

// Complete marks a queue item as completed and sends it to the output channel.
func (q *BackgroundQueue) Complete(item *QueueItem) {
	q.itemLock.Lock()
	item.Status = StatusCompleted
	q.itemLock.Unlock()

	select {
	case q.outputChan <- item:
	default:
	}
}

// Fail marks a queue item as failed and sends it to the output channel.
func (q *BackgroundQueue) Fail(item *QueueItem, err error) {
	q.itemLock.Lock()
	item.Status = StatusFailed
	item.Error = err.Error()
	q.itemLock.Unlock()

	select {
	case q.outputChan <- item:
	default:
	}
}

// Output returns a channel for receiving completed/failed items.
func (q *BackgroundQueue) Output() <-chan *QueueItem {
	return q.outputChan
}

// Status returns the current status of a specific request.
func (q *BackgroundQueue) Status(id string) (*QueueItem, bool) {
	q.itemLock.RLock()
	defer q.itemLock.RUnlock()

	item, exists := q.items[id]
	return item, exists
}

// QueueStats returns statistics about the queue.
func (q *BackgroundQueue) QueueStats() QueueStats {
	q.itemLock.RLock()
	defer q.itemLock.RUnlock()

	stats := QueueStats{
		Pending:    0,
		Processing: 0,
		Completed:  0,
		Failed:     0,
		Total:      len(q.items),
	}

	for _, item := range q.items {
		switch item.Status {
		case StatusPending:
			stats.Pending++
		case StatusProcessing:
			stats.Processing++
		case StatusCompleted:
			stats.Completed++
		case StatusFailed:
			stats.Failed++
		}
	}

	return stats
}

// Stop gracefully shuts down the queue, processing remaining items.
func (q *BackgroundQueue) Stop() {
	q.stopLock.Lock()
	if q.stopped {
		q.stopLock.Unlock()
		return
	}
	q.stopped = true
	q.stopLock.Unlock()

	close(q.stopChan)

	close(q.inputChan)
	for range q.inputChan {
	}

	time.Sleep(q.retryDelay)
}

// IsStopped returns true if the queue has been stopped.
func (q *BackgroundQueue) IsStopped() bool {
	q.stopLock.RLock()
	defer q.stopLock.RUnlock()
	return q.stopped
}

// QueueStats represents queue statistics.
type QueueStats struct {
	Pending    int `json:"pending"`
	Processing int `json:"processing"`
	Completed  int `json:"completed"`
	Failed     int `json:"failed"`
	Total      int `json:"total"`
}

// Queue errors.
var (
	ErrQueueFull      = &QueueError{msg: "queue is full"}
	ErrQueueShutdown  = &QueueError{msg: "queue is shut down"}
	ErrInvalidRequest = &QueueError{msg: "invalid enhancement request"}
)

// QueueError represents a queue-specific error.
type QueueError struct {
	msg string
}

func (e *QueueError) Error() string {
	return e.msg
}
