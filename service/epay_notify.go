package service

import (
	"errors"
	"maps"
	"net/url"
	"regexp"
	"strings"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/shopspring/decimal"
)

var (
	ErrEpayNotifyNotConfigured   = errors.New("epay client not configured")
	ErrEpayNotifyUnexpectedField = errors.New("epay notify contains unexpected field")
	ErrEpayNotifyUnsafeValue     = errors.New("epay notify value contains unsafe characters")
	ErrEpayNotifyMissingField    = errors.New("epay notify missing required field")
	ErrEpayNotifyBadSign         = errors.New("epay notify signature mismatch")
	ErrEpayNotifyNotPaid         = errors.New("epay notify trade is not paid")
	ErrEpayNotifyPidMismatch     = errors.New("epay notify pid mismatch")
	ErrEpayNotifyInvalidMoney    = errors.New("epay notify money is invalid")
	ErrEpayCallbackURLUnsafe     = errors.New("epay callback url is unsafe")
)

// Fields our gateway (and standard EasyPay notify) may send. Purchase-only
// keys such as notify_url / return_url / device are intentionally absent so a
// submit.php signature cannot be replayed as a paid callback.
var epayNotifyAllowedKeys = map[string]struct{}{
	"pid":          {},
	"type":         {},
	"trade_no":     {},
	"out_trade_no": {},
	"name":         {},
	"money":        {},
	"trade_status": {},
	"param":        {},
	"sign":         {},
	"sign_type":    {},
	"buyer":        {},
	"api_trade_no": {},
}

var epayNotifyRequiredKeys = []string{
	"pid",
	"out_trade_no",
	"money",
	"trade_status",
	"sign",
}

var epayNotifyMoneyPattern = regexp.MustCompile(`^[0-9]+(\.[0-9]{1,2})?$`)

// VerifyEpayNotify allowlists notify fields, rejects concatenation-injection
// characters in values, then verifies the MD5 sign against the merchant key.
func VerifyEpayNotify(client *epay.Client, params map[string]string) (*epay.VerifyRes, error) {
	if client == nil {
		return nil, ErrEpayNotifyNotConfigured
	}
	sanitized, err := sanitizeEpayNotifyParams(params)
	if err != nil {
		return nil, err
	}
	if sanitized["pid"] != client.Config.PartnerID {
		return nil, ErrEpayNotifyPidMismatch
	}
	if !epayNotifyMoneyPattern.MatchString(sanitized["money"]) {
		return nil, ErrEpayNotifyInvalidMoney
	}
	if signType := sanitized["sign_type"]; signType != "" && !strings.EqualFold(signType, "MD5") {
		return nil, ErrEpayNotifyBadSign
	}

	verifyParams := maps.Clone(sanitized)
	info, err := client.Verify(verifyParams)
	if err != nil {
		return nil, err
	}
	if info == nil || !info.VerifyStatus {
		return nil, ErrEpayNotifyBadSign
	}
	if info.TradeStatus != epay.StatusTradeSuccess {
		return nil, ErrEpayNotifyNotPaid
	}
	if info.ServiceTradeNo == "" {
		return nil, ErrEpayNotifyMissingField
	}
	return info, nil
}

func sanitizeEpayNotifyParams(params map[string]string) (map[string]string, error) {
	if len(params) == 0 {
		return nil, ErrEpayNotifyMissingField
	}
	out := make(map[string]string, len(params))
	for key, value := range params {
		if _, ok := epayNotifyAllowedKeys[key]; !ok {
			return nil, ErrEpayNotifyUnexpectedField
		}
		if key != "sign" && key != "sign_type" && strings.ContainsAny(value, "&=") {
			return nil, ErrEpayNotifyUnsafeValue
		}
		out[key] = value
	}
	for _, key := range epayNotifyRequiredKeys {
		if strings.TrimSpace(out[key]) == "" {
			return nil, ErrEpayNotifyMissingField
		}
	}
	return out, nil
}

func EpayMoneyMatches(callbackMoney string, orderMoney float64) bool {
	if !epayNotifyMoneyPattern.MatchString(callbackMoney) {
		return false
	}
	callback, err := decimal.NewFromString(callbackMoney)
	if err != nil {
		return false
	}
	return callback.Equal(decimal.NewFromFloat(orderMoney).Round(2))
}

func SanitizeEpayGoodsName(name string) string {
	return strings.Map(func(r rune) rune {
		if r == '&' || r == '=' {
			return '-'
		}
		return r
	}, name)
}

func CheckEpayCallbackURL(raw string) error {
	if raw == "" || strings.ContainsAny(raw, "&=") {
		return ErrEpayCallbackURLUnsafe
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ErrEpayCallbackURLUnsafe
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return ErrEpayCallbackURLUnsafe
	}
	switch parsed.Scheme {
	case "http", "https":
		return nil
	default:
		return ErrEpayCallbackURLUnsafe
	}
}
