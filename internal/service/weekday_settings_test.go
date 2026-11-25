package service

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"cowtime/internal/config"
	"cowtime/internal/salary"
	"cowtime/internal/workhours"
)

// clock 返回指向 s 的指针，用来填写覆盖项里的时间。
func clock(s string) *string { return &s }

func TestSaveSettingsWeekdayHours(t *testing.T) {
	overrides := map[string]workhours.Partial{
		"1": {End: clock("20:30")},
		"2": {End: clock("20:30")},
		"4": {End: clock("20:30")},
	}
	svc, _ := newTestService(t, configured(10000))

	got := svc.SaveSettings(Settings{
		MonthlySalary:   10000,
		WorkHours:       workhours.Default(),
		BackgroundStyle: config.StyleAurora,
		WeekdayHours:    overrides,
	})
	if !got.OK {
		t.Fatalf("SaveSettings() 意外失败：%q", got.Message)
	}
	if saved := svc.store.Get().WeekdayHours; !reflect.DeepEqual(saved, overrides) {
		t.Errorf("保存的 weekdayHours = %+v，期望 %+v", saved, overrides)
	}
	// GetStatus 把单独设置带回前端，设置视图据此显示小圆点。
	if got := svc.GetStatus().Settings.WeekdayHours; !reflect.DeepEqual(got, overrides) {
		t.Errorf("GetStatus 返回的 weekdayHours = %+v，期望 %+v", got, overrides)
	}
}

func TestSaveSettingsRejectsInvalidWeekdayHours(t *testing.T) {
	tests := []struct {
		name        string
		overrides   map[string]workhours.Partial
		wantMessage string
	}{
		{
			name:        "某一天的作息顺序不对",
			overrides:   map[string]workhours.Partial{"3": {Start: clock("13:00")}},
			wantMessage: "周三",
		},
		{
			name:        "某一天的时间格式不对",
			overrides:   map[string]workhours.Partial{"5": {LunchEnd: clock("25:00")}},
			wantMessage: "周五",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _ := newTestService(t, configured(10000))
			got := svc.SaveSettings(Settings{
				MonthlySalary:   10000,
				WorkHours:       workhours.Default(),
				BackgroundStyle: config.StyleAurora,
				WeekdayHours:    tt.overrides,
			})
			if got.OK {
				t.Fatalf("SaveSettings() 应该失败，却成功了")
			}
			if !strings.Contains(got.Message, tt.wantMessage) {
				t.Errorf("提示 = %q，期望包含 %q", got.Message, tt.wantMessage)
			}
			saved := svc.store.Get()
			if saved.WeekdayHours != nil || saved.WorkHours != workhours.Default() {
				t.Errorf("保存失败之后原来的设置不应该改变，得到 (%+v, %+v)", saved.WeekdayHours, saved.WorkHours)
			}
		})
	}
}

func TestGetStatusUsesWeekdayHours(t *testing.T) {
	cfg := configured(10000)
	cfg.WeekdayHours = map[string]workhours.Partial{"1": {End: clock("20:30")}} // 周一 20:30 下班
	svc, _ := newTestService(t, cfg)

	svc.now = func() time.Time { return atTime("2026-11-23 19:00:00") } // 周一
	if got := svc.GetStatus(); got.State != salary.StateWorking {
		t.Errorf("周一 19 点的状态 = %q，期望 %q", got.State, salary.StateWorking)
	}

	svc.now = func() time.Time { return atTime("2026-11-25 19:00:00") } // 周三
	if got := svc.GetStatus(); got.State != salary.StateAfterWork {
		t.Errorf("周三 19 点的状态 = %q，期望 %q", got.State, salary.StateAfterWork)
	}
}
