package service

import (
	"context"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cowtime/internal/config"
	"cowtime/internal/holiday"
	"cowtime/internal/salary"
	"cowtime/internal/wingeo"
	"cowtime/internal/workhours"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// fixedNow 是测试里默认使用的「当前时间」：2026 年 11 月 24 日（周二）中午。
const fixedNow = "2026-11-24 12:30:00"

// fullHD 是 1920×1080 屏幕、任务栏在底部时的工作区。
var fullHD = wingeo.Rect{X: 0, Y: 0, Width: 1920, Height: 1032}

// fakeWindow 记录服务对窗口做的操作，并允许测试手动触发窗口事件。
type fakeWindow struct {
	readyCB   func()
	movedCB   func(x, y int)
	pos       [2]int
	size      [2]int
	workAreas []wingeo.Rect
	shown     bool
}

func (f *fakeWindow) OnReady(cb func()) { f.readyCB = cb }

func (f *fakeWindow) OnMoved(cb func(x, y int)) { f.movedCB = cb }

func (f *fakeWindow) Position() (int, int) { return f.pos[0], f.pos[1] }

func (f *fakeWindow) SetPosition(x, y int) { f.pos = [2]int{x, y} }

func (f *fakeWindow) SetSize(width, height int) { f.size = [2]int{width, height} }

func (f *fakeWindow) Show() { f.shown = true }

func (f *fakeWindow) WorkAreas() []wingeo.Rect { return f.workAreas }

// fireReady 触发「页面就绪」。
func (f *fakeWindow) fireReady() { f.readyCB() }

// fireMoved 触发一次「窗口移动」。
func (f *fakeWindow) fireMoved(x, y int) {
	if f.movedCB != nil {
		f.movedCB(x, y)
	}
}

// atTime 把 "YYYY-MM-DD HH:MM:SS" 解析成本地时间。
func atTime(value string) time.Time {
	parsed, err := time.ParseInLocation("2006-01-02 15:04:05", value, time.Local)
	if err != nil {
		panic(err)
	}
	return parsed
}

// configured 返回一份设置了月薪的默认配置。
func configured(monthly float64) config.Config {
	cfg := config.Default()
	cfg.MonthlySalary = monthly
	return cfg
}

// newTestService 用给定的初始配置创建一个服务和它的假窗口。
// 日历里加载了内置的节假日数据，时间固定在 fixedNow。
func newTestService(t *testing.T, cfg config.Config) (*Service, *fakeWindow) {
	t.Helper()

	store, err := config.Load(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatalf("加载配置：%v", err)
	}
	if err := store.Update(func(c *config.Config) { *c = cfg }); err != nil {
		t.Fatalf("写入初始配置：%v", err)
	}

	cal := holiday.NewCalendar()
	if err := holiday.LoadEmbedded(cal); err != nil {
		t.Fatalf("加载内置的节假日数据：%v", err)
	}

	svc := New(store, cal, holiday.NewUpdater(filepath.Join(t.TempDir(), "holidays")), log.New(io.Discard, "", 0))
	svc.now = func() time.Time { return atTime(fixedNow) }
	svc.retryPause = time.Millisecond

	win := &fakeWindow{workAreas: []wingeo.Rect{fullHD}}
	AttachWindow(svc, win)
	return svc, win
}

// startService 模拟应用启动：Wails 会在这时注册「页面就绪」回调。
func startService(t *testing.T, svc *Service) {
	t.Helper()
	if err := svc.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup() 意外返回错误：%v", err)
	}
}

func TestFQNMatchesPackagePath(t *testing.T) {
	want := reflect.TypeOf(Service{}).PkgPath() + ".Service"
	if FQN != want {
		t.Errorf("FQN = %q，与实际的包路径 %q 不一致，前端会调用不到服务", FQN, want)
	}
}

