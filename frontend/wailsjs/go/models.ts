export namespace models {
	
	export class AttackPhase {
	    phase: string;
	    description: string;
	    evidence: string[];
	
	    static createFrom(source: any = {}) {
	        return new AttackPhase(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.phase = source["phase"];
	        this.description = source["description"];
	        this.evidence = source["evidence"];
	    }
	}
	export class TimelineEvent {
	    // Go type: time
	    timestamp: any;
	    event: string;
	    source: string;
	    severity: string;
	    log_entry_ids: string[];
	
	    static createFrom(source: any = {}) {
	        return new TimelineEvent(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.timestamp = this.convertValues(source["timestamp"], null);
	        this.event = source["event"];
	        this.source = source["source"];
	        this.severity = source["severity"];
	        this.log_entry_ids = source["log_entry_ids"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class AIAnalysisResult {
	    timeline: TimelineEvent[];
	    attack_phases: AttackPhase[];
	    risk_level: string;
	    findings: string[];
	    suggestions: string[];
	    // Go type: time
	    generated_at: any;
	
	    static createFrom(source: any = {}) {
	        return new AIAnalysisResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.timeline = this.convertValues(source["timeline"], TimelineEvent);
	        this.attack_phases = this.convertValues(source["attack_phases"], AttackPhase);
	        this.risk_level = source["risk_level"];
	        this.findings = source["findings"];
	        this.suggestions = source["suggestions"];
	        this.generated_at = this.convertValues(source["generated_at"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class AIConfigSection {
	    default_provider: string;
	    default_sample_size: number;
	    max_sample_size: number;
	    request_timeout_seconds: number;
	    max_logs_per_request: number;
	
	    static createFrom(source: any = {}) {
	        return new AIConfigSection(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.default_provider = source["default_provider"];
	        this.default_sample_size = source["default_sample_size"];
	        this.max_sample_size = source["max_sample_size"];
	        this.request_timeout_seconds = source["request_timeout_seconds"];
	        this.max_logs_per_request = source["max_logs_per_request"];
	    }
	}
	export class AIModelTestRequest {
	    provider: string;
	    endpoint?: string;
	    model: string;
	    api_key?: string;
	
	    static createFrom(source: any = {}) {
	        return new AIModelTestRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provider = source["provider"];
	        this.endpoint = source["endpoint"];
	        this.model = source["model"];
	        this.api_key = source["api_key"];
	    }
	}
	export class AIModelsRequest {
	    provider: string;
	    endpoint?: string;
	    api_key?: string;
	
	    static createFrom(source: any = {}) {
	        return new AIModelsRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provider = source["provider"];
	        this.endpoint = source["endpoint"];
	        this.api_key = source["api_key"];
	    }
	}
	export class AIProviderInfo {
	    provider: string;
	    name: string;
	    endpoint: string;
	    model: string;
	    enabled: boolean;
	    priority: number;
	    has_key: boolean;
	    configured: boolean;
	    // Go type: time
	    updated_at: any;
	
	    static createFrom(source: any = {}) {
	        return new AIProviderInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provider = source["provider"];
	        this.name = source["name"];
	        this.endpoint = source["endpoint"];
	        this.model = source["model"];
	        this.enabled = source["enabled"];
	        this.priority = source["priority"];
	        this.has_key = source["has_key"];
	        this.configured = source["configured"];
	        this.updated_at = this.convertValues(source["updated_at"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class AITemplateRequest {
	    samples: string[];
	    provider: string;
	    template_name: string;
	    category: string;
	
	    static createFrom(source: any = {}) {
	        return new AITemplateRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.samples = source["samples"];
	        this.provider = source["provider"];
	        this.template_name = source["template_name"];
	        this.category = source["category"];
	    }
	}
	export class TopItem {
	    key: string;
	    count: number;
	
	    static createFrom(source: any = {}) {
	        return new TopItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.count = source["count"];
	    }
	}
	export class AggregateDimension {
	    field: string;
	    items: TopItem[];
	    total: number;
	
	    static createFrom(source: any = {}) {
	        return new AggregateDimension(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.field = source["field"];
	        this.items = this.convertValues(source["items"], TopItem);
	        this.total = source["total"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class FieldFilter {
	    field: string;
	    operator: string;
	    value: any;
	
	    static createFrom(source: any = {}) {
	        return new FieldFilter(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.field = source["field"];
	        this.operator = source["operator"];
	        this.value = source["value"];
	    }
	}
	export class Query {
	    text?: string;
	    // Go type: time
	    start_time?: any;
	    // Go type: time
	    end_time?: any;
	    levels?: string[];
	    hosts?: string[];
	    sources?: string[];
	    event_ids?: number[];
	    field_filters?: FieldFilter[];
	    sort_by?: string;
	    sort_order?: string;
	    offset: number;
	    limit: number;
	
	    static createFrom(source: any = {}) {
	        return new Query(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.text = source["text"];
	        this.start_time = this.convertValues(source["start_time"], null);
	        this.end_time = this.convertValues(source["end_time"], null);
	        this.levels = source["levels"];
	        this.hosts = source["hosts"];
	        this.sources = source["sources"];
	        this.event_ids = source["event_ids"];
	        this.field_filters = this.convertValues(source["field_filters"], FieldFilter);
	        this.sort_by = source["sort_by"];
	        this.sort_order = source["sort_order"];
	        this.offset = source["offset"];
	        this.limit = source["limit"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class AggregateRequest {
	    query: Query;
	    aggregate_by: string[];
	    top_n: number;
	    time_bucket?: string;
	
	    static createFrom(source: any = {}) {
	        return new AggregateRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.query = this.convertValues(source["query"], Query);
	        this.aggregate_by = source["aggregate_by"];
	        this.top_n = source["top_n"];
	        this.time_bucket = source["time_bucket"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class DistributionItem {
	    key: string;
	    count: number;
	    percentage: number;
	
	    static createFrom(source: any = {}) {
	        return new DistributionItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.count = source["count"];
	        this.percentage = source["percentage"];
	    }
	}
	export class TimeSeriesPoint {
	    // Go type: time
	    timestamp: any;
	    count: number;
	
	    static createFrom(source: any = {}) {
	        return new TimeSeriesPoint(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.timestamp = this.convertValues(source["timestamp"], null);
	        this.count = source["count"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class AggregateResult {
	    time_series?: TimeSeriesPoint[];
	    distribution?: DistributionItem[];
	    top_items?: TopItem[];
	    total: number;
	    took: number;
	
	    static createFrom(source: any = {}) {
	        return new AggregateResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.time_series = this.convertValues(source["time_series"], TimeSeriesPoint);
	        this.distribution = this.convertValues(source["distribution"], DistributionItem);
	        this.top_items = this.convertValues(source["top_items"], TopItem);
	        this.total = source["total"];
	        this.took = source["took"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Anomaly {
	    // Go type: time
	    timestamp: any;
	    level: string;
	    message: string;
	    reason: string;
	    score: number;
	    doc_ids?: number[];
	
	    static createFrom(source: any = {}) {
	        return new Anomaly(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.timestamp = this.convertValues(source["timestamp"], null);
	        this.level = source["level"];
	        this.message = source["message"];
	        this.reason = source["reason"];
	        this.score = source["score"];
	        this.doc_ids = source["doc_ids"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class AnomalyDetectionRequest {
	    project_id: string;
	    provider: string;
	    query: Query;
	    max_logs: number;
	    aggregate_by?: string[];
	
	    static createFrom(source: any = {}) {
	        return new AnomalyDetectionRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.project_id = source["project_id"];
	        this.provider = source["provider"];
	        this.query = this.convertValues(source["query"], Query);
	        this.max_logs = source["max_logs"];
	        this.aggregate_by = source["aggregate_by"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class AnomalyDetectionResult {
	    summary: string;
	    risk_level: string;
	    findings: string[];
	    suggestions: string[];
	    anomalies: Anomaly[];
	    aggregates?: AggregateDimension[];
	    analyzed: number;
	    provider: string;
	    raw_response?: string;
	    // Go type: time
	    generated_at: any;
	
	    static createFrom(source: any = {}) {
	        return new AnomalyDetectionResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.summary = source["summary"];
	        this.risk_level = source["risk_level"];
	        this.findings = source["findings"];
	        this.suggestions = source["suggestions"];
	        this.anomalies = this.convertValues(source["anomalies"], Anomaly);
	        this.aggregates = this.convertValues(source["aggregates"], AggregateDimension);
	        this.analyzed = source["analyzed"];
	        this.provider = source["provider"];
	        this.raw_response = source["raw_response"];
	        this.generated_at = this.convertValues(source["generated_at"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class UIConfig {
	    theme: string;
	    language: string;
	    log_page_size: number;
	    virtual_scroll_overscan: number;
	
	    static createFrom(source: any = {}) {
	        return new UIConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.theme = source["theme"];
	        this.language = source["language"];
	        this.log_page_size = source["log_page_size"];
	        this.virtual_scroll_overscan = source["virtual_scroll_overscan"];
	    }
	}
	export class ParserConfig {
	    multiline_timeout_ms: number;
	    multiline_max_lines: number;
	    default_time_format: string;
	
	    static createFrom(source: any = {}) {
	        return new ParserConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.multiline_timeout_ms = source["multiline_timeout_ms"];
	        this.multiline_max_lines = source["multiline_max_lines"];
	        this.default_time_format = source["default_time_format"];
	    }
	}
	export class IndexerConfig {
	    index_flush_interval_seconds: number;
	    index_batch_size: number;
	    compression_algorithm: string;
	    bloom_filter_size: number;
	    bloom_filter_hash_functions: number;
	
	    static createFrom(source: any = {}) {
	        return new IndexerConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.index_flush_interval_seconds = source["index_flush_interval_seconds"];
	        this.index_batch_size = source["index_batch_size"];
	        this.compression_algorithm = source["compression_algorithm"];
	        this.bloom_filter_size = source["bloom_filter_size"];
	        this.bloom_filter_hash_functions = source["bloom_filter_hash_functions"];
	    }
	}
	export class CollectorConfig {
	    ssh_timeout_seconds: number;
	    ssh_concurrency: number;
	    ssh_compression: string;
	    local_buffer_size_kb: number;
	    local_batch_size: number;
	
	    static createFrom(source: any = {}) {
	        return new CollectorConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ssh_timeout_seconds = source["ssh_timeout_seconds"];
	        this.ssh_concurrency = source["ssh_concurrency"];
	        this.ssh_compression = source["ssh_compression"];
	        this.local_buffer_size_kb = source["local_buffer_size_kb"];
	        this.local_batch_size = source["local_batch_size"];
	    }
	}
	export class PerformanceConfig {
	    max_goroutines: number;
	    parser_workers: number;
	    indexer_workers: number;
	    ai_task_workers: number;
	    max_memory_mb: number;
	    cache_size_mb: number;
	    lru_cache_items: number;
	    query_timeout_seconds: number;
	    query_cache_ttl_minutes: number;
	
	    static createFrom(source: any = {}) {
	        return new PerformanceConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.max_goroutines = source["max_goroutines"];
	        this.parser_workers = source["parser_workers"];
	        this.indexer_workers = source["indexer_workers"];
	        this.ai_task_workers = source["ai_task_workers"];
	        this.max_memory_mb = source["max_memory_mb"];
	        this.cache_size_mb = source["cache_size_mb"];
	        this.lru_cache_items = source["lru_cache_items"];
	        this.query_timeout_seconds = source["query_timeout_seconds"];
	        this.query_cache_ttl_minutes = source["query_cache_ttl_minutes"];
	    }
	}
	export class GeneralConfig {
	    version: string;
	    data_path: string;
	    temp_path: string;
	    log_path: string;
	
	    static createFrom(source: any = {}) {
	        return new GeneralConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.data_path = source["data_path"];
	        this.temp_path = source["temp_path"];
	        this.log_path = source["log_path"];
	    }
	}
	export class AppConfig {
	    general: GeneralConfig;
	    performance: PerformanceConfig;
	    collector: CollectorConfig;
	    indexer: IndexerConfig;
	    parser: ParserConfig;
	    ai: AIConfigSection;
	    ui: UIConfig;
	
	    static createFrom(source: any = {}) {
	        return new AppConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.general = this.convertValues(source["general"], GeneralConfig);
	        this.performance = this.convertValues(source["performance"], PerformanceConfig);
	        this.collector = this.convertValues(source["collector"], CollectorConfig);
	        this.indexer = this.convertValues(source["indexer"], IndexerConfig);
	        this.parser = this.convertValues(source["parser"], ParserConfig);
	        this.ai = this.convertValues(source["ai"], AIConfigSection);
	        this.ui = this.convertValues(source["ui"], UIConfig);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class CollectResult {
	    task_id: string;
	    project_id: string;
	    source_id: string;
	    source_ids: string[];
	    file_count: number;
	    total_bytes: number;
	    log_count: number;
	    failed_count: number;
	    duration: number;
	    status: string;
	    error_message?: string;
	    // Go type: time
	    finished_at: any;
	
	    static createFrom(source: any = {}) {
	        return new CollectResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.task_id = source["task_id"];
	        this.project_id = source["project_id"];
	        this.source_id = source["source_id"];
	        this.source_ids = source["source_ids"];
	        this.file_count = source["file_count"];
	        this.total_bytes = source["total_bytes"];
	        this.log_count = source["log_count"];
	        this.failed_count = source["failed_count"];
	        this.duration = source["duration"];
	        this.status = source["status"];
	        this.error_message = source["error_message"];
	        this.finished_at = this.convertValues(source["finished_at"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class LogEntry {
	    doc_id: number;
	    id: string;
	    // Go type: time
	    timestamp: any;
	    level: string;
	    message: string;
	    source: string;
	    source_host: string;
	    source_file: string;
	    source_offset: number;
	    event_id?: number;
	    provider?: string;
	    computer?: string;
	    user_id?: string;
	    fields?: Record<string, any>;
	    raw_log: string;
	    tags?: string[];
	    is_anomaly: boolean;
	    risk_score?: number;
	    is_evidence: boolean;
	
	    static createFrom(source: any = {}) {
	        return new LogEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.doc_id = source["doc_id"];
	        this.id = source["id"];
	        this.timestamp = this.convertValues(source["timestamp"], null);
	        this.level = source["level"];
	        this.message = source["message"];
	        this.source = source["source"];
	        this.source_host = source["source_host"];
	        this.source_file = source["source_file"];
	        this.source_offset = source["source_offset"];
	        this.event_id = source["event_id"];
	        this.provider = source["provider"];
	        this.computer = source["computer"];
	        this.user_id = source["user_id"];
	        this.fields = source["fields"];
	        this.raw_log = source["raw_log"];
	        this.tags = source["tags"];
	        this.is_anomaly = source["is_anomaly"];
	        this.risk_score = source["risk_score"];
	        this.is_evidence = source["is_evidence"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ContextResult {
	    entries: LogEntry[];
	    anchor_index: number;
	
	    static createFrom(source: any = {}) {
	        return new ContextResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.entries = this.convertValues(source["entries"], LogEntry);
	        this.anchor_index = source["anchor_index"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class TimeRange {
	    // Go type: time
	    start: any;
	    // Go type: time
	    end: any;
	
	    static createFrom(source: any = {}) {
	        return new TimeRange(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.start = this.convertValues(source["start"], null);
	        this.end = this.convertValues(source["end"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class CreateInvestigationRequest {
	    project_id: string;
	    title: string;
	    description: string;
	    clue_type: string;
	    clue_value: string;
	    tags: string[];
	    risk_score?: number;
	    time_range?: TimeRange;
	    assigned_to?: string;
	    doc_ids?: number[];
	
	    static createFrom(source: any = {}) {
	        return new CreateInvestigationRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.project_id = source["project_id"];
	        this.title = source["title"];
	        this.description = source["description"];
	        this.clue_type = source["clue_type"];
	        this.clue_value = source["clue_value"];
	        this.tags = source["tags"];
	        this.risk_score = source["risk_score"];
	        this.time_range = this.convertValues(source["time_range"], TimeRange);
	        this.assigned_to = source["assigned_to"];
	        this.doc_ids = source["doc_ids"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class CreateProjectRequest {
	    name: string;
	    description: string;
	    tags: string[];
	    priority: string;
	
	    static createFrom(source: any = {}) {
	        return new CreateProjectRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	        this.tags = source["tags"];
	        this.priority = source["priority"];
	    }
	}
	export class DetectedLog {
	    id: string;
	    name: string;
	    path: string;
	    type: string;
	    channel?: string;
	    category: string;
	    size_bytes: number;
	    // Go type: time
	    mod_time: any;
	    readable: boolean;
	    template_id?: string;
	    format?: string;
	    preview?: string;
	
	    static createFrom(source: any = {}) {
	        return new DetectedLog(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.path = source["path"];
	        this.type = source["type"];
	        this.channel = source["channel"];
	        this.category = source["category"];
	        this.size_bytes = source["size_bytes"];
	        this.mod_time = this.convertValues(source["mod_time"], null);
	        this.readable = source["readable"];
	        this.template_id = source["template_id"];
	        this.format = source["format"];
	        this.preview = source["preview"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class DetectionMatch {
	    rule_id: string;
	    rule_name: string;
	    severity: string;
	    doc_id: number;
	    // Go type: time
	    timestamp: any;
	    message: string;
	    level: string;
	    source_host: string;
	    group_key?: string;
	    tags?: string[];
	    risk_score: number;
	
	    static createFrom(source: any = {}) {
	        return new DetectionMatch(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.rule_id = source["rule_id"];
	        this.rule_name = source["rule_name"];
	        this.severity = source["severity"];
	        this.doc_id = source["doc_id"];
	        this.timestamp = this.convertValues(source["timestamp"], null);
	        this.message = source["message"];
	        this.level = source["level"];
	        this.source_host = source["source_host"];
	        this.group_key = source["group_key"];
	        this.tags = source["tags"];
	        this.risk_score = source["risk_score"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class GroupSummary {
	    group_key: string;
	    count: number;
	    doc_ids?: number[];
	
	    static createFrom(source: any = {}) {
	        return new GroupSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.group_key = source["group_key"];
	        this.count = source["count"];
	        this.doc_ids = source["doc_ids"];
	    }
	}
	export class RuleRunResult {
	    rule_id: string;
	    rule_name: string;
	    severity: string;
	    count: number;
	    matches: DetectionMatch[];
	    error?: string;
	    summaries?: GroupSummary[];
	
	    static createFrom(source: any = {}) {
	        return new RuleRunResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.rule_id = source["rule_id"];
	        this.rule_name = source["rule_name"];
	        this.severity = source["severity"];
	        this.count = source["count"];
	        this.matches = this.convertValues(source["matches"], DetectionMatch);
	        this.error = source["error"];
	        this.summaries = this.convertValues(source["summaries"], GroupSummary);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class DetectionResult {
	    task_id: string;
	    results: RuleRunResult[];
	    total_matches: number;
	    scanned: number;
	    took: number;
	    // Go type: time
	    started_at: any;
	    // Go type: time
	    finished_at: any;
	
	    static createFrom(source: any = {}) {
	        return new DetectionResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.task_id = source["task_id"];
	        this.results = this.convertValues(source["results"], RuleRunResult);
	        this.total_matches = source["total_matches"];
	        this.scanned = source["scanned"];
	        this.took = source["took"];
	        this.started_at = this.convertValues(source["started_at"], null);
	        this.finished_at = this.convertValues(source["finished_at"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class DetectionRule {
	    id: string;
	    name: string;
	    description: string;
	    severity: string;
	    enabled: boolean;
	    tags?: string[];
	    mitre_tactics?: string[];
	    mitre_techniques?: string[];
	    match_count: number;
	    // Go type: time
	    last_matched_at?: any;
	    file_path?: string;
	    builtin: boolean;
	
	    static createFrom(source: any = {}) {
	        return new DetectionRule(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.severity = source["severity"];
	        this.enabled = source["enabled"];
	        this.tags = source["tags"];
	        this.mitre_tactics = source["mitre_tactics"];
	        this.mitre_techniques = source["mitre_techniques"];
	        this.match_count = source["match_count"];
	        this.last_matched_at = this.convertValues(source["last_matched_at"], null);
	        this.file_path = source["file_path"];
	        this.builtin = source["builtin"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class EvidenceMark {
	    id: string;
	    investigation_id: string;
	    doc_id: number;
	    log_entry_id: string;
	    // Go type: time
	    marked_at: any;
	    note: string;
	
	    static createFrom(source: any = {}) {
	        return new EvidenceMark(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.investigation_id = source["investigation_id"];
	        this.doc_id = source["doc_id"];
	        this.log_entry_id = source["log_entry_id"];
	        this.marked_at = this.convertValues(source["marked_at"], null);
	        this.note = source["note"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class EvidenceItem {
	    mark: EvidenceMark;
	    entry?: LogEntry;
	
	    static createFrom(source: any = {}) {
	        return new EvidenceItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mark = this.convertValues(source["mark"], EvidenceMark);
	        this.entry = this.convertValues(source["entry"], LogEntry);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	export class FileInfo {
	    name: string;
	    path: string;
	    is_dir: boolean;
	    size: number;
	    // Go type: time
	    mod_time: any;
	
	    static createFrom(source: any = {}) {
	        return new FileInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.is_dir = source["is_dir"];
	        this.size = source["size"];
	        this.mod_time = this.convertValues(source["mod_time"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ForensicsRequest {
	    project_id: string;
	    investigation_id?: string;
	    output_path: string;
	    doc_ids?: number[];
	    ioc_values?: string[];
	    query?: Query;
	    include_entries: boolean;
	    include_evidence: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ForensicsRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.project_id = source["project_id"];
	        this.investigation_id = source["investigation_id"];
	        this.output_path = source["output_path"];
	        this.doc_ids = source["doc_ids"];
	        this.ioc_values = source["ioc_values"];
	        this.query = this.convertValues(source["query"], Query);
	        this.include_entries = source["include_entries"];
	        this.include_evidence = source["include_evidence"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ForensicsResult {
	    path: string;
	    files: string[];
	    entry_count: number;
	    bytes: number;
	    sha256: string;
	    // Go type: time
	    generated_at: any;
	
	    static createFrom(source: any = {}) {
	        return new ForensicsResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.files = source["files"];
	        this.entry_count = source["entry_count"];
	        this.bytes = source["bytes"];
	        this.sha256 = source["sha256"];
	        this.generated_at = this.convertValues(source["generated_at"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class FormatDetection {
	    detected_format: string;
	    confidence: number;
	    suggested_templates: string[];
	
	    static createFrom(source: any = {}) {
	        return new FormatDetection(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.detected_format = source["detected_format"];
	        this.confidence = source["confidence"];
	        this.suggested_templates = source["suggested_templates"];
	    }
	}
	
	
	export class HostConnection {
	    protocol: string;
	    local_address: string;
	    local_port: number;
	    remote_address: string;
	    remote_port: number;
	    state: string;
	    process?: string;
	    pid?: number;
	    external: boolean;
	
	    static createFrom(source: any = {}) {
	        return new HostConnection(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.protocol = source["protocol"];
	        this.local_address = source["local_address"];
	        this.local_port = source["local_port"];
	        this.remote_address = source["remote_address"];
	        this.remote_port = source["remote_port"];
	        this.state = source["state"];
	        this.process = source["process"];
	        this.pid = source["pid"];
	        this.external = source["external"];
	    }
	}
	export class HostPort {
	    protocol: string;
	    local_address: string;
	    local_port: number;
	    process?: string;
	    pid?: number;
	    exposed: boolean;
	
	    static createFrom(source: any = {}) {
	        return new HostPort(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.protocol = source["protocol"];
	        this.local_address = source["local_address"];
	        this.local_port = source["local_port"];
	        this.process = source["process"];
	        this.pid = source["pid"];
	        this.exposed = source["exposed"];
	    }
	}
	export class HostNetworkSnapshot {
	    // Go type: time
	    generated_at: any;
	    hostname: string;
	    platform: string;
	    arch: string;
	    ports: HostPort[];
	    connections: HostConnection[];
	    external_ips: string[];
	    errors: string[];
	    truncated: boolean;
	    total_ports: number;
	    total_connections: number;
	
	    static createFrom(source: any = {}) {
	        return new HostNetworkSnapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.generated_at = this.convertValues(source["generated_at"], null);
	        this.hostname = source["hostname"];
	        this.platform = source["platform"];
	        this.arch = source["arch"];
	        this.ports = this.convertValues(source["ports"], HostPort);
	        this.connections = this.convertValues(source["connections"], HostConnection);
	        this.external_ips = source["external_ips"];
	        this.errors = source["errors"];
	        this.truncated = source["truncated"];
	        this.total_ports = source["total_ports"];
	        this.total_connections = source["total_connections"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class HostStat {
	    host: string;
	    log_count: number;
	
	    static createFrom(source: any = {}) {
	        return new HostStat(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.host = source["host"];
	        this.log_count = source["log_count"];
	    }
	}
	export class IOCItem {
	    id: string;
	    type: string;
	    value: string;
	    threat_level: string;
	    description: string;
	    source: string;
	    // Go type: time
	    created_at: any;
	
	    static createFrom(source: any = {}) {
	        return new IOCItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.type = source["type"];
	        this.value = source["value"];
	        this.threat_level = source["threat_level"];
	        this.description = source["description"];
	        this.source = source["source"];
	        this.created_at = this.convertValues(source["created_at"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class IOCImportResult {
	    imported: number;
	    skipped: number;
	    errors: string[];
	    items: IOCItem[];
	    format: string;
	
	    static createFrom(source: any = {}) {
	        return new IOCImportResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.imported = source["imported"];
	        this.skipped = source["skipped"];
	        this.errors = source["errors"];
	        this.items = this.convertValues(source["items"], IOCItem);
	        this.format = source["format"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class IOCMatch {
	    ioc: IOCItem;
	    doc_id: number;
	    entry?: LogEntry;
	    matched_field: string;
	
	    static createFrom(source: any = {}) {
	        return new IOCMatch(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ioc = this.convertValues(source["ioc"], IOCItem);
	        this.doc_id = source["doc_id"];
	        this.entry = this.convertValues(source["entry"], LogEntry);
	        this.matched_field = source["matched_field"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class IOCSearchRequest {
	    project_id: string;
	    values: string[];
	    query?: Query;
	
	    static createFrom(source: any = {}) {
	        return new IOCSearchRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.project_id = source["project_id"];
	        this.values = source["values"];
	        this.query = this.convertValues(source["query"], Query);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class IOCSearchResult {
	    matches: IOCMatch[];
	    total: number;
	    scanned: number;
	    took: number;
	
	    static createFrom(source: any = {}) {
	        return new IOCSearchResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.matches = this.convertValues(source["matches"], IOCMatch);
	        this.total = source["total"];
	        this.scanned = source["scanned"];
	        this.took = source["took"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ParseConfig {
	    mode: string;
	    template_id?: string;
	    ai_provider?: string;
	    save_template: boolean;
	    sample_size?: number;
	
	    static createFrom(source: any = {}) {
	        return new ParseConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.mode = source["mode"];
	        this.template_id = source["template_id"];
	        this.ai_provider = source["ai_provider"];
	        this.save_template = source["save_template"];
	        this.sample_size = source["sample_size"];
	    }
	}
	export class ImportFileRequest {
	    project_id: string;
	    file_path: string;
	    parse: ParseConfig;
	
	    static createFrom(source: any = {}) {
	        return new ImportFileRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.project_id = source["project_id"];
	        this.file_path = source["file_path"];
	        this.parse = this.convertValues(source["parse"], ParseConfig);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class Investigation {
	    id: string;
	    project_id: string;
	    title: string;
	    description: string;
	    clue_type: string;
	    clue_value: string;
	    status: string;
	    related_log_count: number;
	    time_range?: TimeRange;
	    ai_analysis?: AIAnalysisResult;
	    evidence_ids: string[];
	    tags?: string[];
	    risk_score?: number;
	    // Go type: time
	    created_at: any;
	    // Go type: time
	    updated_at: any;
	    // Go type: time
	    closed_at?: any;
	    assigned_to?: string;
	
	    static createFrom(source: any = {}) {
	        return new Investigation(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.project_id = source["project_id"];
	        this.title = source["title"];
	        this.description = source["description"];
	        this.clue_type = source["clue_type"];
	        this.clue_value = source["clue_value"];
	        this.status = source["status"];
	        this.related_log_count = source["related_log_count"];
	        this.time_range = this.convertValues(source["time_range"], TimeRange);
	        this.ai_analysis = this.convertValues(source["ai_analysis"], AIAnalysisResult);
	        this.evidence_ids = source["evidence_ids"];
	        this.tags = source["tags"];
	        this.risk_score = source["risk_score"];
	        this.created_at = this.convertValues(source["created_at"], null);
	        this.updated_at = this.convertValues(source["updated_at"], null);
	        this.closed_at = this.convertValues(source["closed_at"], null);
	        this.assigned_to = source["assigned_to"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class InvestigationUpdates {
	    title?: string;
	    description?: string;
	    status?: string;
	    tags?: string[];
	    risk_score?: number;
	    assigned_to?: string;
	    clue_type?: string;
	    clue_value?: string;
	    time_range?: TimeRange;
	    clear_time_range?: boolean;
	    clear_risk_score?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new InvestigationUpdates(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.description = source["description"];
	        this.status = source["status"];
	        this.tags = source["tags"];
	        this.risk_score = source["risk_score"];
	        this.assigned_to = source["assigned_to"];
	        this.clue_type = source["clue_type"];
	        this.clue_value = source["clue_value"];
	        this.time_range = this.convertValues(source["time_range"], TimeRange);
	        this.clear_time_range = source["clear_time_range"];
	        this.clear_risk_score = source["clear_risk_score"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class LevelRule {
	    range?: number[];
	    match?: string;
	    level: string;
	
	    static createFrom(source: any = {}) {
	        return new LevelRule(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.range = source["range"];
	        this.match = source["match"];
	        this.level = source["level"];
	    }
	}
	export class LevelExtraction {
	    field: string;
	    rules: LevelRule[];
	    default: string;
	
	    static createFrom(source: any = {}) {
	        return new LevelExtraction(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.field = source["field"];
	        this.rules = this.convertValues(source["rules"], LevelRule);
	        this.default = source["default"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class LocalCollectRequest {
	    project_id: string;
	    file_paths: string[];
	    recursive: boolean;
	    source_type: string;
	    mode: string;
	    file_pattern?: string;
	    exclude_paths?: string[];
	    concurrency: number;
	    buffer_size: number;
	    parse: ParseConfig;
	
	    static createFrom(source: any = {}) {
	        return new LocalCollectRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.project_id = source["project_id"];
	        this.file_paths = source["file_paths"];
	        this.recursive = source["recursive"];
	        this.source_type = source["source_type"];
	        this.mode = source["mode"];
	        this.file_pattern = source["file_pattern"];
	        this.exclude_paths = source["exclude_paths"];
	        this.concurrency = source["concurrency"];
	        this.buffer_size = source["buffer_size"];
	        this.parse = this.convertValues(source["parse"], ParseConfig);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class LocalLogDiscovery {
	    // Go type: time
	    scanned_at: any;
	    roots: string[];
	    items: DetectedLog[];
	    errors: string[];
	    truncated: boolean;
	
	    static createFrom(source: any = {}) {
	        return new LocalLogDiscovery(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.scanned_at = this.convertValues(source["scanned_at"], null);
	        this.roots = source["roots"];
	        this.items = this.convertValues(source["items"], DetectedLog);
	        this.errors = source["errors"];
	        this.truncated = source["truncated"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class LogAnalysisRequest {
	    project_id: string;
	    doc_id: number;
	    provider: string;
	    context_before: number;
	    context_after: number;
	    question?: string;
	
	    static createFrom(source: any = {}) {
	        return new LogAnalysisRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.project_id = source["project_id"];
	        this.doc_id = source["doc_id"];
	        this.provider = source["provider"];
	        this.context_before = source["context_before"];
	        this.context_after = source["context_after"];
	        this.question = source["question"];
	    }
	}
	export class LogAnalysisResult {
	    summary: string;
	    risk_level: string;
	    findings: string[];
	    suggestions: string[];
	    related_doc_ids?: number[];
	    analyzed: number;
	    provider: string;
	    raw_response?: string;
	    // Go type: time
	    generated_at: any;
	
	    static createFrom(source: any = {}) {
	        return new LogAnalysisResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.summary = source["summary"];
	        this.risk_level = source["risk_level"];
	        this.findings = source["findings"];
	        this.suggestions = source["suggestions"];
	        this.related_doc_ids = source["related_doc_ids"];
	        this.analyzed = source["analyzed"];
	        this.provider = source["provider"];
	        this.raw_response = source["raw_response"];
	        this.generated_at = this.convertValues(source["generated_at"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class LogPattern {
	    id: string;
	    template: string;
	    count: number;
	    percentage: number;
	    // Go type: time
	    first_seen: any;
	    // Go type: time
	    last_seen: any;
	    examples: string[];
	    parameters: Record<string, Array<string>>;
	
	    static createFrom(source: any = {}) {
	        return new LogPattern(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.template = source["template"];
	        this.count = source["count"];
	        this.percentage = source["percentage"];
	        this.first_seen = this.convertValues(source["first_seen"], null);
	        this.last_seen = this.convertValues(source["last_seen"], null);
	        this.examples = source["examples"];
	        this.parameters = source["parameters"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class LogSource {
	    id: string;
	    type: string;
	    host: string;
	    path: string;
	    parse_mode: string;
	    template_id: string;
	    log_count: number;
	    size_bytes: number;
	    time_range?: TimeRange;
	    // Go type: time
	    created_at: any;
	    ai_provider?: string;
	
	    static createFrom(source: any = {}) {
	        return new LogSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.type = source["type"];
	        this.host = source["host"];
	        this.path = source["path"];
	        this.parse_mode = source["parse_mode"];
	        this.template_id = source["template_id"];
	        this.log_count = source["log_count"];
	        this.size_bytes = source["size_bytes"];
	        this.time_range = this.convertValues(source["time_range"], TimeRange);
	        this.created_at = this.convertValues(source["created_at"], null);
	        this.ai_provider = source["ai_provider"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class MiddlewarePreset {
	    name: string;
	    description: string;
	    category: string;
	    windows_paths: string[];
	    linux_paths: string[];
	    template_id: string;
	    multiline: boolean;
	
	    static createFrom(source: any = {}) {
	        return new MiddlewarePreset(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	        this.category = source["category"];
	        this.windows_paths = source["windows_paths"];
	        this.linux_paths = source["linux_paths"];
	        this.template_id = source["template_id"];
	        this.multiline = source["multiline"];
	    }
	}
	export class MultiLineConfig {
	    enabled: boolean;
	    pattern: string;
	    negate: boolean;
	    match: string;
	    max_lines: number;
	    timeout_ms: number;
	
	    static createFrom(source: any = {}) {
	        return new MultiLineConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.pattern = source["pattern"];
	        this.negate = source["negate"];
	        this.match = source["match"];
	        this.max_lines = source["max_lines"];
	        this.timeout_ms = source["timeout_ms"];
	    }
	}
	
	export class ParseTemplate {
	    id: string;
	    name: string;
	    description: string;
	    type: string;
	    category: string;
	    format: string;
	    priority: number;
	    regex: string;
	    field_map: Record<string, number>;
	    time_format: string;
	    time_field: string;
	    level_map: Record<string, string>;
	    level_field: string;
	    level_extraction?: LevelExtraction;
	    multiline?: MultiLineConfig;
	    examples?: string[];
	    // Go type: time
	    created_at: any;
	    // Go type: time
	    updated_at: any;
	    usage_count: number;
	    source: string;
	
	    static createFrom(source: any = {}) {
	        return new ParseTemplate(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.type = source["type"];
	        this.category = source["category"];
	        this.format = source["format"];
	        this.priority = source["priority"];
	        this.regex = source["regex"];
	        this.field_map = source["field_map"];
	        this.time_format = source["time_format"];
	        this.time_field = source["time_field"];
	        this.level_map = source["level_map"];
	        this.level_field = source["level_field"];
	        this.level_extraction = this.convertValues(source["level_extraction"], LevelExtraction);
	        this.multiline = this.convertValues(source["multiline"], MultiLineConfig);
	        this.examples = source["examples"];
	        this.created_at = this.convertValues(source["created_at"], null);
	        this.updated_at = this.convertValues(source["updated_at"], null);
	        this.usage_count = source["usage_count"];
	        this.source = source["source"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ParsedSample {
	    raw_log: string;
	    parsed?: LogEntry;
	    success: boolean;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new ParsedSample(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.raw_log = source["raw_log"];
	        this.parsed = this.convertValues(source["parsed"], LogEntry);
	        this.success = source["success"];
	        this.error = source["error"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class PatternClusterResult {
	    patterns: LogPattern[];
	    total_count: number;
	    took: number;
	
	    static createFrom(source: any = {}) {
	        return new PatternClusterResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.patterns = this.convertValues(source["patterns"], LogPattern);
	        this.total_count = source["total_count"];
	        this.took = source["took"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class PreviewParseRequest {
	    template: ParseTemplate;
	    samples: string[];
	
	    static createFrom(source: any = {}) {
	        return new PreviewParseRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.template = this.convertValues(source["template"], ParseTemplate);
	        this.samples = source["samples"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class PreviewResult {
	    samples: ParsedSample[];
	    success_rate: number;
	
	    static createFrom(source: any = {}) {
	        return new PreviewResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.samples = this.convertValues(source["samples"], ParsedSample);
	        this.success_rate = source["success_rate"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Project {
	    id: string;
	    name: string;
	    description: string;
	    // Go type: time
	    created_at: any;
	    // Go type: time
	    updated_at: any;
	    status: string;
	    project_path: string;
	    log_count: number;
	    size_bytes: number;
	    time_range?: TimeRange;
	    sources: LogSource[];
	    level_distribution: Record<string, number>;
	    tags?: string[];
	    priority: string;
	
	    static createFrom(source: any = {}) {
	        return new Project(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.created_at = this.convertValues(source["created_at"], null);
	        this.updated_at = this.convertValues(source["updated_at"], null);
	        this.status = source["status"];
	        this.project_path = source["project_path"];
	        this.log_count = source["log_count"];
	        this.size_bytes = source["size_bytes"];
	        this.time_range = this.convertValues(source["time_range"], TimeRange);
	        this.sources = this.convertValues(source["sources"], LogSource);
	        this.level_distribution = source["level_distribution"];
	        this.tags = source["tags"];
	        this.priority = source["priority"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ProjectFilter {
	    status: string;
	    tags: string[];
	    sort_by: string;
	    sort_desc: boolean;
	    search: string;
	
	    static createFrom(source: any = {}) {
	        return new ProjectFilter(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.tags = source["tags"];
	        this.sort_by = source["sort_by"];
	        this.sort_desc = source["sort_desc"];
	        this.search = source["search"];
	    }
	}
	export class ProjectStatistics {
	    total_logs: number;
	    size_bytes: number;
	    time_range?: TimeRange;
	    level_distribution: Record<string, number>;
	    top_hosts: HostStat[];
	    source_count: number;
	    index_size_bytes: number;
	    unique_terms: number;
	
	    static createFrom(source: any = {}) {
	        return new ProjectStatistics(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.total_logs = source["total_logs"];
	        this.size_bytes = source["size_bytes"];
	        this.time_range = this.convertValues(source["time_range"], TimeRange);
	        this.level_distribution = source["level_distribution"];
	        this.top_hosts = this.convertValues(source["top_hosts"], HostStat);
	        this.source_count = source["source_count"];
	        this.index_size_bytes = source["index_size_bytes"];
	        this.unique_terms = source["unique_terms"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ProjectUpdates {
	    name?: string;
	    description?: string;
	    tags?: string[];
	    priority?: string;
	    status?: string;
	
	    static createFrom(source: any = {}) {
	        return new ProjectUpdates(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	        this.tags = source["tags"];
	        this.priority = source["priority"];
	        this.status = source["status"];
	    }
	}
	
	export class QueryResult {
	    total: number;
	    hits: LogEntry[];
	    took: number;
	    has_more: boolean;
	    query_id?: string;
	
	    static createFrom(source: any = {}) {
	        return new QueryResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.total = source["total"];
	        this.hits = this.convertValues(source["hits"], LogEntry);
	        this.took = source["took"];
	        this.has_more = source["has_more"];
	        this.query_id = source["query_id"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class QuerySnapshot {
	    id: string;
	    project_id: string;
	    name: string;
	    description: string;
	    query: Query;
	    result_count: number;
	    // Go type: time
	    created_at: any;
	
	    static createFrom(source: any = {}) {
	        return new QuerySnapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.project_id = source["project_id"];
	        this.name = source["name"];
	        this.description = source["description"];
	        this.query = this.convertValues(source["query"], Query);
	        this.result_count = source["result_count"];
	        this.created_at = this.convertValues(source["created_at"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ReportRequest {
	    project_id: string;
	    investigation_id?: string;
	    provider: string;
	    title: string;
	    query: Query;
	    max_logs: number;
	    aggregate_by?: string[];
	    output_path?: string;
	
	    static createFrom(source: any = {}) {
	        return new ReportRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.project_id = source["project_id"];
	        this.investigation_id = source["investigation_id"];
	        this.provider = source["provider"];
	        this.title = source["title"];
	        this.query = this.convertValues(source["query"], Query);
	        this.max_logs = source["max_logs"];
	        this.aggregate_by = source["aggregate_by"];
	        this.output_path = source["output_path"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ReportResult {
	    title: string;
	    content: string;
	    format: string;
	    output_path?: string;
	    provider: string;
	    aggregates?: AggregateDimension[];
	    // Go type: time
	    generated_at: any;
	
	    static createFrom(source: any = {}) {
	        return new ReportResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.content = source["content"];
	        this.format = source["format"];
	        this.output_path = source["output_path"];
	        this.provider = source["provider"];
	        this.aggregates = this.convertValues(source["aggregates"], AggregateDimension);
	        this.generated_at = this.convertValues(source["generated_at"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class RootCause {
	    cause: string;
	    confidence: number;
	    evidence: string[];
	    description: string;
	
	    static createFrom(source: any = {}) {
	        return new RootCause(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.cause = source["cause"];
	        this.confidence = source["confidence"];
	        this.evidence = source["evidence"];
	        this.description = source["description"];
	    }
	}
	export class RootCauseRequest {
	    project_id: string;
	    provider: string;
	    query: Query;
	    question: string;
	    max_logs: number;
	
	    static createFrom(source: any = {}) {
	        return new RootCauseRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.project_id = source["project_id"];
	        this.provider = source["provider"];
	        this.query = this.convertValues(source["query"], Query);
	        this.question = source["question"];
	        this.max_logs = source["max_logs"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class RootCauseResult {
	    summary: string;
	    root_causes: RootCause[];
	    timeline: string[];
	    suggestions: string[];
	    analyzed: number;
	    provider: string;
	    raw_response?: string;
	    // Go type: time
	    generated_at: any;
	
	    static createFrom(source: any = {}) {
	        return new RootCauseResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.summary = source["summary"];
	        this.root_causes = this.convertValues(source["root_causes"], RootCause);
	        this.timeline = source["timeline"];
	        this.suggestions = source["suggestions"];
	        this.analyzed = source["analyzed"];
	        this.provider = source["provider"];
	        this.raw_response = source["raw_response"];
	        this.generated_at = this.convertValues(source["generated_at"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class RunDetectionRequest {
	    project_id: string;
	    rule_ids: string[];
	
	    static createFrom(source: any = {}) {
	        return new RunDetectionRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.project_id = source["project_id"];
	        this.rule_ids = source["rule_ids"];
	    }
	}
	export class SSHCollectRequest {
	    project_id: string;
	    host_id: string;
	    remote_paths: string[];
	    strategy: string;
	    filter_pattern?: string;
	    compression: string;
	    concurrency: number;
	    since?: string;
	    until?: string;
	    unit?: string;
	    parse: ParseConfig;
	
	    static createFrom(source: any = {}) {
	        return new SSHCollectRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.project_id = source["project_id"];
	        this.host_id = source["host_id"];
	        this.remote_paths = source["remote_paths"];
	        this.strategy = source["strategy"];
	        this.filter_pattern = source["filter_pattern"];
	        this.compression = source["compression"];
	        this.concurrency = source["concurrency"];
	        this.since = source["since"];
	        this.until = source["until"];
	        this.unit = source["unit"];
	        this.parse = this.convertValues(source["parse"], ParseConfig);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SSHHost {
	    id: string;
	    name: string;
	    host: string;
	    port: number;
	    username: string;
	    auth_method: string;
	    key_path?: string;
	    proxy_jump?: string;
	    has_password: boolean;
	    // Go type: time
	    last_used_at?: any;
	
	    static createFrom(source: any = {}) {
	        return new SSHHost(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.username = source["username"];
	        this.auth_method = source["auth_method"];
	        this.key_path = source["key_path"];
	        this.proxy_jump = source["proxy_jump"];
	        this.has_password = source["has_password"];
	        this.last_used_at = this.convertValues(source["last_used_at"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SSHLogSearchRequest {
	    host_id: string;
	    roots?: string[];
	    full_scan: boolean;
	    keyword?: string;
	    search_content: boolean;
	    include_rotated: boolean;
	    max_results?: number;
	
	    static createFrom(source: any = {}) {
	        return new SSHLogSearchRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.host_id = source["host_id"];
	        this.roots = source["roots"];
	        this.full_scan = source["full_scan"];
	        this.keyword = source["keyword"];
	        this.search_content = source["search_content"];
	        this.include_rotated = source["include_rotated"];
	        this.max_results = source["max_results"];
	    }
	}
	export class SSHRemoteLogFile {
	    path: string;
	    size: number;
	    // Go type: time
	    mod_time: any;
	
	    static createFrom(source: any = {}) {
	        return new SSHRemoteLogFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.size = source["size"];
	        this.mod_time = this.convertValues(source["mod_time"], null);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SSHLogSearchResult {
	    files: SSHRemoteLogFile[];
	    truncated: boolean;
	    took_ms: number;
	
	    static createFrom(source: any = {}) {
	        return new SSHLogSearchResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.files = this.convertValues(source["files"], SSHRemoteLogFile);
	        this.truncated = source["truncated"];
	        this.took_ms = source["took_ms"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class SaveAIConfigRequest {
	    provider: string;
	    endpoint: string;
	    model: string;
	    api_key?: string;
	    enabled: boolean;
	    priority: number;
	    parameters?: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new SaveAIConfigRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provider = source["provider"];
	        this.endpoint = source["endpoint"];
	        this.model = source["model"];
	        this.api_key = source["api_key"];
	        this.enabled = source["enabled"];
	        this.priority = source["priority"];
	        this.parameters = source["parameters"];
	    }
	}
	export class SaveSSHHostRequest {
	    id?: string;
	    name: string;
	    host: string;
	    port: number;
	    username: string;
	    auth_method: string;
	    password?: string;
	    key_path?: string;
	    proxy_jump?: string;
	
	    static createFrom(source: any = {}) {
	        return new SaveSSHHostRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.host = source["host"];
	        this.port = source["port"];
	        this.username = source["username"];
	        this.auth_method = source["auth_method"];
	        this.password = source["password"];
	        this.key_path = source["key_path"];
	        this.proxy_jump = source["proxy_jump"];
	    }
	}
	export class SystemInfo {
	    os: string;
	    arch: string;
	    version: string;
	    data_path: string;
	    projects_path: string;
	    temp_path: string;
	    log_path: string;
	    db_path: string;
	    executable: string;
	
	    static createFrom(source: any = {}) {
	        return new SystemInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.os = source["os"];
	        this.arch = source["arch"];
	        this.version = source["version"];
	        this.data_path = source["data_path"];
	        this.projects_path = source["projects_path"];
	        this.temp_path = source["temp_path"];
	        this.log_path = source["log_path"];
	        this.db_path = source["db_path"];
	        this.executable = source["executable"];
	    }
	}
	export class SystemdJournalRequest {
	    project_id: string;
	    since?: string;
	    until?: string;
	    unit?: string;
	    priority?: string;
	    lines?: number;
	    follow: boolean;
	    parse: ParseConfig;
	
	    static createFrom(source: any = {}) {
	        return new SystemdJournalRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.project_id = source["project_id"];
	        this.since = source["since"];
	        this.until = source["until"];
	        this.unit = source["unit"];
	        this.priority = source["priority"];
	        this.lines = source["lines"];
	        this.follow = source["follow"];
	        this.parse = this.convertValues(source["parse"], ParseConfig);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class TaskInfo {
	    id: string;
	    type: string;
	    project_id: string;
	    source_id?: string;
	    status: string;
	    progress: number;
	    message: string;
	    processed: number;
	    total: number;
	    error?: string;
	    // Go type: time
	    started_at: any;
	    // Go type: time
	    finished_at?: any;
	    result?: CollectResult;
	
	    static createFrom(source: any = {}) {
	        return new TaskInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.type = source["type"];
	        this.project_id = source["project_id"];
	        this.source_id = source["source_id"];
	        this.status = source["status"];
	        this.progress = source["progress"];
	        this.message = source["message"];
	        this.processed = source["processed"];
	        this.total = source["total"];
	        this.error = source["error"];
	        this.started_at = this.convertValues(source["started_at"], null);
	        this.finished_at = this.convertValues(source["finished_at"], null);
	        this.result = this.convertValues(source["result"], CollectResult);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class TemplateFilter {
	    type: string;
	    category: string;
	    search: string;
	
	    static createFrom(source: any = {}) {
	        return new TemplateFilter(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.category = source["category"];
	        this.search = source["search"];
	    }
	}
	
	
	
	
	
	export class WindowsEventLogRequest {
	    project_id: string;
	    channels: string[];
	    // Go type: time
	    start_time?: any;
	    // Go type: time
	    end_time?: any;
	    event_ids?: number[];
	    levels?: string[];
	    parse: ParseConfig;
	
	    static createFrom(source: any = {}) {
	        return new WindowsEventLogRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.project_id = source["project_id"];
	        this.channels = source["channels"];
	        this.start_time = this.convertValues(source["start_time"], null);
	        this.end_time = this.convertValues(source["end_time"], null);
	        this.event_ids = source["event_ids"];
	        this.levels = source["levels"];
	        this.parse = this.convertValues(source["parse"], ParseConfig);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

