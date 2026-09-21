package runner

import (
	"context"
	"os/exec"
	"path/filepath"
)

type commandResult struct {
	OK       bool   `json:"ok"`
	ExitCode int    `json:"exit_code"`
	Log      string `json:"log"`
	TimedOut bool   `json:"timed_out"`
}

func runSandbox(ctx context.Context, runtime, image, project string) commandResult {
	absolute, _ := filepath.Abs(project)
	script := "cmake -S /workspace -B /tmp/build -DCMAKE_BUILD_TYPE=Release && cmake --build /tmp/build --parallel 2 && ctest --test-dir /tmp/build --output-on-failure"
	args := []string{"run", "--rm", "--network", "none", "--cpus", "1.0", "--memory", "512m", "--memory-swap", "512m", "--pids-limit", "128", "--read-only", "--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--tmpfs", "/tmp:rw,noexec,nosuid,size=1g", "--user", "65532:65532", "-v", absolute + ":/workspace:ro", image, "sh", "-lc", script}
	command := exec.CommandContext(ctx, runtime, args...)
	var output limitedBuffer
	command.Stdout, command.Stderr = &output, &output
	err := command.Run()
	result := commandResult{OK: err == nil, Log: output.String(), TimedOut: ctx.Err() != nil}
	if exit, ok := err.(*exec.ExitError); ok {
		result.ExitCode = exit.ExitCode()
	} else if err != nil {
		result.ExitCode = -1
	}
	return result
}
