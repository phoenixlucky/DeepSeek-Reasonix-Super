package usecap

import (
	"context"
	"encoding/json"
	"reasonix/internal/contract/tool"
)

// runtimeBoundMCPTool keeps the provider-visible MCP adapter unchanged while
// binding execution to the current controller runtime. The underlying Host may
// be shared by sibling tabs, so a server name alone must never authorize reuse.
type runtimeBoundMCPTool struct {
	proxy      *UseCapabilityTool
	target     tool.Tool
	server     string
	authorized bool
}

func (b *runtimeBoundMCPTool) Name() string              { return b.target.Name() }
func (b *runtimeBoundMCPTool) Description() string       { return b.target.Description() }
func (b *runtimeBoundMCPTool) Schema() json.RawMessage   { return b.target.Schema() }
func (b *runtimeBoundMCPTool) ReadOnly() bool            { return b.target.ReadOnly() }
func (b *runtimeBoundMCPTool) MCPServerAuthorized() bool { return b.authorized }
func (b *runtimeBoundMCPTool) MCPServerName() string     { return b.server }
func (b *runtimeBoundMCPTool) MCPRawToolName() string    { return mcpRawToolName(b.target) }
func (b *runtimeBoundMCPTool) MCPDestructiveHint() bool  { return tool.HasMCPDestructiveHint(b.target) }
func (b *runtimeBoundMCPTool) MCPVisibleToolName() string {
	if meta, ok := b.target.(tool.MCPVisibleMetadata); ok {
		return meta.MCPVisibleToolName()
	}
	return b.MCPRawToolName()
}
func (b *runtimeBoundMCPTool) MCPPackageName() string {
	if meta, ok := b.target.(tool.MCPPackageMetadata); ok {
		return meta.MCPPackageName()
	}
	return ""
}

func (b *runtimeBoundMCPTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	var out string
	err := b.proxy.withRuntimeBoundMCP(ctx, b.server, b.target, func() error {
		var execErr error
		out, execErr = b.target.Execute(ctx, args)
		return execErr
	})
	return out, err
}

func (b *runtimeBoundMCPTool) ExecuteWithImages(ctx context.Context, args json.RawMessage) (string, []string, error) {
	var out string
	var images []string
	err := b.proxy.withRuntimeBoundMCP(ctx, b.server, b.target, func() error {
		if imageTool, ok := b.target.(tool.ImageTool); ok {
			var execErr error
			out, images, execErr = imageTool.ExecuteWithImages(ctx, args)
			return execErr
		}
		var execErr error
		out, execErr = b.target.Execute(ctx, args)
		return execErr
	})
	return out, images, err
}

func mcpRawToolName(target tool.Tool) string {
	if meta, ok := target.(tool.MCPMetadata); ok {
		return meta.MCPRawToolName()
	}
	return ""
}
