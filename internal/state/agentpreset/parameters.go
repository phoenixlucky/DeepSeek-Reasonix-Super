package agentpreset

import (
	"encoding/json"
	"reasonix/internal/contract/agentcomposition"
	"reasonix/internal/contract/config"
)

func ApplyMCPParameters(entry config.PluginEntry, raw json.RawMessage) (config.PluginEntry, error) {
	p, err := agentcomposition.ParseParameters(raw)
	if err != nil {
		return entry, err
	}
	if p.Args != nil {
		entry.Args = *p.Args
	}
	if p.URL != nil {
		entry.URL = *p.URL
	}
	if p.Env != nil {
		entry.Env = p.Env
	}
	if p.Headers != nil {
		entry.Headers = p.Headers
	}
	if p.CallTimeoutSeconds != nil {
		entry.CallTimeoutSeconds = *p.CallTimeoutSeconds
	}
	if p.StartupTimeoutSeconds != nil {
		entry.StartupTimeoutSeconds = *p.StartupTimeoutSeconds
	}
	return entry, nil
}
