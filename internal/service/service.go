// Package service 把程序的功能暴露给前端，并管理悬浮窗的位置和尺寸。
package service

import (
	"context"
	"log"
	"maps"
	"sync"
	"time"

	"cowtime/internal/config"
	"cowtime/internal/holiday"
	"cowtime/internal/salary"
	"cowtime/internal/wingeo"
	"cowtime/internal/workhours"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// FQN 是本服务在 Wails 绑定表里的全名，格式是「包路径.类型名」。
// 前端通过 Call.ByName(FQN + ".方法名") 调用这里的方法。
const FQN = "cowtime/internal/service.Service"

const (
	// holidayRefreshInterval 是两次节假日数据更新尝试之间的最短间隔。
	holidayRefreshInterval = 24 * time.Hour
	// moveGracePeriod 是程序自己移动窗口之后忽略移动事件的时长。
	// WindowDidMove 事件在窗口停止移动约 300 毫秒之后才触发，所以它要比 300 毫秒长。
	moveGracePeriod = time.Second
	// fetchAttempts 是一次更新里最多尝试下载的次数。
	// 实测新建连接偶尔会在 TLS 握手阶段停顿，重试一次通常就成功。
	fetchAttempts = 3
)

// Window 是服务需要的窗口操作，由 main 用 Wails 窗口实现并注入。
type Window interface {
	// OnReady 在页面加载完成、可以安全摆放窗口之后调用 cb。
	OnReady(cb func())
	// OnMoved 在窗口移动停止之后调用 cb。
	OnMoved(cb func(x, y int))
	Position() (x, y int)
	SetPosition(x, y int)
	SetSize(width, height int)
	Show()
	// WorkAreas 返回所有屏幕的工作区，主屏幕排在最前面。
	WorkAreas() []wingeo.Rect
}

// Settings 是设置视图里填写的内容。
type Settings struct {
	MonthlySalary   float64                      `json:"monthlySalary"`
	WorkHours       workhours.Hours              `json:"workHours"`              // 默认作息
	WeekdayHours    map[string]workhours.Partial `json:"weekdayHours,omitempty"` // 某些星期与默认作息不同的地方，键是 "1" 到 "7"
	BackgroundStyle string                       `json:"backgroundStyle"`
}

// SaveResult 是保存设置的结果。失败时 Message 是给用户看的原因。
type SaveResult struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

// Status 是前端每一次轮询拿到的全部显示数据。
type Status struct {
	SalaryConfigured bool            `json:"salaryConfigured"` // 是否已经设置月薪
	MonthEarned      float64         `json:"monthEarned"`      // 本月已赚，单位是元
	TodayEarned      float64         `json:"todayEarned"`      // 今日已赚，单位是元
	State            salary.State    `json:"state"`
	DayOffReason     string          `json:"dayOffReason,omitempty"` // 仅 State 为 dayOff 时有值
	Holiday          holiday.Holiday `json:"holiday"`
	Settings         Settings        `json:"settings"`     // 当前生效的设置，设置视图用它填充表单
	Transparency     map[string]int  `json:"transparency"` // 四种背景风格各自的透明度，单位是百分比
}

// Service 实现程序的全部功能。
type Service struct {
	store   *config.Store
	cal     *holiday.Calendar
	updater *holiday.Updater
	log     *log.Logger
	now     func() time.Time
	win     Window

	retryPause       time.Duration // 两次下载尝试之间的间隔
	mu               sync.Mutex
	inSettings       bool        // 当前是否处于设置视图
	displayPos       wingeo.Rect // 进入设置视图之前，显示视图所在的位置
	settingsMoved    bool        // 进入设置视图时，窗口是否被向上移动过
	ignoreMovesUntil time.Time   // 在这之前到达的移动事件不保存
}

// New 创建一个服务。窗口通过 AttachWindow 注入。
func New(store *config.Store, cal *holiday.Calendar, updater *holiday.Updater, logger *log.Logger) *Service {
	return &Service{store: store, cal: cal, updater: updater, log: logger, now: time.Now, retryPause: time.Second}
}

// AttachWindow 注入窗口操作。它需要在应用启动之前调用。
func AttachWindow(s *Service, w Window) {
	s.win = w
}

// ServiceStartup 在应用启动时被 Wails 调用：
// 注册窗口就绪之后的处理，并在后台尝试更新节假日数据。
func (s *Service) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	s.win.OnReady(s.placeWindow)
	go s.refreshHolidays(ctx)
	return nil
}

