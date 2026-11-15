// evtxtest 采集系统导出的真实 EVTX 文件并验证解析/检索。
// 依赖先执行：wevtutil epl System build/hostlogs/System.evtx
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

	projects, err := application.ListProjects(models.ProjectFilter{Status: "all", Search: "主机日志采集测试"})
	if err != nil || len(projects) == 0 {
		fatal(fmt.Errorf("请先运行 hostlogtest 创建测试项目: %v", err))
	}
	project := projects[0]
	if err := application.OpenProject(project.ID); err != nil {
		fatal(err)
	}
	fmt.Printf("project: %s (%s)\n", project.Name, project.ID)

	files := []string{
		filepath.Join("build", "hostlogs", "System.evtx"),
		filepath.Join("build", "hostlogs", "Application.evtx"),
	}
	for _, file := range files {
		stat, err := os.Stat(file)
		if err != nil {
			fmt.Printf("skip %s: %v\n", file, err)
			continue
		}
		started := time.Now()
		task, err := application.ImportFile(models.ImportFileRequest{
			ProjectID: project.ID,
			FilePath:  file,
			Parse:     models.ParseConfig{Mode: "native_evtx", TemplateID: "windows-evtx"},
		})
		if err != nil {
			fmt.Printf("import %s error: %v\n", file, err)
			continue
		}
		done, err := wait(application, task.ID, 5*time.Minute)
		if err != nil {
			fmt.Printf("import %s failed: %v\n", file, err)
			continue
		}
		fmt.Printf("import %s (%.2f MB): status=%s logs=%d failed=%d bytes=%d duration=%dms err=%s\n",
			filepath.Base(file), float64(stat.Size())/1024/1024, done.Status, done.Result.LogCount,
			done.Result.FailedCount, done.Result.TotalBytes, done.Result.Duration, done.Result.ErrorMessage)
		fmt.Printf("  wall time: %s\n", time.Since(started).Round(time.Second))
	}

	stats, err := application.GetProjectStatistics(project.ID)
	if err != nil {
		fatal(err)
	}
	fmt.Printf("stats: logs=%d sources=%d size=%.2f MB\n", stats.TotalLogs, stats.SourceCount, float64(stats.SizeBytes)/1024/1024)

	queries := []models.Query{
		{Levels: []string{"ERROR", "CRITICAL"}, Limit: 5, SortBy: "timestamp", SortOrder: "desc"},
		{Text: "error", Limit: 5, SortBy: "timestamp", SortOrder: "desc"},
		{EventIDs: []uint32{4625, 4688, 7045}, Limit: 5, SortBy: "timestamp", SortOrder: "desc"},
	}
	for _, q := range queries {
		res, err := application.Search(project.ID, q)
		if err != nil {
			fmt.Println("search error:", err)
			continue
		}
		fmt.Printf("search %+v: total=%d took=%dms\n", q, res.Total, res.Took)
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
		time.Sleep(300 * time.Millisecond)
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
	fmt.Fprintln(os.Stderr, "evtxtest:", err)
	os.Exit(1)
}
