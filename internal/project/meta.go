package project

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"glog/internal/errs"
	"glog/pkg/models"
)

// WriteMeta 原子写入 project.meta（同目录临时文件 + rename）。
func (s *Service) WriteMeta(p *models.Project) error {
	if p == nil {
		return errs.InvalidInput("项目不能为空", nil)
	}
	if err := validateProjectID(p.ID); err != nil {
		return errs.InvalidInput("项目ID无效", err)
	}
	dir, err := s.projectDirChecked(p.ID)
	if err != nil {
		return errs.Internal("项目服务未初始化", err)
	}

	copyProject := *p
	if copyProject.ProjectPath == "" {
		copyProject.ProjectPath = dir
	}
	normalizeProject(&copyProject)

	data, err := json.MarshalIndent(&copyProject, "", "  ")
	if err != nil {
		return errs.Internal("序列化项目元数据失败", err)
	}
	data = append(data, '\n')

	metaPath := filepath.Join(dir, "project.meta")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return errs.Internal("创建项目目录失败", err)
	}
	if err := writeFileAtomic(metaPath, data, 0o644); err != nil {
		return errs.Internal("写入项目元数据失败", err)
	}
	return nil
}

// ReadMeta 读取并校验 project.meta。
func (s *Service) ReadMeta(id string) (*models.Project, error) {
	metaPath := s.MetaPath(id)
	if metaPath == "" {
		return nil, errs.InvalidInput("项目ID无效", nil)
	}

	data, err := os.ReadFile(metaPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, errs.NotFound("项目元数据文件不存在", err)
		}
		return nil, errs.Internal("读取项目元数据失败", err)
	}

	var p models.Project
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, errs.Internal("解析项目元数据失败",
			fmt.Errorf("%w: %v", errs.ErrProjectCorrupted, err))
	}
	if strings.TrimSpace(p.ID) == "" || p.ID != id {
		return nil, errs.Internal("项目元数据损坏：ID不匹配",
			fmt.Errorf("%w: 期望 %q，实际 %q", errs.ErrProjectCorrupted, id, p.ID))
	}
	if strings.TrimSpace(p.Name) == "" {
		return nil, errs.Internal("项目元数据损坏：名称为空",
			fmt.Errorf("%w: 项目 %s", errs.ErrProjectCorrupted, id))
	}
	normalizeProject(&p)
	return &p, nil
}

// writeFileAtomic 通过同目录临时文件 + rename 原子写入文件。
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := f.Name()
	cleanup := func() {
		_ = f.Close()
		_ = os.Remove(tmpName)
	}

	if err := f.Chmod(perm); err != nil {
		cleanup()
		return err
	}
	if _, err := f.Write(data); err != nil {
		cleanup()
		return err
	}
	if err := f.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}
