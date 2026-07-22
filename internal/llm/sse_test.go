package llm

import (
	"strings"
	"testing"
)

func TestScanSSEData(t *testing.T) {
	body := "event: start\n" +
		"data: {\"a\":1}\n" +
		"\n" +
		"data: [DONE]\n" +
		"data: after-done\n"
	var got []string
	err := scanSSEData(strings.NewReader(body), func(data string) (bool, error) {
		got = append(got, data)
		return data == "[DONE]", nil
	})
	if err != nil {
		t.Fatalf("scanSSEData: %v", err)
	}
	if len(got) != 2 || got[0] != `{"a":1}` || got[1] != "[DONE]" {
		t.Fatalf("unexpected payloads: %#v", got)
	}
}
