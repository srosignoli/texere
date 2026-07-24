package llm

// anthropicMessages builds the Anthropic /v1/messages "messages" array: a plain
// string content for text-only messages, or a content-block array when a message
// carries images (text block first, then image blocks).
func anthropicMessages(msgs []Message) []map[string]any {
	out := make([]map[string]any, 0, len(msgs))
	for _, m := range msgs {
		if len(m.Images) == 0 {
			out = append(out, map[string]any{"role": m.Role, "content": m.Content})
			continue
		}
		blocks := make([]map[string]any, 0, len(m.Images)+1)
		if m.Content != "" {
			blocks = append(blocks, map[string]any{"type": "text", "text": m.Content})
		}
		for _, im := range m.Images {
			blocks = append(blocks, map[string]any{
				"type":   "image",
				"source": map[string]any{"type": "base64", "media_type": im.MediaType, "data": im.Data},
			})
		}
		out = append(out, map[string]any{"role": m.Role, "content": blocks})
	}
	return out
}

// openaiMessages builds the OpenAI /chat/completions "messages" array with the
// same string-or-parts rule (image_url data URIs for images).
func openaiMessages(msgs []Message) []map[string]any {
	out := make([]map[string]any, 0, len(msgs))
	for _, m := range msgs {
		if len(m.Images) == 0 {
			out = append(out, map[string]any{"role": m.Role, "content": m.Content})
			continue
		}
		parts := make([]map[string]any, 0, len(m.Images)+1)
		if m.Content != "" {
			parts = append(parts, map[string]any{"type": "text", "text": m.Content})
		}
		for _, im := range m.Images {
			parts = append(parts, map[string]any{
				"type":      "image_url",
				"image_url": map[string]any{"url": "data:" + im.MediaType + ";base64," + im.Data},
			})
		}
		out = append(out, map[string]any{"role": m.Role, "content": parts})
	}
	return out
}
