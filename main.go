package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"sync"
	"time"

	"github.com/wailsapp/wails/v2"

	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	"github.com/Schuyler2025/Punktime/pkg/log"
)

//go:embed all:frontend
var assets embed.FS

/**
 * @description: 计时器管理器结构体，负责管理应用程序的计时器状态和逻辑
 * 结构体封装了计时器系统的核心状态和同步机制，包括：
 * - 计时器运行状态管理（运行/暂停/停止）
 * - 时间计算和跟踪
 * - 显示模式切换（时间模式/倒计时模式）
 * - 线程安全的并发控制
 *
 * 字段说明：
 * @field ctx: Wails应用程序上下文
 * @field mu: 互斥锁，确保计时器状态的线程安全访问
 * @field isTimerRunning: 计时器是否正在运行的标志
 * @field timerStartTime: 计时器开始运行的时间点
 * @field timerElapsed: 计时器已运行的时间长度
 * @field ticker: 每秒触发一次的定时器，驱动更新循环
 * @field displayMode: 当前显示模式（"time"时间模式/"countdown"倒计时模式）
 * @field countdownDuration: 倒计时总时长设置
 * @field isPaused: 计时器是否处于暂停状态
 *
 *
 * @see NewTimerManager 创建计时器管理器的工厂函数
 * @see updateLoop 计时器更新循环函数
 * @see Start 启动计时器函数
 *
 * @note 所有对结构体字段的修改都应在互斥锁保护下进行
 * @note 倒计时模式支持暂停/继续功能
 * @note 时间模式显示当前系统时间，倒计时模式显示剩余时间
 */
type TimerManager struct {
	ctx               context.Context
	mu                sync.Mutex
	isTimerRunning    bool
	timerStartTime    time.Time
	timerElapsed      time.Duration
	ticker            *time.Ticker
	displayMode       string
	countdownDuration time.Duration

	isPaused bool
}

type App struct {
	ctx         context.Context
	startHidden bool
	timerMgr    *TimerManager
}

var (
	timerManager *TimerManager
)

func NewApp() *App {
	return &App{}
}

/**
 * @description: 应用程序启动时的初始化函数，Wails框架生命周期钩子
 * 函数在应用程序启动时被自动调用，负责完成以下初始化工作：
 * 1. 上下文保存：保存Wails上下文供后续使用
 * 2. 计时器管理：创建并初始化计时器管理器
 * 3. 窗口管理：根据启动参数决定是否隐藏窗口
 * 4. 屏幕适配：获取主屏幕信息并居中显示窗口
 * 5. 窗口属性：设置窗口置顶显示
 * 6. 系统托盘：启动系统托盘功能
 * 7. 计时器启动：开始计时器更新循环
 *
 *
 *
 * @see NewTimerManager 创建计时器管理器实例
 * @see StartTray 启动系统托盘功能
 *
 * @note 如果启动时设置了隐藏参数，窗口会先隐藏再显示
 * @note 窗口位置会自动适配主屏幕，居中显示在屏幕顶部
 * @note 窗口默认设置为置顶显示，确保始终可见
 */
func (a *App) OnStartup(ctx context.Context) {
	a.ctx = ctx

	timerManager = NewTimerManager(ctx)
	a.timerMgr = timerManager

	if a.startHidden {
		runtime.WindowHide(ctx)
	}

	screens, err := runtime.ScreenGetAll(a.ctx)
	if err != nil {
		log.LOG_ERROR("Error getting screens: %v", err)
		runtime.WindowSetPosition(a.ctx, 100, 50)
	} else {
		primaryScreen := screens[0]
		windowWidth := 160
		centerX := (primaryScreen.Width - windowWidth) / 2
		topY := 0
		runtime.WindowSetPosition(a.ctx, centerX, topY)
	}

	runtime.WindowSetAlwaysOnTop(a.ctx, true)

	StartTray(a)
	if a.startHidden {
		runtime.WindowShow(ctx)
	}

	timerManager.Start()
}

func NewTimerManager(ctx context.Context) *TimerManager {
	return &TimerManager{
		ctx:               ctx,
		displayMode:       "time",
		isTimerRunning:    false,
		timerElapsed:      0,
		countdownDuration: 25 * time.Minute,
		isPaused:          false,
	}
}

