package jellyfin

import (
	"encoding/json"
	"testing"
)

func TestWebhookEventUnmarshalNestedTaskDetails(t *testing.T) {
	var event WebhookEvent
	err := json.Unmarshal([]byte(`{"EventType":"TaskCompleted","Payload":{"Task":{"Name":"Scan Media Library","Category":"Library"}}}`), &event)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if event.NotificationType != EventTaskCompleted || event.TaskName != "Scan Media Library" || event.TaskCategory != "Library" {
		t.Fatalf("event = %+v", event)
	}
}
