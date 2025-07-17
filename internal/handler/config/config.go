package config

import "flag"

type Config struct {
	cfg string
}

func GetConfig() string {
	cfg := Config{}
	flag.StringVar(&cfg.cfg, "a", "localhost:8080", "addres of HTTP server")

	flag.Parse()

	return cfg.cfg
}
