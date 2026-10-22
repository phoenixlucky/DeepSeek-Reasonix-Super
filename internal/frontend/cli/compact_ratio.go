package cli

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"reasonix/internal/contract/config"
)

func compactRatioPercentageRequirement() string {
	return fmt.Sprintf("percentage above %g and below %g", config.CompactRatioMin*100, config.CompactRatioMax*100)
}

func parseCLICompactRatio(value string) (float64, error) {
	typed := strings.TrimSpace(value)
	percent, err := strconv.ParseFloat(typed, 64)
	ratio := percent / 100
	if exact, ok := new(big.Rat).SetString(typed); err == nil && percent > 0 && percent < 100 && ok {
		ratio, _ = exact.Quo(exact, big.NewRat(100, 1)).Float64()
	}
	if err == nil {
		err = config.ValidateCompactRatio(ratio)
	}
	if err != nil {
		return 0, fmt.Errorf("compact ratio must be a %s: %w", compactRatioPercentageRequirement(), err)
	}
	return ratio, nil
}
