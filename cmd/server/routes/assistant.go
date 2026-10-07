package routes

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/DislikesSchool/EduPage2-server/cmd/server/assistant"
	"github.com/DislikesSchool/EduPage2-server/edupage"
	"github.com/gin-gonic/gin"
	"net/http"
	"sync"
	"time"
)

var assistantSlots = make(chan struct{}, 4)
var limits = struct {
	sync.Mutex
	Entries map[string][]time.Time
}{Entries: map[string][]time.Time{}}

func AssistantStatusHandler(c *gin.Context) {
	cfg, err := assistant.LoadConfig()
	c.JSON(200, gin.H{"available": err == nil, "model": cfg.Model})
}
func AssistantHandler(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 128<<10)
	var req assistant.Request
	if c.ShouldBindJSON(&req) != nil || assistant.Validate(req) != nil {
		c.JSON(400, gin.H{"error": "invalid_chat_request"})
		return
	}
	cfg, err := assistant.LoadConfig()
	if err != nil {
		c.JSON(503, gin.H{"error": "assistant_not_configured"})
		return
	}
	client := c.MustGet("client").(*edupage.EdupageClient)
	key := client.Credentials.Username + "@" + client.Credentials.Server
	limits.Lock()
	now := time.Now()
	recent := []time.Time{}
	for _, t := range limits.Entries[key] {
		if now.Sub(t) < time.Minute {
			recent = append(recent, t)
		}
	}
	if len(recent) >= 8 {
		limits.Unlock()
		c.JSON(429, gin.H{"error": "assistant_rate_limit"})
		return
	}
	if len(limits.Entries) > 1000 {
		for k, v := range limits.Entries {
			if len(v) == 0 || now.Sub(v[len(v)-1]) > time.Hour {
				delete(limits.Entries, k)
			}
		}
	}
	limits.Entries[key] = append(recent, now)
	limits.Unlock()
	select {
	case assistantSlots <- struct{}{}:
		defer func() { <-assistantSlots }()
	default:
		c.JSON(429, gin.H{"error": "assistant_busy"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 150*time.Second)
	defer cancel()
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache, no-transform")
	c.Header("X-Accel-Buffering", "no")
	c.Status(200)
	var writeLock sync.Mutex
	emit := func(event string, data interface{}) error {
		writeLock.Lock()
		defer writeLock.Unlock()
		if err := ctx.Err(); err != nil {
			return err
		}
		b, err := json.Marshal(data)
		if err != nil {
			return err
		}
		if _, err = fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", event, b); err != nil {
			return err
		}
		c.Writer.Flush()
		return nil
	}
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				writeLock.Lock()
				if ctx.Err() == nil {
					fmt.Fprint(c.Writer, ": keepalive\n\n")
					c.Writer.Flush()
				}
				writeLock.Unlock()
			}
		}
	}()
	defer func() { cancel(); <-heartbeatDone }()

	service := assistant.New(cfg, assistant.SchoolTools(client))
	if err := service.Stream(ctx, req, emit); err != nil && ctx.Err() == nil {
		fmt.Printf("edudz assistant failed: %v\n", err)
		emit("error", gin.H{"message": "The assistant could not finish this request. Please try again."})
	}
}
