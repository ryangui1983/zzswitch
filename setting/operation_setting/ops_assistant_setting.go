package operation_setting

import (
	"time"

	"github.com/QuantumNous/new-api/common"
)

func init() {
	// Log the URL after a short delay to ensure common.OpsAssistantURL is loaded
	go func() {
		time.Sleep(2 * time.Second)
		url := GetOpsAssistantURL()
		if url != "" {
			common.SysLog("OpsAssistantURL configured: " + url)
		} else {
			common.SysLog("OpsAssistantURL is empty - webhooks will not be sent")
		}
	}()
}

func GetOpsAssistantURL() string {
	return common.OpsAssistantURL
}
