import type {
  AggregateRequest,
  AggregateResult,
  AIModelsRequest,
  AIModelTestRequest,
  AITemplateRequest,
  AnomalyDetectionRequest,
  AnomalyDetectionResult,
  AppConfig,
  ContextResult,
  CreateInvestigationRequest,
  CreateProjectRequest,
  DetectionResult,
  DetectionRule,
  DetectedLog,
  EvidenceItem,
  FileInfo,
  ForensicsRequest,
  LocalLogDiscovery,
  ForensicsResult,
  FormatDetection,
  IOCImportResult,
  IOCItem,
  IOCSearchRequest,
  IOCSearchResult,
  ImportFileRequest,
  Investigation,
  InvestigationUpdates,
  LocalCollectRequest,
  LogAnalysisRequest,
  LogAnalysisResult,
  LogEntry,
  LogSource,
  HostNetworkSnapshot,
  MiddlewarePreset,
  ParseTemplate,
  PatternClusterResult,
  PreviewParseRequest,
  PreviewResult,
  Project,
  ProjectFilter,
  ProjectStatistics,
  ProjectUpdates,
  Query,
  QueryResult,
  QuerySnapshot,
  ReportRequest,
  ReportResult,
  RootCauseRequest,
  RootCauseResult,
  RunDetectionRequest,
  SaveAIConfigRequest,
  SaveSSHHostRequest,
  SSHCollectRequest,
  SSHHost,
  SSHLogSearchRequest,
  SSHLogSearchResult,
  SystemdJournalRequest,
  SystemInfo,
  TaskInfo,
  TemplateFilter,
  TimelineEvent,
  WindowsEventLogRequest,
  AIProviderInfo,
} from './types'

export class WailsUnavailableError extends Error {
  constructor() {
    super('Wails 运行时不可用：请通过 GLog 桌面应用启动，而不是普通浏览器。')
    this.name = 'WailsUnavailableError'
  }
}

type WailsBinding = Record<string, (...args: unknown[]) => Promise<unknown>>

function binding(): WailsBinding {
  const w = window as unknown as {
    go?: { app?: { App?: WailsBinding } }
  }
  const b = w.go?.app?.App
  if (!b) throw new WailsUnavailableError()
  return b
}

/** 是否运行在 Wails 容器内。 */
export function isWails(): boolean {
  const w = window as unknown as { go?: { app?: { App?: unknown } } }
  return Boolean(w.go?.app?.App)
}

function call<T>(method: string, ...args: unknown[]): Promise<T> {
  const b = binding()
  const fn = b[method]
  if (typeof fn !== 'function') {
    return Promise.reject(new Error(`后端方法不存在：${method}`))
  }
  return fn(...args) as Promise<T>
}

/** 订阅 Wails 事件，返回取消函数。 */
export function onEvent<T>(name: string, handler: (data: T) => void): () => void {
  const runtime = (window as unknown as {
    runtime?: {
      EventsOn?: (event: string, cb: (data: T) => void) => unknown
      EventsOff?: (event: string) => void
    }
  }).runtime
  if (!runtime?.EventsOn) return () => {}
  const off = runtime.EventsOn(name, handler)
  if (typeof off === 'function') return off as () => void
  return () => runtime.EventsOff?.(name)
}

export function errorText(err: unknown): string {
  if (err == null) return '未知错误'
  if (typeof err === 'string') return err
  if (err instanceof Error) return err.message
  if (typeof err === 'object' && 'message' in err) {
    const message = (err as { message?: unknown }).message
    if (typeof message === 'string') return message
  }
  return JSON.stringify(err)
}

