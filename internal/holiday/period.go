package holiday

import (
	"math"
	"slices"
	"sort"
	"strings"
	"time"
)

// 「最近的假期」的三种状态。
const (
	KindUpcoming = "upcoming" // 还没开始
	KindOngoing  = "ongoing"  // 正在假期中
	KindUnknown  = "unknown"  // 找不到下一个假期，通常是因为数据还没有发布
)

// Unknown 表示找不到下一个假期。
var Unknown = Holiday{Kind: KindUnknown}

// Holiday 是「最近的假期」在某一时刻的状态。
type Holiday struct {
	Kind     string `json:"kind"`
	Name     string `json:"name,omitempty"`     // 假期段的名字
	DaysLeft int    `json:"daysLeft,omitempty"` // 距离开启还剩的天数，或者假期还剩的天数
}

// period 是日历上连续的放假日合并而成的一个假期段。
type period struct {
	first time.Time
	last  time.Time
	names []string // 段内出现过的节日名，按出现的先后顺序排列
}

// Next 返回结束日期不早于 today 的第一个假期段；找不到时返回 Unknown 和 false。
func (c *Calendar) Next(today time.Time) (Holiday, bool) {
	today = Midnight(today)
	for _, p := range c.periods() {
		if p.last.Before(today) {
			continue
		}
		name := strings.Join(p.names, "、")
		if today.Before(p.first) {
			return Holiday{Kind: KindUpcoming, Name: name, DaysLeft: daysBetween(today, p.first)}, true
		}
		return Holiday{Kind: KindOngoing, Name: name, DaysLeft: daysBetween(today, p.last) + 1}, true
	}
	return Unknown, false
}

// periods 把日历上连续的放假日合并成假期段，按时间先后排列。
func (c *Calendar) periods() []period {
	type offDay struct {
		date time.Time
		name string
	}

	c.mu.RLock()
	days := make([]offDay, 0, len(c.days))
	for _, d := range c.days {
		if !d.IsOffDay {
			continue
		}
		if date, err := ParseDate(d.Date); err == nil {
			days = append(days, offDay{date: date, name: d.Name})
		}
	}
	c.mu.RUnlock()

	sort.Slice(days, func(i, j int) bool { return days[i].date.Before(days[j].date) })

	var result []period
	for _, d := range days {
		n := len(result)
		if n > 0 && result[n-1].last.AddDate(0, 0, 1).Equal(d.date) {
			result[n-1].last = d.date
			if !slices.Contains(result[n-1].names, d.name) {
				result[n-1].names = append(result[n-1].names, d.name)
			}
			continue
		}
		result = append(result, period{first: d.date, last: d.date, names: []string{d.name}})
	}
	return result
}

// daysBetween 返回从 a 到 b 相隔的自然日数。
func daysBetween(a, b time.Time) int {
	return int(math.Round(b.Sub(a).Hours() / 24))
}
