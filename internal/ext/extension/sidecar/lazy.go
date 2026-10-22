package sidecar

import (
	"context"
	"fmt"
	"sort"

	"reasonix/internal/contract/agentcomposition"
	"reasonix/internal/ext/extension/protocol"
	"reasonix/internal/ext/pluginpkg"
)

type lazyPackages struct {
	resolve  func([]pluginpkg.InstalledPackage) ([]pluginpkg.InstalledPackage, error)
	active   map[string]string
	calls    map[*Client]int
	retired  map[*Client]bool
	life     context.Context
	cancel   context.CancelFunc
	home     string
	session  protocol.SessionContext
	ui       UIHandler
	packages map[string]pluginpkg.InstalledPackage
	pending  map[string]*lazyStart
}

type lazyStart struct {
	done   chan struct{}
	client *Client
	err    error
}

func (m *Manager) OnDemand() bool {
	return m != nil && m.lazy != nil
}

func toolOnlyPackage(p pluginpkg.InstalledPackage) bool {
	rt := p.Package.Manifest.Runtime
	if rt == nil || rt.Required || len(rt.Intercepts) != 0 || len(rt.Replaces) != 0 {
		return false
	}
	for _, c := range rt.Capabilities {
		if c != "tools" {
			return false
		}
	}
	for _, c := range p.Package.Manifest.Provides {
		if c.Kind != "tool" {
			return false
		}
	}
	return len(rt.Tools) > 0 || rt.DynamicTools
}

func StartLivePackages(ctx context.Context, home string, packages []pluginpkg.InstalledPackage, session protocol.SessionContext, ui UIHandler, resolve ...func([]pluginpkg.InstalledPackage) ([]pluginpkg.InstalledPackage, error)) (*Manager, []string, error) {
	life, cancel := context.WithCancel(ctx)
	lazy := &lazyPackages{life: life, cancel: cancel, home: home, session: session, ui: ui,
		packages: map[string]pluginpkg.InstalledPackage{}, pending: map[string]*lazyStart{}, active: map[string]string{},
		calls: map[*Client]int{}, retired: map[*Client]bool{}}
	if len(resolve) > 0 {
		lazy.resolve = resolve[0]
	}
	var eager []pluginpkg.InstalledPackage
	for _, p := range packages {
		if toolOnlyPackage(p) {
			lazy.packages[p.Installed.Name] = p
		} else {
			eager = append(eager, p)
		}
	}
	startup, stop := context.WithTimeout(life, packageStartupBudget)
	defer stop()
	m, warnings, err := startLoadedPackages(startup, eager, session, ui, StartClient)
	if err != nil {
		cancel()
		return m, warnings, err
	}
	m.lazy = lazy
	for _, p := range eager {
		lazy.active[p.Installed.Name] = livePackageKey(p)
	}
	return m, warnings, nil
}

func (m *Manager) DeferredPackages() []pluginpkg.InstalledPackage {
	if m == nil {
		return nil
	}
	if m.lazy != nil && m.lazy.resolve != nil {
		packages, _ := LoadRuntimePackagesFor(m.lazy.home, m.lazy.session.WorkspaceRoot)
		packages, err := m.lazy.resolve(packages)
		if err != nil {
			return nil
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.closed {
			return nil
		}
		var out []pluginpkg.InstalledPackage
		for _, p := range packages {
			if toolOnlyPackage(p) {
				m.lazy.packages[p.Installed.Name] = p
				out = append(out, p)
			}
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Installed.Name < out[j].Installed.Name })
		return out
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []pluginpkg.InstalledPackage
	if m.lazy != nil && !m.closed {
		for _, p := range m.lazy.packages {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Installed.Name < out[j].Installed.Name })
	return out
}

// EnsureReady coalesces activation and keeps process lifetime with the manager.
func (m *Manager) EnsureReady(ctx context.Context, name string) (*Client, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil, fmt.Errorf("extension manager closed")
	}
	lazy := m.lazy
	if lazy == nil {
		client := m.clients[name]
		m.mu.Unlock()
		if client == nil || client.Exited() {
			return nil, fmt.Errorf("extension %s is unavailable", name)
		}
		return client, nil
	}
	_, deferred := lazy.packages[name]
	m.mu.Unlock()
	packages, _ := LoadRuntimePackagesFor(lazy.home, lazy.session.WorkspaceRoot)
	if lazy.resolve != nil {
		var err error
		packages, err = lazy.resolve(packages)
		if err != nil {
			return nil, err
		}
	}
	var current *pluginpkg.InstalledPackage
	for _, p := range packages {
		if p.Installed.Name == name {
			current = &p
			break
		}
	}
	if current == nil || deferred && !toolOnlyPackage(*current) {
		m.retireLiveClient(name)
		return nil, fmt.Errorf("extension %s was removed, disabled or requires a new runtime", name)
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil, fmt.Errorf("extension manager closed")
	}
	if client := m.clients[name]; client != nil {
		if !client.Exited() && lazy.active[name] == livePackageKey(*current) {
			m.mu.Unlock()
			return client, nil
		}
		if !deferred {
			m.mu.Unlock()
			return nil, fmt.Errorf("extension %s changed its startup contract; reload the runtime", name)
		}
		delete(m.clients, name)
		closeNow := lazy.calls[client] == 0
		if !closeNow {
			lazy.retired[client] = true
		}
		m.mu.Unlock()
		if closeNow {
			_ = client.Close()
		}
		m.mu.Lock()
		if m.closed {
			m.mu.Unlock()
			return nil, fmt.Errorf("extension manager closed")
		}
		if m.clients[name] != nil {
			m.mu.Unlock()
			return m.EnsureReady(ctx, name)
		}
	}
	if !deferred {
		m.mu.Unlock()
		return nil, fmt.Errorf("extension %s requires runtime initialization", name)
	}
	if err := ctx.Err(); err != nil {
		m.mu.Unlock()
		return nil, err
	}
	start := lazy.pending[name]
	if start == nil {
		start = &lazyStart{done: make(chan struct{})}
		lazy.pending[name] = start
		go m.startDeferred(*current, lazy, start)
	}
	m.mu.Unlock()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-start.done:
		return start.client, start.err
	}
}