// GetStatus 返回当前时刻的全部显示数据。前端每秒调用一次。
func (s *Service) GetStatus() Status {
	now := s.now()
	cfg := s.store.Get()

	// 计薪状态按照今天的作息判断。
	status := Status{
		SalaryConfigured: cfg.MonthlySalary > 0,
		State:            salary.StateOf(s.cal, cfg.ScheduleOn(now), now),
		Settings: Settings{
			MonthlySalary:   cfg.MonthlySalary,
			WorkHours:       cfg.WorkHours,
			WeekdayHours:    cfg.WeekdayHours,
			BackgroundStyle: cfg.BackgroundStyle,
		},
	}
	status.Transparency = make(map[string]int, len(config.BackgroundStyles))
	for _, style := range config.BackgroundStyles {
		status.Transparency[style] = cfg.TransparencyFor(style)
	}
	if status.State == salary.StateDayOff {
		status.DayOffReason = s.cal.DayOffKind(now)
	}
	if status.SalaryConfigured {
		earned := salary.Earned(s.cal, cfg.MonthlySalary, cfg.ScheduleOn, now)
		status.MonthEarned, status.TodayEarned = earned.Month, earned.Today
	}
	status.Holiday, _ = s.cal.Next(now)
	return status
}

// SaveSettings 校验并保存设置。
func (s *Service) SaveSettings(settings Settings) SaveResult {
	if settings.MonthlySalary <= 0 {
		return SaveResult{Message: "月薪必须大于 0"}
	}
	if _, err := settings.WorkHours.Parse(); err != nil {
		return SaveResult{Message: "作息时间需要满足：上班时间 < 午休开始 < 午休结束 < 下班时间"}
	}
	// 每一天的生效作息都要合法。
	candidate := config.Config{WorkHours: settings.WorkHours, WeekdayHours: settings.WeekdayHours}
	for _, key := range config.WeekdayKeys {
		day, _ := config.ParseWeekdayKey(key)
		if _, err := candidate.HoursOn(day); err != nil {
			return SaveResult{Message: config.WeekdayName(key) + "的作息时间需要满足：上班时间 < 午休开始 < 午休结束 < 下班时间"}
		}
	}

	style := settings.BackgroundStyle
	if !config.ValidBackgroundStyle(style) {
		return SaveResult{Message: "背景风格无效"}
	}

	monthly, hours := settings.MonthlySalary, settings.WorkHours
	// 写入之前统一清理一次，丢弃不认识的星期。
	weekday := config.CleanWeekdayHours(settings.WeekdayHours, settings.WorkHours)
	err := s.store.Update(func(c *config.Config) {
		c.MonthlySalary = monthly
		c.WorkHours = hours
		c.WeekdayHours = weekday
		c.BackgroundStyle = style
	})
	if err != nil {
		s.log.Printf("保存设置失败：%v", err)
		return SaveResult{Message: "保存失败，详情见日志文件"}
	}
	return SaveResult{OK: true}
}

// SetTransparency 立即保存某种背景风格的透明度，单位是百分比。前端在滑块拖动时调用。
// 调回初始值时删除记录。
func (s *Service) SetTransparency(style string, value int) SaveResult {
	if !config.ValidBackgroundStyle(style) {
		return SaveResult{Message: "背景风格无效"}
	}
	if value < 0 || value > config.MaxTransparency {
		return SaveResult{Message: "透明度需要在 0% 到 100% 之间"}
	}

	err := s.store.Update(func(c *config.Config) {
		next := maps.Clone(c.Transparency)
		if next == nil {
			next = make(map[string]int)
		}
		if value == config.DefaultTransparency(style) {
			delete(next, style)
		} else {
			next[style] = value
		}
		c.Transparency = next
	})
	if err != nil {
		s.log.Printf("保存透明度失败：%v", err)
		return SaveResult{Message: "保存失败，详情见日志文件"}
	}
	return SaveResult{OK: true}
}

// EnterSettings 把窗口切换成设置视图：窗口变高，放不下时向上移动。
func (s *Service) EnterSettings() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.inSettings {
		return
	}
	s.inSettings = true

	x, y := s.win.Position()
	s.displayPos = wingeo.Rect{X: x, Y: y, Width: wingeo.DisplayWidth, Height: wingeo.DisplayHeight}
	s.settingsMoved = false

	s.win.SetSize(wingeo.SettingsWidth, wingeo.SettingsHeight)
	area, ok := wingeo.CurrentWorkArea(s.displayPos, s.win.WorkAreas())
	if !ok {
		return
	}
	newX, newY := wingeo.FitUpward(s.displayPos, area, wingeo.SettingsHeight)
	if newY != y {
		s.win.SetPosition(newX, newY)
		s.settingsMoved = true
	}
}

