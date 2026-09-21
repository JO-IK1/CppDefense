package main

import (
	"fmt"
	"os"

	"github.com/JO-IK1/CppDefense/backend/internal/runner"
)

func main() {
	if err := runner.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
