package app

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"glog/internal/errs"
	"glog/internal/logx"
	"glog/pkg/models"
)

type taskReporter func(processed, total int64, message string, progress float64)

type taskRunner func(ctx context.Context, report taskReporter) (*models.CollectResult, error)

type taskEntry struct {
	info   models.TaskInfo
	cancel context.CancelFunc
}

// taskManager 管理采集等长任务的生命周期，并通过 Wails 事件推送进度。
type taskManager struct {
	mu      sync.Mutex
	entries map[string]*taskEntry
	order   []string
	wg      sync.WaitGroup
	emit    func(name string, payload interface{})
}

func newTaskManager(emit func(name string, payload interface{})) *taskManager {
	return &taskManager{
		entries: make(map[string]*taskEntry),
		emit:    emit,
	}
}

func (tm *taskManager) Start(projectID, taskType, sourceID, message string, runner taskRunner) models.TaskInfo {
	id := newID()
	ctx, cancel := context.WithCancel(context.Background())
	info := models.TaskInfo{
		ID:        id,
		Type:      taskType,
		ProjectID: projectID,
		SourceID:  sourceID,
		Status:    "pending",
		Progress:  0,
		Message:   message,
		StartedAt: nowUTC(),
	}

	tm.mu.Lock()
	tm.entries[id] = &taskEntry{info: info, cancel: cancel}
	tm.order = append(tm.order, id)
	tm.mu.Unlock()

	if tm.emit != nil {
		tm.emit("collect:started", info)
	}

	tm.wg.Add(1)
	go func() {
		defer tm.wg.Done()
		tm.setRunning(id)

		report := func(processed, total int64, message string, progress float64) {
			tm.updateProgress(id, processed, total, message, progress)
		}

		result, err := runner(ctx, report)

		finished := nowUTC()
		tm.mu.Lock()
		entry, ok := tm.entries[id]
		if !ok {
			tm.mu.Unlock()
			cancel()
			return
		}
		current := entry.info
		current.FinishedAt = &finished
		switch {
		case ctx.Err() != nil:
			current.Status = "stopped"
			if current.Error == "" {
				current.Error = "任务已取消"
			}
			current.Message = "任务已停止"
		case err != nil:
			current.Status = "failed"
			current.Error = err.Error()
			current.Message = "采集失败"
		default:
			current.Status = "success"
			current.Progress = 1
			current.Message = "采集完成"
			if result != nil {
				current.Result = result
				if result.Status == "partial" {
					current.Message = "采集完成（部分文件失败）"
				} else if result.Status == "stopped" {
					current.Status = "stopped"
					current.Message = "采集已停止"
				}
			}
		}
		entry.info = current
		payload := current
		tm.mu.Unlock()

		tm.progressEvent(payload)
		if tm.emit != nil {
			tm.emit("collect:done", payload)
		}
		tm.trimFinished()
		cancel()
	}()

	return info
}

func (tm *taskManager) setRunning(id string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if entry, ok := tm.entries[id]; ok {
		entry.info.Status = "running"
		entry.info.Message = "正在采集…"
		entry.info.StartedAt = nowUTC()
	}
}

func (tm *taskManager) updateProgress(id string, processed, total int64, message string, progress float64) {
	if progress < 0 {
		progress = 0
	}
	if progress > 1 {
		progress = 1
	}

	tm.mu.Lock()
	entry, ok := tm.entries[id]
	if !ok {
		tm.mu.Unlock()
		return
	}
	entry.info.Status = "running"
	entry.info.Processed = processed
	entry.info.Total = total
	entry.info.Progress = progress
	if message != "" {
		entry.info.Message = message
	}
	progressPayload := models.CollectionProgress{
		TaskID:    id,
		Progress:  progress,
		Message:   entry.info.Message,
		Processed: processed,
		Total:     total,
	}
	tm.mu.Unlock()

	if tm.emit != nil {
		tm.emit("collect:progress", progressPayload)
	}
}

func (tm *taskManager) progressEvent(info models.TaskInfo) {
	if tm.emit == nil {
		return
	}
	tm.emit("collect:progress", models.CollectionProgress{
		TaskID:    info.ID,
		Progress:  info.Progress,
		Message:   info.Message,
		Processed: info.Processed,
		Total:     info.Total,
	})
}

func (tm *taskManager) List() []models.TaskInfo {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	list := make([]models.TaskInfo, 0, len(tm.order))
	for _, id := range tm.order {
		if entry, ok := tm.entries[id]; ok {
			list = append(list, entry.info)
		}
	}
	sort.SliceStable(list, func(i, j int) bool {
		return list[i].StartedAt.After(list[j].StartedAt)
	})
	return list
}

func (tm *taskManager) Stop(id string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	entry, ok := tm.entries[id]
	if !ok {
		return errs.NotFound("任务不存在", nil)
	}
	if entry.info.Status != "running" && entry.info.Status != "pending" {
		return errs.InvalidInput("任务已结束", nil)
	}
	entry.info.Message = "正在停止…"
	entry.cancel()
	return nil
}

func (tm *taskManager) StopAllAndWait(timeout time.Duration) error {
	tm.mu.Lock()
	hasRunning := false
	for _, entry := range tm.entries {
		if entry.info.Status == "running" || entry.info.Status == "pending" {
			hasRunning = true
			entry.info.Message = "正在停止…"
			entry.cancel()
		}
	}
	tm.mu.Unlock()

	if !hasRunning {
		return nil
	}

	done := make(chan struct{})
	go func() {
		tm.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-time.After(timeout):
		logx.Warn("等待任务停止超时")
		return errors.New("等待采集任务停止超时")
	}
}

func (tm *taskManager) trimFinished() {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	const maxFinished = 50
	finished := 0
	for _, id := range tm.order {
		if entry, ok := tm.entries[id]; ok {
			if entry.info.Status != "running" && entry.info.Status != "pending" {
				finished++
			}
		}
	}
	if finished <= maxFinished {
		return
	}
	remove := finished - maxFinished
	newOrder := make([]string, 0, len(tm.order))
	for _, id := range tm.order {
		entry, ok := tm.entries[id]
		if !ok {
			continue
		}
		if remove > 0 && entry.info.Status != "running" && entry.info.Status != "pending" {
			delete(tm.entries, id)
			remove--
			continue
		}
		newOrder = append(newOrder, id)
	}
	tm.order = newOrder
}
