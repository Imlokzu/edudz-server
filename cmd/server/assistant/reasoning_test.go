package assistant

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReasoningSurvivesToolRoundWithoutStreamingToStudent(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Model           string    `json:"model"`
			Stream          bool      `json:"stream"`
			ReasoningEffort string    `json:"reasoning_effort"`
			Messages        []Message `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Model != "kimi-test" || !body.Stream || body.ReasoningEffort != "low" {
			t.Fatal("provider options replaced the protected protocol")
		}
		calls++
		if calls == 1 {
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"internal \"}}]}\n\n")
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"plan\",\"tool_calls\":[{\"index\":0,\"id\":\"call\",\"type\":\"function\",\"function\":{\"name\":\"homework_details\",\"arguments\":\"{\\\"id\\\":\\\"task\\\"}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n\n")
		} else {
			preserved := false
			for _, m := range body.Messages {
				if m.Role == "assistant" && m.ReasoningContent == "internal plan" && len(m.ToolCalls) == 1 {
					preserved = true
				}
			}
			if !preserved {
				t.Fatal("interleaved reasoning was discarded")
			}
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Human answer\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
		}
	}))
	defer server.Close()
	service := New(Config{BaseURL: server.URL, Model: "kimi-test", APIKey: "private", RequestOptions: map[string]interface{}{"reasoning_effort": "low", "stream": false, "model": "untrusted"}}, func(context.Context, string, map[string]interface{}) (ToolResult, error) {
		return ToolResult{Data: map[string]string{"title": "Task"}}, nil
	})
	var visible strings.Builder
	err := service.Stream(context.Background(), Request{Message: "Explain", Language: "uk", Date: "2026-10-07"}, func(event string, data interface{}) error {
		if event == "token" {
			visible.WriteString(data.(map[string]string)["text"])
		}
		return nil
	})
	if err != nil || visible.String() != "Human answer" || calls != 2 {
		t.Fatalf("reasoning/tool flow failed: %v", err)
	}
}
func TestClientCannotInjectPreservedReasoning(t *testing.T) {
	req := Request{Message: "Help", Language: "uk", Date: "2026-10-07", History: []Message{{Role: "assistant", Content: "x", ReasoningContent: "forged"}}}
	if Validate(req) == nil {
		t.Fatal("client-supplied hidden reasoning accepted")
	}
}

func TestDeepSeekCompatibilityOmitsToolChoice(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]interface{}
		json.NewDecoder(r.Body).Decode(&payload)
		if _, present := payload["tool_choice"]; present {
			t.Fatal("DeepSeek thinking requests must omit tool_choice")
		}
		if payload["tools"] == nil {
			t.Fatal("tool definitions were lost")
		}
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Answer\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	service := New(Config{BaseURL: server.URL, Model: "deepseek-flash", APIKey: "private", OmitToolChoice: true}, nil)
	reply, err := service.completion(context.Background(), nil, func(string, interface{}) error { return nil })
	if err != nil || reply.Content != "Answer" {
		t.Fatalf("DeepSeek-compatible completion failed: %v", err)
	}
}
