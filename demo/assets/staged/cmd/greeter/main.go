package main

import (
	"fmt"
	"os"

	"example.com/greeter/internal/config"
)

func main() {
	cfg := config.Load(os.Args[1:])
	for i := 0; i < cfg.Count; i++ {
		fmt.Printf("Hello, %s!\n", cfg.Name)
	}
}
