// Command cow-time 是一个 Windows 桌面悬浮窗，
// 实时显示本月已赚、今日已赚和距离最近一个法定节假日的天数。
package main

import (
	"embed"
	"io"
	"io/fs"
	"log"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"cowtime/internal/config"
	"cowtime/internal/holiday"
	"cowtime/internal/service"
	"cowtime/internal/wingeo"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:frontend
var frontend embed.FS

const (
	appName    = "Cow Time"
	windowName = "main"
	// singleInstanceID 用来识别同一个程序的另一个实例，取值需要全局唯一。
	singleInstanceID = "cowtime-5e2a7c41-9b3d-4f6a-8e1c-2d7b0a9f4c63"
)

func main() {
	logOut, err := openLog()
	if err != nil {
		// 日志文件打不开不影响使用，输出退回到标准错误。
		logOut = os.Stderr
	}
	logger := log.New(logOut, "[cow-time] ", log.LstdFlags|log.Lmsgprefix)

	configDir, err := os.UserConfigDir()
	if err != nil {
		logger.Fatalf("找不到配置目录：%v", err)
	}
	appDir := filepath.Join(configDir, "cow-time")
	holidayDir := filepath.Join(appDir, "holidays")

	store, err := config.Load(filepath.Join(appDir, "config.json"))
	if err != nil {
		logger.Fatalf("读取配置文件失败：%v", err)
	}

	cal := holiday.NewCalendar()
	if err := holiday.LoadEmbedded(cal); err != nil {
		logger.Fatalf("加载内置的节假日数据失败：%v", err)
	}
	if err := holiday.LoadCache(cal, holidayDir); err != nil {
		logger.Printf("读取节假日数据的缓存失败：%v", err)
	}

	svc := service.New(store, cal, holiday.NewUpdater(holidayDir), logger)

	app := application.New(application.Options{
		Name:     appName,
		Services: []application.Service{application.NewService(svc)},
		Logger:   slog.New(slog.NewTextHandler(logOut, nil)),
		Assets: application.AssetOptions{
			Handler:        http.FileServer(http.FS(assets())),
			DisableLogging: true,
		},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID:               singleInstanceID,
			OnSecondInstanceLaunch: func(application.SecondInstanceData) { activateExistingWindow() },
		},
	})

	// 屏幕信息要到 app.Run 之后才可用，所以窗口先以隐藏状态创建。
	// 页面加载完成之后，service 会把它摆到保存的位置或默认位置，再显示出来。
	win := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:          windowName,
		Title:         appName,
		Width:         wingeo.DisplayWidth,
		Height:        wingeo.DisplayHeight,
		Frameless:     true,
		AlwaysOnTop:   true,
		DisableResize: true,
		Hidden:        true,
		// 窗口使用逐像素透明：卡片以外的区域完全透明，圆角由页面决定。
		// 不用系统的磨砂背景，因为它铺满整个矩形窗口，会在页面的圆角外面露出一圈长方形。
		// 系统的窗口装饰也要关掉，否则系统会自己再画一圈圆角和边线。
		BackgroundType: application.BackgroundTypeTransparent,
		Windows: application.WindowsWindow{
			HiddenOnTaskbar:                   true,
			DisableFramelessWindowDecorations: true,
			WindowDidMoveDebounceMS:           300,
		},
	})
	service.AttachWindow(svc, &wailsWindow{win: win, app: app})

	if err := app.Run(); err != nil {
		logger.Fatalf("程序异常退出：%v", err)
	}
}

// openLog 打开日志文件，返回一个同时写入日志文件和标准错误的输出。
// 日志文件是 %APPDATA%\cow-time\cow-time.log。
func openLog() (io.Writer, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(configDir, "cow-time", "cow-time.log")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	return io.MultiWriter(file, os.Stderr), nil
}

// assets 返回前端静态文件的根目录。
func assets() fs.FS {
	sub, err := fs.Sub(frontend, "frontend")
	if err != nil {
		log.Fatal(err)
	}
	return sub
}

// activateExistingWindow 在程序已经在运行时被再次启动调用：
// 把已有的悬浮窗恢复、显示并置于前面。
func activateExistingWindow() {
	win, ok := application.Get().Window.Get(windowName)
	if !ok {
		return
	}
	win.Restore()
	win.Show()
	win.Focus()
}

// wailsWindow 用 Wails 的窗口实现 service.Window。
type wailsWindow struct {
	win *application.WebviewWindow
	app *application.App
}

func (w *wailsWindow) OnReady(cb func()) {
	w.win.OnWindowEvent(events.Common.WindowRuntimeReady, func(*application.WindowEvent) { cb() })
}

func (w *wailsWindow) OnMoved(cb func(x, y int)) {
	w.win.OnWindowEvent(events.Common.WindowDidMove, func(*application.WindowEvent) { cb(w.win.Position()) })
}

func (w *wailsWindow) Position() (int, int) {
	return w.win.Position()
}

func (w *wailsWindow) SetPosition(x, y int) {
	w.win.SetPosition(x, y)
}

func (w *wailsWindow) SetSize(width, height int) {
	w.win.SetSize(width, height)
}

func (w *wailsWindow) Show() {
	w.win.Show()
}

// WorkAreas 返回所有屏幕的工作区，主屏幕排在最前面。
func (w *wailsWindow) WorkAreas() []wingeo.Rect {
	screens := w.app.Screen.GetAll()
	primary := w.app.Screen.GetPrimary()
	result := make([]wingeo.Rect, 0, len(screens))
	if primary != nil {
		result = append(result, toRect(primary.WorkArea))
	}
	for _, screen := range screens {
		if screen != primary {
			result = append(result, toRect(screen.WorkArea))
		}
	}
	return result
}

// toRect 把 Wails 的矩形换算成 wingeo 的矩形。
func toRect(r application.Rect) wingeo.Rect {
	return wingeo.Rect{X: r.X, Y: r.Y, Width: r.Width, Height: r.Height}
}
