package service

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"

	"github.com/gin-gonic/gin"
)

func CloseResponseBodyGracefully(httpResponse *http.Response) {
	if httpResponse == nil || httpResponse.Body == nil {
		return
	}
	err := httpResponse.Body.Close()
	if err != nil {
		common.SysError("failed to close response body: " + err.Error())
	}
}

// sensitiveUpstreamHeaders lists headers that could reveal the upstream
// provider's identity (CDN, cloud vendor, origin host). Stripping them
// prevents clients from discovering and directly connecting to the upstream.
var sensitiveUpstreamHeaders = map[string]bool{
	"server":            true,
	"cf-ray":            true,
	"cf-cache-status":   true,
	"cf-request-id":     true,
	"cf-connecting-ip":  true,
	"via":               true,
	"x-powered-by":      true,
	"x-served-by":       true,
	"x-cache":           true,
	"x-cache-hits":      true,
	"x-timer":           true,
	"x-amz-request-id":  true,
	"x-amzn-requestid":  true,
	"x-amzn-trace-id":   true,
	"x-azure-ref":       true,
	"x-ms-request-id":   true,
	"x-request-id":      true,
	"x-envoy-upstream-service-time": true,
}

// ShouldCopyUpstreamHeader checks whether a given upstream response header
// should be copied to the client response. It returns false for Content-Length
// (managed separately), X-Oneapi-Request-Id (to preserve the local instance
// ID), and headers that could reveal upstream provider identity.
func ShouldCopyUpstreamHeader(c *gin.Context, k string, v []string) bool {
	if strings.EqualFold(k, "Content-Length") {
		return false
	}
	if strings.EqualFold(k, common.RequestIdKey) {
		if c != nil && len(v) > 0 {
			c.Set(common.UpstreamRequestIdKey, v[0])
		}
		return false
	}
	if sensitiveUpstreamHeaders[strings.ToLower(k)] {
		return false
	}
	return true
}

func IOCopyBytesGracefully(c *gin.Context, src *http.Response, data []byte) {
	if c.Writer == nil {
		return
	}

	body := io.NopCloser(bytes.NewBuffer(data))

	// We shouldn't set the header before we parse the response body, because the parse part may fail.
	// And then we will have to send an error response, but in this case, the header has already been set.
	// So the httpClient will be confused by the response.
	// For example, Postman will report error, and we cannot check the response at all.
	if src != nil {
		for k, v := range src.Header {
			if !ShouldCopyUpstreamHeader(c, k, v) {
				continue
			}
			c.Writer.Header().Set(k, v[0])
		}
	}

	// set Content-Length header manually BEFORE calling WriteHeader
	c.Writer.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))

	// Write header with status code (this sends the headers)
	if src != nil {
		c.Writer.WriteHeader(src.StatusCode)
	} else {
		c.Writer.WriteHeader(http.StatusOK)
	}

	_, err := io.Copy(c.Writer, body)
	if err != nil {
		logger.LogError(c, fmt.Sprintf("failed to copy response body: %s", err.Error()))
	}
	c.Writer.Flush()
}
