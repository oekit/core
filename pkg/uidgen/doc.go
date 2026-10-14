// Package uidgen 提供分布式唯一 ID 生成器.
//
// 推荐使用 Snowflake（全局单例 + 极简 API）：
//
//	uidgen.InitSnowflake(7)                  // 一次性初始化, machineID ∈ [0, 1023]
//	id := uidgen.SnowflakeID()               // int64, 不返错, auto-init 兜底
//	uidgen.ExtractSnowflakeTime(id)          // epoch = 2026-01-01
//	uidgen.ExtractSnowflakeMachineID(id)
//	uidgen.ExtractSnowflakeSequence(id)
//
// Extract 函数（epoch = 2024-01-01）用于从旧式 ID 中提取元数据：
//
//	uidgen.ExtractTime(id)
//	uidgen.ExtractWorkerID(id)
//	uidgen.ExtractSequence(id)
//
// UUID 系列（与 Snowflake 无关）见 uuid.go.
package uidgen
