package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"cowtime/internal/workhours"
)

// loadWeekdayConfig 把 body 写成配置文件并读取，返回读出来的配置。
func loadWeekdayConfig(t *testing.T, body string) Config {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := Load(path)
	if err != nil {
		t.Fatalf("Load() 意外返回错误：%v", err)
	}
	return store.Get()
}

func TestWeekdayKey(t *testing.T) {
	// 键是 1 到 7：1 是周一，7 是周日。
	for day, want := range map[time.Weekday]string{
		time.Monday: "1", time.Tuesday: "2", time.Wednesday: "3", time.Thursday: "4",
		time.Friday: "5", time.Saturday: "6", time.Sunday: "7",
	} {
		if got := WeekdayKey(day); got != want {
			t.Errorf("WeekdayKey(%v) = %q，期望 %q", day, got, want)
		}
		parsed, ok := ParseWeekdayKey(want)
		if !ok || parsed != day {
			t.Errorf("ParseWeekdayKey(%q) = (%v, %v)，期望 (%v, true)", want, parsed, ok, day)
		}
	}
	for _, key := range []string{"", "0", "8", "monday", "1 ", "-1"} {
		if _, ok := ParseWeekdayKey(key); ok {
			t.Errorf("ParseWeekdayKey(%q) 不应该认识这个键", key)
		}
	}
	for key, want := range map[string]string{"1": "周一", "4": "周四", "7": "周日"} {
		if got := WeekdayName(key); got != want {
			t.Errorf("WeekdayName(%q) = %q，期望 %q", key, got, want)
		}
	}
}

func TestHoursOn(t *testing.T) {
	late, early := "20:30", "11:30"
	cfg := Default()
	cfg.WeekdayHours = map[string]workhours.Partial{
		"1": {End: &late},         // 周一下班更晚
		"7": {LunchStart: &early}, // 周日午休更早
	}

	monday, err := cfg.HoursOn(time.Monday)
	if err != nil {
		t.Fatalf("周一的作息意外地不合法：%v", err)
	}
	want := workhours.Hours{Start: "09:00", LunchStart: "12:00", LunchEnd: "13:00", End: "20:30"}
	if monday != want {
		t.Errorf("周一的作息 = %+v，期望 %+v", monday, want)
	}

	wednesday, err := cfg.HoursOn(time.Wednesday)
	if err != nil || wednesday != workhours.Default() {
		t.Errorf("没有覆盖的星期应该使用默认作息，得到 (%+v, %v)", wednesday, err)
	}

	sunday, err := cfg.HoursOn(time.Sunday)
	if want := (workhours.Hours{Start: "09:00", LunchStart: "11:30", LunchEnd: "13:00", End: "18:00"}); err != nil || sunday != want {
		t.Errorf("键 7 是周日：得到 (%+v, %v)，期望 (%+v, nil)", sunday, err, want)
	}

	// 套用之后不合法的作息会报错。
	broken := "13:00"
	cfg.WeekdayHours["5"] = workhours.Partial{Start: &broken}
	if _, err := cfg.HoursOn(time.Friday); err == nil {
		t.Error("上班时间晚于午休开始时 HoursOn() 应该返回错误")
	}
}

func TestScheduleOn(t *testing.T) {
	late := "20:30"
	cfg := Default()
	cfg.WeekdayHours = map[string]workhours.Partial{"1": {End: &late}}

	monday := time.Date(2026, 11, 23, 12, 0, 0, 0, time.Local) // 周一
	want, _ := (workhours.Hours{Start: "09:00", LunchStart: "12:00", LunchEnd: "13:00", End: "20:30"}).Parse()
	if got := cfg.ScheduleOn(monday); got != want {
		t.Errorf("周一的作息 = %+v，期望 %+v", got, want)
	}

	// 这一天的覆盖不合法时，使用默认作息。
	broken := "25:00"
	cfg.WeekdayHours["1"] = workhours.Partial{End: &broken}
	fallback, _ := workhours.Default().Parse()
	if got := cfg.ScheduleOn(monday); got != fallback {
		t.Errorf("覆盖不合法时应该使用默认作息，得到 %+v，期望 %+v", got, fallback)
	}
}

func TestLoadWeekdayHours(t *testing.T) {
	const hours = `"workHours": {"start": "09:00", "lunchStart": "12:00", "lunchEnd": "13:00", "end": "18:00"}`
	late, early := "20:30", "11:30"

	tests := []struct {
		name string
		body string
		want map[string]workhours.Partial
	}{
		{
			name: "没有这一项",
			body: `{"monthlySalary": 10000, ` + hours + `}`,
			want: nil,
		},
		{
			name: "合法的覆盖被保留",
			body: `{"monthlySalary": 10000, ` + hours + `, "weekdayHours": {"1": {"end": "20:30"}, "4": {"lunchStart": "11:30"}}}`,
			want: map[string]workhours.Partial{"1": {End: &late}, "4": {LunchStart: &early}},
		},
		{
			name: "不认识的星期被忽略",
			body: `{"monthlySalary": 10000, ` + hours + `, "weekdayHours": {"8": {"end": "20:30"}, "monday": {"end": "20:30"}, "2": {"end": "20:30"}}}`,
			want: map[string]workhours.Partial{"2": {End: &late}},
		},
		{
			name: "套用之后作息不合法的覆盖被忽略",
			body: `{"monthlySalary": 10000, ` + hours + `, "weekdayHours": {"3": {"start": "13:00"}, "5": {"end": "25:00"}, "6": {"end": "20:30"}}}`,
			want: map[string]workhours.Partial{"6": {End: &late}},
		},
		{
			name: "全都不合法时这一项为空",
			body: `{"monthlySalary": 10000, ` + hours + `, "weekdayHours": {"3": {"start": "13:00"}}}`,
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := loadWeekdayConfig(t, tt.body)
			if !reflect.DeepEqual(got.WeekdayHours, tt.want) {
				t.Errorf("weekdayHours = %+v，期望 %+v", got.WeekdayHours, tt.want)
			}
			// 无论 weekdayHours 有没有问题，其余配置都照常生效。
			if got.MonthlySalary != 10000 || got.WorkHours != workhours.Default() {
				t.Errorf("月薪和默认作息 = (%v, %+v)，期望 (10000, %+v)", got.MonthlySalary, got.WorkHours, workhours.Default())
			}
		})
	}
}

func TestUpdateWritesWeekdayHours(t *testing.T) {
	store, path := newStore(t)
	late := "20:30"
	want := map[string]workhours.Partial{"1": {End: &late}}

	err := store.Update(func(c *Config) {
		c.MonthlySalary = 10000
		c.WeekdayHours = map[string]workhours.Partial{"1": {End: &late}}
	})
	if err != nil {
		t.Fatalf("Update() 意外返回错误：%v", err)
	}

	reloaded, err := Load(path)
	if err != nil {
		t.Fatalf("重新加载：%v", err)
	}
	if got := reloaded.Get().WeekdayHours; !reflect.DeepEqual(got, want) {
		t.Errorf("重新加载之后 weekdayHours = %+v，期望 %+v", got, want)
	}
}
