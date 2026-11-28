// Command tasks runs repository maintenance tasks.
//
// From the repository root: go run ./cmd/tasks check
package main

import (
	"fmt"
	"os"
	"os/exec"
)

func main() {
	// Default usage if no arguments
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run ./cmd/tasks check")
		os.Exit(1)
	}

	target := os.Args[1]

	switch target {
	case "check":
		checkLintInstalled()
		fmt.Println("🔍 Running lint checks...")
		run("golangci-lint", "run", "./...")
		fmt.Println("✅ Lint passed!")
	default:
		fmt.Printf("Unknown target: %s\n", target)
		os.Exit(1)
	}
}

// run executes a command and exits if it fails
func run(name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Printf("\n❌ Command '%s %v' failed! See above for lint errors.\n", name, args)
		os.Exit(1)
	}
}

// checkLintInstalled ensures golangci-lint is available
func checkLintInstalled() {
	if _, err := exec.LookPath("golangci-lint"); err != nil {
		fmt.Println("❌ golangci-lint is not installed or not in PATH.")
		fmt.Println("Install it: https://golangci-lint.run/usage/install/")
		os.Exit(1)
	}
}
