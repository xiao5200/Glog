package project

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"glog/internal/errs"
	"glog/pkg/models"
)

var validPriorities = map[string]struct{}{
	"high": {}, "medium": {}, "low": {},
}

// Create 创建项目：生成 UUID、建立目录结构、写入 project.meta 并注册到数据库。
func (s *Service) Create(req models.CreateProjectRequest) (*models.Project, error) {
	if s == nil || s.store == nil {
		return nil, errs.Internal("项目服务未初始化", nil)
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, errs.InvalidInput("项目名称不能为空", nil)
	}
	priority := strings.ToLower(strings.TrimSpace(req.Priority))
	if priority == "" {
		priority = "medium"
	}
	if _, ok := validPriorities[priority]; !ok {
		return nil, errs.InvalidInput("项目优先级必须为 high/medium/low", nil)
	}

	id := uuid.NewString()
	dir, err := s.projectDirChecked(id)
	if err != nil {
		return nil, errs.Internal("生成项目路径失败", err)
	}

	now := time.Now().UTC()
	tags := cloneStrings(req.Tags)
	if tags == nil {
		tags = []string{}
	}
	p := &models.Project{
		ID:          id,
		Name:        name,
		Description: req.Description,
		CreatedAt:   now,
		UpdatedAt:   now,
		Status:      "active",
		ProjectPath: dir,
		LogCount:    0,
		SizeBytes:   0,
		Sources:     []models.LogSource{},
		Levels:      map[string]int64{},
		Tags:        tags,
		Priority:    priority,
	}

	if err := os.MkdirAll(filepath.Join(dir, "raw"), 0o755); err != nil {
		return nil, errs.Internal("创建项目目录失败", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "indexes"), 0o755); err != nil {
		_ = os.RemoveAll(dir)
		return nil, errs.Internal("创建项目索引目录失败", err)
	}
	if err := s.WriteMeta(p); err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	if err := s.store.CreateProject(p); err != nil {
		_ = os.RemoveAll(dir)
		if errors.Is(err, errs.ErrAlreadyExists) {
			return nil, errs.New(errs.CodeInternal, "项目ID冲突，请重试", err)
		}
		return nil, errs.Internal("保存项目记录失败", err)
	}
	return p, nil
}

// List 按筛选条件列出项目（不附带日志源明细）。
func (s *Service) List(f models.ProjectFilter) ([]models.Project, error) {
	if s == nil || s.store == nil {
		return nil, errs.Internal("项目服务未初始化", nil)
	}
	list, err := s.store.ListProjects(f)
	if err != nil {
		return nil, err
	}
	normalizeProjectSlice(list)
	return list, nil
}

// Get 读取项目详情。以数据库为准，并在缺失时用 project.meta 补齐
// Sources/统计等文件侧信息。
func (s *Service) Get(id string) (*models.Project, error) {
	dir, err := s.projectDirChecked(id)
	if err != nil {
		return nil, errs.InvalidInput("项目ID无效", err)
	}
	p, err := s.store.GetProject(id)
	if err != nil {
		return nil, err
	}

	sources, err := s.store.ListLogSources(id)
	if err != nil {
		return nil, errs.Internal("读取项目日志源失败", err)
	}
	p.Sources = sources

	if meta, metaErr := s.ReadMeta(id); metaErr == nil && meta != nil {
		if len(p.Sources) == 0 && len(meta.Sources) > 0 {
			p.Sources = meta.Sources
		}
		if p.LogCount == 0 && meta.LogCount != 0 {
			p.LogCount = meta.LogCount
		}
		if p.SizeBytes == 0 && meta.SizeBytes != 0 {
			p.SizeBytes = meta.SizeBytes
		}
		if p.TimeRange == nil {
			p.TimeRange = meta.TimeRange
		}
		if len(p.Levels) == 0 && len(meta.Levels) > 0 {
			p.Levels = meta.Levels
		}
	}
	if p.ProjectPath == "" {
		p.ProjectPath = dir
	}
	normalizeProject(p)
	return p, nil
}

