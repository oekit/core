package cron

import (
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		spec    string
		wantErr bool
	}{
		{"5 fields", "* * * * *", false},
		{"6 fields", "0 * * * * *", false},
		{"with seconds", "30 * * * * *", false},
		{"daily", "@daily", false},
		{"hourly", "@hourly", false},
		{"every 5m", "@every 5m", false},
		{"invalid", "invalid", true},
		{"too few fields", "* * *", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.spec)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestSpecScheduleNext(t *testing.T) {
	schedule, err := Parse("0 * * * * *")
	if err != nil {
		t.Fatal(err)
	}

	from := time.Date(2021, 1, 1, 0, 0, 0, 0, time.Local)
	next := schedule.Next(from)
	t.Logf("Next() = %v", next)
	if next.IsZero() {
		t.Error("Next() should not return zero time")
	}
}

func TestEvery(t *testing.T) {
	schedule := Every(5 * time.Minute)
	from := time.Date(2021, 1, 1, 0, 0, 0, 0, time.Local)
	next := schedule.Next(from)
	diff := next.Sub(from)
	if diff != 5*time.Minute {
		t.Errorf("Next() diff = %v, want 5m", diff)
	}
}

func TestConstantDelayScheduleNext(t *testing.T) {
	schedule := ConstantDelaySchedule{Delay: 10 * time.Second}
	from := time.Date(2021, 1, 1, 0, 0, 0, 0, time.Local)
	next := schedule.Next(from)
	diff := next.Sub(from)
	if diff != 10*time.Second {
		t.Errorf("Next() diff = %v, want 10s", diff)
	}
}

func TestParseSpecialExpressions(t *testing.T) {
	tests := []struct {
		name string
		spec string
	}{
		{"yearly", "@yearly"},
		{"annually", "@annually"},
		{"monthly", "@monthly"},
		{"weekly", "@weekly"},
		{"midnight", "@midnight"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schedule, err := Parse(tt.spec)
			if err != nil {
				t.Errorf("Parse(%s) error = %v", tt.spec, err)
				return
			}
			if schedule == nil {
				t.Errorf("Parse(%s) returned nil schedule", tt.spec)
			}
		})
	}
}

func TestNextSchedule(t *testing.T) {
	schedule, err := Parse("0 * * * * *")
	if err != nil {
		t.Fatal(err)
	}

	from := time.Date(2021, 1, 1, 0, 0, 0, 0, time.Local)
	times := NextSchedule(schedule, from, 3)
	t.Logf("NextSchedule() = %v", times)
	// 至少应该返回一个时间
	if len(times) == 0 {
		t.Error("NextSchedule() should return at least one time")
	}
}

func TestSortEntries(t *testing.T) {
	now := time.Now()
	entries := []Entry{
		{Next: now.Add(2 * time.Hour)},
		{Next: now.Add(1 * time.Hour)},
		{Next: now.Add(3 * time.Hour)},
	}

	SortEntries(entries)

	for i := 1; i < len(entries); i++ {
		if entries[i].Next.Before(entries[i-1].Next) {
			t.Error("SortEntries() not sorted correctly")
		}
	}
}

func TestNewDelaySchedule(t *testing.T) {
	schedule := NewDelaySchedule(5 * time.Second)
	from := time.Now()
	next := schedule.Next(from)
	if next.Sub(from) != 5*time.Second {
		t.Errorf("Next() diff = %v, want 5s", next.Sub(from))
	}
}

func TestRoundDuration(t *testing.T) {
	tests := []struct {
		name      string
		d         time.Duration
		precision time.Duration
		want      time.Duration
	}{
		{"round up", 7 * time.Second, 5 * time.Second, 10 * time.Second},
		{"exact", 10 * time.Second, 5 * time.Second, 10 * time.Second},
		{"zero precision", 7 * time.Second, 0, 7 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RoundDuration(tt.d, tt.precision); got != tt.want {
				t.Errorf("RoundDuration() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestParseField_NSlashStep 回归：fmt.Sscanf("%d") 不校验剩余输入，
// 单值 "5/2" 曾被静默解析为 5（丢掉 /2 步长语义）且不报错。
// 钉住：N/step 按 robfig/cron 语义（从 N 步进到字段最大值）展开。
func TestParseField_NSlashStep(t *testing.T) {
	// 分钟字段 "5/15" → 5,20,35,50
	bits, err := getField("5/15", 0, 59)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range []uint64{5, 20, 35, 50} {
		if bits&(1<<m) == 0 {
			t.Errorf("minute %d should be set for 5/15", m)
		}
	}
	if bits&(1<<6) != 0 {
		t.Error("minute 6 should not be set for 5/15")
	}

	// 非法后缀必须报错，不能静默截断
	if _, err := getField("10abc", 0, 59); err == nil {
		t.Error("10abc should be rejected, not parsed as 10")
	}
	if _, err := getField("+5", 0, 59); err == nil {
		t.Error("+5 should be rejected (strict digits)")
	}
	if _, err := getField("5/0", 0, 59); err == nil {
		t.Error("step 0 should be rejected")
	}
}