// LeaveSettings 把窗口切回显示视图，恢复原来的位置和大小。
func (s *Service) LeaveSettings() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.inSettings {
		return
	}
	s.win.SetSize(wingeo.DisplayWidth, wingeo.DisplayHeight)
	if s.settingsMoved {
		s.win.SetPosition(s.displayPos.X, s.displayPos.Y)
	}
	s.inSettings = false
	// 恢复位置也会触发移动事件，在它到达之前不要保存。
	s.ignoreMovesUntil = time.Now().Add(moveGracePeriod)
}

// placeWindow 在页面就绪之后决定窗口的位置、显示窗口，并开始监听用户的拖动。
func (s *Service) placeWindow() {
	s.mu.Lock()
	defer s.mu.Unlock()

	x, y := s.initialPosition()
	s.win.SetPosition(x, y)
	s.win.Show()
	s.win.OnMoved(s.savePosition)
	// 窗口创建时和刚才的摆放各会产生一次移动事件，在它们到达之前不要保存。
	s.ignoreMovesUntil = time.Now().Add(moveGracePeriod)
}

// initialPosition 返回窗口的初始位置：保存过并且仍然可见的位置，否则是默认位置。
func (s *Service) initialPosition() (x, y int) {
	workAreas := s.win.WorkAreas()
	if saved := s.store.Get().Window; saved != nil {
		rect := wingeo.Rect{X: saved.X, Y: saved.Y, Width: wingeo.DisplayWidth, Height: wingeo.DisplayHeight}
		if wingeo.IsVisible(rect, workAreas) {
			return saved.X, saved.Y
		}
	}
	if len(workAreas) == 0 {
		// 拿不到屏幕信息时保持窗口现在的位置。
		return s.win.Position()
	}
	return wingeo.DefaultPosition(wingeo.DisplayWidth, wingeo.DisplayHeight, workAreas[0])
}

// savePosition 在窗口移动之后把新位置写入配置。
func (s *Service) savePosition(x, y int) {
	s.mu.Lock()
	skip := s.inSettings || time.Now().Before(s.ignoreMovesUntil)
	s.mu.Unlock()
	if skip {
		return
	}

	saved := config.WindowPos{X: x, Y: y}
	if err := s.store.Update(func(c *config.Config) { c.Window = &saved }); err != nil {
		s.log.Printf("保存窗口位置失败：%v", err)
	}
}

// refreshHolidays 在还没有明年的数据时，尝试从远端下载一次。
func (s *Service) refreshHolidays(ctx context.Context) {
	now := s.now()
	year := now.Year() + 1
	if s.cal.HasYear(year) {
		return
	}
	cfg := s.store.Get()
	if cfg.HolidayCheckAt != nil && now.Sub(*cfg.HolidayCheckAt) < holidayRefreshInterval {
		return
	}

	// 无论成败都记下这次尝试，避免每次启动都联网。
	attemptedAt := now
	if err := s.store.Update(func(c *config.Config) { c.HolidayCheckAt = &attemptedAt }); err != nil {
		s.log.Printf("记录节假日数据的更新时间失败：%v", err)
		return
	}

	data, err := s.fetchWithRetry(ctx, year)
	if err != nil {
		s.log.Printf("更新 %d 年的节假日数据失败：%v", year, err)
		return
	}
	if err := s.updater.Cache(data); err != nil {
		s.log.Printf("缓存 %d 年的节假日数据失败：%v", year, err)
		return
	}
	if err := s.cal.AddYear(data); err != nil {
		s.log.Printf("加入 %d 年的节假日数据失败：%v", year, err)
		return
	}
	s.log.Printf("已更新 %d 年的节假日数据", year)
}

// fetchWithRetry 下载某一年的数据：失败时隔 s.retryPause 重试，最多尝试 fetchAttempts 次。
// ctx 被取消（程序退出）时立即停止。
func (s *Service) fetchWithRetry(ctx context.Context, year int) (holiday.YearData, error) {
	var lastErr error
	for attempt := 1; attempt <= fetchAttempts; attempt++ {
		data, err := s.updater.Fetch(ctx, year)
		if err == nil {
			return data, nil
		}
		lastErr = err
		if attempt == fetchAttempts {
			break
		}
		select {
		case <-ctx.Done():
			return holiday.YearData{}, ctx.Err()
		case <-time.After(s.retryPause):
		}
	}
	return holiday.YearData{}, lastErr
}
