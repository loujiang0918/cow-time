// Package salary 计算到某一时刻为止已经赚到的薪资。
package salary

import (
	"time"

	"cowtime/internal/workhours"
)

// Calendar 回答某一天是否是工作日。
type Calendar interface {
	IsWorkday(time.Time) bool
}

// ScheduleFor 返回某一天生效的作息。
type ScheduleFor func(day time.Time) workhours.Schedule

// State 是当前时刻所处的计薪状态。
type State string

const (
	StateWorking    State = "working"    // 工作时间内，金额在增长
	StateBeforeWork State = "beforeWork" // 工作日，还没到上班时间
	StateLunch      State = "lunch"      // 午休
	StateAfterWork  State = "afterWork"  // 工作日，已经下班
	StateDayOff     State = "dayOff"     // 非工作日
)

// Amounts 是某一时刻的已赚金额，单位是元。
type Amounts struct {
	Month float64 // 本月已赚
	Today float64 // 今日已赚
}

// WorkdaysInMonth 返回 t 所在月份的工作日数。
func WorkdaysInMonth(cal Calendar, t time.Time) int {
	count := 0
	for d := monthStart(t); d.Month() == t.Month(); d = d.AddDate(0, 0, 1) {
		if cal.IsWorkday(d) {
			count++
		}
	}
	return count
}

// PaidSecondsInMonth 返回 t 所在月份的计薪总秒数：
// 当月每一个工作日按照各自的作息算出的每天计薪秒数之和。
func PaidSecondsInMonth(cal Calendar, sched ScheduleFor, t time.Time) float64 {
	total := 0.0
	for d := monthStart(t); d.Month() == t.Month(); d = d.AddDate(0, 0, 1) {
		if cal.IsWorkday(d) {
			total += float64(sched(d).DailySeconds())
		}
	}
	return total
}

// Rate 返回 t 所在月份的秒薪：月薪除以当月计薪总秒数。
// 当月计薪总秒数为 0 时返回 0。
func Rate(cal Calendar, monthly float64, sched ScheduleFor, t time.Time) float64 {
	total := PaidSecondsInMonth(cal, sched, t)
	if total == 0 {
		return 0
	}
	return monthly / total
}

// Earned 计算到 now 为止的本月已赚和今日已赚。
// 每次都用 now 重新计算，不做累加，所以睡眠、关机、修改系统时间都不影响结果。
func Earned(cal Calendar, monthly float64, sched ScheduleFor, now time.Time) Amounts {
	rate := Rate(cal, monthly, sched, now)

	todaySeconds := 0
	if cal.IsWorkday(now) {
		todaySeconds = sched(now).PaidSecondsAt(workhours.SecondOfDay(now))
	}

	monthSeconds := 0.0
	for d := monthStart(now); !d.After(now); d = d.AddDate(0, 0, 1) {
		if !cal.IsWorkday(d) {
			continue
		}
		if sameDay(d, now) {
			monthSeconds += float64(todaySeconds)
		} else {
			monthSeconds += float64(sched(d).DailySeconds())
		}
	}
	return Amounts{Month: rate * monthSeconds, Today: rate * float64(todaySeconds)}
}

// StateOf 返回 now 这一时刻的计薪状态，按照 sched 判断。sched 是今天的作息。
func StateOf(cal Calendar, sched workhours.Schedule, now time.Time) State {
	if !cal.IsWorkday(now) {
		return StateDayOff
	}
	second := workhours.SecondOfDay(now)
	switch {
	case second < sched.Start*60:
		return StateBeforeWork
	case second < sched.LunchStart*60:
		return StateWorking
	case second < sched.LunchEnd*60:
		return StateLunch
	case second < sched.End*60:
		return StateWorking
	default:
		return StateAfterWork
	}
}

// monthStart 返回 t 所在月份第一天的零点。
func monthStart(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
}

// sameDay 报告 a 和 b 是否是同一天。
func sameDay(a, b time.Time) bool {
	return a.Year() == b.Year() && a.YearDay() == b.YearDay()
}
