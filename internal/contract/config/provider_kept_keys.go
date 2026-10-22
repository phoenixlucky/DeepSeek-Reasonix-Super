package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"reasonix/internal/base/fileutil"
	"reasonix/internal/contract/provider"
	"reasonix/internal/contract/walletformat"
)

// providerKeysFile records, beside the user config, the provider-entry keys the 1.x
// line does not know. It rewrites the shared file from the keys it knows, so such a
// key is gone after its next save; this record puts it back while what the key
// describes is unchanged: the wallet address, or the price. Not config_version.
const providerKeysFile = "provider-keys.json"

// legacyBalanceFormatsFile is the record the balance format kept alone; it is read
// once when the general record does not exist yet, and left as it is.
const legacyBalanceFormatsFile = "balance-formats.json"

type balanceKey struct {
	URL    string `json:"url"`
	Format string `json:"format"`
}

type priceSourceKey struct {
	Guard     string `json:"guard"`
	SourceURL string `json:"source_url,omitempty"`
	Checked   string `json:"checked,omitempty"`
}

type providerKeys struct {
	Balance     *balanceKey               `json:"balance,omitempty"`
	BillingMode string                    `json:"billing_mode,omitempty"`
	Prices      map[string]priceSourceKey `json:"prices,omitempty"`
}

type providerKeyLedger struct {
	Providers map[string]*providerKeys `json:"providers"`
}

func (l *providerKeyLedger) entry(name string) *providerKeys {
	k := l.Providers[name]
	if k == nil {
		k = &providerKeys{}
		l.Providers[name] = k
	}
	return k
}

// readProviderKeyLedger returns the record and whether any record existed, the
// general one or the balance format's own.
func readProviderKeyLedger(dir string) (providerKeyLedger, bool) {
	l := providerKeyLedger{Providers: map[string]*providerKeys{}}
	if data, err := os.ReadFile(filepath.Join(dir, providerKeysFile)); err == nil {
		if json.Unmarshal(data, &l) == nil {
			if l.Providers == nil {
				l.Providers = map[string]*providerKeys{}
			}
			return l, true
		}
		return providerKeyLedger{Providers: map[string]*providerKeys{}}, true
	}
	data, err := os.ReadFile(filepath.Join(dir, legacyBalanceFormatsFile))
	if err != nil {
		return l, false
	}
	var old struct {
		Entries map[string]balanceKey `json:"entries"`
	}
	if json.Unmarshal(data, &old) == nil {
		for name, rec := range old.Entries {
			l.Providers[name] = &providerKeys{Balance: &rec}
		}
	}
	return l, true
}

func writeProviderKeyLedger(dir string, l providerKeyLedger) error {
	for name, k := range l.Providers {
		if k.Balance == nil && k.BillingMode == "" && len(k.Prices) == 0 {
			delete(l.Providers, name)
		}
	}
	data, err := json.Marshal(l)
	if err != nil {
		return err
	}
	return fileutil.AtomicWriteFile(filepath.Join(dir, providerKeysFile), append(data, '\n'), 0o600)
}

// relayBalanceEntry is an entry whose wallet address is not a vendor's own host,
// the only kind that needs a declared format.
func relayBalanceEntry(p *ProviderEntry) bool {
	return strings.TrimSpace(p.BalanceURL) != "" && walletformat.Implied(p.BalanceURL) == ""
}

// priceGuard is what a price's source describes. A price changed by hand has a
// different guard, so it does not inherit the source of the one it replaced.
func priceGuard(model string, price *provider.Pricing) string {
	return fmt.Sprintf("%s|%v|%v|%v|%s", model, price.Input, price.Output, price.CacheHit, price.Currency)
}

// statedBillingMode is the mode an entry states, "" for the default (payg), which
// needs no record.
func statedBillingMode(p *ProviderEntry) string {
	mode := strings.TrimSpace(p.BillingMode)
	if mode == "" || mode == "payg" {
		return ""
	}
	return mode
}

// reconcileProviderKeys keeps these keys across a rewrite by the 1.x line. The
// first run declares "deepseek" where a relay names no balance format, which is how
// such an address was always read; later runs put back what the file lost while
// what it describes is unchanged. A removal on purpose goes through
// ForgetProviderKeys.
func reconcileProviderKeys(path string) (bool, error) {
	dir := filepath.Dir(path)
	ledger, existed := readProviderKeyLedger(dir)
	cfg := LoadForEdit(path)
	stating := providersStatingBillingMode(path)
	changed := false
	for i := range cfg.Providers {
		p := &cfg.Providers[i]
		k := ledger.entry(p.Name)
		changed = reconcileBalance(p, k, existed) || changed
		changed = reconcileBilling(p, k, stating[p.Name]) || changed
		changed = reconcilePriceSources(p, k) || changed
	}
	if changed {
		if err := cfg.SaveTo(path); err != nil {
			return false, err
		}
	}
	return changed, writeProviderKeyLedger(dir, ledger)
}