func (m *Manager) retireLiveClient(name string) {
	m.mu.Lock()
	client := m.clients[name]
	delete(m.clients, name)
	closeNow := client != nil && m.lazy.calls[client] == 0
	if client != nil && !closeNow {
		m.lazy.retired[client] = true
	}
	m.mu.Unlock()
	if closeNow {
		_ = client.Close()
	}
}

func (m *Manager) startDeferred(p pluginpkg.InstalledPackage, lazy *lazyPackages, start *lazyStart) {
	ctx, cancel := context.WithTimeout(lazy.life, packageStartupBudget)
	defer cancel()
	client, err := startOne(ctx, p, lazy.session, lazy.ui)
	m.mu.Lock()
	closed := m.closed
	if closed {
		err = context.Canceled
	} else if err == nil {
		m.clients[p.Installed.Name] = client
		lazy.active[p.Installed.Name] = livePackageKey(p)
	}
	delete(lazy.pending, p.Installed.Name)
	m.mu.Unlock()
	if closed && client != nil {
		_ = client.Close()
	}
	start.client, start.err = client, err
	close(start.done)
}

func livePackageKey(p pluginpkg.InstalledPackage) string {
	return agentcomposition.Digest(struct {
		Installed pluginpkg.InstalledPlugin
		Manifest  pluginpkg.Manifest
	}{p.Installed, p.Package.Manifest})
}

func (m *Manager) WithReady(ctx context.Context, name string, call func(context.Context, *Client) error) error {
	for {
		client, err := m.EnsureReady(ctx, name)
		if err != nil {
			return err
		}
		if m.lazy == nil {
			return call(ctx, client)
		}
		if err := m.validateLiveCall(name); err != nil {
			return err
		}
		m.mu.Lock()
		if m.closed {
			m.mu.Unlock()
			return context.Canceled
		}
		if m.clients[name] != client {
			m.mu.Unlock()
			continue
		}
		m.lazy.calls[client]++
		m.mu.Unlock()
		return m.callWithLiveLease(ctx, client, call)
	}
}

func (m *Manager) callWithLiveLease(ctx context.Context, client *Client, call func(context.Context, *Client) error) error {
	defer m.releaseLiveCall(client)
	callCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(m.lazy.life, cancel)
	defer stop()
	if err := callCtx.Err(); err != nil {
		return err
	}
	return call(callCtx, client)
}

func (m *Manager) releaseLiveCall(client *Client) {
	m.mu.Lock()
	m.lazy.calls[client]--
	closeNow := m.lazy.calls[client] == 0 && m.lazy.retired[client]
	if m.lazy.calls[client] == 0 {
		delete(m.lazy.calls, client)
		delete(m.lazy.retired, client)
	}
	m.mu.Unlock()
	if closeNow {
		_ = client.Close()
	}
}

func (m *Manager) validateLiveCall(name string) error {
	packages, _ := LoadRuntimePackagesFor(m.lazy.home, m.lazy.session.WorkspaceRoot)
	if m.lazy.resolve != nil {
		var err error
		packages, err = m.lazy.resolve(packages)
		if err != nil {
			return err
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, p := range packages {
		if p.Installed.Name == name && m.lazy.active[name] == livePackageKey(p) && !m.closed {
			return nil
		}
	}
	return fmt.Errorf("extension %s changed or was disabled before dispatch", name)
}
