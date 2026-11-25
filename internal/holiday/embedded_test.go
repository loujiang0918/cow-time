package holiday

import (
	"encoding/json"
	"fmt"
	"testing"
)

// loadEmbeddedCalendar 返回加载了全部内置数据的日历。
func loadEmbeddedCalendar(t *testing.T) *Calendar {
	t.Helper()
	cal := NewCalendar()
	if err := LoadEmbedded(cal); err != nil {
		t.Fatalf("LoadEmbedded() 意外返回错误：%v", err)
	}
	return cal
}

func TestLoadEmbeddedCovers2026And2027(t *testing.T) {
	cal := loadEmbeddedCalendar(t)
	for _, year := range []int{2026, 2027} {
		if !cal.HasYear(year) {
			t.Errorf("内置数据应该包含 %d 年", year)
		}
	}
	if cal.HasYear(2025) {
		t.Error("内置数据只包含 2026 和 2027 两年")
	}
}

func TestEmbeddedDayCounts(t *testing.T) {
	tests := []struct {
		year int
		want int
	}{
		{year: 2026, want: 39},
		{year: 2027, want: 34},
	}

	for _, tt := range tests {
		body, err := dataFiles.ReadFile(fmt.Sprintf("data/%d.json", tt.year))
		if err != nil {
			t.Fatalf("读取内置数据：%v", err)
		}
		var data YearData
		if err := json.Unmarshal(body, &data); err != nil {
			t.Fatalf("解析 %d 年的内置数据：%v", tt.year, err)
		}
		if data.Year != tt.year {
			t.Errorf("data/%d.json 里的 year 是 %d", tt.year, data.Year)
		}
		if got := len(data.Days); got != tt.want {
			t.Errorf("%d 年的内置数据有 %d 条记录，期望 %d 条", tt.year, got, tt.want)
		}
	}
}

func TestEmbeddedWorkdays(t *testing.T) {
	cal := loadEmbeddedCalendar(t)

	tests := []struct {
		name     string
		date     string
		wantWork bool
		wantKind string // 非工作日的类型；工作日留空
	}{
		{name: "国庆节首日（周四）", date: "2026-10-01", wantWork: false, wantKind: "holiday"},
		{name: "调休上班的周六", date: "2026-10-10", wantWork: true},
		{name: "普通周日", date: "2026-10-11", wantWork: false, wantKind: "weekend"},
		{name: "春节调休上班的周日", date: "2027-01-31", wantWork: true},
		{name: "春节假期中", date: "2027-02-06", wantWork: false, wantKind: "holiday"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			date := mustDate(t, tt.date)
			if got := cal.IsWorkday(date); got != tt.wantWork {
				t.Errorf("IsWorkday(%s) = %v，期望 %v", tt.date, got, tt.wantWork)
			}
			if !tt.wantWork {
				if got := cal.DayOffKind(date); got != tt.wantKind {
					t.Errorf("DayOffKind(%s) = %q，期望 %q", tt.date, got, tt.wantKind)
				}
			}
		})
	}
}

func TestEmbeddedNextHoliday(t *testing.T) {
	cal := loadEmbeddedCalendar(t)
	want := Holiday{Kind: KindUpcoming, Name: "元旦", DaysLeft: 38}
	if got, ok := cal.Next(mustDate(t, "2026-11-24")); !ok || got != want {
		t.Errorf("2026-11-24 的下一个假期 = %+v（找到：%v），期望 %+v", got, ok, want)
	}
}
