# Cow Time

Windows 桌面上的一个小悬浮窗：实时显示本月已经赚到多少钱、今天已经赚到多少钱，以及距离最近的法定节假日还有多少天。

用 Go 和 [Wails v3](https://github.com/wailsapp/wails) 写成，编译结果是一个约 11.5 MB 的 exe，不需要安装。

| 显示视图 | 设置视图 |
| :---: | :---: |
| ![显示视图](docs/screenshots/display.png) | ![设置视图](docs/screenshots/settings.png) |

## 功能

- **本月已赚、今日已赚**：每秒刷新，数字按位滚动，保留 3 位小数。
- **按当月实际工作日折算**：每个月的工作日数量不同，秒薪也各自不同。
- **按星期几设置作息**：有一套默认作息，星期一到星期日可以各自设置不同的时间。
- **假期倒计时**：自动识别法定节假日和调休上班日，显示距离最近的假期还有多少天。
- **四种背景风格**：素色、极光、霓虹、星空。背景透明度可以在 0 到 100 之间调节，拖动时立即生效，并自动保存。
- **窗口**：始终置顶，可以拖动，会记住位置，不在任务栏显示。程序已经在运行时再次启动，不会出现第二个窗口。

## 运行

- 需要 Windows 11。程序只在 Windows 11 上构建和测试过。
- 仓库里不包含编译好的 exe，需要自己构建，见下一节。
- 第一次启动会直接进入设置视图，填入月薪后保存。
- 鼠标移到窗口上，右上角会出现 ⚙（设置）和 ✕（退出）两个按钮。

## 构建和测试

需要 Go 1.26 或更高版本。界面是原生的 HTML、CSS 和 JavaScript，编译时嵌入 exe，所以不需要 Node.js，也不需要安装 Wails 的命令行工具。

```bash
go test ./...
go build -tags production -trimpath -ldflags "-s -w -H=windowsgui" -o build/bin/cow-time.exe .
```

产物在 `build/bin/cow-time.exe`。

如果连不上 Go 官方的模块代理，可以换一个代理再执行上面的命令：

```bash
GOPROXY=https://goproxy.cn,direct go build -tags production -trimpath -ldflags "-s -w -H=windowsgui" -o build/bin/cow-time.exe .
```

PowerShell 里先执行 `$env:GOPROXY = "https://goproxy.cn,direct"`。

## 薪资的计算规则

只在工作日的工作时间内计薪，并按当月实际工作日折算。

- **工作日**：周一到周五，且不是法定节假日。调休上班的周六、周日也是工作日。
- **每天的计薪时段**：从上班到午休开始，以及从午休结束到下班。没有午休时，就是从上班到下班。
- **当月计薪总秒数**：当月每个工作日按各自作息算出的计薪秒数之和。
- **当月秒薪**：月薪除以当月计薪总秒数。
- **今日已赚**：当月秒薪乘以今天已经计薪的秒数。
- **本月已赚**：当月秒薪乘以本月 1 日到今天已经计薪的秒数之和。

由此得到几个性质：

- 月底最后一个工作日下班时，本月已赚正好等于月薪。
- 秒薪每个月不一样。月薪 10000、使用默认作息（9 点上班，午休一小时，18 点下班）时，2026 年 11 月有 21 个工作日，每个工作日约 476.19 元；2026 年 10 月有 18 个工作日，每个工作日约 555.56 元。
- 金额每秒都按当前时间重新计算，不做累加。睡眠、关机或者修改系统时间，都不会让数字出错。

当前时刻不计薪时，「今日已赚」的右侧会显示原因：休息日、假期、未上班、午休中、已下班。

## 作息

默认作息是 9 点上班，12 点到 13 点午休，18 点下班。星期一到星期日可以各自设置不同的时间，某一天没有单独设置的项目沿用默认作息。

周六、周日平时不是工作日。这两天的作息只在调休上班时使用。

## 节假日数据

- 来源是 [holiday-cn](https://github.com/NateScarlet/holiday-cn) 公开数据集，内容整理自国务院办公厅每年发布的通知。
- 程序内置了 2026、2027 两年的数据，在 `internal/holiday/data/`。
- 这是程序唯一的联网行为。启动时，如果还没有明年的数据，并且距离上一次尝试超过 24 小时，程序会从 `https://cdn.jsdelivr.net/gh/NateScarlet/holiday-cn@master/<年份>.json` 下载一次。单次超时 10 秒，最多尝试 3 次。下载成功后保存在 `%APPDATA%\cow-time\holidays\`。
- 如果某一年的数据还没有拿到，那一年的工作日先只按周一到周五统计，假期一栏显示「假期数据待更新」。数据拿到之后自动纠正。

## 配置文件

位置是 `%APPDATA%\cow-time\config.json`，由程序自动读写。在设置视图里修改的内容都保存在这里。

```json
{
  "monthlySalary": 10000,
  "workHours": { "start": "09:00", "lunchStart": "12:00", "lunchEnd": "13:00", "end": "18:00" },
  "weekdayHours": { "1": { "end": "20:30" }, "2": { "end": "20:30" }, "4": { "end": "20:30" } },
  "backgroundStyle": "aurora",
  "transparency": { "plain": 10 },
  "window": { "x": 1664, "y": 884 },
  "holidayCheckAt": "2026-11-25T10:37:17+08:00"
}
```

上面的例子表示：周一、周二、周四 20:30 下班，其余日子 18 点下班。

| 字段 | 说明 |
|---|---|
| `monthlySalary` | 月薪，单位是元。0 表示还没有设置。 |
| `workHours` | 默认作息。`lunchStart` 和 `lunchEnd` 都留空表示没有午休。 |
| `weekdayHours` | 某些星期与默认作息不同的地方。键是 `"1"`（周一）到 `"7"`（周日），只写要改的时间项。 |
| `backgroundStyle` | 背景风格：`plain`（素色）、`aurora`（极光）、`neon`（霓虹）、`stars`（星空）。 |
| `transparency` | 各种风格的背景透明度，0 到 100 的百分比。只记录改过的风格。初始值是素色 28、极光 60、霓虹 32、星空 42。 |
| `window` | 窗口位置，拖动之后自动保存。 |
| `holidayCheckAt` | 上一次尝试更新节假日数据的时间，由程序写入。 |

日志在 `%APPDATA%\cow-time\cow-time.log`。

## 目录结构

```
.
├── main.go                  程序入口：窗口、单实例、日志
├── frontend/                界面，编译时嵌入 exe
├── internal/
│   ├── config/              配置的读写和校验
│   ├── holiday/             节假日日历、假期段、数据更新；data/ 是内置数据
│   ├── salary/              薪资计算
│   ├── service/             界面调用的接口，窗口位置
│   ├── wingeo/              窗口的尺寸和位置
│   └── workhours/           作息时间，按星期的覆盖
└── docs/
    ├── screenshots/
    └── superpowers/specs/   设计文档
```

## 设计文档

- [Cow Time 悬浮窗设计](docs/superpowers/specs/2026-11-24-cow-time-design.md)：技术选型、界面、薪资计算、假期规则、异常处理
- [视觉风格设计](docs/superpowers/specs/2026-11-24-visual-style-design.md)：滚动的数字，四种背景风格
- [按星期几设置作息的设计](docs/superpowers/specs/2026-11-24-weekday-hours-design.md)
- [背景透明度滑块的设计](docs/superpowers/specs/2026-11-25-transparency-design.md)

## 已知限制

- 只在 Windows 11 上构建和测试过，Windows 10 没有测试。
- 没有安装包，也没有开机自启。exe 使用系统默认的图标。
- 界面部分没有自动化测试，界面的改动需要在构建之后人工检查。
