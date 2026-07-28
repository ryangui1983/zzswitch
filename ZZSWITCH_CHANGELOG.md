# ZZSwitch 自定义开发变更记录

> 说明：以功能为单位分组，同一功能的 feat+fix 合并为一条。
> 状态标记：✅ 正常 | ⚠️ 需确认 | ❌ 已丢失（待修复）
>
> 用途：审计每次合并是否错误覆盖自定义内容，以及是否与官方代码存在逻辑冲突。

---

## 功能列表

### [F-01] ✅ 渠道池模式 + 健康自动化
**Commits:** `d1085bbe` feat + `8bd666f8` fix + `63b4a10a` fix + `8d6acbea` fix  
**日期：** 2026-06-21 ~ 2026-07-04  
**描述：**
- 渠道支持池模式（同渠道按配置重试，再轮转到下一优先级）
- 短窗口错误率自动禁用（可配置窗口/阈值/最小请求数/状态码范围）
- 每个渠道可独立开启自动恢复（auto-recovery）探测
- 中间失败（流转失败）独立写 type=8 日志，普通用户不可见

**相关文件（Go）：**
- `controller/relay.go` — 重试逻辑、concurrencyAcquiredChannelId、webhook 触发
- `controller/channel_health.go` — 自动恢复探测任务
- `controller/ops_webhook.go` — dispatch/complete 异步 webhook 队列
- `dto/channel_settings.go` — SchedulerPoolMode*/ErrorRatio*/ProbeBlock* 字段
- `model/ability.go` — ExhaustedChannelIds 渠道排除逻辑
- `model/channel_cache.go` — 并发限制过滤
- `service/channel.go` — ShouldDisableChannel/EnableChannel
- `service/channel_health.go` — 错误率计算（新建文件）
- `service/channel_select.go` — RetryParam.SelectRetryChannel
- `setting/operation_setting/status_code_ranges.go` — HTTP 状态码范围解析

**相关文件（前端 default）：**
- `features/channels/components/drawers/channel-mutate-drawer.tsx` — Channel Health Automation section
- `features/channels/lib/channel-form.ts` — schema 新增字段
- `features/channels/types.ts` — ChannelSettings 新增字段

**潜在冲突点：** 官方也有 auto-recovery 相关逻辑；`shouldRetry`/`shouldDisableChannel` 逻辑要注意版本漂移

---

### [F-02] ✅ 分组状态监控页（iframe 嵌入）
**Commits:** `9feeed15` feat  
**日期：** 2026-06-23  
**描述：** sidebar 新增"分组状态"入口，嵌入 `https://monitor.zzswitch.com/status/status`

**相关文件：**
- `web/default/src/routes/_authenticated/group-status/index.tsx` — 新建页面
- `web/default/src/hooks/use-sidebar-data.ts` — sidebar 入口（Monitor 图标）
- `web/default/src/routeTree.gen.ts` — 路由注册

**历史状态：** ❌ 曾在合并中丢失 sidebar 入口，已在 `0b11a664` 修复

---

### [F-03] ✅ 定价/倍率输入精度提升
**Commits:** `8f79e5c0` chore  
**日期：** 2026-06-23  
**描述：** pricing 和 group-ratio 编辑器步进从 0.01 → 0.0001

**相关文件：**
- `web/default/src/features/system-settings/general/pricing-section.tsx`
- `web/default/src/features/system-settings/models/group-ratio-visual-editor.tsx`

**潜在冲突点：** 官方 UI 重构时可能被覆盖精度设置

---

### [F-04] ✅ 每渠道探测拦截 + 错误率自动禁用
**Commits:** `dab70d16` feat  
**日期：** 2026-06-23  
**描述：**
- ProbeBlockEnabled：拦截 max_tokens≤5 的非流式探测请求
- ProbeBlockFakeSuccess：对探测返回假成功而非报错
- 日志泛滥防护（中间渠道失败不写普通用户日志）

**相关文件：**
- `controller/relay.go` — 探测判断逻辑
- `dto/channel_settings.go` — ProbeBlock* 字段
- `features/channels/components/drawers/channel-mutate-drawer.tsx`
- `features/channels/lib/channel-form.ts`

---

### [F-05] ✅ 上游成本追踪（upstream_cost 字段）
**Commits:** `90fcd200` feat  
**日期：** 2026-06-23  
**描述：**
- Channel 模型新增 `upstream_cost float64` 字段
- 结算时锁定当时倍率写入 upstream_cost
- 前端 BalanceCell 直接读 channel.upstream_cost 展示成本（非实时计算）
- tag 行聚合时累加 upstream_cost

**相关文件（Go）：**
- `model/channel.go` — upstream_cost 字段
- `service/quota.go`、`service/text_quota.go` — 结算时写 upstream_cost

