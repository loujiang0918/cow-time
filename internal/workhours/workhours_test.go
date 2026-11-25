package workhours

import (
	"errors"
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		hours   Hours
		want    Schedule
		wantErr bool
	}{
		{
			name:  "默认作息",
			hours: Default(),
			want:  Schedule{Start: 540, LunchStart: 720, LunchEnd: 780, End: 1080},
		},
		{
			name:  "没有午休",
			hours: Hours{Start: "09:00", End: "17:00"},
			want:  Schedule{Start: 540, LunchStart: 540, LunchEnd: 540, End: 1020},
		},
		{name: "只填了午休开始", hours: Hours{Start: "09:00", LunchStart: "12:00", End: "18:00"}, wantErr: true},
		{name: "只填了午休结束", hours: Hours{Start: "09:00", LunchEnd: "13:00", End: "18:00"}, wantErr: true},
		{
			name:    "午休开始早于上班",
			hours:   Hours{Start: "09:00", LunchStart: "08:00", LunchEnd: "09:00", End: "18:00"},
			wantErr: true,
		},
		{
			name:    "午休结束晚于下班",
			hours:   Hours{Start: "09:00", LunchStart: "12:00", LunchEnd: "19:00", End: "18:00"},
			wantErr: true,
		},
		{
			name:    "午休时长为零",
			hours:   Hours{Start: "09:00", LunchStart: "12:00", LunchEnd: "12:00", End: "18:00"},
			wantErr: true,
		},
		{name: "下班不晚于上班", hours: Hours{Start: "18:00", End: "09:00"}, wantErr: true},
		{
			name:    "时间格式不对",
			hours:   Hours{Start: "9点", LunchStart: "12:00", LunchEnd: "13:00", End: "18:00"},
			wantErr: true,
		},
		{
			name:    "小时越界",
			hours:   Hours{Start: "24:00", LunchStart: "12:00", LunchEnd: "13:00", End: "18:00"},
			wantErr: true,
		},
		{
			name:    "分钟越界",
			hours:   Hours{Start: "09:60", LunchStart: "12:00", LunchEnd: "13:00", End: "18:00"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.hours.Parse()
			if tt.wantErr {
				if !errors.Is(err, ErrInvalid) {
					t.Fatalf("Parse() error = %v，期望 ErrInvalid", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse() 意外返回错误：%v", err)
			}
			if got != tt.want {
				t.Errorf("Parse() = %+v，期望 %+v", got, tt.want)
			}
		})
	}
}

func TestDailySeconds(t *testing.T) {
	tests := []struct {
		name  string
		hours Hours
		want  int
	}{
		{name: "默认作息，每天 8 小时", hours: Default(), want: 8 * 3600},
		{name: "没有午休，9 点到 17 点", hours: Hours{Start: "09:00", End: "17:00"}, want: 8 * 3600},
		{name: "午休半小时", hours: Hours{Start: "09:00", LunchStart: "12:00", LunchEnd: "12:30", End: "18:00"}, want: 8*3600 + 30*60},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sched, err := tt.hours.Parse()
			if err != nil {
				t.Fatalf("Parse() 意外返回错误：%v", err)
			}
			if got := sched.DailySeconds(); got != tt.want {
				t.Errorf("DailySeconds() = %d，期望 %d", got, tt.want)
			}
		})
	}
}

func TestPaidSecondsAt(t *testing.T) {
	sched, err := Default().Parse()
	if err != nil {
		t.Fatalf("Parse() 意外返回错误：%v", err)
	}

	tests := []struct {
		name        string
		secondOfDay int
		want        int
	}{
		{name: "上班前一秒", secondOfDay: 8*3600 + 59*60 + 59, want: 0},
		{name: "刚到上班时间", secondOfDay: 9 * 3600, want: 0},
		{name: "上班一秒后", secondOfDay: 9*3600 + 1, want: 1},
		{name: "午休开始", secondOfDay: 12 * 3600, want: 3 * 3600},
		{name: "午休中", secondOfDay: 12*3600 + 30*60, want: 3 * 3600},
		{name: "午休结束", secondOfDay: 13 * 3600, want: 3 * 3600},
		{name: "下班时", secondOfDay: 18 * 3600, want: 8 * 3600},
		{name: "下班后", secondOfDay: 23*3600 + 3599, want: 8 * 3600},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sched.PaidSecondsAt(tt.secondOfDay); got != tt.want {
				t.Errorf("PaidSecondsAt(%d) = %d，期望 %d", tt.secondOfDay, got, tt.want)
			}
		})
	}
}

func TestPaidSecondsAtWithoutLunch(t *testing.T) {
	sched, err := (Hours{Start: "09:00", End: "17:00"}).Parse()
	if err != nil {
		t.Fatalf("Parse() 意外返回错误：%v", err)
	}
	if got, want := sched.PaidSecondsAt(12*3600), 3*3600; got != want {
		t.Errorf("没有午休时，中午 12 点已计薪 %d 秒，期望 %d 秒", got, want)
	}
	if got, want := sched.PaidSecondsAt(17*3600), 8*3600; got != want {
		t.Errorf("没有午休时，下午 5 点已计薪 %d 秒，期望 %d 秒", got, want)
	}
}

func TestSecondOfDay(t *testing.T) {
	at := time.Date(2026, 11, 24, 12, 30, 15, 0, time.Local)
	if got, want := SecondOfDay(at), 12*3600+30*60+15; got != want {
		t.Errorf("SecondOfDay() = %d，期望 %d", got, want)
	}
}
