package openai_compatible

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	contract "github.com/tep/llm-cost-gateway/internal/provider/contract"
)

func TestStreamChatSendsSSERequestAndParsesEvents(t *testing.T) {
	var capturedAuth string
	var capturedAccept string
	var capturedModel string
	var capturedStream bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		capturedAccept = r.Header.Get("Accept")
		var request struct {
			Model  string `json:"model"`
			Stream bool   `json:"stream"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		capturedModel = request.Model
		capturedStream = request.Stream

		w.Header().Set("Content-Type", "text/event-stream")
		writeSSE(t, w, `{"id":"chatcmpl_123","choices":[{"index":0,"delta":{"role":"assistant","content":"hello"}}]}`)
		writeSSE(t, w, `{"id":"chatcmpl_123","choices":[],"usage":{"prompt_tokens":11,"completion_tokens":13,"total_tokens":24}}`)
		writeSSE(t, w, `[DONE]`)
	}))
	defer server.Close()

	adapter := NewAdapter()
	request := validChatRequest(server.URL)
	request.Stream = true
	stream, err := adapter.StreamChat(context.Background(), request)
	if err != nil {
		t.Fatalf("StreamChat returned error: %v", err)
	}
	defer stream.Close()

	events := collectStreamEvents(t, stream)

	if capturedAuth != "Bearer provider-secret-key" {
		t.Fatalf("Authorization = %q, want bearer API key", capturedAuth)
	}
	if capturedAccept != "text/event-stream" {
		t.Fatalf("Accept = %q, want text/event-stream", capturedAccept)
	}
	if capturedModel != "gpt-test" || !capturedStream {
		t.Fatalf("upstream model=%q stream=%v, want gpt-test stream=true", capturedModel, capturedStream)
	}
	if len(events) != 3 {
		t.Fatalf("events len = %d, want 3; events=%+v", len(events), events)
	}
	if events[0].Data == "" || events[0].Done {
		t.Fatalf("first event = %+v, want data event", events[0])
	}
	if events[1].Usage == nil {
		t.Fatalf("second event usage = nil, want usage")
	}
	if events[1].Usage.PromptTokens != 11 || events[1].Usage.CompletionTokens != 13 || events[1].Usage.TotalTokens != 24 {
		t.Fatalf("usage = %+v, want 11/13/24", events[1].Usage)
	}
	if !events[2].Done || events[2].Data != "[DONE]" {
		t.Fatalf("done event = %+v, want [DONE]", events[2])
	}
}

func TestStreamChatCancelsUpstreamWhenContextIsCanceled(t *testing.T) {
	upstreamCanceled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		writeSSE(t, w, `{"choices":[{"delta":{"content":"hello"}}]}`)
		<-r.Context().Done()
		close(upstreamCanceled)
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	adapter := NewAdapter()
	request := validChatRequest(server.URL)
	request.Stream = true
	stream, err := adapter.StreamChat(ctx, request)
	if err != nil {
		t.Fatalf("StreamChat returned error: %v", err)
	}
	defer stream.Close()

	<-stream.Events()
	cancel()

	select {
	case <-upstreamCanceled:
	case <-time.After(time.Second):
		t.Fatal("upstream request context was not canceled")
	}
}

func TestStreamChatMissingUsageDoesNotInventUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		writeSSE(t, w, `{"choices":[{"delta":{"content":"hello"}}]}`)
		writeSSE(t, w, `[DONE]`)
	}))
	defer server.Close()

	adapter := NewAdapter()
	request := validChatRequest(server.URL)
	request.Stream = true
	stream, err := adapter.StreamChat(context.Background(), request)
	if err != nil {
		t.Fatalf("StreamChat returned error: %v", err)
	}
	defer stream.Close()

	events := collectStreamEvents(t, stream)
	for _, event := range events {
		if event.Usage != nil {
			t.Fatalf("event usage = %+v, want nil", event.Usage)
		}
	}
}

func writeSSE(t *testing.T, w http.ResponseWriter, data string) {
	t.Helper()
	_, err := fmt.Fprintf(w, "data: %s\n\n", data)
	if err != nil {
		t.Fatalf("write SSE: %v", err)
	}
	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}
}

func collectStreamEvents(t *testing.T, stream contract.ChatStream) []contract.StreamEvent {
	t.Helper()
	var events []contract.StreamEvent
	for event := range stream.Events() {
		events = append(events, event)
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("stream error = %v", err)
	}
	return events
}
