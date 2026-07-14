package main

import (
	"context"
	"os"

	"github.com/summerchaserwwz/summer-harness/internal/cli"
)

func main() {
	cwd, err := os.Getwd()
	if err != nil {
		os.Exit(1)
	}
	os.Exit(cli.Run(context.Background(), os.Args[1:], cwd, os.Stdout, os.Stderr))
}