func (tm *TimerManager) Start() {
	tm.ticker = time.NewTicker(time.Second)
	go tm.updateLoop()
}

/**
 * @description: 计时器更新循环，每秒执行一次的时间/倒计时更新逻辑
 * 函数作为计时器管理器的核心循环，负责：
 * 1. 每秒更新计时器状态和显示内容
 * 2. 根据当前显示模式（倒计时/时间）执行不同的更新逻辑
 * 3. 处理倒计时结束事件和状态转换
 * 4. 通过Wails事件系统向前端发送更新数据
 *
 * 倒计时模式逻辑：
 * - 运行中：计算剩余时间，倒计时结束时触发timerEnd事件
 * - 暂停中：显示暂停时的剩余时间
 * - 未开始：显示设置的倒计时总时长
 *
 * 时间模式逻辑：
 * - 显示当前系统时间（MM:SS格式）
 *
 * @example
 * // 每秒自动执行，无需手动调用
 * // 倒计时模式：显示"25:00" → "24:59" → ... → "00:00"（触发结束事件）
 * // 时间模式：显示当前时间如"14:30"
 *
 * @see runtime.EventsEmit 发送事件到前端
 * @see time.Since 计算时间间隔
 *
 * @note 使用互斥锁确保线程安全，避免并发访问计时器状态
 * @note 倒计时结束时自动重置状态并发送结束事件
 * @note 时间格式统一为两位数（如"05:09"而非"5:9"）
 */
func (tm *TimerManager) updateLoop() {
	for range tm.ticker.C {
		tm.mu.Lock()
		if tm.isTimerRunning {
			tm.timerElapsed = time.Since(tm.timerStartTime)
		}
		switch tm.displayMode {
		case "countdown":
			if tm.isTimerRunning {
				remaining := tm.countdownDuration - tm.timerElapsed
				if remaining <= 0 {
					remaining = 0
					tm.isTimerRunning = true
					tm.timerElapsed = tm.countdownDuration
					tm.isPaused = false
					runtime.EventsEmit(tm.ctx, "timerEnd")

					timerText := "00:00"
					runtime.EventsEmit(tm.ctx, "timerUpdate", timerText)
				} else {
					minutes := int(remaining.Minutes())
					seconds := int(remaining.Seconds()) % 60
					timerText := fmt.Sprintf("%02d:%02d", minutes, seconds)
					runtime.EventsEmit(tm.ctx, "timerUpdate", timerText)
				}

			} else {
				if tm.isPaused {
					remaining := tm.countdownDuration - tm.timerElapsed
					if remaining < 0 {
						remaining = 0
					}
					minutes := int(remaining.Minutes())
					seconds := int(remaining.Seconds()) % 60
					timerText := fmt.Sprintf("%02d:%02d", minutes, seconds)
					runtime.EventsEmit(tm.ctx, "timerUpdate", timerText)
				} else {
					minutes := int(tm.countdownDuration.Minutes())
					seconds := int(tm.countdownDuration.Seconds()) % 60
					timerText := fmt.Sprintf("%02d:%02d", minutes, seconds)
					runtime.EventsEmit(tm.ctx, "timerUpdate", timerText)
				}
			}

		case "time":
			currentTime := time.Now().Format("15:04")
			runtime.EventsEmit(tm.ctx, "timeUpdate", currentTime)

		}
		tm.mu.Unlock()
	}
}

func (a *App) SetCountdownDuration(minutes int, seconds int) {
	a.timerMgr.mu.Lock()
	defer a.timerMgr.mu.Unlock()
	a.timerMgr.countdownDuration = time.Duration(minutes)*time.Minute + time.Duration(seconds)*time.Second

	a.timerMgr.timerStartTime = time.Now()
	a.timerMgr.isTimerRunning = true
	a.timerMgr.isPaused = false
}

func (a *App) GetCountdownDuration() (int, int) {
	a.timerMgr.mu.Lock()
	defer a.timerMgr.mu.Unlock()
	minutes := int(a.timerMgr.countdownDuration.Minutes())
	seconds := int(a.timerMgr.countdownDuration.Seconds()) % 60
	return minutes, seconds
}

func (a *App) SwitchToTimeMode() {
	a.timerMgr.mu.Lock()
	defer a.timerMgr.mu.Unlock()
	a.timerMgr.displayMode = "time"
	a.timerMgr.isTimerRunning = false
}

