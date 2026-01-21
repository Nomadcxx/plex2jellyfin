package database

import (
	"testing"
)

func TestUpsertAIImprovement_Insert(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	imp := &AIImprovement{
		RequestID:       "test-req-1",
		Filename:        "Test.Movie.2024.1080p.mkv",
		UserTitle:       "Test Movie",
		UserType:        "movie",
		UserYear:        intPtr(2024),
		AITitle:         strPtr("test movie"),
		AIType:          strPtr("movie"),
		AIYear:          intPtr(2024),
		AIConfidence:    float64Ptr(0.75),
		Status:          "pending",
		Attempts:        0,
		Model:           strPtr("llama3.2"),
		OriginalRequest: strPtr(`{"title": "Test Movie", "type": "movie"}`),
	}

	err := db.UpsertAIImprovement(imp)
	if err != nil {
		t.Fatalf("expected no error upserting improvement, got %v", err)
	}

	if imp.ID == 0 {
		t.Error("expected ID to be set")
	}
}

func TestUpsertAIImprovement_Update(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	imp := &AIImprovement{
		RequestID: "test-req-1",
		Filename:  "test.mkv",
		UserTitle: "Original Title",
		UserType:  "movie",
		Status:    "pending",
	}

	err := db.UpsertAIImprovement(imp)
	if err != nil {
		t.Fatalf("failed to insert: %v", err)
	}

	originalID := imp.ID
	imp.UserTitle = "Updated Title"
	imp.Status = "completed"

	err = db.UpsertAIImprovement(imp)
	if err != nil {
		t.Fatalf("failed to update: %v", err)
	}

	if imp.ID != originalID {
		t.Errorf("expected ID %d, got %d", originalID, imp.ID)
	}

	retrieved, err := db.GetAIImprovement("test-req-1")
	if err != nil {
		t.Fatalf("failed to retrieve: %v", err)
	}

	if retrieved.UserTitle != "Updated Title" {
		t.Errorf("expected title 'Updated Title', got '%s'", retrieved.UserTitle)
	}

	if retrieved.Status != "completed" {
		t.Errorf("expected status 'completed', got '%s'", retrieved.Status)
	}
}

func TestGetAIImprovement(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	imp := &AIImprovement{
		RequestID: "test-req-1",
		Filename:  "test.mkv",
		UserTitle: "Test Movie",
		UserType:  "movie",
		Status:    "pending",
	}

	err := db.UpsertAIImprovement(imp)
	if err != nil {
		t.Fatalf("failed to insert: %v", err)
	}

	retrieved, err := db.GetAIImprovement("test-req-1")
	if err != nil {
		t.Fatalf("failed to retrieve: %v", err)
	}

	if retrieved.RequestID != "test-req-1" {
		t.Errorf("expected request_id 'test-req-1', got '%s'", retrieved.RequestID)
	}

	if retrieved.UserTitle != "Test Movie" {
		t.Errorf("expected title 'Test Movie', got '%s'", retrieved.UserTitle)
	}

	nonExistent, err := db.GetAIImprovement("non-existent")
	if err != nil {
		t.Errorf("expected no error for non-existent, got %v", err)
	}
	if nonExistent != nil {
		t.Error("expected nil for non-existent request")
	}
}

func TestGetPendingAIImprovements(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	for i := 0; i < 3; i++ {
		imp := &AIImprovement{
			RequestID: string(rune('a' + i)),
			Filename:  "test.mkv",
			UserTitle: "Test Movie",
			UserType:  "movie",
			Status:    "pending",
		}
		if err := db.UpsertAIImprovement(imp); err != nil {
			t.Fatalf("failed to insert: %v", err)
		}
	}

	imp2 := &AIImprovement{
		RequestID: "completed-1",
		Filename:  "test2.mkv",
		UserTitle: "Test Movie 2",
		UserType:  "movie",
		Status:    "completed",
	}
	if err := db.UpsertAIImprovement(imp2); err != nil {
		t.Fatalf("failed to insert: %v", err)
	}

	pending, err := db.GetPendingAIImprovements(10)
	if err != nil {
		t.Fatalf("failed to get pending: %v", err)
	}

	if len(pending) != 3 {
		t.Errorf("expected 3 pending improvements, got %d", len(pending))
	}
}

