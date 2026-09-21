package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"

	"github.com/JO-IK1/CppDefense/backend/internal/infrastructure/postgres"
)

type prepareResponse struct {
	Status string `json:"status"`
	Result struct {
		Candidates    []postgres.Candidate `json:"candidates"`
		SelectedIndex int                  `json:"selected_index"`
		MaskedSource  string               `json:"masked_source"`
	} `json:"result"`
}

func runWorker(ctx context.Context, binary, dir string, input, output any) error {
	data, _ := json.Marshal(input)
	command := exec.CommandContext(ctx, binary)
	command.Dir = dir
	command.Stdin = bytes.NewReader(data)
	var stdout, stderr limitedBuffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("worker: %w: %s", err, stderr.String())
	}
	if err := json.Unmarshal(stdout.Bytes(), output); err != nil {
		return fmt.Errorf("worker JSON: %w", err)
	}
	return nil
}

type limitedBuffer struct{ bytes.Buffer }

func (b *limitedBuffer) Write(payload []byte) (int, error) {
	remaining := (1 << 20) - b.Len()
	if remaining <= 0 {
		return len(payload), nil
	}
	if len(payload) > remaining {
		_, _ = b.Buffer.Write(payload[:remaining])
		return len(payload), nil
	}
	return b.Buffer.Write(payload)
}
