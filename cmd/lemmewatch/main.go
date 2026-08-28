package main

import (
	"fmt"
	"os"

	"lemmewatch/internal/cli"
	"lemmewatch/internal/config"
)

func main() {
	if err := config.LoadDotenv(".env"); err != nil {
		_ = config.LogFailure("load .env", err)
		fmt.Fprintf(os.Stderr, "load .env: %v\n", err)
		os.Exit(1)
	}
	if err := cli.New().Execute(); err != nil {
		_ = config.LogFailure("command", err)
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
