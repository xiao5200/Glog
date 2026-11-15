package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"glog/internal/errs"
	"glog/pkg/models"
)

const sshHostSelectColumns = `id, name, host, COALESCE(port, 22), username,
	COALESCE(auth_method, 'password'), password_encrypted, COALESCE(key_path, ''),
	COALESCE(proxy_jump, ''), last_used_at`

func scanSSHHost(row rowScanner) (*models.SSHHost, string, error) {
	var (
		host     models.SSHHost
		pwd      sql.NullString
		lastUsed sql.NullString
	)
	err := row.Scan(
		&host.ID, &host.Name, &host.Host, &host.Port, &host.Username,
		&host.AuthMethod, &pwd, &host.KeyPath, &host.ProxyJump, &lastUsed,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, "", fmt.Errorf("SSH主机不存在: %w", errs.ErrNotFound)
		}
		return nil, "", fmt.Errorf("读取SSH主机失败: %w", err)
	}

	lastUsedAt, err := scanTimePtr(lastUsed)
	if err != nil {
		return nil, "", fmt.Errorf("解析SSH主机最后使用时间失败: %w", err)
	}
	host.LastUsedAt = lastUsedAt
	encPwd := ""
	if pwd.Valid {
		encPwd = pwd.String
	}
	host.HasPassword = encPwd != ""
	return &host, encPwd, nil
}

// SaveSSHHost 保存 SSH 主机（按 ID 插入或更新）。
// encPwd 为已加密的密码；更新时传入空串表示保留原密码。
func (s *MetadataStore) SaveSSHHost(host *models.SSHHost, encPwd string) error {
	if host == nil {
		return invalidInput("SSH主机不能为空")
	}
	if strings.TrimSpace(host.Name) == "" {
		return invalidInput("SSH主机名称不能为空")
	}
	if strings.TrimSpace(host.Host) == "" {
		return invalidInput("SSH主机地址不能为空")
	}
	if strings.TrimSpace(host.Username) == "" {
		return invalidInput("SSH用户名不能为空")
	}
	if strings.TrimSpace(host.ID) == "" {
		host.ID = generateID()
	}
	port := host.Port
	if port <= 0 || port > 65535 {
		port = 22
	}
	authMethod := strings.TrimSpace(host.AuthMethod)
	if authMethod == "" {
		authMethod = "password"
	}

	createdAt := nowUTC()
	_, err := s.db.Exec(
		`INSERT INTO ssh_hosts
			(id, name, host, port, username, auth_method, password_encrypted,
			 key_path, proxy_jump, last_used_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			host = excluded.host,
			port = excluded.port,
			username = excluded.username,
			auth_method = excluded.auth_method,
			password_encrypted = CASE
				WHEN excluded.password_encrypted IS NULL OR excluded.password_encrypted = ''
				THEN ssh_hosts.password_encrypted
				ELSE excluded.password_encrypted
			END,
			key_path = excluded.key_path,
			proxy_jump = excluded.proxy_jump,
			last_used_at = COALESCE(excluded.last_used_at, ssh_hosts.last_used_at)`,
		host.ID, host.Name, host.Host, port, host.Username, authMethod,
		nullIfEmpty(encPwd), host.KeyPath, host.ProxyJump, timePtrArg(host.LastUsedAt),
		formatTime(createdAt),
	)
	if err != nil {
		return fmt.Errorf("保存SSH主机失败: %w", err)
	}
	host.HasPassword = host.HasPassword || encPwd != ""
	return nil
}

// GetSSHHost 读取 SSH 主机及加密密码。
func (s *MetadataStore) GetSSHHost(id string) (*models.SSHHost, string, error) {
	if strings.TrimSpace(id) == "" {
		return nil, "", invalidInput("SSH主机ID不能为空")
	}
	row := s.db.QueryRow("SELECT "+sshHostSelectColumns+" FROM ssh_hosts WHERE id = ?", id)
	host, encPwd, err := scanSSHHost(row)
	if err != nil {
		return nil, "", err
	}
	return host, encPwd, nil
}

// ListSSHHosts 列出全部 SSH 主机（按最后使用时间倒序）。
func (s *MetadataStore) ListSSHHosts() ([]models.SSHHost, error) {
	rows, err := s.db.Query(
		"SELECT " + sshHostSelectColumns + ` FROM ssh_hosts
		 ORDER BY last_used_at IS NULL, last_used_at DESC, name COLLATE NOCASE ASC, id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("查询SSH主机列表失败: %w", err)
	}
	defer rows.Close()

	out := make([]models.SSHHost, 0, 8)
	for rows.Next() {
		host, _, err := scanSSHHost(rows)
		if err != nil {
			return nil, fmt.Errorf("读取SSH主机列表失败: %w", err)
		}
		out = append(out, *host)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历SSH主机列表失败: %w", err)
	}
	return out, nil
}

// DeleteSSHHost 删除 SSH 主机。
func (s *MetadataStore) DeleteSSHHost(id string) error {
	if strings.TrimSpace(id) == "" {
		return invalidInput("SSH主机ID不能为空")
	}
	res, err := s.db.Exec("DELETE FROM ssh_hosts WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("删除SSH主机失败: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("读取SSH主机删除结果失败: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("SSH主机不存在（ID=%s）: %w", id, errs.ErrNotFound)
	}
	return nil
}

// TouchSSHHost 更新最后使用时间。
func (s *MetadataStore) TouchSSHHost(id string) error {
	if strings.TrimSpace(id) == "" {
		return invalidInput("SSH主机ID不能为空")
	}
	res, err := s.db.Exec("UPDATE ssh_hosts SET last_used_at = ? WHERE id = ?", formatTime(nowUTC()), id)
	if err != nil {
		return fmt.Errorf("更新SSH主机使用时间失败: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("读取SSH主机更新结果失败: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("SSH主机不存在（ID=%s）: %w", id, errs.ErrNotFound)
	}
	return nil
}