**相关文件（前端）：**
- `features/channels/components/channels-columns.tsx` — BalanceCell costDisplay 逻辑
- `features/channels/lib/channel-utils.ts` — getChannelUpstreamRateMultiplier、upstream_cost 聚合
- `features/channels/types.ts` — upstream_cost 字段

**潜在冲突点：** 官方 balance 列后来改为 "Used/Remaining"，与我们的 "Used/Cost" 有概念差异

---

### [F-06] ✅ 推广返佣系统（AffCommissionRate）
**Commits:** `5ad14e14` feat  
**日期：** 2026-06-23  
**描述：** 新增 aff_commission_earned 字段，返佣比率可配置

**相关文件（Go）：**
- `common/constants.go` — AffCommissionRate 变量
- `model/option.go` — 读写 AffCommissionRate
- `model/user.go` — AffCommissionEarned int64 字段

**相关文件（前端）：**
- `features/wallet/components/affiliate-rewards-card.tsx` — Commission Earned 统计项
- `features/wallet/types.ts` — aff_commission_earned 字段

**历史状态：** ❌ 曾在合并中丢失 aff_commission_earned 统计项，已在 `0b5e4738` 修复

---

### [F-07] ✅ 中间失败独立日志（type=8）
**Commits:** `9564b793` feat + `eadec2f6` feat  
**日期：** 2026-06-24 ~ 2026-07-10  
**描述：**
- 渠道流转中间失败写 type=8 日志（仅 root 可见）
- 前端 classic/default 日志筛选对非管理员隐藏 type=8 选项
- 后端 GetUserLogs 强制排除 type=8

**相关文件（Go）：**
- `controller/relay.go` — 中间失败标记写日志
- `model/log.go` — GetUserLogs 排除 type=8

**相关文件（前端）：**
- `web/classic/.../UsageLogsFilters.jsx` — type 7/8 选项（isAdminUser 控制）
- `web/classic/.../UsageLogsColumnDefs.jsx` — case 7/8 渲染
- `features/usage-logs/components/common-logs-filter-bar.tsx` — isAdmin 过滤
- `features/usage-logs/constants.ts` — type 8 Intermediate Error

**历史状态：** ⚠️ `6530176c` 修复过 classic 丢失，`aa2396f9` 补了后端 guard

---

### [F-08] ✅ 运营助手 Webhook 集成
**Commits:** `b686608a` feat + `44805c76` feat  
**日期：** 2026-06-26 ~ 2026-06-29  
**描述：**
- 新增 OpsAssistantURL 系统配置
- 渠道 dispatch/complete/ttfb 事件实时推送
- TTFB webhook 事件（首字节时间上报）

**相关文件（Go）：**
- `controller/ops_webhook.go` — 异步队列 + worker（新建文件）
- `controller/relay.go` — 事件触发点
- `common/constants.go` — OpsAssistantURL
- `model/option.go` — OpsAssistantURL 读写
- `setting/operation_setting/ops_assistant_setting.go` — 设置封装（新建文件）

**相关文件（前端）：**
- `features/system-settings/integrations/monitoring-settings-section.tsx` — Webhook URL 输入
- `web/classic/.../SettingsMonitoring.jsx` — classic 主题同步

**潜在冲突点：** relay.go 修改较重，每次合并都容易被官方逻辑覆盖

---

### [F-09] ✅ 禁用 image_generation 工具（DisableImageGenerationTool）
**Commits:** `0c9d6384` feat  
**日期：** 2026-07-01  
**描述：** 渠道级开关，转发前过滤 tools 数组中的 image_generation，避免上游不支持时返回 403

**相关文件：**
- `dto/channel_settings.go` — DisableImageGenerationTool 字段
- `relay/common/relay_info.go` — RemoveDisabledFields 处理
- `features/channels/components/drawers/channel-mutate-drawer.tsx`
- `features/channels/types.ts`

---

### [F-10] ✅ Token 细分统计 + 缓存命中率
**Commits:** `17973864` feat + `6f18fefb` fix + `48eb8074` fix + `c42ae099` fix  
**日期：** 2026-07-02  
**描述：**
- 日志列新增 prompt/completion/cache token 细分展示
- 缓存命中率计算（区分 Anthropic 和 OpenAI 语义）
- Dashboard 统计卡新增 token 统计

**相关文件：**
- `features/usage-logs/components/columns/common-logs-columns.tsx` — 缓存命中率列
- `features/dashboard/lib/stats.ts` — totalPromptTokens 等
- `features/dashboard/components/models/log-stat-cards.tsx` — token 统计卡

---

