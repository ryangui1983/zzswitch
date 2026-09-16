package operation_setting

import (
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/setting/config"
)

type QuotaSetting struct {
	EnableFreeModelPreConsume  bool    `json:"enable_free_model_pre_consume"`
	TokenMarkupRatio           float64 `json:"token_markup_ratio"`            // 输入/输出/缓存写入加价比例，0.1 = 10%
	TokenMarkupInputThreshold  int     `json:"token_markup_input_threshold"`  // 输入超过该值才加价
	TokenMarkupOutputThreshold int     `json:"token_markup_output_threshold"` // 输出超过该值才加价
	CacheHitBoostProbability   float64 `json:"cache_hit_boost_probability"`   // 命中率低于目标时提升的概率，0-1
	CacheHitBoostTarget        float64 `json:"cache_hit_boost_target"`        // 目标命中率，0-1，默认 0.9
	CacheHitBoostChannelIds    string  `json:"cache_hit_boost_channel_ids"`   // 逗号分隔的渠道 ID，仅这些渠道做命中率提升
}

var quotaSetting = QuotaSetting{
	EnableFreeModelPreConsume:  true,
	TokenMarkupRatio:           0.1,
	TokenMarkupInputThreshold:  1000,
	TokenMarkupOutputThreshold: 100,
	CacheHitBoostProbability:   0.9,
	CacheHitBoostTarget:        0.9,
	CacheHitBoostChannelIds:    "",
}

func init() {
	config.GlobalConfig.Register("quota_setting", &quotaSetting)
}

func GetQuotaSetting() *QuotaSetting {
	return &quotaSetting
}

func (s *QuotaSetting) EffectiveTokenMarkupRatio() float64 {
	if s == nil {
		return 0.1
	}
	if s.TokenMarkupRatio < 0 {
		return 0
	}
	return s.TokenMarkupRatio
}

func (s *QuotaSetting) EffectiveCacheHitBoostTarget() float64 {
	if s == nil || s.CacheHitBoostTarget <= 0 {
		return 0.9
	}
	if s.CacheHitBoostTarget > 1 {
		return 1
	}
	return s.CacheHitBoostTarget
}

func (s *QuotaSetting) AllowsCacheHitBoost(channelId int) bool {
	if s == nil || channelId <= 0 {
		return false
	}
	for _, part := range strings.FieldsFunc(s.CacheHitBoostChannelIds, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\n' || r == '\t' || r == ';'
	}) {
		id, err := strconv.Atoi(strings.TrimSpace(part))
		if err == nil && id == channelId {
			return true
		}
	}
	return false
}
