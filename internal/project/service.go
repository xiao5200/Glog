// Package project 实现项目文件系统与生命周期管理。
//
// 每个项目对应 <projectsDir>/<projectID>/ 目录，结构为：
//
//	<projectID>/
//	├── project.meta
//	├── raw/
//	└── indexes/
//
// project.meta 保存 models.Project 的完整 JSON（含日志源与统计），
// 元数据库（SQLite）为运行时权威数据源，meta 文件用于导出与离线查看。
package project

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"glog/internal/storage"
	"glog/pkg/models"
)

// Service 项目生命周期服务。
type Service struct {
	store       *storage.MetadataStore
	projectsDir string
}

// New 创建项目服务。projectsDir 会被转换为绝对路径（失败时保留原值）。
func New(store *storage.MetadataStore, projectsDir string) *Service {
	dir := strings.TrimSpace(projectsDir)
	if dir == "" {
		dir = "projects"
	}
	if abs, err := filepath.Abs(dir); err == nil {
		dir = abs
	}
	return &Service{store: store, projectsDir: filepath.Clean(dir)}
}

// ProjectsDir 返回项目根目录。
func (s *Service) ProjectsDir() string {
	if s == nil {
		return ""
	}
	return s.projectsDir
}

// Store 返回元数据存储。
func (s *Service) Store() *storage.MetadataStore {
	if s == nil {
		return nil
	}
	return s.store
}

// Dir 返回项目目录。项目ID非法时返回空字符串。
func (s *Service) Dir(id string) string {
	if s == nil || validateProjectID(id) != nil {
		return ""
	}
	return filepath.Join(s.projectsDir, id)
}

// MetaPath 返回 project.meta 路径。项目ID非法时返回空字符串。
func (s *Service) MetaPath(id string) string {
	dir := s.Dir(id)
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "project.meta")
}

// projectIDPattern 限制项目ID字符集，防止路径穿越。
var projectIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,127}$`)

// windowsReservedNames Windows 下不能作为文件/目录名的保留名称。
var windowsReservedNames = map[string]struct{}{
	"CON": {}, "PRN": {}, "AUX": {}, "NUL": {},
	"COM1": {}, "COM2": {}, "COM3": {}, "COM4": {}, "COM5": {},
	"COM6": {}, "COM7": {}, "COM8": {}, "COM9": {},
	"LPT1": {}, "LPT2": {}, "LPT3": {}, "LPT4": {}, "LPT5": {},
	"LPT6": {}, "LPT7": {}, "LPT8": {}, "LPT9": {},
}

// validateProjectID 校验项目ID，确保可安全用作单级目录名。
func validateProjectID(id string) error {
	if id == "" {
		return fmt.Errorf("项目ID不能为空")
	}
	if id != strings.TrimSpace(id) {
		return fmt.Errorf("项目ID不能包含首尾空白")
	}
	if !projectIDPattern.MatchString(id) {
		return fmt.Errorf("项目ID包含非法字符（仅允许字母、数字、下划线和短横线）")
	}
	if _, reserved := windowsReservedNames[strings.ToUpper(id)]; reserved {
		return fmt.Errorf("项目ID不能使用系统保留名称")
	}
	return nil
}

// projectDirChecked 校验并返回项目目录。
func (s *Service) projectDirChecked(id string) (string, error) {
	if s == nil {
		return "", fmt.Errorf("项目服务未初始化")
	}
	if s.store == nil {
		return "", fmt.Errorf("元数据存储未初始化")
	}
	if err := validateProjectID(id); err != nil {
		return "", err
	}
	return filepath.Join(s.projectsDir, id), nil
}

// isWithinDir 判断 target 是否位于 root 目录内（不含 root 本身）。
func isWithinDir(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	if rel == "." || filepath.IsAbs(rel) {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func cloneStrings(in []string) []string {
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func cloneInt64Map(in map[string]int64) map[string]int64 {
	out := make(map[string]int64, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// normalizeProject 补齐读入项目的 nil 集合，避免前端收到 null。
func normalizeProject(p *models.Project) {
	if p == nil {
		return
	}
	if p.Sources == nil {
		p.Sources = []models.LogSource{}
	}
	if p.Tags == nil {
		p.Tags = []string{}
	}
	if p.Levels == nil {
		p.Levels = map[string]int64{}
	}
}
