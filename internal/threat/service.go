// Package threat 实现 IOC 导入/检索、YAML 检测规则、规则检测与取证包生成。
//
// 本包通过 Store/Reader 接口与持久化和日志检索解耦，不直接依赖
// internal/storage、internal/indexer 或 internal/query。
package threat

import (
	"time"

	"glog/pkg/models"
)

const (
	iocCandidateLimit = 1000
	iocResultLimit    = 2000
	maxIOCErrors      = 100
	maxIOCFileBytes   = 50 << 20 // 50 MiB

	detectionCandidateLimit = 200000
	detectionMatchLimit     = 200

	maxRuleFileBytes = 5 << 20 // 5 MiB

	maxForensicsEntries  = 5000
	maxForensicsZipBytes = int64(1 << 30) // 1 GiB
)

// Store 是 IOC 与检测规则的持久化接口。
type Store interface {
	SaveIOC(item *models.IOCItem) error
	ListIOCs() ([]models.IOCItem, error)
	DeleteIOC(id string) error
	FindIOCByValue(value string) (*models.IOCItem, error)
	ListDetectionRules() ([]models.DetectionRule, error)
	UpsertDetectionRule(rule *models.DetectionRule) error
	SetDetectionRuleEnabled(id string, enabled bool) error
	UpdateDetectionRuleMatch(id string, matchedAt time.Time) error
}

// Reader 是日志检索接口，通常由 query 服务实现。
type Reader interface {
	Search(q models.Query) (*models.QueryResult, error)
	ReadDoc(docID uint32) (*models.LogEntry, error)
	ReadDocs(docIDs []uint32) ([]*models.LogEntry, error)
	DocCount() uint32
	AllDocIDs() []uint32
}

// Service 威胁狩猎服务。
type Service struct {
	store  Store
	reader Reader
}

// NewService 创建威胁狩猎服务。
func NewService(store Store, reader Reader) *Service {
	return &Service{store: store, reader: reader}
}
