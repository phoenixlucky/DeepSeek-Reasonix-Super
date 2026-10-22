package billing

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"reasonix/internal/contract/walletformat"
)

// Format is a declared balance response shape; the set lives in walletformat so
// the configuration can validate it without importing a reader.
type Format = walletformat.Format

const (
	FormatDeepSeek   = walletformat.DeepSeek
	FormatMoonshot   = walletformat.Moonshot
	FormatOpenRouter = walletformat.OpenRouter
)

// Formats is every format a provider may declare.
func Formats() []Format { return walletformat.All() }

// walletDecoders maps each format to the decoder for its response.
var walletDecoders = map[Format]func(host string, body []byte) (*Balance, error){
	FormatDeepSeek:   decodeDeepSeek,
	FormatMoonshot:   decodeMoonshot,
	FormatOpenRouter: decodeOpenRouter,
}

// ImpliedFormat is the format a vendor's own hostname answers in, empty for any
// other host.
func ImpliedFormat(endpoint string) Format { return walletformat.Implied(endpoint) }

// Resolve is the format a read of endpoint decodes with: the declared one, else
// the one the host implies, else none. A relay on its own domain has no implied
// format, so it must declare one before its answer is read as any vendor's.
func Resolve(endpoint string, declared Format) (Format, bool) {
	if declared != "" {
		return declared, declared.Valid()
	}
	f := ImpliedFormat(endpoint)
	return f, f != ""
}

// decodeWallet applies the decoder of the resolved format to a response.
func decodeWallet(endpoint string, declared Format, body []byte) (*Balance, error) {
	f, ok := Resolve(endpoint, declared)
	if !ok {
		return nil, ErrFormatNotDeclared
	}
	return walletDecoders[f](walletformat.Host(endpoint), body)
}

// deepseekResp mirrors the GET /user/balance response shape.
type deepseekResp struct {
	IsAvailable  bool `json:"is_available"`
	BalanceInfos []struct {
		Currency        string `json:"currency"`
		TotalBalance    string `json:"total_balance"`
		GrantedBalance  string `json:"granted_balance"`
		ToppedUpBalance string `json:"topped_up_balance"`
	} `json:"balance_infos"`
}

func decodeDeepSeek(_ string, body []byte) (*Balance, error) {
	var dr deepseekResp
	if err := json.Unmarshal(body, &dr); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnreadable, err)
	}
	b := &Balance{Available: dr.IsAvailable}
	for _, bi := range dr.BalanceInfos {
		b.Infos = append(b.Infos, Info{
			Currency:        bi.Currency,
			TotalBalance:    bi.TotalBalance,
			GrantedBalance:  bi.GrantedBalance,
			ToppedUpBalance: bi.ToppedUpBalance,
		})
	}
	// A response that parses as JSON but carries no wallet is not an empty
	// wallet: every other shape decodes into this one without erroring, and
	// reporting that as a zero balance states a fact nobody sent.
	if len(b.Infos) == 0 {
		return nil, ErrUnreadable
	}
	return b, nil
}

// moonshotResp mirrors GET /v1/users/me/balance. Amounts are numbers here, and
// the currency is not in the body at all — the .cn and .ai endpoints are the
// CNY and USD accounts. https://platform.moonshot.cn/docs/api/misc
type moonshotResp struct {
	Code int `json:"code"`
	Data *struct {
		AvailableBalance float64 `json:"available_balance"`
		VoucherBalance   float64 `json:"voucher_balance"`
		CashBalance      float64 `json:"cash_balance"`
	} `json:"data"`
}

func decodeMoonshot(host string, body []byte) (*Balance, error) {
	var mr moonshotResp
	if err := json.Unmarshal(body, &mr); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnreadable, err)
	}
	if mr.Code != 0 || mr.Data == nil {
		return nil, fmt.Errorf("%w: code %d", ErrUnreadable, mr.Code)
	}
	return &Balance{
		// The vendor documents available_balance <= 0 as the point where calls
		// stop being served, so this is the same fact is_available carries.
		Available: mr.Data.AvailableBalance > 0,
		Infos: []Info{{
			Currency:        moonshotCurrency(host),
			TotalBalance:    amount(mr.Data.AvailableBalance),
			GrantedBalance:  amount(mr.Data.VoucherBalance),
			ToppedUpBalance: amount(mr.Data.CashBalance),
		}},
	}, nil
}

// openRouterResp mirrors GET /api/v1/credits: lifetime purchases and lifetime
// spend in USD, with the remaining wallet left to the reader to subtract.
type openRouterResp struct {
	Data *struct {
		TotalCredits *float64 `json:"total_credits"`
		TotalUsage   *float64 `json:"total_usage"`
	} `json:"data"`
}

func decodeOpenRouter(_ string, body []byte) (*Balance, error) {
	var or openRouterResp
	if err := json.Unmarshal(body, &or); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnreadable, err)
	}
	if or.Data == nil || or.Data.TotalCredits == nil || or.Data.TotalUsage == nil {
		return nil, ErrUnreadable
	}
	remaining := *or.Data.TotalCredits - *or.Data.TotalUsage
	return &Balance{
		Available: remaining > 0,
		Infos:     []Info{{Currency: "USD", TotalBalance: amount(remaining)}},
	}, nil
}

func moonshotCurrency(host string) string {
	if strings.HasSuffix(host, ".ai") {
		return "USD"
	}
	return "CNY"
}

func amount(v float64) string { return strconv.FormatFloat(v, 'f', 2, 64) }