func TestUpdateAIImprovementStatus(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	imp := &AIImprovement{
		RequestID: "test-req-1",
		Filename:  "test.mkv",
		UserTitle: "Test Movie",
		UserType:  "movie",
		Status:    "pending",
		Attempts:  0,
	}

	if err := db.UpsertAIImprovement(imp); err != nil {
		t.Fatalf("failed to insert: %v", err)
	}

	err := db.UpdateAIImprovementStatus("test-req-1", "processing", "")
	if err != nil {
		t.Fatalf("failed to update status: %v", err)
	}

	retrieved, _ := db.GetAIImprovement("test-req-1")
	if retrieved.Status != "processing" {
		t.Errorf("expected status 'processing', got '%s'", retrieved.Status)
	}

	err = db.UpdateAIImprovementStatus("test-req-1", "completed", "")
	if err != nil {
		t.Fatalf("failed to complete: %v", err)
	}

	retrieved, _ = db.GetAIImprovement("test-req-1")
	if retrieved.Status != "completed" {
		t.Errorf("expected status 'completed', got '%s'", retrieved.Status)
	}

	if retrieved.CompletedAt == nil {
		t.Error("expected completed_at to be set")
	}

	err = db.UpdateAIImprovementStatus("test-req-1", "failed", "test error")
	if err != nil {
		t.Fatalf("failed to fail: %v", err)
	}

	retrieved, _ = db.GetAIImprovement("test-req-1")
	if retrieved.Status != "failed" {
		t.Errorf("expected status 'failed', got '%s'", retrieved.Status)
	}

	if retrieved.ErrorMessage == nil || *retrieved.ErrorMessage != "test error" {
		t.Errorf("expected error message 'test error', got %v", retrieved.ErrorMessage)
	}

	if retrieved.Attempts != 1 {
		t.Errorf("expected 1 attempt, got %d", retrieved.Attempts)
	}
}

func TestDeleteAIImprovement(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	imp := &AIImprovement{
		RequestID: "test-req-1",
		Filename:  "test.mkv",
		UserTitle: "Test Movie",
		UserType:  "movie",
		Status:    "pending",
	}

	if err := db.UpsertAIImprovement(imp); err != nil {
		t.Fatalf("failed to insert: %v", err)
	}

	err := db.DeleteAIImprovement("test-req-1")
	if err != nil {
		t.Fatalf("failed to delete: %v", err)
	}

	retrieved, err := db.GetAIImprovement("test-req-1")
	if err != nil {
		t.Errorf("expected no error deleting non-existent, got %v", err)
	}
	if retrieved != nil {
		t.Error("expected nil after deletion")
	}
}

func TestGetAIImprovementsByModel(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	model1 := "llama3.2"
	model2 := "mistral7b"

	for i := 0; i < 3; i++ {
		imp := &AIImprovement{
			RequestID: string(rune('a' + i)),
			Filename:  "test.mkv",
			UserTitle: "Test Movie",
			UserType:  "movie",
			Status:    "pending",
			Model:     &model1,
		}
		if err := db.UpsertAIImprovement(imp); err != nil {
			t.Fatalf("failed to insert: %v", err)
		}
	}

	imp := &AIImprovement{
		RequestID: "model2-1",
		Filename:  "test2.mkv",
		UserTitle: "Test Movie 2",
		UserType:  "movie",
		Status:    "pending",
		Model:     &model2,
	}
	if err := db.UpsertAIImprovement(imp); err != nil {
		t.Fatalf("failed to insert: %v", err)
	}

	results, err := db.GetAIImprovementsByModel(model1, 10)
	if err != nil {
		t.Fatalf("failed to get by model: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("expected 3 results for %s, got %d", model1, len(results))
	}

	for _, result := range results {
		if result.Model == nil || *result.Model != model1 {
			t.Errorf("expected model %s, got %v", model1, result.Model)
		}
	}
}

func TestCountAIImprovementsByStatus(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	for i := 0; i < 3; i++ {
		imp := &AIImprovement{
			RequestID: string(rune('a' + i)),
			Filename:  "test.mkv",
			UserTitle: "Test Movie",
			UserType:  "movie",
			Status:    "pending",
		}
		if err := db.UpsertAIImprovement(imp); err != nil {
			t.Fatalf("failed to insert: %v", err)
		}
	}

	for i := 0; i < 2; i++ {
		imp := &AIImprovement{
			RequestID: string(rune('x' + i)),
			Filename:  "test2.mkv",
			UserTitle: "Test Movie 2",
			UserType:  "movie",
			Status:    "completed",
		}
		if err := db.UpsertAIImprovement(imp); err != nil {
			t.Fatalf("failed to insert: %v", err)
		}
	}

	pendingCount, err := db.CountAIImprovementsByStatus("pending")
	if err != nil {
		t.Fatalf("failed to count pending: %v", err)
	}

	if pendingCount != 3 {
		t.Errorf("expected 3 pending, got %d", pendingCount)
	}

	completedCount, err := db.CountAIImprovementsByStatus("completed")
	if err != nil {
		t.Fatalf("failed to count completed: %v", err)
	}

	if completedCount != 2 {
		t.Errorf("expected 2 completed, got %d", completedCount)
	}
}

func intPtr(i int) *int {
	return &i
}

func strPtr(s string) *string {
	return &s
}

func float64Ptr(f float64) *float64 {
	return &f
}
