package agent

import (
	"testing"

	"reasonix/internal/contract/config"
	"reasonix/internal/contract/provider"
)

func TestProviderRetriesFromConfig(t *testing.T) {
	for in, want := range map[int]int{0: provider.RetriesUntilCancelled, -3: 0, 1: 1, 25: 25} {
		if got := ProviderRetriesFromConfig(in); got != want {
			t.Errorf("max_provider_retries %d -> %d, want %d", in, got, want)
		}
	}
}

func TestDefaultConfigKeepsTheProviderPackageDefault(t *testing.T) {
	if got := ProviderRetriesFromConfig(config.Default().Agent.MaxProviderRetries); got != provider.MaxRetries {
		t.Fatalf("default config retries %d, provider default %d: an untouched install would change behaviour", got, provider.MaxRetries)
	}
}