func TestGetStatusAcceptanceExample(t *testing.T) {
	svc, _ := newTestService(t, configured(10000))
	got := svc.GetStatus()

	if !got.SalaryConfigured {
		t.Error("SalaryConfigured = false，期望 true")
	}
	// 2026 年 11 月有 21 个工作日。到 11 月 24 日 12:30 为止，本月已计薪 16 个完整工作日外加 3 小时。
	if want := 10000.0 * 471600 / 604800; math.Abs(got.MonthEarned-want) > 1e-6 {
		t.Errorf("本月已赚 = %v，期望 %v", got.MonthEarned, want)
	}
	if want := 10000.0 * 10800 / 604800; math.Abs(got.TodayEarned-want) > 1e-6 {
		t.Errorf("今日已赚 = %v，期望 %v", got.TodayEarned, want)
	}
	if got.State != salary.StateLunch {
		t.Errorf("State = %q，期望 %q", got.State, salary.StateLunch)
	}
	if got.DayOffReason != "" {
		t.Errorf("工作日的 DayOffReason = %q，期望为空", got.DayOffReason)
	}
	wantHoliday := holiday.Holiday{Kind: holiday.KindUpcoming, Name: "元旦", DaysLeft: 38}
	if got.Holiday != wantHoliday {
		t.Errorf("Holiday = %+v，期望 %+v", got.Holiday, wantHoliday)
	}
	wantSettings := Settings{MonthlySalary: 10000, WorkHours: workhours.Default(), BackgroundStyle: config.DefaultBackgroundStyle}
	if !reflect.DeepEqual(got.Settings, wantSettings) {
		t.Errorf("Settings = %+v，期望 %+v", got.Settings, wantSettings)
	}
}

func TestGetStatusOnWeekend(t *testing.T) {
	svc, _ := newTestService(t, configured(10000))
	svc.now = func() time.Time { return atTime("2026-11-28 10:00:00") } // 周六

	got := svc.GetStatus()
	if got.State != salary.StateDayOff || got.DayOffReason != "weekend" {
		t.Errorf("周六的状态是 (%q, %q)，期望 (dayOff, weekend)", got.State, got.DayOffReason)
	}
	if got.TodayEarned != 0 {
		t.Errorf("周六的今日已赚 = %v，期望 0", got.TodayEarned)
	}
	if want := 10000.0 * 576000 / 604800; math.Abs(got.MonthEarned-want) > 1e-6 {
		t.Errorf("周六的本月已赚 = %v，期望 %v", got.MonthEarned, want)
	}
}

func TestGetStatusOnHoliday(t *testing.T) {
	svc, _ := newTestService(t, configured(10000))
	svc.now = func() time.Time { return atTime("2026-10-01 10:00:00") } // 国庆节第一天，周四

	got := svc.GetStatus()
	if got.State != salary.StateDayOff || got.DayOffReason != "holiday" {
		t.Errorf("国庆节的状态是 (%q, %q)，期望 (dayOff, holiday)", got.State, got.DayOffReason)
	}
	if got.MonthEarned != 0 || got.TodayEarned != 0 {
		t.Errorf("国庆节第一天的已赚金额是 (%v, %v)，期望都是 0", got.MonthEarned, got.TodayEarned)
	}
	want := holiday.Holiday{Kind: holiday.KindOngoing, Name: "国庆节", DaysLeft: 7}
	if got.Holiday != want {
		t.Errorf("Holiday = %+v，期望 %+v", got.Holiday, want)
	}
}

func TestGetStatusWithoutSalary(t *testing.T) {
	svc, _ := newTestService(t, config.Default())
	got := svc.GetStatus()

	if got.SalaryConfigured {
		t.Error("还没有设置月薪时 SalaryConfigured 应该是 false")
	}
	if got.MonthEarned != 0 || got.TodayEarned != 0 {
		t.Errorf("还没有设置月薪时金额应该是 0，得到 (%v, %v)", got.MonthEarned, got.TodayEarned)
	}
	if got.Holiday.Kind != holiday.KindUpcoming {
		t.Errorf("还没有设置月薪时也应该显示假期，Kind = %q", got.Holiday.Kind)
	}
}

