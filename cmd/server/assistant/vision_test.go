package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAttachmentVisionUsesOneImagePerRequest(t *testing.T) {
	streamCalls := 0
	visionCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Stream   bool      `json:"stream"`
			Messages []Message `json:"messages"`
		}
		json.NewDecoder(r.Body).Decode(&payload)
		images := 0
		for _, m := range payload.Messages {
			if content, ok := m.Content.([]interface{}); ok {
				for _, v := range content {
					if value, ok := v.(map[string]interface{}); ok && value["type"] == "image_url" {
						images++
					}
				}
			}
		}
		if !payload.Stream {
			visionCalls++
			if images != 1 {
				t.Fatalf("vision received %d images", images)
			}
			fmt.Fprint(w, `{"choices":[{"message":{"content":"Worksheet text"}}]}`)
			return
		}
		if images != 0 {
			t.Fatal("streaming tool completion received images")
		}
		streamCalls++
		if streamCalls == 1 {
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"f\",\"type\":\"function\",\"function\":{\"name\":\"read_attachment\",\"arguments\":\"{\\\"src\\\":\\\"/cloud/file.pdf\\\"}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n\n")
		} else {
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Explanation\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
		}
	}))
	defer server.Close()
	service := New(Config{BaseURL: server.URL, Model: "vision", APIKey: "private"}, func(_ context.Context, name string, args map[string]interface{}) (ToolResult, error) {
		if name == "read_attachment" {
			return ToolResult{Data: map[string]interface{}{}, Images: []string{"data:image/png;base64,a", "data:image/png;base64,b"}}, nil
		}
		return ToolResult{Data: map[string]string{}}, nil
	})
	err := service.Stream(context.Background(), Request{Message: "Read it", Language: "uk", Date: "2026-10-07"}, func(string, interface{}) error { return nil })
	if err != nil || visionCalls != 2 || streamCalls != 2 {
		t.Fatalf("vision flow failed: %v", err)
	}
}
