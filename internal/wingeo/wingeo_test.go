package wingeo

import "testing"

// fullHD 是 1920×1080 屏幕、任务栏在底部时的工作区。
var fullHD = Rect{X: 0, Y: 0, Width: 1920, Height: 1032}

func TestDefaultPosition(t *testing.T) {
	tests := []struct {
		name    string
		primary Rect
		wantX   int
		wantY   int
	}{
		{name: "全高清屏，任务栏在底部", primary: fullHD, wantX: 1664, wantY: 884},
		{name: "2K 屏", primary: Rect{X: 0, Y: 0, Width: 2560, Height: 1400}, wantX: 2304, wantY: 1252},
		{name: "主屏的原点不在 (0, 0)", primary: Rect{X: -1920, Y: 0, Width: 1920, Height: 1040}, wantX: -256, wantY: 892},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotX, gotY := DefaultPosition(DisplayWidth, DisplayHeight, tt.primary)
			if gotX != tt.wantX || gotY != tt.wantY {
				t.Errorf("DefaultPosition() = (%d, %d)，期望 (%d, %d)", gotX, gotY, tt.wantX, tt.wantY)
			}
		})
	}
}

func TestIsVisible(t *testing.T) {
	secondScreen := Rect{X: 1920, Y: 0, Width: 1920, Height: 1080}

	tests := []struct {
		name  string
		win   Rect
		areas []Rect
		want  bool
	}{
		{name: "完整落在工作区内", win: Rect{X: 100, Y: 100, Width: 240, Height: 132}, areas: []Rect{fullHD}, want: true},
		{name: "露出左边正好 48 宽", win: Rect{X: -192, Y: 100, Width: 240, Height: 132}, areas: []Rect{fullHD}, want: true},
		{name: "露出右边只有 20 宽", win: Rect{X: 1900, Y: 100, Width: 240, Height: 132}, areas: []Rect{fullHD}, want: false},
		{name: "落在任务栏上，可见高度只有 24", win: Rect{X: 100, Y: 1056, Width: 240, Height: 132}, areas: []Rect{fullHD}, want: false},
		{name: "完全在屏幕外", win: Rect{X: 5000, Y: 5000, Width: 240, Height: 132}, areas: []Rect{fullHD}, want: false},
		{name: "落在副屏上", win: Rect{X: 2000, Y: 100, Width: 240, Height: 132}, areas: []Rect{fullHD, secondScreen}, want: true},
		{name: "拿不到屏幕信息", win: Rect{X: 100, Y: 100, Width: 240, Height: 132}, areas: nil, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsVisible(tt.win, tt.areas); got != tt.want {
				t.Errorf("IsVisible() = %v，期望 %v", got, tt.want)
			}
		})
	}
}

func TestFitUpward(t *testing.T) {
	tests := []struct {
		name  string
		win   Rect
		area  Rect
		newH  int
		wantX int
		wantY int
	}{
		{
			name:  "变高之后底部超出工作区，向上移",
			win:   Rect{X: 1664, Y: 884, Width: 240, Height: 132},
			area:  fullHD,
			newH:  SettingsHeight,
			wantX: 1664,
			wantY: 604,
		},
		{
			name:  "变高之后仍然放得下，保持原位",
			win:   Rect{X: 100, Y: 100, Width: 240, Height: 132},
			area:  fullHD,
			newH:  SettingsHeight,
			wantX: 100,
			wantY: 100,
		},
		{
			name:  "变高之后正好贴着底边，保持原位",
			win:   Rect{X: 100, Y: 604, Width: 240, Height: 132},
			area:  fullHD,
			newH:  SettingsHeight,
			wantX: 100,
			wantY: 604,
		},
		{
			name:  "工作区比变高后的窗口还矮，贴着顶边",
			win:   Rect{X: 100, Y: 50, Width: 240, Height: 132},
			area:  Rect{X: 0, Y: 0, Width: 1920, Height: 200},
			newH:  SettingsHeight,
			wantX: 100,
			wantY: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotX, gotY := FitUpward(tt.win, tt.area, tt.newH)
			if gotX != tt.wantX || gotY != tt.wantY {
				t.Errorf("FitUpward() = (%d, %d)，期望 (%d, %d)", gotX, gotY, tt.wantX, tt.wantY)
			}
		})
	}
}

func TestCurrentWorkArea(t *testing.T) {
	secondScreen := Rect{X: 1920, Y: 0, Width: 1920, Height: 1080}
	areas := []Rect{fullHD, secondScreen}

	tests := []struct {
		name string
		win  Rect
		want Rect
		ok   bool
	}{
		{name: "在主屏上", win: Rect{X: 100, Y: 100, Width: 240, Height: 132}, want: fullHD, ok: true},
		{name: "在副屏上", win: Rect{X: 2000, Y: 100, Width: 240, Height: 132}, want: secondScreen, ok: true},
		{name: "横跨两块屏幕，大部分在副屏", win: Rect{X: 1850, Y: 100, Width: 240, Height: 132}, want: secondScreen, ok: true},
		{name: "完全在屏幕外", win: Rect{X: 5000, Y: 5000, Width: 240, Height: 132}, ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := CurrentWorkArea(tt.win, areas)
			if ok != tt.ok || got != tt.want {
				t.Errorf("CurrentWorkArea() = (%+v, %v)，期望 (%+v, %v)", got, ok, tt.want, tt.ok)
			}
		})
	}
}
