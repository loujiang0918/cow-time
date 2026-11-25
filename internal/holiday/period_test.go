package holiday

import (
	"testing"
	"time"
)

// autumn2026 用来验证假期段的合并：9 月 24 日到 27 日是连续的 4 天，中间跨了中秋节和国庆节；
// 9 月 28 日上班，把 29 日隔成另一个假期段。
var autumn2026 = YearData{Year: 2026, Days: []Day{
	{Name: "中秋节", Date: "2026-09-24", IsOffDay: true},
	{Name: "中秋节", Date: "2026-09-25", IsOffDay: true},
	{Name: "国庆节", Date: "2026-09-26", IsOffDay: true},
	{Name: "国庆节", Date: "2026-09-27", IsOffDay: true},
	{Name: "国庆节", Date: "2026-09-28", IsOffDay: false},
	{Name: "国庆节", Date: "2026-09-29", IsOffDay: true},
}}

// spring2027 包含 2027 年的元旦（1 月 1 日到 3 日）和春节假期的首尾两天。
var spring2027 = YearData{Year: 2027, Days: []Day{
	{Name: "元旦", Date: "2027-01-01", IsOffDay: true},
	{Name: "元旦", Date: "2027-01-02", IsOffDay: true},
	{Name: "元旦", Date: "2027-01-03", IsOffDay: true},
	{Name: "春节", Date: "2027-01-31", IsOffDay: false},
	{Name: "春节", Date: "2027-02-05", IsOffDay: true},
	{Name: "春节", Date: "2027-02-13", IsOffDay: true},
}}

func TestNext(t *testing.T) {
	cal := NewCalendar()
	if err := cal.AddYear(spring2027); err != nil {
		t.Fatalf("AddYear() 意外返回错误：%v", err)
	}

	tests := []struct {
		name  string
		today string
		want  Holiday
	}{
		{name: "假期之前", today: "2026-11-24", want: Holiday{Kind: KindUpcoming, Name: "元旦", DaysLeft: 38}},
		{name: "假期前一天", today: "2026-12-31", want: Holiday{Kind: KindUpcoming, Name: "元旦", DaysLeft: 1}},
		{name: "假期第一天", today: "2027-01-01", want: Holiday{Kind: KindOngoing, Name: "元旦", DaysLeft: 3}},
		{name: "假期最后一天", today: "2027-01-03", want: Holiday{Kind: KindOngoing, Name: "元旦", DaysLeft: 1}},
		{name: "两个假期之间", today: "2027-01-04", want: Holiday{Kind: KindUpcoming, Name: "春节", DaysLeft: 32}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := cal.Next(mustDate(t, tt.today))
			if !ok {
				t.Fatalf("Next(%s) 没有找到假期", tt.today)
			}
			if got != tt.want {
				t.Errorf("Next(%s) = %+v，期望 %+v", tt.today, got, tt.want)
			}
		})
	}
}

func TestNextIgnoresTimeOfDay(t *testing.T) {
	cal := NewCalendar()
	if err := cal.AddYear(spring2027); err != nil {
		t.Fatalf("AddYear() 意外返回错误：%v", err)
	}

	evening := mustDate(t, "2027-01-03").Add(23 * time.Hour)
	want := Holiday{Kind: KindOngoing, Name: "元旦", DaysLeft: 1}
	if got, _ := cal.Next(evening); got != want {
		t.Errorf("假期最后一天的晚上，Next() = %+v，期望 %+v", got, want)
	}
}

func TestNextMergesAdjacentHolidays(t *testing.T) {
	cal := NewCalendar()
	if err := cal.AddYear(autumn2026); err != nil {
		t.Fatalf("AddYear() 意外返回错误：%v", err)
	}

	tests := []struct {
		today string
		want  Holiday
	}{
		{today: "2026-09-20", want: Holiday{Kind: KindUpcoming, Name: "中秋节、国庆节", DaysLeft: 4}},
		{today: "2026-09-26", want: Holiday{Kind: KindOngoing, Name: "中秋节、国庆节", DaysLeft: 2}},
		{today: "2026-09-28", want: Holiday{Kind: KindUpcoming, Name: "国庆节", DaysLeft: 1}},
	}

	for _, tt := range tests {
		got, ok := cal.Next(mustDate(t, tt.today))
		if !ok {
			t.Fatalf("Next(%s) 没有找到假期", tt.today)
		}
		if got != tt.want {
			t.Errorf("Next(%s) = %+v，期望 %+v", tt.today, got, tt.want)
		}
	}
}

func TestNextWhenNothingIsUpcoming(t *testing.T) {
	cal := NewCalendar()
	if err := cal.AddYear(spring2027); err != nil {
		t.Fatalf("AddYear() 意外返回错误：%v", err)
	}

	got, ok := cal.Next(mustDate(t, "2027-03-01"))
	if ok {
		t.Fatalf("所有假期都结束时不应该找到假期，却得到 %+v", got)
	}
	if got != Unknown {
		t.Errorf("找不到假期时应该返回 Unknown，却得到 %+v", got)
	}

	if _, ok := NewCalendar().Next(mustDate(t, "2026-11-24")); ok {
		t.Error("空日历不应该找到假期")
	}
}
