package service

import (
	"maps"
	"testing"

	"github.com/Calcium-Ion/go-epay/epay"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testEpayClient(t *testing.T) *epay.Client {
	t.Helper()
	client, err := epay.NewClient(&epay.Config{PartnerID: "1000", Key: "test-merchant-key"}, "https://pay.example.com")
	require.NoError(t, err)
	return client
}

func signedNotify(key string, extra map[string]string) map[string]string {
	params := map[string]string{
		"pid":          "1000",
		"type":         "alipay",
		"trade_no":     "E20261007001",
		"out_trade_no": "USR1NOabc123",
		"name":         "TUC10",
		"money":        "10.00",
		"trade_status": epay.StatusTradeSuccess,
		"sign_type":    "MD5",
	}
	maps.Copy(params, extra)
	return epay.GenerateParams(params, key)
}

func TestVerifyEpayNotifyAcceptsGatewayPayload(t *testing.T) {
	client := testEpayClient(t)
	params := signedNotify(client.Config.Key, nil)

	info, err := VerifyEpayNotify(client, params)
	require.NoError(t, err)
	require.NotNil(t, info)
	assert.True(t, info.VerifyStatus)
	assert.Equal(t, "USR1NOabc123", info.ServiceTradeNo)
	assert.Equal(t, "10.00", info.Money)
	assert.Equal(t, epay.StatusTradeSuccess, info.TradeStatus)
}

func TestVerifyEpayNotifyRejectsPurchaseReplayAndInjectedStatus(t *testing.T) {
	client := testEpayClient(t)
	key := client.Config.Key

	t.Run("purchase params replay", func(t *testing.T) {
		purchase := epay.GenerateParams(map[string]string{
			"pid":          "1000",
			"type":         "alipay",
			"out_trade_no": "USR1NOabc123",
			"notify_url":   "https://api.example.com/api/user/epay/notify",
			"return_url":   "https://api.example.com/usage-logs",
			"name":         "TUC10",
			"money":        "10.00",
			"device":       "pc",
			"sign_type":    "MD5",
		}, key)
		_, err := VerifyEpayNotify(client, purchase)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrEpayNotifyUnexpectedField)
	})

	t.Run("return_url concatenation injects trade_status", func(t *testing.T) {
		purchase := epay.GenerateParams(map[string]string{
			"pid":          "1000",
			"type":         "alipay",
			"out_trade_no": "USR1NOabc123",
			"notify_url":   "https://api.example.com/api/user/epay/notify",
			"return_url":   "https://api.example.com/return&trade_status=TRADE_SUCCESS",
			"name":         "TUC10",
			"money":        "10.00",
			"sign_type":    "MD5",
		}, key)
		replay := map[string]string{
			"pid":          purchase["pid"],
			"type":         purchase["type"],
			"out_trade_no": purchase["out_trade_no"],
			"notify_url":   purchase["notify_url"],
			"return_url":   "https://api.example.com/return",
			"trade_status": epay.StatusTradeSuccess,
			"name":         purchase["name"],
			"money":        purchase["money"],
			"sign":         purchase["sign"],
			"sign_type":    "MD5",
		}
		_, err := VerifyEpayNotify(client, replay)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrEpayNotifyUnexpectedField)
	})

	t.Run("ampersand in signed value", func(t *testing.T) {
		params := signedNotify(key, map[string]string{"name": "TUC&hidden=1"})
		_, err := VerifyEpayNotify(client, params)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrEpayNotifyUnsafeValue)
	})

	t.Run("equals in signed value", func(t *testing.T) {
		params := signedNotify(key, map[string]string{"name": "TUC=10"})
		_, err := VerifyEpayNotify(client, params)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrEpayNotifyUnsafeValue)
	})
}

func TestVerifyEpayNotifyRejectsTamperedOrIncompletePayload(t *testing.T) {
	client := testEpayClient(t)
	key := client.Config.Key

	t.Run("wrong sign", func(t *testing.T) {
		params := signedNotify(key, nil)
		params["sign"] = "deadbeef"
		_, err := VerifyEpayNotify(client, params)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrEpayNotifyBadSign)
	})

	t.Run("unpaid status", func(t *testing.T) {
		params := signedNotify(key, map[string]string{"trade_status": "TRADE_PENDING"})
		_, err := VerifyEpayNotify(client, params)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrEpayNotifyNotPaid)
	})

	t.Run("pid mismatch", func(t *testing.T) {
		params := signedNotify(key, map[string]string{"pid": "9999"})
		_, err := VerifyEpayNotify(client, params)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrEpayNotifyPidMismatch)
	})

	t.Run("missing out_trade_no", func(t *testing.T) {
		params := signedNotify(key, map[string]string{"out_trade_no": ""})
		delete(params, "out_trade_no")
		_, err := VerifyEpayNotify(client, params)
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrEpayNotifyMissingField)
	})

	t.Run("invalid money", func(t *testing.T) {
		params := signedNotify(key, map[string]string{"money": "10.00&trade_status=TRADE_SUCCESS"})
		_, err := VerifyEpayNotify(client, params)
		require.Error(t, err)
	})

	t.Run("nil client", func(t *testing.T) {
		_, err := VerifyEpayNotify(nil, signedNotify(key, nil))
		require.Error(t, err)
		assert.ErrorIs(t, err, ErrEpayNotifyNotConfigured)
	})
}

func TestEpayMoneyMatchesOrder(t *testing.T) {
	assert.True(t, EpayMoneyMatches("10.00", 10))
	assert.True(t, EpayMoneyMatches("9.99", 9.99))
	assert.True(t, EpayMoneyMatches("10", 10))
	assert.False(t, EpayMoneyMatches("10.01", 10))
	assert.False(t, EpayMoneyMatches("", 10))
	assert.False(t, EpayMoneyMatches("abc", 10))
}

func TestSanitizeEpayGoodsName(t *testing.T) {
	assert.Equal(t, "SUB:Pro-Max", SanitizeEpayGoodsName("SUB:Pro&Max"))
	assert.Equal(t, "TUC10", SanitizeEpayGoodsName("TUC10"))
}

func TestEpayCallbackURLSafe(t *testing.T) {
	assert.NoError(t, CheckEpayCallbackURL("https://api.example.com/api/user/epay/notify"))
	assert.NoError(t, CheckEpayCallbackURL("https://api.example.com/usage-logs"))
	assert.ErrorIs(t, CheckEpayCallbackURL("https://api.example.com/notify?trade_status=TRADE_SUCCESS"), ErrEpayCallbackURLUnsafe)
	assert.ErrorIs(t, CheckEpayCallbackURL("https://api.example.com/return&trade_status=TRADE_SUCCESS"), ErrEpayCallbackURLUnsafe)
	assert.Error(t, CheckEpayCallbackURL("not-a-url"))
	assert.Error(t, CheckEpayCallbackURL(""))
}
