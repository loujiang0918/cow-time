// Package config 负责读写程序的配置文件。
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"cowtime/internal/workhours"
)

// WindowPos 是窗口左上角的位置，单位是 DIP。
type WindowPos struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// 背景风格的四个取值，与前端 body 元素 data-style 的取值一致。
const (
	StylePlain  = "plain"
	StyleAurora = "aurora"
	StyleNeon   = "neon"
	StyleStars  = "stars"

	// DefaultBackgroundStyle 是配置里没有写背景风格，或写了不认识的值时使用的风格。
	DefaultBackgroundStyle = StyleAurora
)

// ValidBackgroundStyle 报告 style 是否是四个合法的背景风格之一。
func ValidBackgroundStyle(style string) bool {
	switch style {
	case StylePlain, StyleAurora, StyleNeon, StyleStars:
		return true
	}
	return false
}

// BackgroundStyles 是全部背景风格，依次是素色、极光、霓虹、星空。
var BackgroundStyles = [4]string{StylePlain, StyleAurora, StyleNeon, StyleStars}

// MaxTransparency 是透明度的上限，单位是百分比：100 表示卡片底色完全透明。
const MaxTransparency = 100

// DefaultTransparency 返回某种背景风格的初始透明度，单位是百分比：0 表示卡片底色完全不透明。
// style 不是背景风格时返回 0。
func DefaultTransparency(style string) int {
	switch style {
	case StylePlain:
		return 28
	case StyleAurora:
		return 60
	case StyleNeon:
		return 32
	case StyleStars:
		return 42
	}
	return 0
}

// WeekdayKeys 是 weekdayHours 里可以使用的全部键，依次是周一到周日。
var WeekdayKeys = [7]string{"1", "2", "3", "4", "5", "6", "7"}

var weekdayNames = [7]string{"周一", "周二", "周三", "周四", "周五", "周六", "周日"}

// WeekdayKey 返回星期几 day 在 weekdayHours 里对应的键：1 是周一，7 是周日。
func WeekdayKey(day time.Weekday) string {
	return WeekdayKeys[(int(day)+6)%7]
}

// ParseWeekdayKey 把 weekdayHours 里的键换算成星期几。键不是 "1" 到 "7" 时返回 false。
func ParseWeekdayKey(key string) (time.Weekday, bool) {
	n, err := strconv.Atoi(key)
	if err != nil || n < 1 || n > 7 {
		return time.Sunday, false
	}
	return time.Weekday(n % 7), true
}

// WeekdayName 返回某个键对应的星期名，例如 "周一"。键不合法时返回空字符串。
func WeekdayName(key string) string {
	day, ok := ParseWeekdayKey(key)
	if !ok {
		return ""
	}
	return weekdayNames[(int(day)+6)%7]
}

// Config 是程序的全部可保存配置。
type Config struct {
	MonthlySalary   float64                      `json:"monthlySalary"`          // 月薪，单位是元；0 表示还没有设置
	WorkHours       workhours.Hours              `json:"workHours"`              // 默认作息
	WeekdayHours    map[string]workhours.Partial `json:"weekdayHours,omitempty"` // 某些星期与默认作息不同的地方，键是 "1" 到 "7"
	BackgroundStyle string                       `json:"backgroundStyle"`
	Transparency    map[string]int               `json:"transparency,omitempty"`   // 与初始值不同的背景风格的透明度，单位是百分比
	Window          *WindowPos                   `json:"window,omitempty"`         // 为空时使用默认位置
	HolidayCheckAt  *time.Time                   `json:"holidayCheckAt,omitempty"` // 上一次尝试更新节假日数据的时间
}

// Default 返回默认配置。
func Default() Config {
	return Config{WorkHours: workhours.Default(), BackgroundStyle: DefaultBackgroundStyle}
}

// HoursOn 返回星期几为 day 的那一天生效的作息：默认作息套上这一天的覆盖。
// 套用之后的作息不合法时返回错误。
func (c Config) HoursOn(day time.Weekday) (workhours.Hours, error) {
	hours := c.WorkHours
	if partial, ok := c.WeekdayHours[WeekdayKey(day)]; ok {
		hours = partial.Apply(hours)
	}
	_, err := hours.Parse()
	return hours, err
}

