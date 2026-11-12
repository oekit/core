// Package logkey 提供本仓单源的结构化日志键常量。
//
// 为什么不直接用 logx.ErrKey：core 是 logx 的下游底座（logx/go.mod 依赖
// github.com/oekit/core），core 引 logx 即成环。按矩阵对底座模块的豁免
// 口径（AGENTS.md 日志小节），本仓以单源常量对齐 logx.ErrKey 的值：
// 各包写错误字段一律引 logkey.Err，禁止手写 "err"/"error" 字面量——
// 手写字面量正是字段名分裂的来源（core#149：network 曾落 "error"，
// 与矩阵统一的 "err" 检索口径分裂，按 err 检索回退告警查不到）。
package logkey

// Err 是结构化日志中错误字段的键名，与 logx.ErrKey 同值（"err"）。
// 两矩阵的错误日志落同一 Loki/ES，字段名必须全矩阵一致。
const Err = "err"
