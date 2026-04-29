package config

import "log/slog"

type ServiceConfig struct {
	Environment string `yaml:"environment"`
	// BaseURL is prepended to generated codes when building ShortLink.short_url.
	// e.g. "https://urlo.example".
	BaseURL string `yaml:"base_url"`
}

func (c *ServiceConfig) Print() {
	slog.Info("service config loaded",
		"environment", c.Environment,
		"base_url", c.BaseURL,
	)
}