func TestSaveSettings(t *testing.T) {
	tests := []struct {
		name        string
		settings    Settings
		wantOK      bool
		wantMessage string
	}{
		{
			name: "合法的设置",
			settings: Settings{
				MonthlySalary:   15000,
				WorkHours:       workhours.Hours{Start: "08:30", LunchStart: "12:00", LunchEnd: "12:30", End: "17:30"},
				BackgroundStyle: config.StyleAurora,
			},
			wantOK: true,
		},
		{
			name: "没有午休",
			settings: Settings{
				MonthlySalary:   15000,
				WorkHours:       workhours.Hours{Start: "09:00", End: "17:00"},
				BackgroundStyle: config.StyleNeon,
			},
			wantOK: true,
		},
		{
			name: "素色背景",
			settings: Settings{
				MonthlySalary:   15000,
				WorkHours:       workhours.Default(),
				BackgroundStyle: config.StylePlain,
			},
			wantOK: true,
		},
		{
			name: "星空背景",
			settings: Settings{
				MonthlySalary:   15000,
				WorkHours:       workhours.Default(),
				BackgroundStyle: config.StyleStars,
			},
			wantOK: true,
		},
		{
			name: "月薪是 0",
			settings: Settings{
				MonthlySalary:   0,
				WorkHours:       workhours.Default(),
				BackgroundStyle: config.StyleAurora,
			},
			wantMessage: "月薪必须大于 0",
		},
		{
			name: "月薪是负数",
			settings: Settings{
				MonthlySalary:   -100,
				WorkHours:       workhours.Default(),
				BackgroundStyle: config.StyleAurora,
			},
			wantMessage: "月薪必须大于 0",
		},
		{
			name: "作息时间的顺序不对",
			settings: Settings{
				MonthlySalary:   10000,
				WorkHours:       workhours.Hours{Start: "09:00", LunchStart: "13:00", LunchEnd: "12:00", End: "18:00"},
				BackgroundStyle: config.StyleAurora,
			},
			wantMessage: "作息时间",
		},
		{
			name: "背景风格不合法",
			settings: Settings{
				MonthlySalary:   10000,
				WorkHours:       workhours.Default(),
				BackgroundStyle: "cyber",
			},
			wantMessage: "背景风格无效",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _ := newTestService(t, configured(10000))
			got := svc.SaveSettings(tt.settings)

			if got.OK != tt.wantOK {
				t.Fatalf("SaveSettings() 的 OK = %v，期望 %v（提示：%q）", got.OK, tt.wantOK, got.Message)
			}
			if !strings.Contains(got.Message, tt.wantMessage) {
				t.Errorf("提示 = %q，期望包含 %q", got.Message, tt.wantMessage)
			}

			saved := svc.store.Get()
			if tt.wantOK {
				if saved.MonthlySalary != tt.settings.MonthlySalary || saved.WorkHours != tt.settings.WorkHours {
					t.Errorf("保存的设置 = (%v, %+v)，期望 (%v, %+v)",
						saved.MonthlySalary, saved.WorkHours, tt.settings.MonthlySalary, tt.settings.WorkHours)
				}
				if saved.BackgroundStyle != tt.settings.BackgroundStyle {
					t.Errorf("保存的背景风格 = %q，期望 %q", saved.BackgroundStyle, tt.settings.BackgroundStyle)
				}
				return
			}
			if saved.MonthlySalary != 10000 || saved.WorkHours != workhours.Default() ||
				saved.BackgroundStyle != config.DefaultBackgroundStyle {
				t.Errorf("保存失败之后原来的设置不应该改变，得到 (%v, %+v, %q)",
					saved.MonthlySalary, saved.WorkHours, saved.BackgroundStyle)
			}
		})
	}
}

