package salary

import (
	"fmt"
	"math"
	"testing"
	"time"

	"cowtime/internal/workhours"
)

// fakeCalendar 用一张「日期 → 是否工作日」的表覆盖默认规则；表里没有的日期按周末判断。
type fakeCalendar struct {
	workdays map[string]bool
}

func (f fakeCalendar) IsWorkday(t time.Time) bool {
	if workday, ok := f.workdays[t.Format("2006-01-02")]; ok {
		return workday
	}
	weekday := t.Weekday()
	return weekday != time.Saturday && weekday != time.Sunday
}

// noWorkdays 表示每一天都不是工作日。
type noWorkdays struct{}

func (noWorkdays) IsWorkday(time.Time) bool { return false }

// october2026 模拟 2026 年 10 月的国庆安排：10 月 1 日到 7 日放假，10 月 10 日（周六）调休上班。
func october2026() fakeCalendar {
	workdays := make(map[string]bool)
	for day := 1; day <= 7; day++ {
		workdays[fmt.Sprintf("2026-10-%02d", day)] = false
	}
	workdays["2026-10-10"] = true
	return fakeCalendar{workdays: workdays}
}

// february2026 模拟 2026 年 2 月的春节安排：
// 2 月 16 日到 20 日放假，2 月 14 日和 28 日（都是周六）调休上班。
func february2026() fakeCalendar {
	workdays := make(map[string]bool)
	for day := 16; day <= 20; day++ {
		workdays[fmt.Sprintf("2026-02-%02d", day)] = false
	}
	workdays["2026-02-14"] = true
	workdays["2026-02-28"] = true
	return fakeCalendar{workdays: workdays}
}

// mustTime 把 "YYYY-MM-DD HH:MM:SS" 解析成本地时间，失败时终止测试。
func mustTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.ParseInLocation("2006-01-02 15:04:05", value, time.Local)
	if err != nil {
		t.Fatalf("解析时间 %q 失败：%v", value, err)
	}
	return parsed
}

// defaultSchedule 返回默认作息：9 点上班，12 点到 13 点午休，18 点下班，每天计薪 8 小时。
func defaultSchedule(t *testing.T) workhours.Schedule {
	t.Helper()
	sched, err := workhours.Default().Parse()
	if err != nil {
		t.Fatalf("解析默认作息：%v", err)
	}
	return sched
}

// longSchedule 返回 9 点上班、20:30 下班、午休 12 点到 13 点的作息，每天计薪 10.5 小时。
func longSchedule(t *testing.T) workhours.Schedule {
	t.Helper()
	sched, err := (workhours.Hours{Start: "09:00", LunchStart: "12:00", LunchEnd: "13:00", End: "20:30"}).Parse()
	if err != nil {
		t.Fatalf("解析下班晚的作息：%v", err)
	}
	return sched
}

// fixed 返回每天都使用同一套作息的函数。
func fixed(sched workhours.Schedule) ScheduleFor {
	return func(time.Time) workhours.Schedule { return sched }
}

// byWeekday 返回按星期几取作息的函数：weekdays 里的星期使用 other，其余日子使用 base。
func byWeekday(base, other workhours.Schedule, weekdays ...time.Weekday) ScheduleFor {
	return func(day time.Time) workhours.Schedule {
		for _, weekday := range weekdays {
			if day.Weekday() == weekday {
				return other
			}
		}
		return base
	}
}

func TestWorkdaysInMonth(t *testing.T) {
	tests := []struct {
		name string
		date string
		cal  Calendar
		want int
	}{
		{name: "没有节假日的月份", date: "2026-11-15", cal: fakeCalendar{}, want: 21},
		{name: "含国庆和调休的月份", date: "2026-10-15", cal: october2026(), want: 18},
		{name: "含春节和调休的月份", date: "2026-02-15", cal: february2026(), want: 17},
		{name: "一个工作日都没有", date: "2026-11-15", cal: noWorkdays{}, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := WorkdaysInMonth(tt.cal, mustTime(t, tt.date+" 12:00:00"))
			if got != tt.want {
				t.Errorf("WorkdaysInMonth() = %d，期望 %d", got, tt.want)
			}
		})
	}
}

func TestRate(t *testing.T) {
	sched := defaultSchedule(t)
	now := mustTime(t, "2026-11-24 12:30:00")

	// 2026 年 11 月有 21 个工作日，每天计薪 8 小时。
	got := Rate(fakeCalendar{}, 10000, fixed(sched), now)
	want := 10000.0 / (21 * 28800)
	if math.Abs(got-want) > 1e-15 {
		t.Errorf("Rate() = %v，期望 %v", got, want)
	}

	if got := Rate(noWorkdays{}, 10000, fixed(sched), now); got != 0 {
		t.Errorf("当月没有工作日时 Rate() = %v，期望 0", got)
	}
}

