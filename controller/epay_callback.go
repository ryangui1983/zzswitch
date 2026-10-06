package controller

import (
	"net/http"
	"net/url"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

func parseEpayCallbackParams(c *gin.Context) (map[string]string, error) {
	if c.Request.Method == http.MethodPost {
		if err := c.Request.ParseForm(); err != nil {
			return nil, err
		}
		return formValuesToMap(c.Request.PostForm), nil
	}
	return formValuesToMap(c.Request.URL.Query()), nil
}

func formValuesToMap(values url.Values) map[string]string {
	out := make(map[string]string, len(values))
	for key := range values {
		out[key] = values.Get(key)
	}
	return out
}

func parseEpayCallbackURL(raw string) (*url.URL, error) {
	if err := service.CheckEpayCallbackURL(raw); err != nil {
		return nil, err
	}
	return url.Parse(raw)
}

// epayBrowserReturnPath never fulfills an order. Browser return is user-controlled
// and must not credit; notify is the only source of truth.
func epayBrowserReturnPath(order *model.SubscriptionOrder) string {
	if order != nil && order.Status == common.TopUpStatusSuccess {
		return paymentReturnPath("/wallet?pay=success")
	}
	if order != nil && order.Status == common.TopUpStatusPending {
		return paymentReturnPath("/wallet?pay=pending")
	}
	return paymentReturnPath("/wallet?pay=fail")
}
