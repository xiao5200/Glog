package app

import (
	"context"
	"encoding/json"
	"fmt"
	stdlog "log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	evtxlog "github.com/0xrawsec/golang-utils/log"
	"github.com/google/uuid"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"go.uber.org/zap"

	"glog/internal/ai"
	"glog/internal/config"
	"glog/internal/errs"
	"glog/internal/indexer"
	"glog/internal/investigation"
	"glog/internal/logx"
	"glog/internal/parser"
	"glog/internal/project"
	"glog/internal/query"
	"glog/internal/security"
	"glog/internal/storage"
	"glog/internal/threat"
	"glog/pkg/models"
)

// App 是 Wails 绑定的应用根对象，负责服务编排与长任务管理。
type App struct {
	ctx context.Context

	mu           sync.RWMutex
	collectMu    sync.Mutex
	configMgr    *config.Manager
	metadata     *storage.MetadataStore
	cipher       *security.Cipher
	projectsSvc  *project.Service
	parserEngine *parser.Engine
	aiMgr        *ai.Manager
	taskMgr      *taskManager

	currentProject   *models.Project
	indexStore       *indexer.Store
	queryEngine      *query.Engine
	investigationSvc *investigation.Service
	threatSvc        *threat.Service

	startupErr error
	logFile    string
}

// NewApp 创建应用实例。
func NewApp() *App {
	return &App{}
}

// Startup 由 Wails 在窗口启动时调用。
func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
	a.taskMgr = newTaskManager(a.emitEvent)

	cfgMgr, err := config.Load()
	if err != nil {
		a.failStartup(fmt.Errorf("加载配置失败: %w", err))
		return
	}
	a.configMgr = cfgMgr
	if err := cfgMgr.EnsureDirs(); err != nil {
		a.failStartup(fmt.Errorf("创建数据目录失败: %w", err))
		return
	}

	a.logFile = filepath.Join(cfgMgr.LogDir(), "glog.log")
	if err := logx.Init(a.logFile, false); err != nil {
		fmt.Fprintf(os.Stderr, "初始化日志失败: %v\n", err)
	}
	// 第三方 EVTX 库会向 stderr 打印内部恢复与堆栈；接管其标准日志输出，降级到 debug。
	initEVTXLibraryLogger()

	metadata, err := storage.Open(cfgMgr.DBPath())
	if err != nil {
		a.failStartup(fmt.Errorf("打开元数据库失败: %w", err))
		return
	}
	a.metadata = metadata

	cipher, err := security.NewCipher(filepath.Join(cfgMgr.DataDir(), "secrets.key"))
	if err != nil {
		a.failStartup(fmt.Errorf("初始化凭据加密失败: %w", err))
		return
	}
	a.cipher = cipher

	a.projectsSvc = project.New(metadata, cfgMgr.ProjectsDir())

	engine, err := parser.NewEngine()
	if err != nil {
		a.failStartup(fmt.Errorf("初始化解析引擎失败: %w", err))
		return
	}
	a.parserEngine = engine

	a.aiMgr = ai.NewManager(a.loadAIConfigs())

	a.seedBuiltinResources()

	logx.Info("GLog 启动完成", zap.String("data", cfgMgr.DataDir()))
}

// Shutdown 由 Wails 在应用退出时调用。
func (a *App) Shutdown(_ context.Context) {
	if a.taskMgr != nil {
		a.taskMgr.StopAllAndWait(10 * time.Second)
	}
	a.mu.Lock()
	if a.indexStore != nil {
		if err := a.indexStore.Close(); err != nil {
			logx.Warn("关闭索引失败", zap.Error(err))
		}
		a.indexStore = nil
	}
	if a.metadata != nil {
		_ = a.metadata.Close()
	}
	a.mu.Unlock()
	logx.Close()
}

type evtxLogSink struct{}

func (evtxLogSink) Write(p []byte) (int, error) {
	msg := strings.TrimSpace(string(p))
	if msg != "" {
		logx.Debug("evtx parser", zap.String("message", msg))
	}
	return len(p), nil
}

func initEVTXLibraryLogger() {
	defer func() { _ = recover() }()
	stdlog.SetOutput(evtxLogSink{})
	evtxlog.SetLogLevel(evtxlog.LError)
}

func (a *App) failStartup(err error) {
	a.startupErr = err
	logx.Error("启动失败", zap.Error(err))
	fmt.Fprintf(os.Stderr, "GLog 启动失败: %v\n", err)
}

func (a *App) emitEvent(name string, payload interface{}) {
	if a.ctx == nil || os.Getenv("GLOG_DISABLE_WAILS_EVENTS") == "1" {
		return
	}
	// 非 Wails 上下文（CLI/测试）没有前端事件总线；吞掉异常保证采集流程不受影响。
	defer func() { _ = recover() }()
	runtime.EventsEmit(a.ctx, name, payload)
}