func TestServiceStartupRegistersReadyCallback(t *testing.T) {
	svc, win := newTestService(t, configured(10000))
	if win.readyCB != nil {
		t.Fatal("应用启动之前不应该有「页面就绪」回调")
	}
	if err := svc.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatalf("ServiceStartup() 意外返回错误：%v", err)
	}
	if win.readyCB == nil {
		t.Fatal("ServiceStartup() 应该注册「页面就绪」回调")
	}
}

func TestPlaceWindowUsesSavedPosition(t *testing.T) {
	cfg := configured(10000)
	cfg.Window = &config.WindowPos{X: 100, Y: 200}
	svc, win := newTestService(t, cfg)
	startService(t, svc)
	win.pos = [2]int{840, 450} // 窗口创建时默认居中

	win.fireReady()

	if win.pos != [2]int{100, 200} {
		t.Errorf("窗口位置 = %v，期望保存过的位置 [100 200]", win.pos)
	}
	if !win.shown {
		t.Error("窗口应该被显示出来")
	}
	if win.movedCB == nil {
		t.Error("窗口显示之后应该开始监听拖动")
	}
}

func TestPlaceWindowFallsBackToDefault(t *testing.T) {
	tests := []struct {
		name  string
		saved *config.WindowPos
	}{
		{name: "还没有保存过位置", saved: nil},
		{name: "保存的位置在屏幕外（换过显示器或分辨率）", saved: &config.WindowPos{X: 5000, Y: 5000}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := configured(10000)
			cfg.Window = tt.saved
			svc, win := newTestService(t, cfg)
			startService(t, svc)

			win.fireReady()

			if win.pos != [2]int{1664, 884} {
				t.Errorf("窗口位置 = %v，期望默认位置 [1664 884]", win.pos)
			}
		})
	}
}

func TestPlaceWindowKeepsPositionWithoutScreens(t *testing.T) {
	svc, win := newTestService(t, configured(10000))
	startService(t, svc)
	win.workAreas = nil
	win.pos = [2]int{300, 400}

	win.fireReady()

	if win.pos != [2]int{300, 400} {
		t.Errorf("拿不到屏幕信息时窗口位置 = %v，期望保持 [300 400]", win.pos)
	}
}

func TestMovesAreSavedOnlyAfterWindowIsReady(t *testing.T) {
	svc, win := newTestService(t, configured(10000))
	startService(t, svc)
	if win.movedCB != nil {
		t.Fatal("页面就绪之前不应该监听拖动，否则窗口创建时的位置会被存下来")
	}

	win.fireReady()
	svc.ignoreMovesUntil = time.Time{} // 结束摆放窗口之后的忽略期
	win.fireMoved(300, 400)

	saved := svc.store.Get().Window
	if saved == nil || *saved != (config.WindowPos{X: 300, Y: 400}) {
		t.Errorf("拖动之后保存的位置 = %+v，期望 {300 400}", saved)
	}
}

func TestMovesRightAfterPlacementAreIgnored(t *testing.T) {
	svc, win := newTestService(t, configured(10000))
	startService(t, svc)
	win.fireReady()

	// 窗口创建时的位置和刚才摆放的位置都会产生移动事件。
	win.fireMoved(840, 450)
	win.fireMoved(1664, 884)

	if svc.store.Get().Window != nil {
		t.Error("程序自己摆放窗口产生的移动事件不应该被保存")
	}
}

func TestMovesAreIgnoredWhileSettingsAreOpen(t *testing.T) {
	savedPos := config.WindowPos{X: 1664, Y: 884}
	cfg := configured(10000)
	cfg.Window = &savedPos
	svc, win := newTestService(t, cfg)
	startService(t, svc)
	win.fireReady()
	svc.ignoreMovesUntil = time.Time{}

	svc.EnterSettings() // 窗口为了放得下设置视图被向上移动
	win.fireMoved(1664, 732)
	if got := svc.store.Get().Window; got == nil || *got != savedPos {
		t.Errorf("设置视图打开期间保存的位置 = %+v，期望保持 %+v", got, savedPos)
	}

	svc.LeaveSettings()
	svc.ignoreMovesUntil = time.Time{}
	win.fireMoved(300, 400)
	if got := svc.store.Get().Window; got == nil || *got != (config.WindowPos{X: 300, Y: 400}) {
		t.Errorf("关闭设置视图之后拖动，保存的位置 = %+v，期望 {300 400}", got)
	}
}

