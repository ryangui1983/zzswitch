package openai

import (
	"fmt"
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/logger"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func OaiResponsesHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	defer service.CloseResponseBodyGracefully(resp)

	// read response body
	var responsesResponse dto.OpenAIResponsesResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	err = common.Unmarshal(responseBody, &responsesResponse)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if oaiError := responsesResponse.GetOpenAIError(); oaiError != nil && oaiError.Type != "" {
		return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
	}

	info.ObserveResponseModel(responsesResponse.Model)
	responseBody = rewriteSGLangResponsesCreatedAt(info, responseBody, "created_at", responsesResponse.CreatedAt)

	// compute usage
	usage := &dto.Usage{}
	service.ApplyResponsesUsage(usage, responsesResponse.Usage)
	if service.InflateUpstreamUsageFromInfo(usage, info) {
		responseBody = service.OverlayResponsesUsageJSON(responseBody, usage, "usage")
	}

	// 写入新的 response body
	service.IOCopyBytesGracefully(c, resp, responseBody)
	// Count actual tool invocations from Output (not tool declarations).
	for _, output := range responsesResponse.Output {
		switch output.Type {
		case dto.BuildInCallWebSearchCall:
			info.CountBillableToolCall(dto.BuildInCallWebSearchCall, "")
		case dto.BuildInCallFileSearchCall:
			info.CountBillableToolCall(dto.BuildInCallFileSearchCall, "")
		case dto.BuildInCallFunctionCall:
			info.CountBillableToolCall(dto.BuildInCallFunctionCall, output.Name)
		}
	}

	imageCounter := &relaycommon.ImageGenerationCallCounter{}
	if !relaycommon.IsNonBillableResponsesStatus(responsesResponse.Status) {
		for i := range responsesResponse.Output {
			idx := i
			imageCounter.Observe(&responsesResponse.Output[i], &idx)
		}
	}
	imageCounter.Commit(info)

	return usage, nil
}

func OaiResponsesStreamHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NewAPIError) {
	if resp == nil || resp.Body == nil {
		logger.LogError(c, "invalid response or response body")
		return nil, types.NewError(fmt.Errorf("invalid response"), types.ErrorCodeBadResponse)
	}

	defer service.CloseResponseBodyGracefully(resp)

	accumulator := service.NewResponsesUsageAccumulator(info)
	var streamFailure *types.OpenAIError

	helper.StreamScannerHandler(c, resp, info, func(data string, sr *helper.StreamResult) {

		// 检查当前数据是否包含 completed 状态和 usage 信息
		var streamResponse dto.ResponsesStreamResponse
		if err := common.UnmarshalJsonStr(data, &streamResponse); err != nil {
			logger.LogError(c, fmt.Sprintf("failed to unmarshal stream response: %s, raw_data=%s", err.Error(), data))
			sr.Error(err)
			return
		}
		if streamResponse.Response != nil {
			data = string(rewriteSGLangResponsesCreatedAt(info, []byte(data), "response.created_at", streamResponse.Response.CreatedAt))
		}
		if streamResponse.Type == "response.completed" || streamResponse.Type == "response.done" {
			logger.LogInfo(c, fmt.Sprintf("responses completed event, request_id=%s data=%s", info.RequestId, data))
		}
		accumulator.Observe(&streamResponse)
		if eventErr := responsesStreamEventError(&streamResponse); eventErr != nil {
			streamFailure = eventErr
		}
		if streamResponse.Response != nil && streamResponse.Response.Usage != nil {
			tmp := &dto.Usage{}
			service.ApplyResponsesUsage(tmp, streamResponse.Response.Usage)
			if inf := service.InflatedUsageCopyFromInfo(tmp, info); inf != nil && (inf.PromptTokens != tmp.PromptTokens || inf.CompletionTokens != tmp.CompletionTokens || inf.PromptTokensDetails.CachedTokens != tmp.PromptTokensDetails.CachedTokens) {
				data = string(service.OverlayResponsesUsageJSON(common.StringToByteSlice(data), inf, "response.usage"))
			}
		}
		sendResponsesStreamData(c, streamResponse, data)
	})

	logger.LogInfo(c, fmt.Sprintf("responses stream ended, request_id=%s end_reason=%s received=%d",
		info.RequestId, info.StreamStatus.EndReason, info.ReceivedResponseCount))
	common.SetContextKey(c, constant.ContextKeyResponseStreamStatus, info.StreamStatus)
	info.StreamStatus.RequireTerminal()
	usage := accumulator.Finish()
	if info.StreamStatus != nil && info.StreamStatus.ResponseFailed() {
		return usage, newResponsesStreamFailedError(streamFailure, info.StreamStatus)
	}
	return usage, nil
}

func responsesStreamEventError(event *dto.ResponsesStreamResponse) *types.OpenAIError {
	if event == nil {
		return nil
	}
	var oai *types.OpenAIError
	if event.Response != nil {
		if nested := event.Response.GetOpenAIError(); nested != nil {
			copied := *nested
			oai = &copied
		}
	}
	failedType := event.Type == "error" || event.Type == "response.error" || event.Type == "response.failed"
	if oai == nil && !failedType {
		return nil
	}
	if oai == nil {
		oai = &types.OpenAIError{}
	}
	if oai.Message == "" {
		oai.Message = event.Message
	}
	if oai.Code == nil && event.Code != "" {
		oai.Code = event.Code
	}
	if oai.Param == "" {
		oai.Param = event.Param
	}
	if oai.Message == "" && oai.Code == nil && (oai.Type == "" || oai.Type == "error") {
		return nil
	}
	return oai
}

func newResponsesStreamFailedError(eventErr *types.OpenAIError, status *relaycommon.StreamStatus) *types.NewAPIError {
	oai := types.OpenAIError{}
	if eventErr != nil {
		oai = *eventErr
	}
	if status != nil {
		snapshot := status.OutcomeSnapshot()
		if oai.Code == nil && snapshot.ErrorCode != "" {
			oai.Code = snapshot.ErrorCode
		}
		if oai.Type == "" {
			oai.Type = snapshot.ErrorType
		}
	}
	if oai.Type == "" || oai.Type == "error" || oai.Type == "response.failed" || oai.Type == "response.error" {
		oai.Type = "upstream_error"
	}
	if oai.Message == "" {
		if code, ok := oai.Code.(string); ok && code != "" {
			oai.Message = "responses stream failed: " + code
		} else {
			oai.Message = "responses stream failed"
		}
	}
	return types.WithOpenAIError(oai, http.StatusInternalServerError, types.ErrOptionWithSkipRetry())
}

func rewriteSGLangResponsesCreatedAt(info *relaycommon.RelayInfo, payload []byte, path string, createdAt dto.IntValue) []byte {
	if info.GetChannelType() != constant.ChannelTypeSGLang {
		return payload
	}
	if !gjson.GetBytes(payload, path).Exists() {
		return payload
	}
	patched, err := sjson.SetBytes(payload, path, int(createdAt))
	if err != nil {
		return payload
	}
	return patched
}
