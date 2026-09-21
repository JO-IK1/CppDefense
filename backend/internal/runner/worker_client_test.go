package runner

import (
	"bytes"
	"testing"
)

func TestLimitedBufferCapsOutput(t *testing.T) {
	var buffer limitedBuffer
	payload := bytes.Repeat([]byte("x"), (1<<20)+100)
	written, err := buffer.Write(payload)
	if err != nil || written != len(payload) || buffer.Len() != 1<<20 {
		t.Fatalf("written=%d len=%d err=%v", written, buffer.Len(), err)
	}
}
