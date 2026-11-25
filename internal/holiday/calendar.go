// Package holiday 维护中国法定节假日数据，并据此判断工作日、计算假期倒计时。
package holiday

import (
	"fmt"
	"strconv"
	"sync"
	"time"
)

const dateLayout = "2006-01-02"

// Day 是节假日数据里的一条记录。
type Day struct {
	Name     string `json:"name"`
	Date     string `json:"date"` // YYYY-MM-DD
	IsOffDay bool   `json:"isOffDay"`
}

// YearData 是某一年的节假日数据，与 holiday-cn 的年度 JSON 文件格式一致。
type YearData struct {
	Year   int      `json:"year"`
	Papers []string `json:"papers"`
	Days   []Day    `json:"days"`
}

// Calendar 保存已知的节假日记录，并回答与工作日、假期有关的问题。
// 它可以被多个 goroutine 同时使用。
type Calendar struct {
	mu    sync.RWMutex
	days  map[string]Day // 键是 "YYYY-MM-DD"
	years map[int]bool
}

// NewCalendar 返回一个不含任何数据的日历。
func NewCalendar() *Calendar {
	return &Calendar{days: make(map[string]Day), years: make(map[int]bool)}
}

// Validate 校验某一年的数据是否自洽：
// 至少有一条记录，每条记录都要有节日名，日期必须是当年合法的 YYYY-MM-DD。
func Validate(data YearData) error {
	if len(data.Days) == 0 {
		return fmt.Errorf("%d 年的节假日数据为空", data.Year)
	}
	for _, d := range data.Days {
		if d.Name == "" {
			return fmt.Errorf("节假日数据里有记录缺少节日名：%s", d.Date)
		}
		t, err := ParseDate(d.Date)
		if err != nil || t.Year() != data.Year {
			return fmt.Errorf("节假日数据里有记录的日期不合法：%q", d.Date)
		}
	}
	return nil
}

// AddYear 校验并加入某一年的数据。同一年重复加入时，新数据替换旧数据。
func (c *Calendar) AddYear(data YearData) error {
	if err := Validate(data); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for date := range c.days {
		if yearOf(date) == data.Year {
			delete(c.days, date)
		}
	}
	for _, d := range data.Days {
		c.days[d.Date] = d
	}
	c.years[data.Year] = true
	return nil
}

// HasYear 报告日历里是否已有某一年的数据。
func (c *Calendar) HasYear(year int) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.years[year]
}

// lookup 返回某一天的记录；这一天没有记录时返回 false。
func (c *Calendar) lookup(t time.Time) (Day, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	day, ok := c.days[t.Format(dateLayout)]
	return day, ok
}

// IsWorkday 报告某一天是否工作日：
// 周一到周五且不是法定节假日，或者是调休上班日。
func (c *Calendar) IsWorkday(t time.Time) bool {
	if day, ok := c.lookup(t); ok {
		return !day.IsOffDay
	}
	return !isWeekend(t)
}

// DayOffKind 返回非工作日的类型："holiday" 表示法定节假日，"weekend" 表示普通周末。
// 只对非工作日调用它。
func (c *Calendar) DayOffKind(t time.Time) string {
	if day, ok := c.lookup(t); ok && day.IsOffDay {
		return "holiday"
	}
	return "weekend"
}

// ParseDate 把 "YYYY-MM-DD" 解析成本地时间的当天零点。
func ParseDate(s string) (time.Time, error) {
	return time.ParseInLocation(dateLayout, s, time.Local)
}

// Midnight 返回 t 所在日期的本地零点。
func Midnight(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func isWeekend(t time.Time) bool {
	weekday := t.Weekday()
	return weekday == time.Saturday || weekday == time.Sunday
}

// yearOf 从 "YYYY-MM-DD" 里取出年份。调用方需要保证格式正确。
func yearOf(date string) int {
	year, _ := strconv.Atoi(date[:4])
	return year
}
