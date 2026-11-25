package service

import (
	"reflect"
	"strings"
	"testing"

	"cowtime/internal/config"
	"cowtime/internal/workhours"
)

func TestSetTransparency(t *testing.T) {
	svc, _ := newTestService(t, configured(10000))

	// 与初始值不同的透明度被保存。
	if got := svc.SetTransparency(config.StylePlain, 10); !got.OK {
		t.Fatalf("SetTransparency() 意外失败：%q", got.Message)
	}
	if saved := svc.store.Get().Transparency; !reflect.DeepEqual(saved, map[string]int{config.StylePlain: 10}) {
		t.Errorf("保存的 transparency = %v，期望 map[plain:10]", saved)
	}

	// GetStatus 返回四种风格各自生效的透明度。
	want := map[string]int{config.StylePlain: 10, config.StyleAurora: 60, config.StyleNeon: 32, config.StyleStars: 42}
	if got := svc.GetStatus().Transparency; !reflect.DeepEqual(got, want) {
		t.Errorf("GetStatus 返回的 transparency = %v，期望 %v", got, want)
	}

	// 调回初始值之后，记录被删除。
	if got := svc.SetTransparency(config.StylePlain, config.DefaultTransparency(config.StylePlain)); !got.OK {
		t.Fatalf("调回初始值时 SetTransparency() 意外失败：%q", got.Message)
	}
	if saved := svc.store.Get().Transparency; len(saved) != 0 {
		t.Errorf("调回初始值之后不应该再有记录，得到 %v", saved)
	}
}

func TestSetTransparencyRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name        string
		style       string
		value       int
		wantMessage string
	}{
		{name: "风格不是背景风格", style: "cyber", value: 10, wantMessage: "背景风格无效"},
		{name: "透明度超过 100", style: config.StyleStars, value: 101, wantMessage: "0% 到 100%"},
		{name: "透明度是负数", style: config.StyleStars, value: -1, wantMessage: "0% 到 100%"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _ := newTestService(t, configured(10000))
			got := svc.SetTransparency(tt.style, tt.value)
			if got.OK {
				t.Fatalf("SetTransparency(%q, %d) 应该失败，却成功了", tt.style, tt.value)
			}
			if !strings.Contains(got.Message, tt.wantMessage) {
				t.Errorf("提示 = %q，期望包含 %q", got.Message, tt.wantMessage)
			}
			if saved := svc.store.Get().Transparency; saved != nil {
				t.Errorf("保存失败时不应该留下记录，得到 %v", saved)
			}
		})
	}
}

func TestSaveSettingsKeepsTransparency(t *testing.T) {
	svc, _ := newTestService(t, configured(10000))
	if got := svc.SetTransparency(config.StyleAurora, 20); !got.OK {
		t.Fatalf("SetTransparency() 意外失败：%q", got.Message)
	}

	// 保存月薪、作息和背景风格时，已经保存的透明度保持不变。
	got := svc.SaveSettings(Settings{
		MonthlySalary:   12000,
		WorkHours:       workhours.Default(),
		BackgroundStyle: config.StyleNeon,
	})
	if !got.OK {
		t.Fatalf("SaveSettings() 意外失败：%q", got.Message)
	}
	saved := svc.store.Get()
	if saved.MonthlySalary != 12000 || saved.BackgroundStyle != config.StyleNeon {
		t.Errorf("月薪和背景风格 = (%v, %q)，期望 (12000, neon)", saved.MonthlySalary, saved.BackgroundStyle)
	}
	if want := map[string]int{config.StyleAurora: 20}; !reflect.DeepEqual(saved.Transparency, want) {
		t.Errorf("保存设置之后 transparency = %v，期望保持为 %v", saved.Transparency, want)
	}
}