func TestEnterSettingsMovesWindowUpWhenNeeded(t *testing.T) {
	cfg := configured(10000)
	cfg.Window = &config.WindowPos{X: 1664, Y: 884} // 贴着屏幕底部
	svc, win := newTestService(t, cfg)
	startService(t, svc)
	win.fireReady()

	svc.EnterSettings()
	if want := [2]int{wingeo.SettingsWidth, wingeo.SettingsHeight}; win.size != want {
		t.Errorf("设置视图的窗口大小 = %v，期望 %v", win.size, want)
	}
	if want := [2]int{1664, 1032 - wingeo.SettingsHeight}; win.pos != want {
		t.Errorf("设置视图的窗口位置 = %v，期望向上移到 %v", win.pos, want)
	}

	svc.LeaveSettings()
	if want := [2]int{wingeo.DisplayWidth, wingeo.DisplayHeight}; win.size != want {
		t.Errorf("回到显示视图之后的窗口大小 = %v，期望 %v", win.size, want)
	}
	if win.pos != [2]int{1664, 884} {
		t.Errorf("回到显示视图之后的窗口位置 = %v，期望恢复为 [1664 884]", win.pos)
	}
}

func TestEnterSettingsKeepsPositionWhenThereIsRoom(t *testing.T) {
	cfg := configured(10000)
	cfg.Window = &config.WindowPos{X: 100, Y: 100}
	svc, win := newTestService(t, cfg)
	startService(t, svc)
	win.fireReady()

	svc.EnterSettings()
	if win.pos != [2]int{100, 100} {
		t.Errorf("放得下时窗口位置 = %v，期望保持 [100 100]", win.pos)
	}

	svc.LeaveSettings()
	if want := [2]int{wingeo.DisplayWidth, wingeo.DisplayHeight}; win.size != want {
		t.Errorf("回到显示视图之后的窗口大小 = %v，期望 %v", win.size, want)
	}
	if win.pos != [2]int{100, 100} {
		t.Errorf("回到显示视图之后的窗口位置 = %v，期望保持 [100 100]", win.pos)
	}
}

func TestSettingsViewIsIdempotent(t *testing.T) {
	svc, win := newTestService(t, configured(10000))
	startService(t, svc)
	win.fireReady()

	svc.EnterSettings()
	svc.EnterSettings()
	svc.LeaveSettings()

	if want := [2]int{wingeo.DisplayWidth, wingeo.DisplayHeight}; win.size != want {
		t.Errorf("重复进入设置视图再返回之后，窗口大小 = %v，期望 %v", win.size, want)
	}
}

// serveEmbeddedYear 启动一个测试服务器，它用内置数据回应任何年份的请求，并统计请求次数。
func serveEmbeddedYear(t *testing.T, year int) (url func(int) string, hits *atomic.Int32) {
	t.Helper()

	body, err := os.ReadFile(filepath.Join("..", "holiday", "data", fmt.Sprintf("%d.json", year)))
	if err != nil {
		t.Fatalf("读取 %d 年的内置数据：%v", year, err)
	}
	hits = &atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)

	return func(int) string { return server.URL }, hits
}

// newRefreshService 创建一个不加载内置数据的服务，时间固定在 fixedNow。
func newRefreshService(t *testing.T) *Service {
	t.Helper()

	store, err := config.Load(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatalf("加载配置：%v", err)
	}
	dir := filepath.Join(t.TempDir(), "holidays")
	svc := New(store, holiday.NewCalendar(), holiday.NewUpdater(dir), log.New(io.Discard, "", 0))
	svc.now = func() time.Time { return atTime(fixedNow) }
	svc.retryPause = time.Millisecond
	return svc
}

