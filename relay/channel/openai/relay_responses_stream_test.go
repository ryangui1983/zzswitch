package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/constant"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newResponsesStreamTestContext(t *testing.T, body string) (*gin.Context, *httptest.ResponseRecorder, *http.Response, *relaycommon.RelayInfo) {
	t.Helper()

	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })

	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
	}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: "gpt-5.6"},
		IsStream:    true,
		DisablePing: true,
		RequestId:   "responses-stream-test",
	}
	return c, recorder, resp, info
}

func TestOaiResponsesStreamHandlerTreatsFailedAsError(t *testing.T) {
	const overloaded = "Our servers are currently overloaded. Please try again later."
	body := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_1","status":"in_progress","model":"gpt-5.6"}}`,
		`data: {"type":"response.in_progress","response":{"id":"resp_1","status":"in_progress"}}`,
		`data: {"type":"error","code":"server_error","message":"` + overloaded + `","param":null}`,
		`data: {"type":"response.failed","response":{"id":"resp_1","status":"failed","error":{"code":"server_error","message":"` + overloaded + `"}}}`,
		``,
	}, "\n")

	c, recorder, resp, info := newResponsesStreamTestContext(t, body)
	usage, apiErr := OaiResponsesStreamHandler(c, info, resp)

	require.NotNil(t, apiErr)
	assert.True(t, types.IsSkipRetryError(apiErr))
	assert.Equal(t, http.StatusInternalServerError, apiErr.StatusCode)
	assert.Contains(t, apiErr.Error(), overloaded)
	assert.Equal(t, types.ErrorCode("server_error"), apiErr.GetErrorCode())
	require.NotNil(t, usage)
	require.NotNil(t, info.StreamStatus)
	assert.True(t, info.StreamStatus.ResponseFailed())
	assert.Contains(t, recorder.Body.String(), `"type":"response.failed"`)
	assert.Contains(t, recorder.Body.String(), overloaded)
}

func TestOaiResponsesStreamHandlerCompletedStillSucceeds(t *testing.T) {
	body := strings.Join([]string{
		`data: {"type":"response.created","response":{"id":"resp_1","status":"in_progress","model":"gpt-5.6"}}`,
		`data: {"type":"response.output_text.delta","delta":"hello"}`,
		`data: {"type":"response.completed","response":{"id":"resp_1","status":"completed","usage":{"input_tokens":2,"output_tokens":1,"total_tokens":3}}}`,
		`data: [DONE]`,
		``,
	}, "\n")

	c, recorder, resp, info := newResponsesStreamTestContext(t, body)
	usage, apiErr := OaiResponsesStreamHandler(c, info, resp)

	require.Nil(t, apiErr)
	require.NotNil(t, usage)
	assert.Equal(t, 2, usage.PromptTokens)
	assert.Equal(t, 1, usage.CompletionTokens)
	require.NotNil(t, info.StreamStatus)
	assert.False(t, info.StreamStatus.ResponseFailed())
	assert.Equal(t, "completed", info.StreamStatus.ResponseOutcome())
	assert.Contains(t, recorder.Body.String(), `"type":"response.completed"`)
}

func TestOaiResponsesStreamHandlerIncompleteIsNotFailure(t *testing.T) {
	body := strings.Join([]string{
		`data: {"type":"response.incomplete","response":{"id":"resp_1","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"}}}`,
		``,
	}, "\n")

	c, _, resp, info := newResponsesStreamTestContext(t, body)
	_, apiErr := OaiResponsesStreamHandler(c, info, resp)

	require.Nil(t, apiErr)
	require.NotNil(t, info.StreamStatus)
	assert.False(t, info.StreamStatus.ResponseFailed())
	assert.Equal(t, "incomplete", info.StreamStatus.ResponseOutcome())
}

func TestNewResponsesStreamFailedErrorSkipsRetry(t *testing.T) {
	status := relaycommon.NewStreamStatus()
	status.MarkFailed("server_is_overloaded", "server_error", 0)
	apiErr := newResponsesStreamFailedError(&types.OpenAIError{
		Message: "Our servers are currently overloaded. Please try again later.",
		Type:    "server_error",
		Code:    "server_is_overloaded",
	}, status)

	require.NotNil(t, apiErr)
	assert.True(t, types.IsSkipRetryError(apiErr))
	assert.Equal(t, http.StatusInternalServerError, apiErr.StatusCode)
	assert.Equal(t, types.ErrorCode("server_is_overloaded"), apiErr.GetErrorCode())
	assert.Contains(t, apiErr.Error(), "overloaded")
}