func (a *App) SwitchToCountdownMode() {
	a.timerMgr.mu.Lock()
	defer a.timerMgr.mu.Unlock()
	a.timerMgr.displayMode = "countdown"
	a.timerMgr.isTimerRunning = true
	a.timerMgr.timerStartTime = time.Now()
	a.timerMgr.timerElapsed = 0

	runtime.EventsEmit(a.ctx, "showCountdownInput")
}

func (a *App) StartTimer() {
	a.timerMgr.mu.Lock()
	defer a.timerMgr.mu.Unlock()
	if !a.timerMgr.isTimerRunning {
		a.timerMgr.isTimerRunning = true
		a.timerMgr.timerStartTime = time.Now().Add(-a.timerMgr.timerElapsed)
		a.timerMgr.isPaused = false
	}
}

func (a *App) StopTimer() {
	a.timerMgr.mu.Lock()
	defer a.timerMgr.mu.Unlock()
	if a.timerMgr.isTimerRunning {
		a.timerMgr.isTimerRunning = false
		a.timerMgr.timerElapsed = time.Since(a.timerMgr.timerStartTime)
		a.timerMgr.isPaused = true

		if a.timerMgr.displayMode == "countdown" {
			remaining := a.timerMgr.countdownDuration - a.timerMgr.timerElapsed
			if remaining < 0 {
				remaining = 0
			}
			minutes := int(remaining.Minutes())
			seconds := int(remaining.Seconds()) % 60
			timerText := fmt.Sprintf("%02d:%02d", minutes, seconds)
			runtime.EventsEmit(a.ctx, "timerUpdate", timerText)
		}
	}

}

func (a *App) ResetTimer() {
	a.timerMgr.mu.Lock()
	defer a.timerMgr.mu.Unlock()

	a.timerMgr.isTimerRunning = false
	a.timerMgr.timerElapsed = 0
	a.timerMgr.timerStartTime = time.Now()
	a.timerMgr.isPaused = false

	if a.timerMgr.displayMode == "countdown" {
		minutes := int(a.timerMgr.countdownDuration.Minutes())
		seconds := int(a.timerMgr.countdownDuration.Seconds()) % 60
		timerText := fmt.Sprintf("%02d:%02d", minutes, seconds)
		runtime.EventsEmit(a.ctx, "timerUpdate", timerText)
	}
}

func (a *App) GetCurrentMode() string {
	a.timerMgr.mu.Lock()
	defer a.timerMgr.mu.Unlock()
	return a.timerMgr.displayMode
}

func (a *App) OnDomReady(ctx context.Context) {

}

func (a *App) OnBeforeClose(ctx context.Context) (prevent bool) {
	return false
}

func (a *App) OnShutdown(ctx context.Context) {

}

func (a *App) CreateNewWindow() error {
	exe, err := os.Executable()
	if err != nil {
		log.LOG_ERROR("Failed to get executable path")
		return err
	}

	cmd := exec.Command(exe, "--startHidden")
	cmd.Dir = filepath.Dir(exe)
	return cmd.Start()

}

func main() {

	if !log.InitializeLogSystem() {
		panic("Failed to initialize log system")
	}

	log.SetupExceptionHandler()

	log.LOG_INFO("Punktime is starting...")

	startHidden := flag.Bool("startHidden", false, "Start the window hidden")

	app := NewApp()

	app.startHidden = *startHidden

	err := wails.Run(&options.App{
		Title:     "Punktime",
		Width:     160,
		Height:    100,
		MinWidth:  150,
		MinHeight: 90,
		Frameless: true,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 255, G: 255, B: 255, A: 0},
		Bind: []interface{}{
			app,
		},
		Windows: &windows.Options{
			WindowIsTranslucent:               true,
			BackdropType:                      windows.Acrylic,
			WebviewIsTransparent:              true,
			DisableFramelessWindowDecorations: true,
		},
		OnStartup:     app.OnStartup,
		OnShutdown:    app.OnShutdown,
		OnDomReady:    app.OnDomReady,
		OnBeforeClose: app.OnBeforeClose,
	})
	if err != nil {
		log.LOG_ERROR("Failed to start Wails app: %s", err.Error())
		panic(err)
	}

	log.LOG_INFO("Hello World")

}
