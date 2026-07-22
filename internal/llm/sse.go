package llm

import (
	"bufio"
	"io"
	"strings"
)

// scanSSEData reads an SSE stream and calls onData with the payload of each
// "data:" line. Non-data lines (event:, id:, comments) and blank lines are
// skipped. It stops when onData returns stop==true, at EOF, or on a read error.
func scanSSEData(r io.Reader, onData func(data string) (stop bool, err error)) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024) // allow long data lines
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimPrefix(line, "data:")
		data = strings.TrimPrefix(data, " ")
		stop, err := onData(data)
		if err != nil {
			return err
		}
		if stop {
			return nil
		}
	}
	return sc.Err()
}