// ScheduleOn 返回某一天生效的作息。
// 这一天的作息不合法时使用默认作息；默认作息也不合法时使用内置的默认值。
func (c Config) ScheduleOn(day time.Time) workhours.Schedule {
	hours, err := c.HoursOn(day.Weekday())
	if err != nil {
		hours = c.WorkHours
	}
	sched, err := hours.Parse()
	if err != nil {
		sched, _ = workhours.Default().Parse()
	}
	return sched
}

// CleanWeekdayHours 返回 overrides 里可以使用的部分：
// 丢弃不是 "1" 到 "7" 的键，以及套用到 base 上之后作息不合法的覆盖。没有剩余时返回 nil。
func CleanWeekdayHours(overrides map[string]workhours.Partial, base workhours.Hours) map[string]workhours.Partial {
	var clean map[string]workhours.Partial
	for key, partial := range overrides {
		if _, ok := ParseWeekdayKey(key); !ok {
			continue
		}
		if _, err := partial.Apply(base).Parse(); err != nil {
			continue
		}
		if clean == nil {
			clean = make(map[string]workhours.Partial)
		}
		clean[key] = partial
	}
	return clean
}

// TransparencyFor 返回某种背景风格的透明度，单位是百分比。
// 没有记录，或记录不在 0 到 MaxTransparency 之间时，返回初始值。
func (c Config) TransparencyFor(style string) int {
	if value, ok := c.Transparency[style]; ok && value >= 0 && value <= MaxTransparency {
		return value
	}
	return DefaultTransparency(style)
}

// CleanTransparency 返回 overrides 里可以使用的部分：
// 丢弃不是背景风格的键，以及不在 0 到 MaxTransparency 之间的值。没有剩余时返回 nil。
func CleanTransparency(overrides map[string]int) map[string]int {
	var clean map[string]int
	for style, value := range overrides {
		if !ValidBackgroundStyle(style) || value < 0 || value > MaxTransparency {
			continue
		}
		if clean == nil {
			clean = make(map[string]int)
		}
		clean[style] = value
	}
	return clean
}

// Store 把一份配置保存在内存里，并在每次修改之后写入磁盘。
// 它可以被多个 goroutine 同时使用。
type Store struct {
	mu   sync.RWMutex
	path string
	cfg  Config
}

// Load 读取 path 处的配置文件。文件不存在、无法解析或内容不合法时，使用默认配置。
func Load(path string) (*Store, error) {
	cfg := Default()

	body, err := os.ReadFile(path)
	switch {
	case err == nil:
		if parsed, err := parse(body); err == nil {
			cfg = parsed
		}
	case errors.Is(err, os.ErrNotExist):
	default:
		return nil, err
	}

	return &Store{path: path, cfg: cfg}, nil
}

// parse 解析并检查配置内容。
func parse(body []byte) (Config, error) {
	var cfg Config
	if err := json.Unmarshal(body, &cfg); err != nil {
		return Config{}, err
	}
	if cfg.MonthlySalary < 0 {
		return Config{}, errors.New("月薪不能是负数")
	}
	if _, err := cfg.WorkHours.Parse(); err != nil {
		return Config{}, fmt.Errorf("作息时间不合法：%w", err)
	}
	// 背景风格缺失或不认识时换成默认值，不影响其他配置。
	if !ValidBackgroundStyle(cfg.BackgroundStyle) {
		cfg.BackgroundStyle = DefaultBackgroundStyle
	}
	// 不认识的星期、套用之后作息不合法的覆盖，都被丢弃，这一天的作息回到默认。
	cfg.WeekdayHours = CleanWeekdayHours(cfg.WeekdayHours, cfg.WorkHours)
	// 不是背景风格的键、超出范围的透明度，都被丢弃，那一种风格使用初始值。
	cfg.Transparency = CleanTransparency(cfg.Transparency)
	return cfg, nil
}

// Get 返回当前配置的副本。
func (s *Store) Get() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

// Update 在锁的保护下修改配置，然后写入磁盘。写入失败时，内存里的配置保持不变。
// fn 应该直接替换字段，而不是通过指针修改字段指向的内容。
func (s *Store) Update(fn func(*Config)) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	next := s.cfg
	fn(&next)
	if err := writeFile(s.path, next); err != nil {
		return err
	}
	s.cfg = next
	return nil
}

// writeFile 把配置写入 path。它先写临时文件再改名，保证不会留下写了一半的配置。
func writeFile(path string, cfg Config) error {
	body, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
