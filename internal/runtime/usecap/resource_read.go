package usecap

import (
	"context"
	"encoding/json"
	"strings"

	"reasonix/internal/contract/tool"
	"reasonix/internal/ext/plugin"
)

type runtimeResourceReadTool struct {
	tool.Tool
	proxy *UseCapabilityTool
}

func (r *MCPCapabilityRuntime) ResourceReadTool() tool.Tool {
	return &runtimeResourceReadTool{Tool: plugin.ResourceReadTool(r.host), proxy: r.NewFrontend(nil, nil)}
}

func (t *runtimeResourceReadTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var in struct {
		Server string `json:"server"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return "", err
	}
	server := strings.TrimSpace(in.Server)
	spec, unlock, err := t.proxy.lockAuthorizedRuntimeServer(ctx, server)
	if err != nil {
		return "", err
	}
	defer unlock()
	var out string
	err = t.proxy.runtime.withServerGate(ctx, server, func() error {
		if _, err := t.proxy.ensureServerToolsForSpec(ctx, server, spec); err != nil {
			return err
		}
		var err error
		out, err = t.Tool.Execute(ctx, args)
		return err
	})
	return out, err
}
