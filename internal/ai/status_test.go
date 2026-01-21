package ai

import (
	"testing"
	"time"
)

func TestNewAIStatus(t *testing.T) {
	status := NewAIStatus()

	if status.CircuitState != CircuitClosed {
		t.Errorf("expected CircuitClosed, got %v", status.CircuitState)
	}

	if !status.ModelAvailable {
		t.Error("expected model to be available")
	}

	if status.LastPingTime.IsZero() {
		t.Error("expected LastPingTime to be set")
	}
}

func TestRecordRequest(t *testing.T) {
	status := NewAIStatus()

	status.RecordRequest(true, 100*time.Millisecond)

	if status.TotalRequests != 1 {
		t.Errorf("expected 1 request, got %d", status.TotalRequests)
	}

	if status.SuccessfulRequests != 1 {
		t.Errorf("expected 1 successful request, got %d", status.SuccessfulRequests)
	}

	if status.FailedRequests != 0 {
		t.Errorf("expected 0 failed requests, got %d", status.FailedRequests)
	}

	if status.LastLatency != 100*time.Millisecond {
		t.Errorf("expected 100ms latency, got %v", status.LastLatency)
	}

	status.RecordRequest(false, 200*time.Millisecond)
	status.RecordRequest(true, 150*time.Millisecond)

	if status.TotalRequests != 3 {
		t.Errorf("expected 3 requests, got %d", status.TotalRequests)
	}

	if status.SuccessfulRequests != 2 {
		t.Errorf("expected 2 successful requests, got %d", status.SuccessfulRequests)
	}

	if status.FailedRequests != 1 {
		t.Errorf("expected 1 failed request, got %d", status.FailedRequests)
	}

	expectedAvg := (100*time.Millisecond*9 + 200*time.Millisecond) / 10
	if status.AverageLatency < expectedAvg-1*time.Millisecond || status.AverageLatency > expectedAvg+5*time.Millisecond {
		t.Errorf("expected average latency around %v, got %v", expectedAvg, status.AverageLatency)
	}
}

func TestUpdateCircuitStatus(t *testing.T) {
	status := NewAIStatus()

	failTime := time.Now()
	status.UpdateCircuitStatus(CircuitOpen, 5, &failTime)

	if status.CircuitState != CircuitOpen {
		t.Errorf("expected CircuitOpen, got %v", status.CircuitState)
	}

	if status.FailureCount != 5 {
		t.Errorf("expected 5 failures, got %d", status.FailureCount)
	}

	if status.OpenTime == nil {
		t.Error("expected OpenTime to be set")
	}

	status.UpdateCircuitStatus(CircuitClosed, 0, nil)

	if status.CircuitState != CircuitClosed {
		t.Errorf("expected CircuitClosed, got %v", status.CircuitState)
	}

	if status.OpenTime != nil {
		t.Error("expected OpenTime to be cleared")
	}
}

func TestUpdateModelAvailability(t *testing.T) {
	status := NewAIStatus()

	status.UpdateModelAvailability(false, "llama3.2")

	if status.ModelAvailable {
		t.Error("expected model to be unavailable")
	}

	if status.ModelName != "llama3.2" {
		t.Errorf("expected model name 'llama3.2', got '%s'", status.ModelName)
	}

	time.Sleep(time.Millisecond)
	status.UpdateModelAvailability(true, "mistral7b")

	if !status.ModelAvailable {
		t.Error("expected model to be available")
	}

	if status.ModelName != "mistral7b" {
		t.Errorf("expected model name 'mistral7b', got '%s'", status.ModelName)
	}

	if status.LastPingTime.Before(time.Now().Add(-time.Second)) {
		t.Error("expected LastPingTime to be recent")
	}
}

func TestUpdateQueueStats(t *testing.T) {
	status := NewAIStatus()

	status.UpdateQueueStats(5, 2, 10, 3)

	if status.PendingItems != 5 {
		t.Errorf("expected 5 pending, got %d", status.PendingItems)
	}

	if status.ProcessingItems != 2 {
		t.Errorf("expected 2 processing, got %d", status.ProcessingItems)
	}

	if status.CompletedItems != 10 {
		t.Errorf("expected 10 completed, got %d", status.CompletedItems)
	}

	if status.FailedItems != 3 {
		t.Errorf("expected 3 failed, got %d", status.FailedItems)
	}
}

func TestUpdateQueueConfig(t *testing.T) {
	status := NewAIStatus()

	status.UpdateQueueConfig(true, 3, 100)

	if !status.QueueRunning {
		t.Error("expected queue to be running")
	}

	if status.QueueWorkers != 3 {
		t.Errorf("expected 3 workers, got %d", status.QueueWorkers)
	}

	if status.QueueCapacity != 100 {
		t.Errorf("expected 100 capacity, got %d", status.QueueCapacity)
	}
}

func TestGetStatus(t *testing.T) {
	status := NewAIStatus()

	status.RecordRequest(true, 100*time.Millisecond)
	status.UpdateModelAvailability(true, "llama3.2")
	status.UpdateQueueStats(1, 0, 0, 0)

	snapshot := status.GetStatus()

	if snapshot.TotalRequests != 1 {
		t.Errorf("expected 1 request, got %d", snapshot.TotalRequests)
	}

	if snapshot.ModelName != "llama3.2" {
		t.Errorf("expected model name 'llama3.2', got '%s'", snapshot.ModelName)
	}

	if snapshot.PendingItems != 1 {
		t.Errorf("expected 1 pending, got %d", snapshot.PendingItems)
	}
}

func TestAIStatusSnapshot_SuccessRate(t *testing.T) {
	var snap AIStatusSnapshot

	rate := snap.SuccessRate()
	if rate != 100.0 {
		t.Errorf("expected 100%% success rate with 0 requests, got %v", rate)
	}

	snap.TotalRequests = 100
	snap.SuccessfulRequests = 80

	rate = snap.SuccessRate()
	if rate != 80.0 {
		t.Errorf("expected 80%% success rate, got %v", rate)
	}

	snap.TotalRequests = 200
	snap.SuccessfulRequests = 100

	rate = snap.SuccessRate()
	if rate != 50.0 {
		t.Errorf("expected 50%% success rate, got %v", rate)
	}
}

func TestAIStatusSnapshot_IsHealthy(t *testing.T) {
	snap := AIStatusSnapshot{
		CircuitState:       CircuitClosed,
		ModelAvailable:     true,
		TotalRequests:      100,
		SuccessfulRequests: 80,
	}

	if !snap.IsHealthy() {
		t.Error("expected healthy state with 80% success rate")
	}

	snap.SuccessfulRequests = 40
	if snap.IsHealthy() {
		t.Error("expected unhealthy state with 40%% success rate")
	}

	snap.CircuitState = CircuitOpen
	if snap.IsHealthy() {
		t.Error("expected unhealthy state with open circuit")
	}

	snap.CircuitState = CircuitClosed
	snap.SuccessfulRequests = 80

	snap.ModelAvailable = false
	if snap.IsHealthy() {
		t.Error("expected unhealthy state with unavailable model")
	}
}
