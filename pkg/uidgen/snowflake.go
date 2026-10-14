package uidgen

import (
	"errors"
	"log/slog"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// snowflakeEpoch 是 Snowflake ID 使用的 epoch 起点（毫秒）.
// 2025-01-01 00:00:00 UTC（1735689600000）。改常量会破坏存量 ID 的
// ExtractSnowflakeTime 解析，只能改注释对齐事实；距溢出余量约 68 年.
// 区别于旧式 ID 的 epoch（2024-01-01）：Snowflake 提取时间用 ExtractSnowflakeTime.
const snowflakeEpoch int64 = 1735689600000

// Snowflake 是 singleton 模式的 Snowflake ID 生成器.
//
// Bit 布局（bit 63 → 0）：
//
//	[1 sign | 41 ts | 10 machine | 12 sequence]
//
// = ~69 years × 1024 machines × 4096 IDs/ms per machine.
//
// 区别于旧式 ID 生成：
//   - Singleton 模式，业务侧直接调 SnowflakeID() 拿下一个 int64 ID.
//   - 时钟回拨持锁忙等，不返错.
//   - API 极简：InitSnowflake + SnowflakeID，不暴露 mu / sequence / lastTS.
//
// 并发模型：
//   - InitSnowflake 用 atomic.Pointer CAS 保证 "只 init 一次" + "成功后立刻可见".
//   - SnowflakeID() auto-init 路径在 CAS 失败时回退到 Load()，避免 nil deref.
//   - next() 持单 mu 串行：临界区短（同 ms 仅几次算术，跨 ms 持锁等下 ms 最坏 1ms）.
//
// Reference: https://github.com/twitter-archive/snowflake (Twitter, 2010)
type Snowflake struct {
	epoch     int64 // 自定义 epoch，毫秒
	machineID int64 // 0..maxWorker，注入不可变
	mu        sync.Mutex
	lastTS    int64
	sequence  int64
}

// defaultSnowflake 用 atomic.Pointer 而不是 mutex+struct 保护：
//
//   - CAS 一次完成 "init 标记 + 指针可见" 两件事，对其他 goroutine 立刻可见.
//   - Load() 无锁读，hot path 零开销.
//   - resetForTest 直接 Store(nil)，单步原子，无需持业务锁.
var defaultSnowflake atomic.Pointer[Snowflake]

// InitSnowflake 初始化全局 Snowflake singleton. machineID 必须在 [0, 1023] 范围.
// 多次调用：第一次生效，后续返错（防止误用）.
//
// 并发安全：CAS(nil -> s) 失败表示已初始化，Load() 拿到的是上次成功的实例.
func InitSnowflake(machineID int64) error {
	if machineID < 0 || machineID > maxWorker {
		return errors.New("uidgen: machineID must be in [0, 1023]")
	}
	s := &Snowflake{
		epoch:     snowflakeEpoch,
		machineID: machineID,
	}
	if !defaultSnowflake.CompareAndSwap(nil, s) {
		return errors.New("uidgen: snowflake already initialized")
	}
	return nil
}

// InitSnowflakeFromEnv 从 MACHINE_ID env 读 machineID，解析失败兜底 0.
// 单进程 dev / 测试默认 0 够用；生产必须显式注入（e.g. docker-compose 配 0/1/2）.
func InitSnowflakeFromEnv() error {
	raw := os.Getenv("MACHINE_ID")
	if raw == "" {
		slog.Warn("uidgen: MACHINE_ID not set, defaulting to 0; set MACHINE_ID in production to avoid ID collisions")
		return InitSnowflake(0)
	}
	mid, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		slog.Warn("uidgen: invalid MACHINE_ID, defaulting to 0", "value", raw, "err", err)
		return InitSnowflake(0)
	}
	return InitSnowflake(mid)
}

// SnowflakeID 拿下一个 int64 ID. 未 InitSnowflake 时自动以 machineID=0 初始化.
//
// 并发说明：auto-init 路径在多 goroutine 同时首次调用时，CAS 失败的 goroutine
// 会回退到 Load() 读已 init 的实例，保证不会 nil deref.
//
// 时钟回拨：阻塞等直到时间追平，不返错. 业务层应监控回拨时长，超过阈值告警.
func SnowflakeID() int64 {
	s := defaultSnowflake.Load()
	if s == nil {
		// auto-init machineID=0. InitSnowflake(0) 不可能因 machineID 范围失败
		// （0 在 [0,1023] 内）；CAS 失败仅表示已被并发初始化。两种情况下
		// 重新 Load 都必非 nil。
		if err := InitSnowflake(0); err != nil {
			slog.Debug("uidgen: snowflake auto-init raced, using winner's instance", "err", err)
		}
		s = defaultSnowflake.Load()
	}
	return s.next()
}

// SnowflakeMachineID 返当前 init 用的 machine ID，未初始化返 -1，测试用.
func SnowflakeMachineID() int64 {
	s := defaultSnowflake.Load()
	if s == nil {
		return -1
	}
	return s.machineID
}

func (s *Snowflake) next() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	for {
		now := time.Now().UnixMilli()

		// 时钟回拨：持锁忙等
		if now < s.lastTS {
			time.Sleep(time.Millisecond)
			continue
		}

		if now == s.lastTS {
			// 同 ms 内：递增 sequence
			s.sequence = (s.sequence + 1) & sequenceMask
			if s.sequence == 0 {
				// sequence 用完（4096/ms），必须等下一 ms
				// 持锁等：不能释放，否则下一个 caller 会从 0 递增
				// 拿到 seq=1 但 seq=1 本 ms 已经发过，撞号.
				for time.Now().UnixMilli() == s.lastTS {
					time.Sleep(time.Microsecond * 100)
				}
				continue
			}
			return (now-s.epoch)<<timestampShift | (s.machineID << workerShift) | s.sequence
		}

		// now > last：新 ms
		s.lastTS = now
		s.sequence = 0
		return (now-s.epoch)<<timestampShift | (s.machineID << workerShift) | s.sequence
	}
}

// ExtractSnowflakeTime 从 Snowflake ID 中提取生成时间.
// 注意：与 ExtractTime 不可混用——epoch 不同（2026 vs 2024）.
func ExtractSnowflakeTime(id int64) time.Time {
	ts := (id >> timestampShift) + snowflakeEpoch
	return time.UnixMilli(ts)
}

// ExtractSnowflakeMachineID 从 Snowflake ID 中提取 machineID.
func ExtractSnowflakeMachineID(id int64) int64 {
	return (id >> workerShift) & maxWorker
}

// ExtractSnowflakeSequence 从 Snowflake ID 中提取 sequence.
func ExtractSnowflakeSequence(id int64) int64 {
	return id & sequenceMask
}

// resetForTest 仅供测试重置全局状态. 业务代码不要调.
// Store(nil) 是单步原子操作，无需持业务锁——next() 持 mu 保证临界区不撕裂.
func resetForTest() {
	defaultSnowflake.Store(nil)
}
