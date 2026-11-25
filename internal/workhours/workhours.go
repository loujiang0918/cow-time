// Package workhours 负责解析和校验每天的作息时间。
package workhours

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Hours 是用户填写的作息时间，每一项的格式都是 "HH:MM"。
// LunchStart 和 LunchEnd 同时为空表示没有午休。
type Hours struct {
	Start      string `json:"start"`
	LunchStart string `json:"lunchStart"`
	LunchEnd   string `json:"lunchEnd"`
	End        string `json:"end"`
}

// Default 返回默认作息：9 点上班，12 点到 13 点午休，18 点下班。
func Default() Hours {
	return Hours{Start: "09:00", LunchStart: "12:00", LunchEnd: "13:00", End: "18:00"}
}

// Schedule 是解析之后的作息时间，每一项都是当天经过的分钟数。
// 没有午休时 LunchStart 和 LunchEnd 都等于 Start，此时午休时段的长度为 0。
type Schedule struct {
	Start      int
	LunchStart int
	LunchEnd   int
	End        int
}

// ErrInvalid 表示作息时间不合法。
var ErrInvalid = errors.New("作息时间无效")

// Parse 校验并解析作息时间。
// 合法的作息需要满足：上班时间 < 午休开始 < 午休结束 < 下班时间。
func (h Hours) Parse() (Schedule, error) {
	var s Schedule
	var err error

	if s.Start, err = parseClock(h.Start); err != nil {
		return Schedule{}, fmt.Errorf("%w：上班时间 %q", ErrInvalid, h.Start)
	}
	if s.End, err = parseClock(h.End); err != nil {
		return Schedule{}, fmt.Errorf("%w：下班时间 %q", ErrInvalid, h.End)
	}

	hasLunchStart := h.LunchStart != ""
	hasLunchEnd := h.LunchEnd != ""
	switch {
	case !hasLunchStart && !hasLunchEnd:
		s.LunchStart, s.LunchEnd = s.Start, s.Start
	case hasLunchStart && hasLunchEnd:
		if s.LunchStart, err = parseClock(h.LunchStart); err != nil {
			return Schedule{}, fmt.Errorf("%w：午休开始 %q", ErrInvalid, h.LunchStart)
		}
		if s.LunchEnd, err = parseClock(h.LunchEnd); err != nil {
			return Schedule{}, fmt.Errorf("%w：午休结束 %q", ErrInvalid, h.LunchEnd)
		}
	default:
		return Schedule{}, fmt.Errorf("%w：午休开始和午休结束需要同时填写或同时留空", ErrInvalid)
	}

	valid := s.Start < s.End
	if hasLunchStart {
		valid = valid && s.Start < s.LunchStart && s.LunchStart < s.LunchEnd && s.LunchEnd < s.End
	}
	if !valid {
		return Schedule{}, fmt.Errorf("%w：需要满足 上班时间 < 午休开始 < 午休结束 < 下班时间", ErrInvalid)
	}
	return s, nil
}

// DailySeconds 返回每一天的计薪秒数。
func (s Schedule) DailySeconds() int {
	return (s.LunchStart - s.Start + s.End - s.LunchEnd) * 60
}

// PaidSecondsAt 返回当天到 secondOfDay 为止已经计薪的秒数。
// secondOfDay 是当天经过的秒数。
func (s Schedule) PaidSecondsAt(secondOfDay int) int {
	return paidOverlap(s.Start, s.LunchStart, secondOfDay) +
		paidOverlap(s.LunchEnd, s.End, secondOfDay)
}

// SecondOfDay 返回 t 是当天经过的第几秒。
func SecondOfDay(t time.Time) int {
	return t.Hour()*3600 + t.Minute()*60 + t.Second()
}

// parseClock 把 "HH:MM" 解析成当天经过的分钟数。
func parseClock(clock string) (int, error) {
	hourText, minuteText, ok := strings.Cut(clock, ":")
	if !ok {
		return 0, fmt.Errorf("时间 %q 不是 HH:MM 格式", clock)
	}
	hour, err := strconv.Atoi(hourText)
	if err != nil {
		return 0, fmt.Errorf("时间 %q 的小时不是数字", clock)
	}
	minute, err := strconv.Atoi(minuteText)
	if err != nil {
		return 0, fmt.Errorf("时间 %q 的分钟不是数字", clock)
	}
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, fmt.Errorf("时间 %q 超出范围", clock)
	}
	return hour*60 + minute, nil
}

// paidOverlap 返回区间 [fromMinute, toMinute)（以分钟计）落在当天前 secondOfDay 秒之内的秒数。
func paidOverlap(fromMinute, toMinute, secondOfDay int) int {
	from := fromMinute * 60
	to := toMinute * 60
	switch {
	case secondOfDay <= from:
		return 0
	case to <= secondOfDay:
		return to - from
	default:
		return secondOfDay - from
	}
}
