package operation_setting

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/setting/config"
)

type QuotaSetting struct {
	EnableFreeModelPreConsume  bool    `json:"enable_free_model_pre_consume"`   // 是否对免费模型启用预消耗
	TrustQuotaUSD              float64 `json:"trust_quota_usd"`                 // 钱包免预扣门槛，0 表示禁用
	PreConsumeMultiplier       float64 `json:"pre_consume_multiplier"`          // 预计输入费用的预扣倍率，仅影响预留
	TokenMarkupRatio           float64 `json:"token_markup_ratio"`              // 输入/输出/缓存写入加价比例上限，实际取值在 [上限-0.5, 上限] 间随机
	TokenMarkupInputThreshold  int     `json:"token_markup_input_threshold"`    // 输入超过该值才加价
	TokenMarkupOutputThreshold int     `json:"token_markup_output_threshold"`   // 输出超过该值才加价
	CacheHitBoostProbability   float64 `json:"cache_hit_boost_probability"`     // 命中率低于目标时提升的概率，0-1
	CacheHitBoostTarget        float64 `json:"cache_hit_boost_target"`          // 目标命中率，0-1，默认 0.9
	CacheHitBoostChannelIds    string  `json:"cache_hit_boost_channel_ids"`     // 逗号分隔的渠道 ID，仅这些渠道做命中率提升
	CacheHitBoostMaxOutputTPS  float64 `json:"cache_hit_boost_max_output_tps"`  // boosted output cap in token/s, default 80; negative disables
	CacheHitBoostMaxExtraRatio float64 `json:"cache_hit_boost_max_extra_ratio"` // when over red line, extra scale cap (0.1 = +10%)
	CacheHitBoostBudgetRatio   float64 `json:"cache_hit_boost_budget_ratio"`    // share of original charge used to raise hit, default 0.05
}

var quotaSetting = QuotaSetting{
	EnableFreeModelPreConsume:  true,
	TrustQuotaUSD:              10,
	PreConsumeMultiplier:       1,
	TokenMarkupRatio:           0.1,
	TokenMarkupInputThreshold:  1000,
	TokenMarkupOutputThreshold: 100,
	CacheHitBoostProbability:   0.9,
	CacheHitBoostTarget:        0.9,
	CacheHitBoostChannelIds:    "",
	CacheHitBoostMaxOutputTPS:  80,
	CacheHitBoostMaxExtraRatio: 0.1,
	CacheHitBoostBudgetRatio:   0.05,
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

func (s *QuotaSetting) EffectiveCacheHitBoostMaxOutputTPS() float64 {
	if s == nil || s.CacheHitBoostMaxOutputTPS == 0 {
		return 80
	}
	if s.CacheHitBoostMaxOutputTPS < 0 {
		return 0
	}
	return s.CacheHitBoostMaxOutputTPS
}

func (s *QuotaSetting) EffectiveCacheHitBoostMaxExtraRatio() float64 {
	if s == nil || s.CacheHitBoostMaxExtraRatio == 0 {
		return 0.1
	}
	if s.CacheHitBoostMaxExtraRatio < 0 {
		return 0
	}
	return s.CacheHitBoostMaxExtraRatio
}

func (s *QuotaSetting) EffectiveCacheHitBoostBudgetRatio() float64 {
	if s == nil || s.CacheHitBoostBudgetRatio == 0 {
		return 0.05
	}
	if s.CacheHitBoostBudgetRatio < 0 {
		return 0
	}
	if s.CacheHitBoostBudgetRatio > 1 {
		return 1
	}
	return s.CacheHitBoostBudgetRatio
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

// ValidateQuotaOption validates reservation settings before they are persisted.
func ValidateQuotaOption(key, value string) error {
	if key != "quota_setting.trust_quota_usd" && key != "quota_setting.pre_consume_multiplier" {
		return nil
	}
	number, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(number) || math.IsInf(number, 0) || number < 0 {
		return fmt.Errorf("%s must be a finite non-negative number", key)
	}
	if key == "quota_setting.pre_consume_multiplier" && number == 0 {
		return fmt.Errorf("%s must be greater than zero", key)
	}
	return nil
}

// InputPreConsumeMultiplier rejects invalid runtime settings as well as invalid saves.
func InputPreConsumeMultiplier() (float64, error) {
	multiplier := quotaSetting.PreConsumeMultiplier
	if multiplier <= 0 || math.IsNaN(multiplier) || math.IsInf(multiplier, 0) {
		return 0, fmt.Errorf("pre-consume multiplier must be a finite number greater than zero")
	}
	return multiplier, nil
}
