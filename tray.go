package main

import (
	_ "embed"
	"time"

	"github.com/getlantern/systray"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed build/windows/icon.ico
var iconData []byte

/**
 * @description: 启动系统托盘功能，创建托盘图标和菜单项
 * 函数初始化系统托盘，提供应用程序的快捷操作入口，包括：
 * - 计时模式切换（计时/时间显示）
 * - 计时器控制（开始/暂停/重置）
 * - 窗口管理（新建/删除窗口）
 * - 应用程序退出
 *
 * 每个菜单项都使用独立的goroutine处理点击事件，避免阻塞主线程。
 *
 * @param {*App} app - 应用程序实例，用于调用各种功能方法
 *
 * @example
 * // 在应用程序启动时调用
 * func main() {
 *     app := &App{}
 *     StartTray(app)
 *     // 其他初始化代码...
 * }
 *
 * @menu 计时[C] - 切换到计时模式
 * @menu 时间[T] - 切换到时间显示模式
 * @menu 重置[R] - 重置计时器
 * @menu 开始[B] - 开始计时器
 * @menu 暂停[E] - 暂停计时器
 * @menu 新建[Ctrl+C] - 创建新窗口
 * @menu 删除[ESC] - 删除当前窗口
 * @menu 退出[Q] - 退出应用程序
 *
 */
func StartTray(app *App) {
	go systray.Run(func() {
		// 托盘初始化
		systray.SetIcon(iconData)
		systray.SetTitle("Punktime")
		systray.SetTooltip("Punktime")

		count := systray.AddMenuItem("计时[C]", "计时")
		timer := systray.AddMenuItem("时间[T]", "时间")
		reset := systray.AddMenuItem("重置[R]", "重置计时器")
		begin := systray.AddMenuItem("开始[B]", "开始计时器")
		end := systray.AddMenuItem("暂停[E]", "暂停计时器")
		systray.AddSeparator()
		newWin := systray.AddMenuItem("新建[Ctrl+C]", "新建一个窗口")
		delWin := systray.AddMenuItem("删除[ESC]", "删除当前窗口")

		systray.AddSeparator()
		quit := systray.AddMenuItem("退出[Q]", "退出应用")

		go func() {
			// 使用独立的goroutine处理每个菜单项，避免阻塞
			go func() {
				for range delWin.ClickedCh {
					runtime.Quit(app.ctx)
				}
			}()

			// 新建窗口
			go func() {
				for range newWin.ClickedCh {
					app.CreateNewWindow()
					time.Sleep(200 * time.Millisecond) // 防抖
				}
			}()

			go func() {
				for range count.ClickedCh {
					app.SwitchToCountdownMode()
					time.Sleep(200 * time.Millisecond) // 防抖
				}
			}()

			go func() {
				for range timer.ClickedCh {
					app.SwitchToTimeMode()
					time.Sleep(200 * time.Millisecond) // 防抖
				}
			}()

			go func() {
				for range reset.ClickedCh {
					app.ResetTimer()
					time.Sleep(200 * time.Millisecond) // 防抖
				}
			}()

			go func() {
				for range begin.ClickedCh {
					app.StartTimer()
					time.Sleep(200 * time.Millisecond) // 防抖
				}
			}()

			go func() {
				for range end.ClickedCh {
					app.StopTimer()
					time.Sleep(200 * time.Millisecond) // 防抖
				}
			}()

			go func() {
				for range quit.ClickedCh {
					systray.Quit()
					runtime.Quit(app.ctx)
					return
				}
			}()

			// 主goroutine保持运行
			select {}
		}()
	}, func() {})

}
