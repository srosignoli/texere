package llm

import (
	"reflect"
	"testing"
)

func TestAnthropicMessagesTextOnly(t *testing.T) {
	got := anthropicMessages([]Message{{Role: "user", Content: "hi"}})
	want := []map[string]any{{"role": "user", "content": "hi"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v", got)
	}
}

func TestAnthropicMessagesWithImage(t *testing.T) {
	got := anthropicMessages([]Message{{
		Role: "user", Content: "what is this?",
		Images: []Image{{MediaType: "image/png", Data: "AAAA"}},
	}})
	want := []map[string]any{{
		"role": "user",
		"content": []map[string]any{
			{"type": "text", "text": "what is this?"},
			{"type": "image", "source": map[string]any{"type": "base64", "media_type": "image/png", "data": "AAAA"}},
		},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v", got)
	}
}

func TestOpenAIMessagesWithImage(t *testing.T) {
	got := openaiMessages([]Message{{
		Role: "user", Content: "caption",
		Images: []Image{{MediaType: "image/jpeg", Data: "BBBB"}},
	}})
	want := []map[string]any{{
		"role": "user",
		"content": []map[string]any{
			{"type": "text", "text": "caption"},
			{"type": "image_url", "image_url": map[string]any{"url": "data:image/jpeg;base64,BBBB"}},
		},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v", got)
	}
}

func TestOpenAIMessagesTextOnly(t *testing.T) {
	got := openaiMessages([]Message{{Role: "system", Content: "sys"}, {Role: "user", Content: "u"}})
	want := []map[string]any{{"role": "system", "content": "sys"}, {"role": "user", "content": "u"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v", got)
	}
}