func TestPaidSecondsInMonth(t *testing.T) {
	// 周一、周二、周四 20:30 下班，其余工作日 18:00 下班。
	sched := byWeekday(defaultSchedule(t), longSchedule(t), time.Monday, time.Tuesday, time.Thursday)

	// 2026 年 11 月有 21 个工作日，其中 13 个下班晚。
	got := PaidSecondsInMonth(fakeCalendar{}, sched, mustTime(t, "2026-11-15 12:00:00"))
	if want := float64(13*10.5*3600 + 8*8*3600); got != want {
		t.Errorf("PaidSecondsInMonth() = %v，期望 %v", got, want)
	}

	// 每天作息相同时，等于工作日数乘以每天计薪秒数。
	got = PaidSecondsInMonth(fakeCalendar{}, fixed(defaultSchedule(t)), mustTime(t, "2026-11-15 12:00:00"))
	if want := float64(21 * 28800); got != want {
		t.Errorf("每天作息相同时 PaidSecondsInMonth() = %v，期望 %v", got, want)
	}

	if got := PaidSecondsInMonth(noWorkdays{}, sched, mustTime(t, "2026-11-15 12:00:00")); got != 0 {
		t.Errorf("当月没有工作日时 PaidSecondsInMonth() = %v，期望 0", got)
	}
}

func TestEarned(t *testing.T) {
	sched := defaultSchedule(t)
	day := 28800.0

	tests := []struct {
		name      string
		cal       Calendar
		now       string
		wantMonth float64
		wantToday float64
	}{
		{
			name:      "工作日的午休时间：本月 16 个完整工作日，今天已计薪 3 小时",
			cal:       fakeCalendar{},
			now:       "2026-11-24 12:30:00",
			wantMonth: 10000 * (16*day + 10800) / (21 * day),
			wantToday: 10000 * 10800 / (21 * day),
		},
		{
			name:      "月末最后一个工作日下班时，正好等于月薪",
			cal:       fakeCalendar{},
			now:       "2026-11-30 18:00:00",
			wantMonth: 10000,
			wantToday: 10000.0 / 21,
		},
		{
			name:      "周六：本月已赚是前 20 个工作日的总和，今天为 0",
			cal:       fakeCalendar{},
			now:       "2026-11-28 10:00:00",
			wantMonth: 10000 * 20 * day / (21 * day),
			wantToday: 0,
		},
		{
			name:      "月初第一个工作日，上班之前",
			cal:       fakeCalendar{},
			now:       "2026-11-02 08:30:00",
			wantMonth: 0,
			wantToday: 0,
		},
		{
			name:      "含国庆和调休的月份，月底正好等于月薪",
			cal:       october2026(),
			now:       "2026-10-31 12:00:00",
			wantMonth: 10000,
			wantToday: 0,
		},
		{
			name:      "当月没有工作日",
			cal:       noWorkdays{},
			now:       "2026-11-24 12:30:00",
			wantMonth: 0,
			wantToday: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Earned(tt.cal, 10000, fixed(sched), mustTime(t, tt.now))
			if math.Abs(got.Month-tt.wantMonth) > 1e-6 {
				t.Errorf("本月已赚 = %v，期望 %v", got.Month, tt.wantMonth)
			}
			if math.Abs(got.Today-tt.wantToday) > 1e-6 {
				t.Errorf("今日已赚 = %v，期望 %v", got.Today, tt.wantToday)
			}
		})
	}
}

