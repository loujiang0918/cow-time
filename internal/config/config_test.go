package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"cowtime/internal/workhours"
)

// newStore 在临时目录里创建一个配置仓库，并返回它和配置文件的路径。
func newStore(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "nested", "config.json")
	store, err := Load(path)
	if err != nil {
		t.Fatalf("Load() 意外返回错误：%v", err)
	}
	return store, path
}

func TestLoadMissingFileUsesDefaults(t *testing.T) {
	store, _ := newStore(t)
	if got, want := store.Get(), Default(); !reflect.DeepEqual(got, want) {
		t.Errorf("文件不存在时 Get() = %+v，期望 %+v", got, want)
	}
	if store.Get().Window != nil {
		t.Error("默认配置不应该带有窗口位置")
	}
}

func TestLoadParsesFile(t *testing.T) {
	checkedAt := time.Date(2026, 11, 24, 16, 40, 0, 0, time.Local)
	body := `{
		"monthlySalary": 12345.5,
		"workHours": {"start": "08:30", "lunchStart": "", "lunchEnd": "", "end": "17:30"},
		"backgroundStyle": "neon",
		"window": {"x": 100, "y": 200},
		"holidayCheckAt": "` + checkedAt.Format(time.RFC3339) + `"
	}`
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := Load(path)
	if err != nil {
		t.Fatalf("Load() 意外返回错误：%v", err)
	}

	got := store.Get()
	if got.MonthlySalary != 12345.5 {
		t.Errorf("月薪 = %v，期望 12345.5", got.MonthlySalary)
	}
	wantHours := workhours.Hours{Start: "08:30", End: "17:30"}
	if got.WorkHours != wantHours {
		t.Errorf("作息时间 = %+v，期望 %+v", got.WorkHours, wantHours)
	}
	if got.BackgroundStyle != StyleNeon {
		t.Errorf("背景风格 = %q，期望 %q", got.BackgroundStyle, StyleNeon)
	}
	if got.Window == nil || *got.Window != (WindowPos{X: 100, Y: 200}) {
		t.Errorf("窗口位置 = %+v，期望 {100 200}", got.Window)
	}
	if got.HolidayCheckAt == nil || !got.HolidayCheckAt.Equal(checkedAt) {
		t.Errorf("上次检查节假日数据的时间 = %v，期望 %v", got.HolidayCheckAt, checkedAt)
	}
}

func TestLoadInvalidContentUsesDefaults(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "不是 JSON", body: "{not json"},
		{
			name: "月薪是负数",
			body: `{"monthlySalary": -1, "workHours": {"start": "09:00", "lunchStart": "12:00", "lunchEnd": "13:00", "end": "18:00"}}`,
		},
		{
			name: "作息时间的顺序不对",
			body: `{"monthlySalary": 10000, "workHours": {"start": "18:00", "lunchStart": "12:00", "lunchEnd": "13:00", "end": "09:00"}}`,
		},
		{name: "缺少作息时间", body: `{"monthlySalary": 10000}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(path, []byte(tt.body), 0o644); err != nil {
				t.Fatal(err)
			}

			store, err := Load(path)
			if err != nil {
				t.Fatalf("Load() 意外返回错误：%v", err)
			}
			if got, want := store.Get(), Default(); !reflect.DeepEqual(got, want) {
				t.Errorf("内容无效时 Get() = %+v，期望默认值 %+v", got, want)
			}
		})
	}
}

func TestUpdateWritesFile(t *testing.T) {
	store, path := newStore(t)
	saved := WindowPos{X: 1664, Y: 884}

	err := store.Update(func(c *Config) {
		c.MonthlySalary = 10000
		c.BackgroundStyle = StyleStars
		c.Window = &saved
	})
	if err != nil {
		t.Fatalf("Update() 意外返回错误：%v", err)
	}

	reloaded, err := Load(path)
	if err != nil {
		t.Fatalf("重新加载：%v", err)
	}
	got := reloaded.Get()
	if got.MonthlySalary != 10000 {
		t.Errorf("重新加载之后月薪 = %v，期望 10000", got.MonthlySalary)
	}
	if got.Window == nil || *got.Window != saved {
		t.Errorf("重新加载之后窗口位置 = %+v，期望 %+v", got.Window, saved)
	}
	if got.WorkHours != workhours.Default() {
		t.Errorf("重新加载之后作息时间 = %+v，期望保持默认值 %+v", got.WorkHours, workhours.Default())
	}
	if got.BackgroundStyle != StyleStars {
		t.Errorf("重新加载之后背景风格 = %q，期望 %q", got.BackgroundStyle, StyleStars)
	}
}

func TestValidBackgroundStyle(t *testing.T) {
	for _, style := range []string{StylePlain, StyleAurora, StyleNeon, StyleStars} {
		if !ValidBackgroundStyle(style) {
			t.Errorf("ValidBackgroundStyle(%q) = false，期望 true", style)
		}
	}
	for _, style := range []string{"", "Aurora", "cyber", "plain "} {
		if ValidBackgroundStyle(style) {
			t.Errorf("ValidBackgroundStyle(%q) = true，期望 false", style)
		}
	}
}

func TestLoadBackgroundStyle(t *testing.T) {
	const hours = `"workHours": {"start": "09:00", "lunchStart": "12:00", "lunchEnd": "13:00", "end": "18:00"}`
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "没有写背景风格",
			body: `{"monthlySalary": 10000, ` + hours + `}`,
			want: DefaultBackgroundStyle,
		},
		{
			name: "写了合法的值",
			body: `{"monthlySalary": 10000, ` + hours + `, "backgroundStyle": "neon"}`,
			want: StyleNeon,
		},
		{
			name: "写了不认识的值",
			body: `{"monthlySalary": 10000, ` + hours + `, "backgroundStyle": "cyber"}`,
			want: DefaultBackgroundStyle,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(path, []byte(tt.body), 0o644); err != nil {
				t.Fatal(err)
			}

			store, err := Load(path)
			if err != nil {
				t.Fatalf("Load() 意外返回错误：%v", err)
			}
			got := store.Get()
			if got.BackgroundStyle != tt.want {
				t.Errorf("背景风格 = %q，期望 %q", got.BackgroundStyle, tt.want)
			}
			// 背景风格有问题时，其他设置照常生效。
			if got.MonthlySalary != 10000 || got.WorkHours != workhours.Default() {
				t.Errorf("月薪和作息 = (%v, %+v)，期望 (10000, %+v)", got.MonthlySalary, got.WorkHours, workhours.Default())
			}
		})
	}
}

func TestUpdateFailureKeepsMemoryUnchanged(t *testing.T) {
	// 把配置文件放到一个「本身是文件」的路径下面，让写入必然失败。
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := Load(filepath.Join(blocker, "config.json"))
	if err != nil {
		t.Fatalf("Load() 意外返回错误：%v", err)
	}

	if err := store.Update(func(c *Config) { c.MonthlySalary = 10000 }); err == nil {
		t.Fatal("写入应该失败，Update() 却成功了")
	}
	if got := store.Get().MonthlySalary; got != 0 {
		t.Errorf("写入失败之后内存里的配置不应该改变，月薪 = %v", got)
	}
}
