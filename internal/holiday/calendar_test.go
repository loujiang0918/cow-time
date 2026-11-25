package holiday

import (
	"strings"
	"testing"
	"time"
)

// mustDate 把 "YYYY-MM-DD" 解析成本地时间的当天零点，失败时终止测试。
func mustDate(t *testing.T, s string) time.Time {
	t.Helper()
	parsed, err := ParseDate(s)
	if err != nil {
		t.Fatalf("解析日期 %q 失败：%v", s, err)
	}
	return parsed
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		data    YearData
		wantErr string // 期望的错误信息里包含的文字；为空表示应该通过校验
	}{
		{
			name: "合法数据",
			data: YearData{Year: 2026, Days: []Day{{Name: "元旦", Date: "2026-01-01", IsOffDay: true}}},
		},
		{name: "没有任何记录", data: YearData{Year: 2026}, wantErr: "为空"},
		{
			name:    "缺少节日名",
			data:    YearData{Year: 2026, Days: []Day{{Date: "2026-01-01", IsOffDay: true}}},
			wantErr: "缺少节日名",
		},
		{
			name:    "日期格式不对",
			data:    YearData{Year: 2026, Days: []Day{{Name: "元旦", Date: "2026/01/01", IsOffDay: true}}},
			wantErr: "日期不合法",
		},
		{
			name:    "不是日历上存在的日期",
			data:    YearData{Year: 2026, Days: []Day{{Name: "元旦", Date: "2026-02-30", IsOffDay: true}}},
			wantErr: "日期不合法",
		},
		{
			name:    "日期属于别的年份",
			data:    YearData{Year: 2026, Days: []Day{{Name: "元旦", Date: "2027-01-01", IsOffDay: true}}},
			wantErr: "日期不合法",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.data)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() 意外返回错误：%v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate() error = %v，期望包含 %q", err, tt.wantErr)
			}
		})
	}
}

// october2026 是 2026 年国庆的两条典型记录：10 月 1 日（周四）放假，10 月 10 日（周六）调休上班。
var october2026 = YearData{Year: 2026, Days: []Day{
	{Name: "国庆节", Date: "2026-10-01", IsOffDay: true},
	{Name: "国庆节", Date: "2026-10-10", IsOffDay: false},
}}

func TestIsWorkday(t *testing.T) {
	cal := NewCalendar()
	if err := cal.AddYear(october2026); err != nil {
		t.Fatalf("AddYear() 意外返回错误：%v", err)
	}

	tests := []struct {
		name string
		date string
		want bool
	}{
		{name: "法定节假日落在周四", date: "2026-10-01", want: false},
		{name: "调休上班的周六", date: "2026-10-10", want: true},
		{name: "普通周一", date: "2026-10-12", want: true},
		{name: "普通周日", date: "2026-10-11", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cal.IsWorkday(mustDate(t, tt.date)); got != tt.want {
				t.Errorf("IsWorkday(%s) = %v，期望 %v", tt.date, got, tt.want)
			}
		})
	}
}

func TestDayOffKind(t *testing.T) {
	cal := NewCalendar()
	if err := cal.AddYear(october2026); err != nil {
		t.Fatalf("AddYear() 意外返回错误：%v", err)
	}

	tests := []struct {
		date string
		want string
	}{
		{date: "2026-10-01", want: "holiday"},
		{date: "2026-10-11", want: "weekend"},
	}

	for _, tt := range tests {
		if got := cal.DayOffKind(mustDate(t, tt.date)); got != tt.want {
			t.Errorf("DayOffKind(%s) = %q，期望 %q", tt.date, got, tt.want)
		}
	}
}

func TestAddYearReplacesSameYear(t *testing.T) {
	cal := NewCalendar()
	first := YearData{Year: 2026, Days: []Day{{Name: "元旦", Date: "2026-01-01", IsOffDay: true}}}
	second := YearData{Year: 2026, Days: []Day{{Name: "春节", Date: "2026-02-17", IsOffDay: true}}}
	other := YearData{Year: 2027, Days: []Day{{Name: "元旦", Date: "2027-01-01", IsOffDay: true}}}

	for _, data := range []YearData{first, other, second} {
		if err := cal.AddYear(data); err != nil {
			t.Fatalf("AddYear() 意外返回错误：%v", err)
		}
	}

	if _, ok := cal.lookup(mustDate(t, "2026-01-01")); ok {
		t.Error("再次加入 2026 年的数据之后，旧的 1 月 1 日记录应该被替换掉")
	}
	if _, ok := cal.lookup(mustDate(t, "2026-02-17")); !ok {
		t.Error("再次加入 2026 年的数据之后，新的 2 月 17 日记录应该存在")
	}
	if _, ok := cal.lookup(mustDate(t, "2027-01-01")); !ok {
		t.Error("替换 2026 年的数据不应该影响 2027 年")
	}
}

func TestAddYearRejectsInvalidData(t *testing.T) {
	cal := NewCalendar()
	invalid := YearData{Year: 2026, Days: []Day{{Name: "元旦", Date: "2027-01-01", IsOffDay: true}}}
	if err := cal.AddYear(invalid); err == nil {
		t.Fatal("AddYear() 应该拒绝日期与年份不符的数据")
	}
	if cal.HasYear(2026) {
		t.Error("被拒绝的数据不应该进入日历")
	}
}
