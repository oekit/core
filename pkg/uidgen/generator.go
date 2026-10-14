package uidgen

import (
	"time"
)

const (
	epoch        = 1704067200000 // 2024-01-01 00:00:00 UTC
	workerBits   = 10
	sequenceBits = 12

	maxWorker    = (1 << workerBits) - 1   // 1023, 10 bits 全部置 1
	maxSequence  = (1 << sequenceBits) - 1 // 4095, 序列号最大合法值
	sequenceMask = (1 << sequenceBits) - 1 // 12 bits 掩码

	workerShift    = sequenceBits              // 12
	timestampShift = workerBits + sequenceBits // 22
)

// ExtractTime 从 ID 中提取生成时间。
func ExtractTime(id int64) time.Time {
	ts := (id >> timestampShift) + epoch
	return time.UnixMilli(ts)
}

// ExtractWorkerID 从 ID 中提取 workerID。
func ExtractWorkerID(id int64) int64 {
	return (id >> workerShift) & maxWorker
}

// ExtractSequence 从 ID 中提取序列号。
func ExtractSequence(id int64) int64 {
	return id & sequenceMask
}
