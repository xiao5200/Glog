package storage

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	sqlite "modernc.org/sqlite"

	"glog/internal/errs"
)

// SQLite 扩展结果码（modernc.org/sqlite 上报扩展码）。
const (
	codeConstraintPrimaryKey = 1555
	codeConstraintUnique     = 2067
	codeConstraintForeignKey = 787
)

// rowScanner 抽象 *sql.Row / *sql.Rows 的 Scan 方法。
type rowScanner interface {
	Scan(dest ...any) error
}

// execer 抽象 *sql.DB / *sql.Tx 的 Exec 方法。
type execer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

func nowUTC() time.Time { return time.Now().UTC() }

func generateID() string { return uuid.NewString() }

// formatTime 统一以 UTC RFC3339Nano 存储时间。
func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

// timeArg 用于不可空的 TEXT 时间列；零值写入 NULL。
func timeArg(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return formatTime(t)
}

// timePtrArg 用于可空的时间列。
func timePtrArg(t *time.Time) any {
	if t == nil || t.IsZero() {
		return nil
	}
	return formatTime(*t)
}

// nullIfEmpty 将空字符串映射为 SQL NULL。
func nullIfEmpty(s string) any {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return s
}

// parseDBTime 解析数据库中的时间字符串，兼容历史数据中可能出现的
// SQLite datetime 格式；无时区的输入按 UTC 解释。
func parseDBTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, errors.New("空时间值")
	}
	layouts := []string{
		time.RFC3339Nano,
		"2006-01-02T15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("无法解析时间 %q", s)
}

// scanTime 读取一个非空时间列。
func scanTime(raw string) (time.Time, error) {
	return parseDBTime(raw)
}

// scanTimePtr 读取一个可空时间列。
func scanTimePtr(ns sql.NullString) (*time.Time, error) {
	if !ns.Valid || strings.TrimSpace(ns.String) == "" {
		return nil, nil
	}
	t, err := parseDBTime(ns.String)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// marshalJSON 将值序列化为 JSON 字符串。
func marshalJSON(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// marshalJSONArg 将值序列化为可写入可空 TEXT 列的参数。
// nil、typed nil 指针、nil 切片等会被序列化为 "null"，这里统一转为 SQL NULL。
func marshalJSONArg(v any) (any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	if string(b) == "null" {
		return nil, nil
	}
	return string(b), nil
}

// decodeJSON 解码可空 JSON 列；NULL、空串、"null" 均视为无数据。
func decodeJSON(ns sql.NullString, dst any) error {
	if !ns.Valid {
		return nil
	}
	s := strings.TrimSpace(ns.String)
	if s == "" || s == "null" {
		return nil
	}
	if err := json.Unmarshal([]byte(ns.String), dst); err != nil {
		return err
	}
	return nil
}

// decodeJSONString 解码非空 JSON 列。
func decodeJSONString(s string, dst any) error {
	s = strings.TrimSpace(s)
	if s == "" || s == "null" {
		return nil
	}
	return json.Unmarshal([]byte(s), dst)
}

// decodeJSONPtr 解码可空 JSON 对象为指针。
func decodeJSONPtr[T any](ns sql.NullString) (*T, error) {
	if !ns.Valid {
		return nil, nil
	}
	s := strings.TrimSpace(ns.String)
	if s == "" || s == "null" {
		return nil, nil
	}
	var v T
	if err := json.Unmarshal([]byte(ns.String), &v); err != nil {
		return nil, err
	}
	return &v, nil
}

// nonNilStrings 返回非 nil 的字符串切片。
func nonNilStrings(v []string) []string {
	if v == nil {
		return []string{}
	}
	return v
}

// nonNilInt64Map 返回非 nil 的计数映射。
func nonNilInt64Map(v map[string]int64) map[string]int64 {
	if v == nil {
		return map[string]int64{}
	}
	return v
}

// likeContains 构造大小写不敏感的 LIKE 模糊匹配模式。
// 使用 '!' 作为转义字符，避免用户输入的 % 和 _ 被当作通配符。
func likeContains(s string) string {
	replacer := strings.NewReplacer("!", "!!", "%", "!%", "_", "!_")
	return "%" + replacer.Replace(s) + "%"
}

// sqliteErrCode 提取 SQLite 错误码。
func sqliteErrCode(err error) (int, bool) {
	var se *sqlite.Error
	if errors.As(err, &se) {
		return se.Code(), true
	}
	return 0, false
}

// isUniqueViolation 判断是否为主键/唯一约束冲突。
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	if code, ok := sqliteErrCode(err); ok {
		if code == codeConstraintPrimaryKey || code == codeConstraintUnique {
			return true
		}
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique constraint failed") ||
		strings.Contains(msg, "primary key constraint failed") ||
		strings.Contains(msg, "constraint failed: unique")
}

// isForeignKeyViolation 判断是否为外键约束冲突。
func isForeignKeyViolation(err error) bool {
	if err == nil {
		return false
	}
	if code, ok := sqliteErrCode(err); ok && code == codeConstraintForeignKey {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "foreign key constraint failed")
}

// notFound 构造统一的中文“不存在”错误，保留 errs.ErrNotFound 供 errors.Is 判断。
func notFound(entity string) error {
	return fmt.Errorf("%s不存在: %w", entity, errs.ErrNotFound)
}

// invalidInput 构造统一的中文“无效输入”错误。
func invalidInput(format string, args ...any) error {
	msg := fmt.Sprintf(format, args...)
	return fmt.Errorf("%s: %w", msg, errs.ErrInvalidInput)
}

// alreadyExists 构造统一的中文“已存在”错误。
func alreadyExists(entity, id string) error {
	if id == "" {
		return fmt.Errorf("%s已存在: %w", entity, errs.ErrAlreadyExists)
	}
	return fmt.Errorf("%s已存在（ID=%s）: %w", entity, id, errs.ErrAlreadyExists)
}
