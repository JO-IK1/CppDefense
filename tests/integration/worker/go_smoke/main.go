package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const sessionID = "0199a123-4568-7abc-8def-0123456789ab"

func main() {
	if len(os.Args) != 2 {
		panic("usage: worker_smoke.go WORKER")
	}
	root, err := os.MkdirTemp("", "cpp-defense-go-worker-")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(root)

	project := filepath.Join(root, sessionID, "project")
	if err := os.MkdirAll(project, 0o700); err != nil {
		panic(err)
	}
	write(filepath.Join(project, "CMakeLists.txt"),
		"cmake_minimum_required(VERSION 3.20)\nproject(Sample LANGUAGES CXX)\n")
	write(filepath.Join(project, "main.cpp"), "int Answer() { return 42; }\n")

	request := map[string]any{
		"protocol_version": "1.0",
		"request_id":       "0199a123-4567-7abc-8def-0123456789ab",
		"session_id":       sessionID,
		"command":          "analyze_project",
		"payload": map[string]any{
			"project_root": "project",
		},
	}
	input, err := json.Marshal(request)
	if err != nil {
		panic(err)
	}

	command := exec.Command(os.Args[1], "--workspace", root)
	command.Stdin = bytes.NewReader(input)
	output, err := command.Output()
	if err != nil {
		panic(err)
	}
	var response struct {
		Status string `json:"status"`
		Result struct {
			FunctionCount int `json:"function_count"`
		} `json:"result"`
	}
	if err := json.Unmarshal(output, &response); err != nil {
		panic(err)
	}
	if response.Status != "ok" || response.Result.FunctionCount != 1 {
		panic(fmt.Sprintf("unexpected worker response: %s", output))
	}
}

func write(path, contents string) {
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		panic(err)
	}
}