func TestRefreshHolidaysFetchesMissingYear(t *testing.T) {
	svc := newRefreshService(t)
	url, hits := serveEmbeddedYear(t, 2027)
	svc.updater.URL = url

	svc.refreshHolidays(context.Background())

	if hits.Load() != 1 {
		t.Errorf("缺少 2027 年的数据时应该下载一次，实际请求了 %d 次", hits.Load())
	}
	if !svc.cal.HasYear(2027) {
		t.Error("下载成功之后日历里应该有 2027 年的数据")
	}
	if _, err := os.Stat(filepath.Join(svc.updater.Dir, "2027.json")); err != nil {
		t.Errorf("下载成功之后应该写入缓存文件：%v", err)
	}
	if got := svc.store.Get().HolidayCheckAt; got == nil || !got.Equal(atTime(fixedNow)) {
		t.Errorf("下载之后记录的尝试时间 = %v，期望 %v", got, fixedNow)
	}
}

func TestRefreshHolidaysSkipsWhenYearIsKnown(t *testing.T) {
	svc := newRefreshService(t)
	url, hits := serveEmbeddedYear(t, 2027)
	svc.updater.URL = url
	known := holiday.YearData{Year: 2027, Days: []holiday.Day{{Name: "元旦", Date: "2027-01-01", IsOffDay: true}}}
	if err := svc.cal.AddYear(known); err != nil {
		t.Fatalf("AddYear() 意外返回错误：%v", err)
	}

	svc.refreshHolidays(context.Background())

	if hits.Load() != 0 {
		t.Errorf("已经有明年的数据时不应该联网，实际请求了 %d 次", hits.Load())
	}
	if svc.store.Get().HolidayCheckAt != nil {
		t.Error("没有尝试更新时不应该记录尝试时间")
	}
}

func TestRefreshHolidaysSkipsRecentAttempt(t *testing.T) {
	svc := newRefreshService(t)
	url, hits := serveEmbeddedYear(t, 2027)
	svc.updater.URL = url
	lastAttempt := atTime(fixedNow).Add(-2 * time.Hour)
	if err := svc.store.Update(func(c *config.Config) { c.HolidayCheckAt = &lastAttempt }); err != nil {
		t.Fatalf("写入配置：%v", err)
	}

	svc.refreshHolidays(context.Background())

	if hits.Load() != 0 {
		t.Errorf("距离上次尝试不到 24 小时不应该联网，实际请求了 %d 次", hits.Load())
	}
}

func TestRefreshHolidaysGivesUpAfterThreeAttempts(t *testing.T) {
	svc := newRefreshService(t)
	hits := &atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)
	svc.updater.URL = func(int) string { return server.URL }

	svc.refreshHolidays(context.Background())
	svc.refreshHolidays(context.Background())

	if hits.Load() != fetchAttempts {
		t.Errorf("一次刷新最多尝试 %d 次，失败后 24 小时内不再刷新；实际请求了 %d 次", fetchAttempts, hits.Load())
	}
	if svc.cal.HasYear(2027) {
		t.Error("下载失败时不应该有 2027 年的数据")
	}
}

func TestRefreshHolidaysRetriesTransientFailures(t *testing.T) {
	svc := newRefreshService(t)
	body, err := os.ReadFile(filepath.Join("..", "holiday", "data", "2027.json"))
	if err != nil {
		t.Fatalf("读取 2027 年的内置数据：%v", err)
	}

	// 前两次请求失败，模拟新建连接时偶发的超时；第三次成功。
	hits := &atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) <= 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(server.Close)
	svc.updater.URL = func(int) string { return server.URL }

	svc.refreshHolidays(context.Background())

	if hits.Load() != 3 {
		t.Errorf("前两次失败之后应该重试到成功，实际请求了 %d 次", hits.Load())
	}
	if !svc.cal.HasYear(2027) {
		t.Error("重试成功之后日历里应该有 2027 年的数据")
	}
}
