package sidecar

import (
	"fmt"
	"slices"

	"reasonix/internal/contract/agentcomposition"
)

func (m *Manager) ToolClients() []*Client {
	clients := m.Clients()
	if m.lazy != nil {
		clients = slices.DeleteFunc(clients, func(c *Client) bool {
			return m.validateLiveCall(c.PluginID()) != nil
		})
	}
	return clients
}

func (m *Manager) ValidateLiveStartup() error {
	if m == nil || m.lazy == nil || m.lazy.resolve == nil {
		return nil
	}
	packages, _ := LoadRuntimePackagesFor(m.lazy.home, m.lazy.session.WorkspaceRoot)
	packages, err := m.lazy.resolve(packages)
	if err != nil {
		return err
	}
	current := map[string]string{}
	for _, p := range packages {
		if !toolOnlyPackage(p) {
			current[p.Installed.Name] = livePackageKey(p)
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for name, key := range current {
		if m.lazy.active[name] != key {
			return fmt.Errorf("%w: extension %s changed its startup contract; start a new session", agentcomposition.ErrUnavailable, name)
		}
	}
	for name := range m.lazy.active {
		if _, deferred := m.lazy.packages[name]; !deferred && current[name] == "" {
			return fmt.Errorf("%w: required runtime extension %s is no longer available", agentcomposition.ErrUnavailable, name)
		}
	}
	return nil
}
