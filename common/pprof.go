// Package common - pprof.go
// 该文件实现了 CPU 使用率监控和 pprof 性能分析文件的自动生成
//
// 功能说明：
// - 定时检测 CPU 使用率
// - 当 CPU 使用率超过 80% 时，自动采集 pprof 性能分析数据
// - pprof 文件保存在 ./pprof/ 目录下
//
// pprof 文件可用于：
// - 分析 CPU 热点
// - 定位性能瓶颈
// - 使用 go tool pprof 命令分析
package common

import (
	"fmt"
	"os"
	"runtime/pprof"
	"time"

	"github.com/shirou/gopsutil/cpu"
)

// Monitor 定时监控 CPU 使用率，超过阈值输出 pprof 文件
//
// 监控流程：
// 1. 每 30 秒检测一次 CPU 使用率
// 2. 如果 CPU 使用率超过 80%，开始采集 pprof 数据
// 3. 采集 10 秒的 CPU 性能数据
// 4. 保存到 ./pprof/ 目录下
//
// pprof 文件命名格式：cpu-{时间戳}.pprof
// 使用方法：go tool pprof ./pprof/cpu-20060102150405.pprof
func Monitor() {
	for {
		// 获取 CPU 使用率（采样 1 秒）
		percent, err := cpu.Percent(time.Second, false)
		if err != nil {
			panic(err)
		}
		// 如果 CPU 使用率超过 80%
		if percent[0] > 80 {
			fmt.Println("cpu usage too high")
			// 创建 pprof 目录（如果不存在）
			if _, err := os.Stat("./pprof"); os.IsNotExist(err) {
				err := os.Mkdir("./pprof", os.ModePerm)
				if err != nil {
					SysLog("创建pprof文件夹失败 " + err.Error())
					continue
				}
			}
			// 创建 pprof 文件
			f, err := os.Create("./pprof/" + fmt.Sprintf("cpu-%s.pprof", time.Now().Format("20060102150405")))
			if err != nil {
				SysLog("创建pprof文件失败 " + err.Error())
				continue
			}
			// 开始 CPU 性能分析
			err = pprof.StartCPUProfile(f)
			if err != nil {
				SysLog("启动pprof失败 " + err.Error())
				continue
			}
			// 采集 10 秒的性能数据
			time.Sleep(10 * time.Second)
			pprof.StopCPUProfile()
			f.Close()
		}
		// 每 30 秒检测一次
		time.Sleep(30 * time.Second)
	}
}
