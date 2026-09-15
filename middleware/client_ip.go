package middleware

import (
	"net"
	"strings"

	"github.com/gin-gonic/gin"
)

// PreferForwardedClientIP rewrites X-Forwarded-For / X-Real-IP from the
// Cloudflare connecting IP (or the first public hop) so gin.Context.ClientIP
// and IP rate limits are not collapsed onto the reverse-proxy address
// (for example 172.18.0.7).
func PreferForwardedClientIP() gin.HandlerFunc {
	return func(c *gin.Context) {
		if ip := publicClientIP(c); ip != "" {
			c.Request.Header.Set("X-Real-IP", ip)
			c.Request.Header.Set("X-Forwarded-For", ip)
		}
		c.Next()
	}
}

func publicClientIP(c *gin.Context) string {
	for _, header := range []string{"CF-Connecting-IP", "True-Client-IP", "X-Real-IP"} {
		if ip := parsePublicIP(c.GetHeader(header)); ip != "" {
			return ip
		}
	}
	for _, part := range strings.Split(c.GetHeader("X-Forwarded-For"), ",") {
		if ip := parsePublicIP(part); ip != "" {
			return ip
		}
	}
	return ""
}

func parsePublicIP(raw string) string {
	host := strings.TrimSpace(raw)
	if host == "" {
		return ""
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	ip := net.ParseIP(host)
	if ip == nil || ip.IsUnspecified() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsPrivate() {
		return ""
	}
	return ip.String()
}
