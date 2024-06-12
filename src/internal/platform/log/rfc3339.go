package log

import (
	"io"
	"os"
	"time"
)

var _ io.Writer = (*rfc3339Writer)(nil)

type rfc3339Writer struct{}

func (w *rfc3339Writer) Write(bytes []byte) (int, error) {
	str := time.Now().UTC().Format(time.RFC3339) + " " + string(bytes)
	return io.WriteString(os.Stderr, str)
}
