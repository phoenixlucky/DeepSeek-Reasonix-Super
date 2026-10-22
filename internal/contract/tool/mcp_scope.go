package tool

// Identities of a call an MCP server refused for want of access. The model reads
// them with the sentence; the server's own scope names are not part of either,
// because they are a third party's text.
const (
	// CodeMCPInsufficientScope is a call that needs access the user has not
	// granted this server. Only the user can grant it.
	CodeMCPInsufficientScope = "mcp.insufficient_scope"
	// CodeMCPScopeUpgradeExhausted is the same refusal after extending the access
	// did not help; retrying this operation cannot succeed.
	CodeMCPScopeUpgradeExhausted = "mcp.insufficient_scope_permanent"
)