func (a *App) ready() error {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.startupErr != nil {
		return errs.Internal("应用初始化失败", a.startupErr)
	}
	if a.metadata == nil || a.configMgr == nil {
		return errs.Internal("应用尚未初始化完成", nil)
	}
	return nil
}

// seedBuiltinResources 将内置模板与规则播种到元数据库。
func (a *App) seedBuiltinResources() {
	const seedKey = "builtin_seed_version"
	version, _ := a.metadata.GetSetting(seedKey)
	if version == config.AppVersion {
		return
	}

	for _, tpl := range a.parserEngine.Templates() {
		if tpl.ID == "" {
			continue
		}
		tpl.Type = "builtin"
		tpl.Source = "builtin"
		if tpl.CreatedAt.IsZero() {
			tpl.CreatedAt = time.Now().UTC()
		}
		tpl.UpdatedAt = time.Now().UTC()
		if err := a.metadata.SaveTemplate(&tpl); err != nil {
			logx.Warn("播种内置模板失败", zap.String("id", tpl.ID), zap.Error(err))
		}
	}

	// SeedBuiltinRules 不依赖项目 reader。
	threatSvc := threat.NewService(a.metadata, nil)
	if err := threatSvc.SeedBuiltinRules(); err != nil {
		logx.Warn("播种检测规则失败", zap.Error(err))
	}

	if err := a.metadata.SetSetting(seedKey, config.AppVersion); err != nil {
		logx.Warn("写入播种版本失败", zap.Error(err))
	}
}

func (a *App) loadAIConfigs() []ai.Config {
	defaults := aiProviderDefaults()
	records := []storage.AIConfigRecord{}
	if a.metadata != nil {
		if list, err := a.metadata.ListAIConfigRecords(); err == nil {
			records = list
		} else {
			logx.Warn("读取 AI 配置失败", zap.Error(err))
		}
	}

	byProvider := make(map[string]storage.AIConfigRecord, len(records))
	for _, rec := range records {
		byProvider[rec.Provider] = rec
	}

	timeout := time.Duration(60) * time.Second
	if a.configMgr != nil {
		sec := a.configMgr.Config().AI.RequestTimeoutSeconds
		if sec > 0 {
			timeout = time.Duration(sec) * time.Second
		}
	}

	configs := make([]ai.Config, 0, len(defaults))
	for provider, def := range defaults {
		cfg := ai.Config{
			Provider: provider,
			Endpoint: def.Endpoint,
			Model:    def.Model,
			Enabled:  false,
			Priority: def.Priority,
			Timeout:  timeout,
		}
		if rec, ok := byProvider[provider]; ok {
			if rec.Endpoint != "" {
				cfg.Endpoint = rec.Endpoint
			}
			if rec.Model != "" {
				cfg.Model = rec.Model
			}
			cfg.Enabled = rec.Enabled
			cfg.Priority = rec.Priority
			if rec.APIKeyEncrypted != "" && a.cipher != nil {
				if plain, err := a.cipher.Decrypt(rec.APIKeyEncrypted); err == nil {
					cfg.APIKey = plain
				} else {
					logx.Warn("解密 AI 密钥失败", zap.String("provider", provider), zap.Error(err))
				}
			}
			if rec.Parameters != "" {
				params := map[string]string{}
				if err := json.Unmarshal([]byte(rec.Parameters), &params); err == nil {
					cfg.Parameters = params
				}
			}
		}
		configs = append(configs, cfg)
	}
	return configs
}

func (a *App) reloadAI() {
	if a.aiMgr == nil {
		return
	}
	a.aiMgr.SetConfigs(a.loadAIConfigs())
}

type providerDefault struct {
	Name     string
	Endpoint string
	Model    string
	Priority int
}

func aiProviderDefaults() map[string]providerDefault {
	return map[string]providerDefault{
		"openai":   {Name: "OpenAI", Endpoint: "https://api.openai.com/v1", Model: "gpt-4o-mini", Priority: 30},
		"deepseek": {Name: "DeepSeek", Endpoint: "https://api.deepseek.com/v1", Model: "deepseek-chat", Priority: 50},
		"ollama":   {Name: "Ollama (本地)", Endpoint: "http://localhost:11434/v1", Model: "llama3", Priority: 100},
		"custom":   {Name: "自定义兼容接口", Endpoint: "", Model: "", Priority: 10},
	}
}

// queryIndex 给索引存储补上查询引擎需要的分词方法。
type queryIndex struct {
	*indexer.Store
}

func (q *queryIndex) Tokenize(text string) []string {
	return indexer.Tokenize(text)
}

// projectReader 将索引与查询引擎适配为调查/威胁模块需要的 Reader。
type projectReader struct {
	store  *indexer.Store
	engine *query.Engine
}

