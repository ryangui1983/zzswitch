package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPreferForwardedClientIPUsesCloudflareConnectingIP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("TRUSTED_PROXIES", "")
	router := gin.New()
	require.NoError(t, ConfigureTrustedProxies(router))
	router.Use(PreferForwardedClientIP())
	router.GET("/ip", func(c *gin.Context) {
		c.String(http.StatusOK, c.ClientIP())
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/ip", nil)
	request.RemoteAddr = "172.18.0.7:12345"
	request.Header.Set("CF-Connecting-IP", "64.83.9.189")
	request.Header.Set("X-Forwarded-For", "172.18.0.7")
	router.ServeHTTP(recorder, request)

	assert.Equal(t, "64.83.9.189", recorder.Body.String())
}

func TestPreferForwardedClientIPIgnoresPrivateSpoof(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("TRUSTED_PROXIES", "")
	router := gin.New()
	require.NoError(t, ConfigureTrustedProxies(router))
	router.Use(PreferForwardedClientIP())
	router.GET("/ip", func(c *gin.Context) {
		c.String(http.StatusOK, c.ClientIP())
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/ip", nil)
	request.RemoteAddr = "172.18.0.7:12345"
	request.Header.Set("CF-Connecting-IP", "10.0.0.8")
	request.Header.Set("X-Forwarded-For", "203.0.113.50, 172.64.1.1")
	router.ServeHTTP(recorder, request)

	assert.Equal(t, "203.0.113.50", recorder.Body.String())
}
