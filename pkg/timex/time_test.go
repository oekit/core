package timex

import (
	"testing"
	"time"
)

func TestGetCurrentTime(t *testing.T) {
	now := GetCurrentTime()
	if now <= 0 {
		t.Error("GetCurrentTime() should return positive value")
	}
}

func TestGetCurrentDate(t *testing.T) {
	date := GetCurrentDate("")
	if len(date) != 10 {
		t.Errorf("GetCurrentDate() length = %v, want 10", len(date))
	}
}

func TestGetCurrentDayStartTime(t *testing.T) {
	start := GetCurrentDayStartTime()
	if start <= 0 {
		t.Error("GetCurrentDayStartTime() should return positive value")
	}
}

func TestStampString2Date(t *testing.T) {
	date := StampString2Date("1609459200", "")
	if date == "" {
		t.Error("StampString2Date() should not return empty")
	}
}

func TestStampInt2Date(t *testing.T) {
	date := StampInt2Date(1609459200, "")
	if date == "" {
		t.Error("StampInt2Date() should not return empty")
	}
}

func TestDate2Stamp(t *testing.T) {
	stamp := Date2Stamp("2021-01-01 00:00:00", "")
	// 注意：这里的期望值取决于本地时区
	// UTC 时间 2021-01-01 00:00:00 是 1609459200
	// 本地时间（UTC+8）2021-01-01 00:00:00 是 1609430400
	t.Logf("Date2Stamp() = %v", stamp)
	if stamp == 0 {
		t.Error("Date2Stamp() should not return 0")
	}
}

func TestNDateBefore(t *testing.T) {
	dates := NDateBefore(3, true, "")
	if len(dates) != 3 {
		t.Errorf("NDateBefore() length = %v, want 3", len(dates))
	}

	dates = NDateBefore(3, false, "")
	if len(dates) != 1 {
		t.Errorf("NDateBefore() length = %v, want 1", len(dates))
	}
}

func TestConvertTimezone(t *testing.T) {
	result, err := ConvertTimezone("2021-01-01 00:00:00", "UTC")
	if err != nil {
		t.Fatal(err)
	}
	// UTC 00:00:00 应该转换为本地时间 08:00:00 (假设本地为 UTC+8)
	expected := time.Unix(1609459200, 0).Format("2006-01-02 15:04:05")
	if result != expected {
		t.Errorf("ConvertTimezone() = %v, want %v", result, expected)
	}
}

func TestTimestampString(t *testing.T) {
	ts := TimestampString()
	if ts == "" {
		t.Error("TimestampString() should not return empty")
	}
}