func reconcileBalance(p *ProviderEntry, k *providerKeys, existed bool) bool {
	if !relayBalanceEntry(p) {
		return false
	}
	url := strings.TrimSpace(p.BalanceURL)
	switch {
	case strings.TrimSpace(p.BalanceFormat) != "":
		k.Balance = &balanceKey{URL: url, Format: p.BalanceFormat}
	case k.Balance != nil && k.Balance.URL == url && walletformat.Format(k.Balance.Format).Valid():
		p.BalanceFormat = k.Balance.Format
		return true
	case !existed:
		p.BalanceFormat = string(walletformat.DeepSeek)
		k.Balance = &balanceKey{URL: url, Format: p.BalanceFormat}
		return true
	}
	return false
}

// reconcileBilling puts back a mode the file lost. A load fills the default in, so
// "lost" is read from the file itself: the entry states no billing_mode there.
func reconcileBilling(p *ProviderEntry, k *providerKeys, stated bool) bool {
	if stated {
		k.BillingMode = statedBillingMode(p)
		return false
	}
	if k.BillingMode != "" {
		p.BillingMode = k.BillingMode
		return true
	}
	return false
}

// providersStatingBillingMode lists the entries whose billing_mode is written in
// the file, as opposed to defaulted by the load.
func providersStatingBillingMode(path string) map[string]bool {
	var raw struct {
		Providers []map[string]any `toml:"providers"`
	}
	out := map[string]bool{}
	if _, err := decodeTOMLFile(path, &raw); err != nil {
		return out
	}
	for _, p := range raw.Providers {
		name, _ := p["name"].(string)
		if mode, ok := p["billing_mode"].(string); ok && strings.TrimSpace(mode) != "" {
			out[name] = true
		}
	}
	return out
}

func reconcilePriceSources(p *ProviderEntry, k *providerKeys) bool {
	changed := false
	for model, price := range p.Prices {
		if price == nil {
			continue
		}
		guard := priceGuard(model, price)
		if price.SourceURL != "" || price.Checked != "" {
			if k.Prices == nil {
				k.Prices = map[string]priceSourceKey{}
			}
			k.Prices[model] = priceSourceKey{Guard: guard, SourceURL: price.SourceURL, Checked: price.Checked}
			continue
		}
		if rec, ok := k.Prices[model]; ok && rec.Guard == guard {
			price.SourceURL, price.Checked = rec.SourceURL, rec.Checked
			changed = true
		}
	}
	return changed
}

// ForgetProviderKeys drops the record of an entry, so a person who removed a key
// or the whole connection is not given it back at the next start.
func ForgetProviderKeys(path, provider string) error {
	dir := filepath.Dir(path)
	ledger, _ := readProviderKeyLedger(dir)
	if _, ok := ledger.Providers[provider]; !ok {
		return nil
	}
	delete(ledger.Providers, provider)
	return writeProviderKeyLedger(dir, ledger)
}

// NoteProviderKeys records what an editor save left on an entry, nothing for a key
// the person cleared, so a removal is not undone.
func NoteProviderKeys(path string, p *ProviderEntry) error {
	if p == nil {
		return nil
	}
	dir := filepath.Dir(path)
	ledger, _ := readProviderKeyLedger(dir)
	delete(ledger.Providers, p.Name)
	k := &providerKeys{}
	if relayBalanceEntry(p) && strings.TrimSpace(p.BalanceFormat) != "" {
		k.Balance = &balanceKey{URL: strings.TrimSpace(p.BalanceURL), Format: p.BalanceFormat}
	}
	k.BillingMode = statedBillingMode(p)
	for model, price := range p.Prices {
		if price != nil && (price.SourceURL != "" || price.Checked != "") {
			if k.Prices == nil {
				k.Prices = map[string]priceSourceKey{}
			}
			k.Prices[model] = priceSourceKey{Guard: priceGuard(model, price), SourceURL: price.SourceURL, Checked: price.Checked}
		}
	}
	ledger.Providers[p.Name] = k
	return writeProviderKeyLedger(dir, ledger)
}
