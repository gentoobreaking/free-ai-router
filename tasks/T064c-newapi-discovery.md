---
github_issue:
title: Implement new-api /v1/models discovery and HA failover from relay sites
type: feature
priority: high
status: pending
assignee: OpenCode with DeepSeek V4 Flash
created: 2026-08-04
updated: 2026-08-04
---

# T064c - Implement new-api /v1/models discovery and HA failover

## 目標
將發現的 new-api 中轉站與現有 providers 系統整合，實現模型發現和自動故障轉移(HA)。

## 驗收標準
- [ ] 實作 ValidateNewApiRelay(baseURL) 檢查中轉站是否可用
- [ ] 發現的中轉站添加到 providers 系統作為動態 provider
- [ ] 實現多中轉站輪詢 (round-robin) 故障轉移
- [ ] Ping engine 能夠 ping 中轉站上的模型
- [ ] 模型發現與現有 discoverModels 流程整合
- [ ] 單元測試驗證中轉站驗證和 HA 邏輯

## 備註
- new-api 實現 OpenAI-compatible /v1/models 端點
- 公益中轉站通常速率限制低，需要HA機制
- 發現的站點需要定期重新檢查 (6小時)
- 相關: T064 (parent task), T064a, T064b