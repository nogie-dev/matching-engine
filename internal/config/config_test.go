package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nogie-dev/clob-trading/internal/engine"
	"github.com/nogie-dev/clob-trading/internal/matchlog"
)

func TestLoadUsesDefaultWhenPathIsEmpty(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Engine.WorkerInputBufferSize != engine.DefaultWorkerInputBufferSize {
		t.Fatalf("worker input buffer size want %d, got %d",
			engine.DefaultWorkerInputBufferSize,
			cfg.Engine.WorkerInputBufferSize,
		)
	}
	if cfg.Engine.MatchLogOutputBufferSize != matchlog.DefaultOutputBufferSize {
		t.Fatalf("match log output buffer size want %d, got %d",
			matchlog.DefaultOutputBufferSize,
			cfg.Engine.MatchLogOutputBufferSize,
		)
	}
}

func TestLoadReadsBufferSizes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"engine":{"worker_input_buffer_size":64,"match_log_output_buffer_size":32}}`), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Engine.WorkerInputBufferSize != 64 {
		t.Fatalf("worker input buffer size want 64, got %d", cfg.Engine.WorkerInputBufferSize)
	}
	if cfg.Engine.MatchLogOutputBufferSize != 32 {
		t.Fatalf("match log output buffer size want 32, got %d", cfg.Engine.MatchLogOutputBufferSize)
	}
}
