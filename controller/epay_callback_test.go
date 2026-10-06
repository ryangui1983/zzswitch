package controller

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEpayBrowserReturnPathNeverCredits(t *testing.T) {
	previous := system_setting.ServerAddress
	system_setting.ServerAddress = "https://api.example.com"
	t.Cleanup(func() { system_setting.ServerAddress = previous })

	assert.Equal(t, "https://api.example.com/wallet?pay=fail", epayBrowserReturnPath(nil))
	assert.Equal(t, "https://api.example.com/wallet?pay=success", epayBrowserReturnPath(&model.SubscriptionOrder{Status: common.TopUpStatusSuccess}))
	assert.Equal(t, "https://api.example.com/wallet?pay=pending", epayBrowserReturnPath(&model.SubscriptionOrder{Status: common.TopUpStatusPending}))
}

func TestParseEpayCallbackParamsPrefersPostForm(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	form := url.Values{}
	form.Set("out_trade_no", "USR1NO1")
	form.Set("trade_status", "TRADE_SUCCESS")
	req := httptest.NewRequest(http.MethodPost, "/api/user/epay/notify?out_trade_no=query", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	c.Request = req

	params, err := parseEpayCallbackParams(c)
	require.NoError(t, err)
	assert.Equal(t, "USR1NO1", params["out_trade_no"])
	assert.Equal(t, "TRADE_SUCCESS", params["trade_status"])
}

func TestParseEpayCallbackURLRejectsQueryInjection(t *testing.T) {
	_, err := parseEpayCallbackURL("https://api.example.com/api/user/epay/notify?trade_status=TRADE_SUCCESS")
	require.Error(t, err)
	u, err := parseEpayCallbackURL("https://api.example.com/api/user/epay/notify")
	require.NoError(t, err)
	assert.Equal(t, "/api/user/epay/notify", u.Path)
}
