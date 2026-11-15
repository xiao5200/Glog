// hostlogtest 在当前主机上执行真实日志采集验证（Windows 事件日志 + 本地文件）。
// 用法：GLOG_DATA_DIR=build/bin/data go run ./cmd/hostlogtest
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"glog/internal/app"
	"glog/pkg/models"
)

func main() {
	dataDir := os.Getenv("GLOG_DATA_DIR")
	if dataDir == "" {
		dataDir = filepath.Join("build", "bin", "data")
	}
	abs, err := filepath.Abs(dataDir)
	if err != nil {
		fatal(err)
	}
	_ = os.Setenv("GLOG_DATA_DIR", abs)
	_ = os.Setenv("GLOG_DISABLE_WAILS_EVENTS", "1")

	application := app.NewApp()
	application.Startup(context.Background())
	defer application.Shutdown(context.Background())

	project, err := application.CreateProject(models.CreateProjectRequest{
		Name:        "主机日志采集测试 " + time.Now().Format("01-02 15:04"),
		Description: "验证当前 Windows 主机的 System/Application 事件日志与本地文件采集。",
		Tags:        []string{"host-test", "evtx"},
		Priority:    "medium",
	})
	if err != nil {
		fatal(err)
	}
	if err := application.OpenProject(project.ID); err != nil {
		fatal(err)
	}
	fmt.Printf("project: %s (%s)\n", project.Name, project.ID)

	now := time.Now()
	start := now.Add(-24 * time.Hour)
	fmt.Println("== collecting Windows event logs: System, Application (last 24h) ==")
	task, err := application.StartWindowsEventCollection(models.WindowsEventLogRequest{
		ProjectID: project.ID,
		Channels:  []string{"System", "Application"},
		StartTime: &start,
		EndTime:   &now,
		Parse:     models.ParseConfig{Mode: "native_evtx"},
	})
	if err != nil {
		fatal(err)
	}
	done, err := wait(application, task.ID, 180*time.Second)
	if err != nil {
		fmt.Println("evtx collection error:", err)
	} else {
		fmt.Printf("evtx task: status=%s logs=%d failed=%d bytes=%d duration=%dms error=%s\n",
			done.Status, done.Result.LogCount, done.Result.FailedCount, done.Result.TotalBytes,
			done.Result.Duration, done.Result.ErrorMessage)
	}

	// 本地文件真实采集：GLog 自身运行日志（JSON 行）。
	glogPath := filepath.Join(abs, "logs", "glog.log")
	if _, err := os.Stat(glogPath); err == nil {
		fmt.Println("== collecting local file:", glogPath, "==")
		task, err = application.StartLocalCollection(models.LocalCollectRequest{
			ProjectID: project.ID,
			FilePaths: []string{glogPath},
			Parse:     models.ParseConfig{Mode: "builtin", TemplateID: "json-generic"},
		})
		if err != nil {
			fmt.Println("local collection error:", err)
		} else {
			done, err = wait(application, task.ID, 30*time.Second)
			if err != nil {
				fmt.Println("local collection error:", err)
			} else {
				fmt.Printf("local task: status=%s logs=%d failed=%d bytes=%d\n",
					done.Status, done.Result.LogCount, done.Result.FailedCount, done.Result.TotalBytes)
			}
		}
	}

	// 系统文本日志可读性 / 预览验证（不强制解析）。
	for _, candidate := range []string{
		`C:\Windows\Logs\CBS\CBS.log`,
		`C:\Windows\INF\setupapi.dev.log`,
		`C:\Windows\PFRO.log`,
	} {
		ok := application.ValidateLocalPath(candidate)
		lines, err := application.GetLogFilePreview(candidate, 3)
		fmt.Printf("file %s readable=%v preview=%d err=%v\n", candidate, ok, len(lines), err)
		if ok && len(lines) > 0 {
			fmt.Printf("  first: %s\n", truncate(lines[0], 160))
		}
	}

	stats, err := application.GetProjectStatistics(project.ID)
	if err != nil {
		fatal(err)
	}
	fmt.Printf("stats: logs=%d sources=%d size=%d bytes\n", stats.TotalLogs, stats.SourceCount, stats.SizeBytes)

	res, err := application.Search(project.ID, models.Query{Levels: []string{"ERROR", "CRITICAL"}, Limit: 5, SortBy: "timestamp", SortOrder: "desc"})
	if err != nil {
		fmt.Println("search error:", err)
	} else {
		fmt.Printf("search ERROR/CRITICAL: total=%d took=%dms\n", res.Total, res.Took)
		for _, hit := range res.Hits {
			eventID := ""
			if hit.EventID != nil {
				eventID = fmt.Sprintf("EVT=%d", *hit.EventID)
			}
			provider := ""
			if hit.Provider != nil {
				provider = *hit.Provider
			}
			fmt.Printf("  [%s] %s %s %s | %s\n",
				hit.Timestamp.Format("01-02 15:04:05"), hit.Level, eventID, provider, truncate(hit.Message, 160))
		}
	}

	all, err := application.Search(project.ID, models.Query{Text: "GLog", Limit: 3})
	if err != nil {
		fmt.Println("search GLog error:", err)
	} else {
		fmt.Printf("search 'GLog': total=%d\n", all.Total)
		for _, hit := range all.Hits {
			fmt.Printf("  [%s] %s | %s\n", hit.Timestamp.Format("15:04:05"), hit.Level, truncate(hit.Message, 120))
		}
	}
}

func wait(application *app.App, taskID string, timeout time.Duration) (models.TaskInfo, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		tasks, err := application.ListTasks()
		if err != nil {
			return models.TaskInfo{}, err
		}
		for _, task := range tasks {
			if task.ID != taskID {
				continue
			}
			switch task.Status {
			case "success":
				return task, nil
			case "failed", "stopped":
				if task.Error != "" {
					return task, errors.New(task.Error)
				}
				return task, fmt.Errorf("task status: %s", task.Status)
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	return models.TaskInfo{}, errors.New("task timeout")
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "hostlogtest:", err)
	os.Exit(1)
}
