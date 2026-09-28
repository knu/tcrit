package config

import (
	"os"
	"strconv"
)

// Config controls how greetings are printed.
type Config struct {
	Name  string
	Count int
}

// Load builds a Config from the arguments and the environment.
func Load(args []string) Config {
	cfg := Config{Name: "world", Count: 1}
	if len(args) > 0 {
		cfg.Name = args[0]
	}
	if raw := os.Getenv("GREET_COUNT"); raw != "" {
		cfg.Count, _ = strconv.Atoi(raw)
	}
	return cfg
}
