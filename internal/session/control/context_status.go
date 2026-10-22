package control

import (
	"context"
	"time"

	"reasonix/internal/contract/pricing"
	"reasonix/internal/contract/provider"
	"reasonix/internal/model/billing"
	"reasonix/internal/runtime/agent"
)

// ContextMaintenanceSnapshot exposes current composition and the last durable
// maintenance receipt without conflating them with cumulative usage cost.
func (c *Controller) ContextMaintenanceSnapshot() agent.ContextMaintenanceSnapshot {
	if c.executor == nil {
		return agent.ContextMaintenanceSnapshot{}
	}
	return c.executor.ContextMaintenanceSnapshot()
}

// ModelVisibleMessages is what the next request would carry: the folded view
// plus host state derived for it. History() is the canonical record; the two
// answer different questions and a caller has to say which it means.
func (c *Controller) ModelVisibleMessages() []provider.Message {
	if c.executor == nil {
		return nil
	}
	return c.executor.ModelVisibleMessages()
}

// TariffSource reads the vendor schedule a model's rate is billed under at an
// instant; false means no schedule applies. Nil answers false.
type TariffSource func(time.Time) (pricing.Tariff, bool)

// Tariff is the vendor schedule window the active model is billed in right now
// and when it next flips; false where no vendor schedule applies to its rate.
func (c *Controller) Tariff() (pricing.Tariff, bool) {
	if c.tariff == nil {
		return pricing.Tariff{}, false
	}
	return c.tariff(time.Now())
}

// Balance reads the active provider's wallet. One declaring no balance_url
// reads back unconfigured rather than failed: "there is no wallet here" and
// "the wallet did not answer" are opposite answers to why there is no number.
func (c *Controller) Balance(ctx context.Context) billing.Reading {
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	reading := c.balance.Read(ctx)
	if reading.Balance != nil {
		c.AdoptWalletCurrency(reading.Balance.PrimaryCurrency())
	}
	return reading
}