### [F-11] ✅ 每渠道并发限制（MaxConcurrentRequests）
**Commits:** `bbe895fe` feat  
**日期：** 2026-07-04  
**描述：** 渠道级并发槽位，CAS 原子操作

**相关文件（Go）：**
- `common/channel_concurrency.go` — TryAcquireChannelSlot 等（新建文件）
- `dto/channel_settings.go` — MaxConcurrentRequests 字段
- `controller/relay.go` — 获取/释放槽位
- `model/channel_cache.go` — 过滤饱和渠道

**相关文件（前端）：**
- `features/channels/components/drawers/channel-mutate-drawer.tsx`
- `features/channels/lib/channel-form.ts`
- `features/channels/types.ts`

---

### [F-12] ✅ CPU 优化（热路径 + gopool）
**Commits:** `b367a6b6` perf + `625dc5a7` perf + `c962b7da` perf  
**日期：** 2026-07-06  
**描述：**
- 删除热路径5处 SysLog
- channel_concurrency 全局 Mutex → sync.Map
- Clone() 替换 copier 深拷贝（Go 中最大 CPU 热点）
- gopool 容量上限调整

**相关文件：**
- `common/channel_concurrency.go`
- `common/copy.go` — Cloner 接口 + 快路径
- `dto/clone.go` — GeneralOpenAIRequest.Clone()、OpenAIResponsesRequest.Clone()
- `common/gopool.go`
- `controller/relay.go`

---

### [F-13] ✅ gpt-5.6 系列 CompletionRatio 可覆盖
**Commits:** `6530176c` fix（含）  
**日期：** 2026-07-10  
**描述：** gpt-5.6 系列默认 completion ratio 设为可由管理员覆盖（return false）

**相关文件：**
- `setting/ratio_setting/model_ratio.go` — CompletionRatioIsCustomizable

---

### [F-14] ✅ Responses API 兼容修复
**Commits:** `09b89fd4` feat + `e9562d4c` fix  
**日期：** 2026-07-09  
**描述：**
- max_output_tokens 在 Chat Completions 路径归一化为 max_completion_tokens
- ConvertOpenAIResponsesRequest：剥离 max_output_tokens + 内置工具 arguments 白名单转对象
- max_concurrent_requests 前端表单

**相关文件：**
- `dto/openai_request.go` — MaxOutputTokens 字段
- `dto/clone.go`
- `relay/compatible_handler.go`
- `relay/channel/openai/adaptor.go` — objectArgumentItemTypes 白名单

---

### [F-15] ✅ BCP-47 语言标签修复
**Commits:** `d0c9e27f` fix  
**日期：** 2026-07-09  
**描述：** channels-columns.tsx 的 Intl.RelativeTimeFormat 使用 toIntlLocale 转换，避免 zhCN 导致 ReferenceError

**相关文件：**
- `features/channels/components/channels-columns.tsx`
- `i18n/languages.ts` — toIntlLocale（官方已有）

---

### [F-16] ✅ 前端各类合并后修复
**Commits:** `9efe0339`、`5cf72049`、`0b5e4738`、`6e4e9c20`、`0b11a664`  
**日期：** 2026-07-10~11  
**描述：** 每次合并后补回丢失内容：
- ChannelEditorNav + SENSITIVE_FORM_FIELDS（9efe0339）
- PriorityCell/WeightCell ConfirmDialog i18n（5cf72049）
- aff_commission_earned 统计项（0b5e4738）
- BalanceCell layout 变量 + Used/Cost 列头（6e4e9c20）
- 分组状态 sidebar 入口（0b11a664）

---

## 已知潜在问题清单

| 编号 | 问题描述 | 涉及文件 | 优先级 |
|------|---------|---------|------|
| P-01 | relay.go 改动量大，每次合并都需人工校验 dispatch/complete webhook 调用点 | `controller/relay.go` | 高 |
| P-02 | BalanceCell 显示"进价成本"，官方显示"剩余余额"，概念不同，合并时易被覆盖 | `channels-columns.tsx` | 中 |
| P-03 | channel_health.go 官方也有同名文件，逻辑需区分 | `controller/channel_health.go`、`service/channel_health.go` | 高 |
| P-04 | type=8 日志后端过滤（GetUserLogs），官方没有，每次后端合并需重新确认 | `model/log.go` | 中 |
| P-05 | ops_webhook.go 是全新文件，合并一般不会删，但 relay.go 里的触发点容易被覆盖 | `controller/relay.go` | 高 |
| P-06 | Clone() 方法（dto/clone.go）每次加新字段都需同步更新，否则深拷贝不完整 | `dto/clone.go` | 中 |
| P-07 | group-status sidebar 入口用的是 use-sidebar-data.ts，合并时该文件有冲突，容易丢 | `hooks/use-sidebar-data.ts` | 中 |
