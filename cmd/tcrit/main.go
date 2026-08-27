package main

import (
	"os"

	"github.com/kevindutra/crit/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
