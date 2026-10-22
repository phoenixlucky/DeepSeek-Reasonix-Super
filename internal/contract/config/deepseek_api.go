package config

import (
	"errors"
	"net/url"
	"strings"
)

// ErrDeepSeekAPIUnavailable means no configured provider reaches the official
// DeepSeek API with a key, so its search and fetch endpoints cannot be called.
var ErrDeepSeekAPIUnavailable = errors.New("no configured provider reaches api.deepseek.com with a key")

// ResolveDeepSeekAPI finds the official DeepSeek API origin and the key a
// configured provider holds for it. The key is read live, so a replaced key
// reaches the next request.
func (c *Config) ResolveDeepSeekAPI() (base string, key func() string, err error) {
	for i := range c.Providers {
		e := &c.Providers[i]
		u, perr := url.Parse(strings.TrimSpace(e.BaseURL))
		if perr != nil || !strings.EqualFold(u.Scheme, "https") || !strings.EqualFold(u.Hostname(), "api.deepseek.com") || u.Port() != "" || u.User != nil {
			continue
		}
		if !e.Configured() {
			continue
		}
		return "https://api.deepseek.com", e.APIKey, nil
	}
	return "", nil, ErrDeepSeekAPIUnavailable
}