func TestEarnedWithWeekdaySchedules(t *testing.T) {
	sched := byWeekday(defaultSchedule(t), longSchedule(t), time.Monday, time.Tuesday, time.Thursday)
	rate := 10000.0 / float64(13*10.5*3600+8*8*3600)

	// 周二 12:30。到这时为止，10 个下班晚的工作日和 6 个平常的工作日已经完成，今天已计薪 3 小时。
	got := Earned(fakeCalendar{}, 10000, sched, mustTime(t, "2026-11-24 12:30:00"))
	if want := rate * float64(10*10.5*3600+6*8*3600+3*3600); math.Abs(got.Month-want) > 1e-6 {
		t.Errorf("本月已赚 = %v，期望 %v", got.Month, want)
	}
	if want := rate * 3 * 3600; math.Abs(got.Today-want) > 1e-6 {
		t.Errorf("今日已赚 = %v，期望 %v", got.Today, want)
	}

	// 下班晚的日子比平常的日子赚得多。
	longDay := Earned(fakeCalendar{}, 10000, sched, mustTime(t, "2026-11-23 20:30:00")).Today   // 周一
	normalDay := Earned(fakeCalendar{}, 10000, sched, mustTime(t, "2026-11-20 18:00:00")).Today // 周五
	if want := rate * 10.5 * 3600; math.Abs(longDay-want) > 1e-6 {
		t.Errorf("下班晚的日子今日已赚 = %v，期望 %v", longDay, want)
	}
	if want := rate * 8 * 3600; math.Abs(normalDay-want) > 1e-6 {
		t.Errorf("平常的日子今日已赚 = %v，期望 %v", normalDay, want)
	}

	// 月底最后一个工作日下班时，本月已赚正好等于月薪。
	got = Earned(fakeCalendar{}, 10000, sched, mustTime(t, "2026-11-30 20:30:00"))
	if math.Abs(got.Month-10000) > 1e-6 {
		t.Errorf("月底时本月已赚 = %v，期望 10000", got.Month)
	}
}

func TestEarnedWithoutLunchBreak(t *testing.T) {
	sched, err := (workhours.Hours{Start: "09:00", End: "17:00"}).Parse()
	if err != nil {
		t.Fatalf("解析作息：%v", err)
	}

	got := Earned(fakeCalendar{}, 10000, fixed(sched), mustTime(t, "2026-11-24 12:30:00"))
	// 2026 年 11 月有 21 个工作日，每天计薪 8 小时，今天已计薪 3 小时 30 分钟。
	wantToday := 10000.0 * 12600 / (21 * 28800)
	if math.Abs(got.Today-wantToday) > 1e-6 {
		t.Errorf("今日已赚 = %v，期望 %v", got.Today, wantToday)
	}
}

func TestStateOf(t *testing.T) {
	sched := defaultSchedule(t)
	noLunch, err := (workhours.Hours{Start: "09:00", End: "17:00"}).Parse()
	if err != nil {
		t.Fatalf("解析作息：%v", err)
	}

	tests := []struct {
		name  string
		cal   Calendar
		sched workhours.Schedule
		now   string
		want  State
	}{
		{name: "上班前一秒", cal: fakeCalendar{}, sched: sched, now: "2026-11-24 08:59:59", want: StateBeforeWork},
		{name: "刚到上班时间", cal: fakeCalendar{}, sched: sched, now: "2026-11-24 09:00:00", want: StateWorking},
		{name: "午休", cal: fakeCalendar{}, sched: sched, now: "2026-11-24 12:30:00", want: StateLunch},
		{name: "午休结束", cal: fakeCalendar{}, sched: sched, now: "2026-11-24 13:00:00", want: StateWorking},
		{name: "下班时", cal: fakeCalendar{}, sched: sched, now: "2026-11-24 18:00:00", want: StateAfterWork},
		{name: "周六", cal: fakeCalendar{}, sched: sched, now: "2026-11-28 10:00:00", want: StateDayOff},
		{name: "法定节假日落在周一", cal: october2026(), sched: sched, now: "2026-10-05 10:00:00", want: StateDayOff},
		{name: "没有午休的作息，中午照常工作", cal: fakeCalendar{}, sched: noLunch, now: "2026-11-24 12:30:00", want: StateWorking},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now := mustTime(t, tt.now)
			if got := StateOf(tt.cal, tt.sched, now); got != tt.want {
				t.Errorf("StateOf() = %q，期望 %q", got, tt.want)
			}
		})
	}
}

func TestStateOfUsesTodaySchedule(t *testing.T) {
	sched := byWeekday(defaultSchedule(t), longSchedule(t), time.Monday, time.Tuesday, time.Thursday)

	// 晚上 7 点：周一还在工作，周三已经下班。
	monday := mustTime(t, "2026-11-23 19:00:00")
	if got := StateOf(fakeCalendar{}, sched(monday), monday); got != StateWorking {
		t.Errorf("周一 19 点的状态 = %q，期望 %q", got, StateWorking)
	}
	wednesday := mustTime(t, "2026-11-25 19:00:00")
	if got := StateOf(fakeCalendar{}, sched(wednesday), wednesday); got != StateAfterWork {
		t.Errorf("周三 19 点的状态 = %q，期望 %q", got, StateAfterWork)
	}
}
