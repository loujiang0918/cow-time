package config

import (
	"reflect"
	"testing"
)

func TestDefaultTransparency(t *testing.T) {
	// 初始透明度就是四种背景风格现在的样子。
	for style, want := range map[string]int{StylePlain: 28, StyleAurora: 60, StyleNeon: 32, StyleStars: 42} {
		if got := DefaultTransparency(style); got != want {
			t.Errorf("DefaultTransparency(%q) = %d，期望 %d", style, got, want)
		}
	}
	if got := DefaultTransparency("cyber"); got != 0 {
		t.Errorf("不是背景风格的取值应该返回 0，得到 %d", got)
	}
}

func TestTransparencyFor(t *testing.T) {
	cfg := Default()
	if got := cfg.TransparencyFor(StyleAurora); got != 60 {
		t.Errorf("没有记录时应该使用初始值 60，得到 %d", got)
	}

	cfg.Transparency = map[string]int{StylePlain: 10}
	if got := cfg.TransparencyFor(StylePlain); got != 10 {
		t.Errorf("有记录时应该使用记录的值 10，得到 %d", got)
	}
	if got := cfg.TransparencyFor(StyleNeon); got != 32 {
		t.Errorf("没有记录的风格应该使用初始值 32，得到 %d", got)
	}
}

func TestCleanTransparency(t *testing.T) {
	tests := []struct {
		name string
		in   map[string]int
		want map[string]int
	}{
		{name: "没有记录", in: nil, want: nil},
		{
			name: "合法的记录被保留，0 和 100 都在范围之内",
			in:   map[string]int{StylePlain: 10, StyleAurora: 100, StyleNeon: 0},
			want: map[string]int{StylePlain: 10, StyleAurora: 100, StyleNeon: 0},
		},
		{
			name: "不是背景风格的键被丢弃",
			in:   map[string]int{StylePlain: 10, "cyber": 20},
			want: map[string]int{StylePlain: 10},
		},
		{
			name: "超出范围的值被丢弃",
			in:   map[string]int{StylePlain: 101, StyleAurora: -1, StyleStars: 50},
			want: map[string]int{StyleStars: 50},
		},
		{
			name: "全都不合法时为空",
			in:   map[string]int{StylePlain: 101},
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CleanTransparency(tt.in); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("CleanTransparency() = %v，期望 %v", got, tt.want)
			}
		})
	}
}

func TestLoadTransparency(t *testing.T) {
	const rest = `"monthlySalary": 10000, "workHours": {"start": "09:00", "lunchStart": "12:00", "lunchEnd": "13:00", "end": "18:00"}`
	tests := []struct {
		name string
		body string
		want map[string]int
	}{
		{
			name: "没有这一项",
			body: `{` + rest + `}`,
			want: nil,
		},
		{
			name: "合法的记录被保留",
			body: `{` + rest + `, "transparency": {"plain": 10, "stars": 0}}`,
			want: map[string]int{StylePlain: 10, StyleStars: 0},
		},
		{
			name: "不合法的记录被忽略",
			body: `{` + rest + `, "transparency": {"cyber": 20, "neon": 200, "aurora": 100}}`,
			want: map[string]int{StyleAurora: 100},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := loadWeekdayConfig(t, tt.body)
			if !reflect.DeepEqual(got.Transparency, tt.want) {
				t.Errorf("transparency = %v，期望 %v", got.Transparency, tt.want)
			}
			if got.MonthlySalary != 10000 {
				t.Errorf("无论 transparency 有没有问题，月薪都应该照常生效，得到 %v", got.MonthlySalary)
			}
		})
	}
}

func TestUpdateWritesTransparency(t *testing.T) {
	store, path := newStore(t)
	want := map[string]int{StylePlain: 10}

	err := store.Update(func(c *Config) {
		c.MonthlySalary = 10000
		c.Transparency = map[string]int{StylePlain: 10}
	})
	if err != nil {
		t.Fatalf("Update() 意外返回错误：%v", err)
	}

	reloaded, err := Load(path)
	if err != nil {
		t.Fatalf("重新加载：%v", err)
	}
	if got := reloaded.Get().Transparency; !reflect.DeepEqual(got, want) {
		t.Errorf("重新加载之后 transparency = %v，期望 %v", got, want)
	}
}