export const api = {
  system: {
    info: () => call<SystemInfo>('GetSystemInfo'),
    hostNetwork: () => call<HostNetworkSnapshot>('GetHostNetworkSnapshot'),
    config: () => call<AppConfig>('GetConfig'),
    updateConfig: (cfg: AppConfig) => call<void>('UpdateConfig', cfg),
    selectFiles: () => call<string[]>('SelectFiles'),
    selectDirectory: (title?: string) => call<string>('SelectDirectory', title ?? ''),
    selectSaveFile: (defaultFilename: string, displayName?: string, pattern?: string) =>
      call<string>(
        'SelectSaveFile',
        defaultFilename,
        displayName ?? '导出文件',
        pattern ?? '*.*',
      ),
    showInExplorer: (path: string) => call<void>('ShowInExplorer', path),
    openExternal: (url: string) => call<void>('OpenExternal', url),
  },

  projects: {
    create: (req: CreateProjectRequest) => call<Project>('CreateProject', req),
    list: (filter: ProjectFilter = {}) => call<Project[]>('ListProjects', filter),
    get: (id: string) => call<Project>('GetProject', id),
    update: (id: string, updates: ProjectUpdates) => call<Project>('UpdateProject', id, updates),
    remove: (id: string) => call<void>('DeleteProject', id),
    archive: (id: string) => call<void>('ArchiveProject', id),
    exportPackage: (id: string, outputPath = '') => call<string>('ExportProject', id, outputPath),
    importPackage: (packagePath: string) => call<Project>('ImportProject', packagePath),
    open: (id: string) => call<void>('OpenProject', id),
    close: () => call<void>('CloseProject'),
    current: () => call<Project | null>('GetCurrentProject'),
    statistics: (id: string) => call<ProjectStatistics>('GetProjectStatistics', id),
    sources: (projectID: string) => call<LogSource[]>('GetProjectSources', projectID),
  },

  collect: {
    startLocal: (req: LocalCollectRequest) => call<TaskInfo>('StartLocalCollection', req),
    startSSH: (req: SSHCollectRequest) => call<TaskInfo>('StartSSHCollection', req),
    startWindowsEvent: (req: WindowsEventLogRequest) =>
      call<TaskInfo>('StartWindowsEventCollection', req),
    startJournal: (req: SystemdJournalRequest) =>
      call<TaskInfo>('StartJournalCollection', req),
    importFile: (req: ImportFileRequest) => call<TaskInfo>('ImportFile', req),
    stop: (taskID: string) => call<void>('StopCollection', taskID),
    tasks: () => call<TaskInfo[]>('ListTasks'),
    browse: (path: string) => call<FileInfo[]>('BrowseLocalDirectory', path),
    previewFile: (path: string, lines = 100) => call<string[]>('GetLogFilePreview', path, lines),
    commonPaths: () => call<Record<string, string[]>>('GetLocalCommonLogPaths'),
    validatePath: (path: string) => call<boolean>('ValidateLocalPath', path),
    presets: () => call<MiddlewarePreset[]>('GetMiddlewarePresets'),
    detectLocalLogs: () => call<LocalLogDiscovery>('DetectLocalLogs'),
  },

  ssh: {
    hosts: () => call<SSHHost[]>('GetSSHHosts'),
    save: (req: SaveSSHHostRequest) => call<void>('SaveSSHHost', req),
    remove: (id: string) => call<void>('DeleteSSHHost', id),
    test: (hostID: string) => call<void>('TestSSHConnection', hostID),
    searchLogs: (req: SSHLogSearchRequest) =>
      call<SSHLogSearchResult>('SearchSSHLogs', req),
  },

  templates: {
    list: (filter: TemplateFilter = {}) => call<ParseTemplate[]>('GetTemplates', filter),
    get: (id: string) => call<ParseTemplate>('GetTemplate', id),
    save: (template: ParseTemplate) => call<void>('SaveTemplate', template),
    remove: (id: string) => call<void>('DeleteTemplate', id),
    detectFormat: (samples: string[]) => call<FormatDetection>('DetectFormat', samples),
    preview: (req: PreviewParseRequest) => call<PreviewResult>('PreviewParse', req),
    generateAI: (req: AITemplateRequest) => call<ParseTemplate>('GenerateTemplateByAI', req),
  },

  query: {
    search: (projectID: string, query: Query) => call<QueryResult>('Search', projectID, query),
    getEntry: (projectID: string, docID: number) =>
      call<LogEntry>('GetLogEntry', projectID, docID),
    context: (projectID: string, docID: number, before = 10, after = 10) =>
      call<ContextResult>('GetLogContext', projectID, docID, before, after),
    aggregate: (projectID: string, req: AggregateRequest) =>
      call<AggregateResult>('Aggregate', projectID, req),
    cluster: (projectID: string, query: Query) =>
      call<PatternClusterResult>('ClusterPatterns', projectID, query),
    snapshots: (projectID: string) => call<QuerySnapshot[]>('GetQuerySnapshots', projectID),
    saveSnapshot: (snapshot: QuerySnapshot) => call<void>('SaveQuerySnapshot', snapshot),
    deleteSnapshot: (id: string) => call<void>('DeleteQuerySnapshot', id),
  },

  ai: {
    providers: () => call<AIProviderInfo[]>('GetAIProviders'),
    saveConfig: (req: SaveAIConfigRequest) => call<void>('SaveAIConfig', req),
    test: (provider: string) => call<void>('TestAIConnection', provider),
    listModels: (req: AIModelsRequest) => call<string[]>('ListAIModels', req),
    testModel: (req: AIModelTestRequest) => call<void>('TestAIModel', req),
    analyzeLogEntry: (req: LogAnalysisRequest) =>
      call<LogAnalysisResult>('AnalyzeLogEntry', req),
    detectAnomaly: (req: AnomalyDetectionRequest) =>
      call<AnomalyDetectionResult>('DetectAnomaly', req),
    rootCause: (req: RootCauseRequest) => call<RootCauseResult>('AnalyzeRootCause', req),
    nlQuery: (projectID: string, question: string) =>
      call<Query>('NaturalLanguageQuery', projectID, question),
    report: (req: ReportRequest) => call<ReportResult>('GenerateReport', req),
  },

  investigations: {
    create: (req: CreateInvestigationRequest) =>
      call<Investigation>('CreateInvestigation', req),
    update: (id: string, updates: InvestigationUpdates) =>
      call<Investigation>('UpdateInvestigation', id, updates),
    get: (id: string) => call<Investigation>('GetInvestigation', id),
    list: (projectID: string) => call<Investigation[]>('ListInvestigations', projectID),
    close: (id: string) => call<void>('CloseInvestigation', id),
    markEvidence: (invID: string, docID: number, note: string) =>
      call<void>('MarkEvidence', invID, docID, note),
    unmarkEvidence: (invID: string, docID: number) =>
      call<void>('UnmarkEvidence', invID, docID),
    evidence: (invID: string) => call<EvidenceItem[]>('ListEvidence', invID),
    timeline: (invID: string) => call<TimelineEvent[]>('GetInvestigationTimeline', invID),
    analyze: (invID: string, provider: string) =>
      call<void>('AnalyzeInvestigation', invID, provider),
    exportEvidence: (invID: string, outputPath = '') =>
      call<string>('ExportEvidence', invID, outputPath),
    relatedLogs: (invID: string, limit = 200) =>
      call<LogEntry[]>('GetInvestigationRelatedLogs', invID, limit),
    remove: (id: string) => call<void>('DeleteInvestigation', id),
  },

  threat: {
    importIOC: (filePath: string) => call<IOCImportResult>('ImportIOC', filePath),
    iocs: () => call<IOCItem[]>('ListIOCs'),
    removeIOC: (id: string) => call<void>('DeleteIOC', id),
    searchIOC: (req: IOCSearchRequest) => call<IOCSearchResult>('SearchIOC', req),
    rules: () => call<DetectionRule[]>('GetDetectionRules'),
    enableRule: (id: string) => call<void>('EnableRule', id),
    disableRule: (id: string) => call<void>('DisableRule', id),
    runDetection: (req: RunDetectionRequest) =>
      call<DetectionResult>('RunDetection', req),
    forensics: (req: ForensicsRequest) =>
      call<ForensicsResult>('GenerateForensicsPackage', req),
  },
}

// 方便页面统一使用
export type Api = typeof api
