package config

import (
	"testing"
	"time"
)

type testConfig struct {
	Addr      string        `env:"ADDR" envDefault:"127.0.0.1:8080" flag:"addr"`
	Userspace bool          `env:"USERSPACE" flag:"userspace"`
	Interval  time.Duration `env:"INTERVAL" envDefault:"45s" flag:"interval"`
}

func TestLoadEnvironmentAndFlags(t *testing.T) {
	t.Setenv("TEST_ADDR", "127.0.0.1:9000")
	t.Setenv("TEST_USERSPACE", "true")
	t.Setenv("TEST_INTERVAL", "1m")
	var cfg testConfig
	if err := Load(&cfg, "TEST_", []string{"--addr", "127.0.0.1:9001", "--interval", "2m"}); err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != "127.0.0.1:9001" || !cfg.Userspace || cfg.Interval != 2*time.Minute {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestLoadDefaults(t *testing.T) {
	var cfg testConfig
	if err := Load(&cfg, "TEST_", nil); err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != "127.0.0.1:8080" || cfg.Userspace || cfg.Interval != 45*time.Second {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}
