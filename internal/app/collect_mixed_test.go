package app

import (
	"testing"
	"time"

	"glog/pkg/models"
)

// TestAutoTemplateFallbackForMixedLogs 验证自动模板模式下的多文件采集：
// 单一自动识别模板无法覆盖所有文件时，逐行回退到其它内置模板，避免混合日志丢失。
func TestAutoTemplateFallbackForMixedLogs(t *testing.T) {
	a := newTestApp(t)

	project, err := a.CreateProject(models.CreateProjectRequest{Name: "混合日志测试"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if err := a.OpenProject(project.ID); err != nil {
		t.Fatalf("open project: %v", err)
	}

	dir := t.TempDir()
	nginxLog := writeFile(t, dir, "access.log",
		`127.0.0.1 - - [10/Oct/2024:13:55:36 +0800] "GET /index.html HTTP/1.1" 200 612 "-" "curl/8.0"`+"\n")
	sshLog := writeFile(t, dir, "auth.log",
		"Oct 10 14:00:01 web-01 sshd[1000]: Accepted password for toor from 192.168.11.1 port 55000 ssh2\n")

	task, err := a.StartLocalCollection(models.LocalCollectRequest{
		ProjectID: project.ID,
		FilePaths: []string{nginxLog, sshLog},
		Parse:     models.ParseConfig{Mode: "builtin"},
	})
	if err != nil {
		t.Fatalf("start collection: %v", err)
	}
	done := waitTask(t, a, task.ID, 15*time.Second)
	if done.Status != "success" {
		t.Fatalf("collection status=%s error=%s result=%+v", done.Status, done.Error, done.Result)
	}
	if done.Result == nil || done.Result.LogCount != 2 || done.Result.FailedCount != 0 {
		t.Fatalf("expected 2 parsed logs and 0 failures, got %+v", done.Result)
	}

	result, err := a.Search(project.ID, models.Query{Limit: 10})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if result.Total != 2 {
		t.Fatalf("expected 2 logs in project, got %d", result.Total)
	}
}
