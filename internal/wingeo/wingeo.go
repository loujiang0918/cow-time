// Package wingeo 计算悬浮窗的位置：默认位置、保存的位置是否可见、设置视图的让位。
package wingeo

// 两种视图下窗口的尺寸，单位是 DIP。
const (
	DisplayWidth   = 240
	DisplayHeight  = 132
	SettingsWidth  = 240
	SettingsHeight = 428

	// Margin 是窗口与屏幕工作区边缘之间的距离。
	Margin = 16

	// minVisibleWidth 和 minVisibleHeight 是窗口仍然算作「可见」所需的最小可见区域。
	minVisibleWidth  = 48
	minVisibleHeight = 32
)

// Rect 是一个矩形，位置和尺寸都以 DIP 计。
type Rect struct {
	X      int
	Y      int
	Width  int
	Height int
}

// DefaultPosition 返回窗口在主屏幕工作区右下角的位置。primary 是主屏幕的工作区。
func DefaultPosition(winW, winH int, primary Rect) (x, y int) {
	return primary.X + primary.Width - winW - Margin,
		primary.Y + primary.Height - winH - Margin
}

// IsVisible 报告窗口是否有足够的部分落在某个屏幕工作区之内。
func IsVisible(win Rect, workAreas []Rect) bool {
	for _, area := range workAreas {
		if overlap(win.X, win.Width, area.X, area.Width) >= minVisibleWidth &&
			overlap(win.Y, win.Height, area.Y, area.Height) >= minVisibleHeight {
			return true
		}
	}
	return false
}

// FitUpward 为变高之后的窗口计算位置：如果底部超出了工作区，就把它向上移。
// 不需要移动时，返回窗口原来的位置。
func FitUpward(win Rect, workArea Rect, newHeight int) (x, y int) {
	x, y = win.X, win.Y
	if y+newHeight > workArea.Y+workArea.Height {
		y = workArea.Y + workArea.Height - newHeight
		if y < workArea.Y {
			y = workArea.Y
		}
	}
	return x, y
}

// CurrentWorkArea 返回与窗口重叠最多的工作区；一个都没有重叠时返回 false。
func CurrentWorkArea(win Rect, workAreas []Rect) (Rect, bool) {
	var best Rect
	bestArea := 0
	found := false

	for _, workArea := range workAreas {
		width := overlap(win.X, win.Width, workArea.X, workArea.Width)
		height := overlap(win.Y, win.Height, workArea.Y, workArea.Height)
		if area := width * height; area > bestArea {
			best, bestArea, found = workArea, area, true
		}
	}
	return best, found
}

// overlap 返回两个区间的重叠长度：第一个从 a0 开始、宽 aw，第二个从 b0 开始、宽 bw。
func overlap(a0, aw, b0, bw int) int {
	left := max(a0, b0)
	right := min(a0+aw, b0+bw)
	return max(right-left, 0)
}
