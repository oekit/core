package timex

import (
	"fmt"
	"strconv"
	"time"
)

const (
	stdLayout = "2006-01-02 15:04:05"
)

// TimestampString 返回当前 Unix 时间戳的字符串。
// 时间戳始终为 UTC 秒数，不含时区偏移（Unix 时间戳是时区无关的）。
func TimestampString() string {
	return fmt.Sprintf("%d", time.Now().Unix())
}

// GetCurrentTime 返回当前 Unix 时间戳。
func GetCurrentTime() int {
	return int(time.Now().Unix())
}

// GetCurrentDate 返回当前日期字符串，默认格式为 "2006-01-02"。
func GetCurrentDate(format string) string {
	timestamp := time.Now().Unix()
	tm := time.Unix(timestamp, 0)
	if format == "" {
		format = "2006-01-02"
	}
	return tm.Format(format)
}

// GetCurrentDayStartTime 返回今天 00:00:00 的 Unix 时间戳。
func GetCurrentDayStartTime() int64 {
	currDate := GetCurrentDate("")
	currDate += " 00:00:00"
	return Date2Stamp(currDate, stdLayout)
}

// StampString2Date 将时间戳字符串转换为日期格式。
// 解析失败时返回空字符串。
func StampString2Date(timestampStr, format string) string {
	timestamp, err := strconv.ParseInt(timestampStr, 10, 64)
	if err != nil {
		return ""
	}
	tm := time.Unix(timestamp, 0)

	if format == "" {
		format = stdLayout
	}
	return tm.Format(format)
}

// StampInt2Date 将 Unix 时间戳转换为日期格式。
func StampInt2Date(timestamp int64, format string) string {
	tm := time.Unix(timestamp, 0)
	if format == "" {
		format = stdLayout
	}
	return tm.Format(format)
}

// Date2Stamp 将日期字符串转换为 Unix 时间戳。
// 使用本地时区解析（time.ParseInLocation），避免 UTC 偏移问题。
// 解析失败时返回 0。
func Date2Stamp(date string, format string) int64 {
	if format == "" {
		format = stdLayout
	}
	tm, err := time.ParseInLocation(format, date, time.Local)
	if err != nil {
		return 0
	}
	return tm.Unix()
}

// NDateBefore 获取前 n 天的日期列表。
// isMulti 为 true 时返回前 n 天每一天的日期，否则只返回第 n 天的日期。
func NDateBefore(n int, isMulti bool, format string) []string {
	var rs []string
	nTime := time.Now()

	if format == "" {
		format = "20060102"
	}

	if isMulti {
		j := -1
		for i := 0; i < n; i++ {
			yesTime := nTime.AddDate(0, 0, j)
			j--
			logDay := yesTime.Format(format)
			rs = append(rs, logDay)
		}
	} else {
		yesTime := nTime.AddDate(0, 0, -1*n)
		logDay := yesTime.Format(format)
		rs = append(rs, logDay)
	}
	return rs
}

// ConvertTimezone 将日期时间从指定时区转换为本地时区。
func ConvertTimezone(datetime string, fromLoc string) (string, error) {
	loc, err := time.LoadLocation(fromLoc)
	if err != nil {
		return "", err
	}
	t, err := time.ParseInLocation(stdLayout, datetime, loc)
	if err != nil {
		return "", err
	}
	return time.Unix(t.Unix(), 0).Format(stdLayout), nil
}