// Update 按非 nil 字段更新项目，并同步写入 project.meta。
func (s *Service) Update(id string, u models.ProjectUpdates) (*models.Project, error) {
	if _, err := s.projectDirChecked(id); err != nil {
		return nil, errs.InvalidInput("项目ID无效", err)
	}
	p, err := s.Get(id)
	if err != nil {
		return nil, err
	}

	if u.Name != nil {
		name := strings.TrimSpace(*u.Name)
		if name == "" {
			return nil, errs.InvalidInput("项目名称不能为空", nil)
		}
		p.Name = name
	}
	if u.Description != nil {
		p.Description = *u.Description
	}
	if u.Tags != nil {
		tags := cloneStrings(*u.Tags)
		if tags == nil {
			tags = []string{}
		}
		p.Tags = tags
	}
	if u.Priority != nil {
		priority := strings.ToLower(strings.TrimSpace(*u.Priority))
		if _, ok := validPriorities[priority]; !ok {
			return nil, errs.InvalidInput("项目优先级必须为 high/medium/low", nil)
		}
		p.Priority = priority
	}
	if u.Status != nil {
		status := strings.ToLower(strings.TrimSpace(*u.Status))
		if status != "active" && status != "archived" {
			return nil, errs.InvalidInput("项目状态必须为 active 或 archived", nil)
		}
		p.Status = status
	}

	p.UpdatedAt = time.Now().UTC()
	if err := s.store.UpdateProject(p); err != nil {
		return nil, err
	}
	if err := s.WriteMeta(p); err != nil {
		return nil, errs.Internal("项目已更新，但写入 project.meta 失败", err)
	}
	return p, nil
}

// Delete 删除项目：先删除项目目录，再删除数据库记录。
// 目录不存在时允许继续删除数据库记录。
func (s *Service) Delete(id string) error {
	dir, err := s.projectDirChecked(id)
	if err != nil {
		return errs.InvalidInput("项目ID无效", err)
	}
	if err := removeProjectDir(dir); err != nil {
		return errs.Internal(describeDeleteError(err), err)
	}
	if err := s.store.DeleteProject(id); err != nil {
		return err
	}
	return nil
}

// removeProjectDir 重试删除项目目录。Windows 上索引/日志文件句柄释放可能有短暂延迟，
// 关闭项目后立即删除偶尔会返回“文件被占用”；短暂重试可覆盖该窗口。
func removeProjectDir(dir string) error {
	const attempts = 6
	const delay = 250 * time.Millisecond
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		if err := os.RemoveAll(dir); err == nil {
			return nil
		} else {
			lastErr = err
		}
		if attempt < attempts-1 {
			time.Sleep(delay)
		}
	}
	return lastErr
}

func describeDeleteError(err error) string {
	message := strings.ToLower(err.Error())
	for _, hint := range []string{
		"being used by another process",
		"另一个程序",
		"resource busy",
		"text file busy",
		"sharing violation",
	} {
		if strings.Contains(message, hint) {
			return "删除项目目录失败：文件仍被其他进程占用。请关闭其他 GLog 实例或正在采集的任务后重试（数据库记录已保留）"
		}
	}
	return "删除项目目录失败，已保留数据库记录"
}

// Archive 归档项目；对已归档项目幂等。
func (s *Service) Archive(id string) error {
	if _, err := s.projectDirChecked(id); err != nil {
		return errs.InvalidInput("项目ID无效", err)
	}
	p, err := s.Get(id)
	if err != nil {
		return err
	}
	if p.Status == "archived" {
		return nil
	}
	p.Status = "archived"
	p.UpdatedAt = time.Now().UTC()
	if err := s.store.UpdateProject(p); err != nil {
		return err
	}
	if err := s.WriteMeta(p); err != nil {
		return errs.Internal("项目已归档，但写入 project.meta 失败", err)
	}
	return nil
}

// normalizeProjectSlice 补齐列表中每个项目的 nil 集合。
func normalizeProjectSlice(list []models.Project) {
	for i := range list {
		normalizeProject(&list[i])
	}
}

// ensureProjectDirs 确保项目目录结构存在。
func ensureProjectDirs(dir string) error {
	if err := os.MkdirAll(filepath.Join(dir, "raw"), 0o755); err != nil {
		return fmt.Errorf("创建 raw 目录失败: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "indexes"), 0o755); err != nil {
		return fmt.Errorf("创建 indexes 目录失败: %w", err)
	}
	return nil
}
