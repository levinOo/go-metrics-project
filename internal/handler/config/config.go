package config

import (
	"flag"

	"github.com/caarlos0/env"
)

type Config struct {
	Addr string `env:"ADDRESS"`
}

func GetConfig() (string, error) {
	cfg := Config{}
	flag.StringVar(&cfg.Addr, "a", "localhost:8080", "addres of HTTP server")
	flag.Parse()

	err := env.Parse(&cfg)

	return cfg.Addr, err
}
