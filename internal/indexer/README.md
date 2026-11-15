# internal/indexer —— 日志存储与内存索引

单项目日志存储：LZ4 块压缩日志 + 常驻内存的倒排/时间/bitmap/Bloom 索引。
所有权仅限本目录；不依赖 `internal/query`。

## 目录与文件

```
<projectDir>/raw/logs.dat          压缩日志块
<projectDir>/indexes/docs.idx      docID -> 日志块位置
<projectDir>/indexes/inverted.idx  倒排索引
<projectDir>/indexes/time.idx      按时间排序的 (ts, docID)
<projectDir>/indexes/bitmap.idx    level/source_host/source/event_id 位图
<projectDir>/indexes/bloom.idx     按小时桶的布隆过滤器
<projectDir>/indexes/meta.json     统计与源区间
```

所有整数为小端。格式版本均为 1。

### raw/logs.dat

`GLOGSEG1` + version(uint32)，随后每条日志一个块：

```
[compressedLen uint32][uncompressedLen uint32][payload]
```

`payload` 为 LZ4 block（`lz4.Compressor.CompressBlock`）；压缩不划算或失败时直接存 JSON，
此时 `compressedLen == uncompressedLen`。读取时该等式即表示未压缩；
`UncompressBlock` 失败也按未压缩处理。

### indexes/docs.idx

`GLOGDOC1` + version + count(uint64) + count × `{offset uint64, storedLen uint32, rawLen uint32}`。
offset 是日志块起始位置（即 compressedLen 字段位置）。加载时校验记录连续且
结束偏移等于 logs.dat 文件大小；不一致说明索引落后/损坏，自动重建。

### indexes/inverted.idx

`GLOGINV1` + version + docCount(uint64) + termCount(uint64)，随后：

```
termCount × {termLen uint16, term, postingOffset uint64, postingLen uint32, df uint32}
postings 区（按词项字节序连续排列）
```

`postingOffset` 是文件内绝对偏移。posting 编码为
`count uint32 + delta-uvarint(docID)`，docID 严格升序。

### indexes/time.idx

`GLOGTIM1` + version + count(uint64) + count × `{ts int64(ns), docID uint32}`，
按 ts 非递减排列（同 ts 再按 docID，写入端保证确定性）。

### indexes/bitmap.idx

`GLOGBMP1` + version + fieldCount(uint32)，每个字段：

```
{nameLen uint16, name, valueCount uint32,
 valueCount × {valueLen uint16, value, dataLen uint32, roaring bytes}}
```

索引字段固定为 `level`、`source_host`、`source`、`event_id`（十进制字符串），
以及派生字段 `event_type`、`source_ip`、`destination_ip`。派生字段的提取规则由
`internal/logfields` 统一提供（与查询/聚合同源）：`event_type` 优先 Channel /
event_type / eventType，回退 Provider、源文件名与来源；IP 字段兼容常见命名。

bitmap.idx 使用独立的 `bitmapFormatVersion`（当前为 2），与 logs.dat 等其它索引的
`formatVersion` 分开演进。加载到旧版本 bitmap.idx 时会触发从 `raw/logs.dat` 的整库
重建，确保派生字段覆盖全部历史日志。空字符串值不建索引；超长（>65535 字节）
字段值跳过并记录日志。

### indexes/bloom.idx

`GLOGBLM1` + version + bucketCount(uint32)，每个小时桶：

```
{startNs int64, endNs int64, bitCount uint32, hashCount uint8, bits []byte}
```

bits 长度 = ceil(bitCount/8)。默认 1<<20 bit、3 个哈希；
哈希为 FNV-1a 双哈希 `(h1 + i*h2) % bitCount`。最多保留最近 720 个小时桶。
`BloomMayContain` 对无桶时间返回 `true`（绝不误判为不存在）。

### indexes/meta.json

缩进 JSON：`version`、`doc_count`、`term_count`、`log_size`、`index_size`
（五个数据索引文件大小之和，不含 meta 自身）、`time_range`、`level_counts`、
`built_time`、`source_ranges`（`source_id -> {start_doc, count}`）。
meta 最后写入，作为一次 Finalize 的提交记录；加载时与其它索引交叉校验。

## API

- `Open(projectDir)`：创建目录并打开日志；索引完整则全量加载，否则从
  logs.dat 重建并落盘。
- `Append(entries)`：就地分配 `DocID`/`ID`，补零值 Timestamp、空 Level、
  nil Fields；每 1000 条及结束时 Flush 日志缓冲。并发安全（进程内写锁）。
- `Finalize()`：刷新 logs.dat，并原子写全部索引（`.tmp` + rename）。幂等。
- `Rebuild()`：从 logs.dat 全量重放并持久化。
- `Flush()`：仅刷新日志缓冲。
- `Close()`：刷新并关闭文件句柄（不隐式 Finalize）。
- `DocCount()` / `AllDocIDs()` / `ReadDoc()` / `ReadDocs()`：文档访问。
  越界返回 `errs.ErrNotFound`，读取/解码失败包装 `errs.ErrIndexCorrupted`。
- `Postings(term)` / `PostingsPrefix(prefix)`：倒排查询。term 应为 Tokenize
  输出；Postings 未命中会再尝试小写。**Postings 返回内部只读视图，调用方
  不得修改**（内部只追加，旧视图仍是安全快照）。Prefix 结果为升序去重并集。
- `TimeRange(start, end)`：闭区间，按时间升序返回 docID 新切片。
- `FieldBitmap(field, value)` / `FieldValues(field)`：bitmap 查询（返回副本）。
- `Timestamp(docID)`：UTC 时间；越界返回零值。
- `LevelCounts()`：级别计数副本。
- `Stats()`：内存统计 + 目录实际大小；`CacheHitRate` 固定 0。
- `BloomMayContain(term, bucketStart)`：按 bucketStart 所在小时桶判断。

## Tokenize

小写化；按 Unicode 字母/数字/下划线切分；CJK 连续段输出单字与相邻双字
bigram；额外整体识别 IPv4、含点域名、`/` 或 `\` 分隔的文件路径；
丢弃超过 64 个 rune 的 token；最后去重并按字节序排序。索引端与查询端必须
使用同一函数，保证 term 一致。

## 并发与内存

- 进程内 `sync.RWMutex`：Append/Finalize/Rebuild/Close 写锁，查询读锁。
  不保证跨进程并发写；不要同时用两个进程打开同一项目。
- 全文索引文本为 `Message`；当 `Message` 为空而 `RawLog` 非空时回退索引
  `RawLog`。
- 常驻内存：docs 元数据约 24 B/条、时间排序 docID 4 B/条、posting 4 B/次出现、
  字段 roaring bitmap、Bloom ≤ 720 桶 × 128 KiB。日志正文不常驻，按需解码。
- `timeOrder` 惰性重建：Append 后首次时间查询或 Finalize 时排序。
