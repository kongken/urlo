package config

import "log/slog"

type ServiceConfig struct {
	Environment string        `yaml:"environment"`
	BaseURL     string        `yaml:"base_url"`
	Storage     StorageConfig `yaml:"storage"`
}

// StorageConfig selects the backing store for short links.
//
// Driver values:
//   - "memory" (default): in-process map; resets on restart.
//   - "s3": uses the butterfly S3 client identified by S3.ConfigName,
//     storing each link as a JSON object under S3.Prefix.
type StorageConfig struct {
	Driver string          `yaml:"driver"`
	S3     S3StorageConfig `yaml:"s3"`
}

type S3StorageConfig struct {
	// ConfigName matches a key under butterfly's `store.s3.<name>` config.
	ConfigName string `yaml:"config_name"`
	// Prefix is the object-key prefix (default "links").
	Prefix string `yaml:"prefix"`
}

func (c *ServiceConfig) Print() {
	slog.Info("service config loaded",
		"environment", c.Environment,
		"base_url", c.BaseURL,
		"storage_driver", c.Storage.Driver,
	)
}
