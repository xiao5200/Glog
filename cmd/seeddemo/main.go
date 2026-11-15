// seeddemo 生成用于本地联调/截图的演示项目数据。
// 用法：GLOG_DATA_DIR=build/bin/data go run ./cmd/seeddemo
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
	if err := os.Setenv("GLOG_DATA_DIR", abs); err != nil {
		fatal(err)
	}
	// CLI 环境没有 Wails 前端事件总线。
	if err := os.Setenv("GLOG_DISABLE_WAILS_EVENTS", "1"); err != nil {
		fatal(err)
	}

	application := app.NewApp()
	application.Startup(context.Background())
	defer application.Shutdown(context.Background())

	projects, err := application.ListProjects(models.ProjectFilter{Status: "all"})
	if err != nil {
		fatal(err)
	}
	var project *models.Project
	for i := range projects {
		if strings.Contains(projects[i].Name, "演示") {
			project = &projects[i]
			break
		}
	}
	if project == nil {
		project, err = application.CreateProject(models.CreateProjectRequest{
			Name:        "演示：Web 入侵调查",
			Description: "Nginx 访问日志 + SSH 认证日志 + 模拟应用 JSON 日志，用于功能演示与联调。",
			Tags:        []string{"演示", "web", "入侵调查"},
			Priority:    "high",
		})
		if err != nil {
			fatal(err)
		}
	}
	if err := application.OpenProject(project.ID); err != nil {
		fatal(err)
	}

	workDir, err := os.MkdirTemp("", "glog-demo-*")
	if err != nil {
		fatal(err)
	}
	defer os.RemoveAll(workDir)

	nginxPath := filepath.Join(workDir, "access.log")
	nginx := strings.Join([]string{
		`203.0.113.9 - admin [10/Oct/2024:13:55:36 +0800] "GET /index.html HTTP/1.1" 200 612 "-" "Mozilla/5.0"`,
		`203.0.113.9 - admin [10/Oct/2024:13:55:37 +0800] "POST /login.php HTTP/1.1" 401 128 "https://example.com/" "curl/8.0"`,
		`203.0.113.9 - admin [10/Oct/2024:13:55:38 +0800] "GET /admin/config.php?id=1%20union%20select%20user,password%20from%20users HTTP/1.1" 500 42 "-" "sqlmap/1.7"`,
		`198.51.100.23 - - [10/Oct/2024:13:56:02 +0800] "GET /uploads/shell.php?cmd=id HTTP/1.1" 200 88 "-" "Mozilla/5.0"`,
		`198.51.100.23 - - [10/Oct/2024:13:56:12 +0800] "GET /../../etc/passwd HTTP/1.1" 403 0 "-" "nikto/2.5"`,
		`192.0.2.55 - - [10/Oct/2024:13:57:01 +0800] "GET /healthz HTTP/1.1" 200 2 "-" "kube-probe/1.28"`,
		`192.0.2.55 - - [10/Oct/2024:13:57:31 +0800] "GET /healthz HTTP/1.1" 200 2 "-" "kube-probe/1.28"`,
		`192.0.2.55 - - [10/Oct/2024:13:58:01 +0800] "GET /healthz HTTP/1.1" 200 2 "-" "kube-probe/1.28"`,
	}, "\n")
	if err := os.WriteFile(nginxPath, []byte(nginx), 0o644); err != nil {
		fatal(err)
	}

	sshPath := filepath.Join(workDir, "auth.log")
	var sshLines []string
	for i := 0; i < 14; i++ {
		sshLines = append(sshLines, fmt.Sprintf(
			`Oct 10 14:0%d:%02d web-01 sshd[%d]: Failed password for root from 203.0.113.9 port %d ssh2`,
			i/60, i%60, 2000+i, 51000+i,
		))
	}
	sshLines = append(sshLines,
		`Oct 10 14:01:20 web-01 sshd[3000]: Accepted publickey for deploy from 192.0.2.55 port 60022 ssh2: RSA SHA256:demo`)
	if err := os.WriteFile(sshPath, []byte(strings.Join(sshLines, "\n")), 0o644); err != nil {
		fatal(err)
	}

	appPath := filepath.Join(workDir, "app.jsonl")
	appLogs := []string{
		`{"timestamp":"2024-10-10T14:00:05Z","level":"info","message":"auth service started","service":"auth","pid":991}`,
		`{"timestamp":"2024-10-10T14:00:40Z","level":"warn","message":"failed login for admin from 203.0.113.9","service":"auth","fields":{"remote_addr":"203.0.113.9"}}`,
		`{"timestamp":"2024-10-10T14:01:02Z","level":"error","message":"database connection timeout after 1200ms","service":"api","fields":{"db":"mysql-01"}}`,
		`{"timestamp":"2024-10-10T14:01:30Z","level":"info","message":"deploy user logged in","service":"auth"}`,
	}
	if err := os.WriteFile(appPath, []byte(strings.Join(appLogs, "\n")), 0o644); err != nil {
		fatal(err)
	}

	collections := []struct {
		name  string
		path  string
		tpl   string
		label string
		mode  string
	}{
		{"nginx", nginxPath, "nginx-combined", "演示 Nginx 访问日志", "builtin"},
		{"ssh", sshPath, "sshd", "演示 SSH 认证日志", "builtin"},
		{"app", appPath, "json-generic", "演示应用 JSON 日志", "builtin"},
	}
	for _, item := range collections {
		task, err := application.StartLocalCollection(models.LocalCollectRequest{
			ProjectID: project.ID,
			FilePaths: []string{item.path},
			Parse:     models.ParseConfig{Mode: item.mode, TemplateID: item.tpl},
		})
		if err != nil {
			fatal(err)
		}
		if err := wait(application, task.ID, 30*time.Second); err != nil {
			fatal(fmt.Errorf("%s: %w", item.label, err))
		}
	}

	stats, err := application.GetProjectStatistics(project.ID)
	if err != nil {
		fatal(err)
	}
	fmt.Printf("demo project: %s (%s)\n", project.Name, project.ID)
	fmt.Printf("logs: %d, size: %d bytes, sources: %d\n", stats.TotalLogs, stats.SizeBytes, stats.SourceCount)
	fmt.Printf("data dir: %s\n", abs)
}

func wait(application *app.App, taskID string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		tasks, err := application.ListTasks()
		if err != nil {
			return err
		}
		for _, task := range tasks {
			if task.ID != taskID {
				continue
			}
			switch task.Status {
			case "success":
				return nil
			case "failed", "stopped":
				if task.Error != "" {
					return errors.New(task.Error)
				}
				return fmt.Errorf("task status: %s", task.Status)
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return errors.New("task timeout")
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "seeddemo:", err)
	os.Exit(1)
}