func (r *projectReader) Search(q models.Query) (*models.QueryResult, error) {
	if r.engine == nil {
		return nil, errs.ErrProjectNotOpen
	}
	if q.Limit <= 0 {
		q.Limit = 1000
	}
	return r.engine.Search(q)
}

func (r *projectReader) ReadDoc(docID uint32) (*models.LogEntry, error) {
	return r.store.ReadDoc(docID)
}

func (r *projectReader) ReadDocs(docIDs []uint32) ([]*models.LogEntry, error) {
	return r.store.ReadDocs(docIDs)
}

func (r *projectReader) DocCount() uint32 { return r.store.DocCount() }

func (r *projectReader) AllDocIDs() []uint32 { return r.store.AllDocIDs() }

// OpenProject 打开项目并加载索引。
func (a *App) OpenProject(projectID string) error {
	if err := a.ready(); err != nil {
		return err
	}
	if projectID == "" {
		return errs.InvalidInput("项目 ID 不能为空", nil)
	}

	a.mu.RLock()
	alreadyOpen := a.currentProject != nil && a.currentProject.ID == projectID && a.indexStore != nil
	a.mu.RUnlock()
	if alreadyOpen {
		return nil
	}
	if a.taskMgr != nil {
		if err := a.taskMgr.StopAllAndWait(10 * time.Second); err != nil {
			return errs.Internal("仍有采集任务正在停止，请稍后重试", err)
		}
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if a.currentProject != nil && a.currentProject.ID == projectID && a.indexStore != nil {
		return nil
	}

	p, err := a.projectsSvc.Get(projectID)
	if err != nil {
		return err
	}

	store, err := indexer.Open(p.ProjectPath)
	if err != nil {
		return errs.Internal("加载项目索引失败", err)
	}
	engine := query.NewEngine(&queryIndex{Store: store})
	reader := &projectReader{store: store, engine: engine}

	old := a.indexStore
	a.currentProject = p
	a.indexStore = store
	a.queryEngine = engine
	a.investigationSvc = investigation.NewService(a.metadata, reader)
	a.threatSvc = threat.NewService(a.metadata, reader)

	if old != nil {
		if err := old.Close(); err != nil {
			logx.Warn("关闭旧项目索引失败", zap.Error(err))
		}
	}
	logx.Info("项目已打开", zap.String("project", projectID))
	return nil
}

// CloseProject 关闭当前项目。
func (a *App) CloseProject() error {
	if err := a.ready(); err != nil {
		return err
	}
	if a.taskMgr != nil {
		if err := a.taskMgr.StopAllAndWait(10 * time.Second); err != nil {
			return errs.Internal("仍有采集任务正在停止，请稍后重试", err)
		}
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	if a.indexStore != nil {
		if err := a.indexStore.Close(); err != nil {
			logx.Warn("关闭索引失败", zap.Error(err))
		}
	}
	a.currentProject = nil
	a.indexStore = nil
	a.queryEngine = nil
	a.investigationSvc = nil
	a.threatSvc = nil
	return nil
}

// GetCurrentProject 返回当前打开项目。
func (a *App) GetCurrentProject() (*models.Project, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.currentProject == nil {
		return nil, nil
	}
	copy := *a.currentProject
	return &copy, nil
}

func (a *App) requireServices() (*investigation.Service, *threat.Service, error) {
	if err := a.ready(); err != nil {
		return nil, nil, err
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.investigationSvc == nil || a.threatSvc == nil {
		return nil, nil, errs.ErrProjectNotOpen
	}
	return a.investigationSvc, a.threatSvc, nil
}

func (a *App) requireCurrent(projectID string) (*models.Project, *indexer.Store, *query.Engine, error) {
	if err := a.ready(); err != nil {
		return nil, nil, nil, err
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.currentProject == nil || a.indexStore == nil || a.queryEngine == nil {
		return nil, nil, nil, errs.ErrProjectNotOpen
	}
	if projectID != "" && projectID != a.currentProject.ID {
		return nil, nil, nil, errs.InvalidInput("项目未打开或 ID 不匹配", fmt.Errorf("当前项目: %s", a.currentProject.ID))
	}
	copy := *a.currentProject
	return &copy, a.indexStore, a.queryEngine, nil
}

func (a *App) refreshCurrentProjectLocked() {
	if a.projectsSvc == nil || a.currentProject == nil {
		return
	}
	fresh, err := a.projectsSvc.Get(a.currentProject.ID)
	if err != nil {
		logx.Warn("刷新项目信息失败", zap.Error(err))
		return
	}
	a.currentProject = fresh
}

func newID() string { return uuid.NewString() }

func nowUTC() time.Time { return time.Now().UTC() }

func hostname() string {
	h, err := os.Hostname()
	if err != nil || h == "" {
		return "localhost"
	}
	return h
}

var _ = runtime.EventsEmit
