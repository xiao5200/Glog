package indexer

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strings"
	"time"
)

// bloomBucket 是一个小时时间桶的布隆过滤器。
// 默认 bitCount = 1<<20（128 KiB），hashCount = 3。
type bloomBucket struct {
	startNs   int64
	endNs     int64
	bitCount  uint32
	hashCount uint8
	bits      []byte
}

const (
	fnvOffset64 = 14695981039346656037
	fnvPrime64  = 1099511628211
)

// fnv1a64 计算带自定义初始值的 FNV-1a 64 位哈希。
func fnv1a64(seed uint64, s string) uint64 {
	h := seed
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= fnvPrime64
	}
	return h
}

// bloomHashes 返回 FNV-1a 双哈希 (h1, h2)，h2 保证非零。
func bloomHashes(term string) (uint64, uint64) {
	h1 := fnv1a64(fnvOffset64, term)
	h2 := fnv1a64(fnvOffset64^0x9e3779b97f4a7c15, term)
	if h2 == 0 {
		h2 = 1
	}
	return h1, h2
}

func newBloomBucket(startNs int64) *bloomBucket {
	bitsLen := int((defaultBloomBits + 7) / 8)
	return &bloomBucket{
		startNs:   startNs,
		endNs:     startNs + nsPerHour,
		bitCount:  defaultBloomBits,
		hashCount: defaultBloomHashCount,
		bits:      make([]byte, bitsLen),
	}
}

func (b *bloomBucket) addTokens(tokens []string) {
	if b.bitCount == 0 || b.hashCount == 0 {
		return
	}
	mod := uint64(b.bitCount)
	for _, term := range tokens {
		h1, h2 := bloomHashes(term)
		for i := uint8(0); i < b.hashCount; i++ {
			pos := (h1 + uint64(i)*h2) % mod
			b.bits[pos>>3] |= 1 << (pos & 7)
		}
	}
}

func (b *bloomBucket) mayContain(term string) bool {
	if b.bitCount == 0 || b.hashCount == 0 {
		return true
	}
	mod := uint64(b.bitCount)
	h1, h2 := bloomHashes(term)
	for i := uint8(0); i < b.hashCount; i++ {
		pos := (h1 + uint64(i)*h2) % mod
		if b.bits[pos>>3]&(1<<(pos&7)) == 0 {
			return false
		}
	}
	return true
}

// bloomAddLocked 把词项加入 timestamp 所在小时桶；超过 720 个桶时淘汰最旧桶。
func (s *Store) bloomAddLocked(ts int64, tokens []string) {
	start := hourStart(ts)
	bucket, ok := s.bloomBuckets[start]
	if !ok {
		bucket = newBloomBucket(start)
		s.bloomBuckets[start] = bucket
		if len(s.bloomBuckets) > maxBloomBuckets {
			s.evictOldestBloomBucketLocked()
		}
	}
	bucket.addTokens(tokens)
}

func (s *Store) evictOldestBloomBucketLocked() {
	var oldest int64
	first := true
	for start := range s.bloomBuckets {
		if first || start < oldest {
			oldest = start
			first = false
		}
	}
	if !first {
		delete(s.bloomBuckets, oldest)
	}
}

// BloomMayContain 判断某小时桶是否「可能」包含 term。
// 无对应小时桶（例如查 720 小时之前的历史）时返回 true，避免误判为不存在。
// term 会先转小写，与 Tokenize 输出一致。
func (s *Store) BloomMayContain(term string, bucketStart time.Time) bool {
	term = strings.ToLower(term)
	start := hourStart(bucketStart.UnixNano())

	s.mu.RLock()
	defer s.mu.RUnlock()
	bucket, ok := s.bloomBuckets[start]
	if !ok {
		return true
	}
	return bucket.mayContain(term)
}

// saveBloomIndex 写出 indexes/bloom.idx：
//
//	magic(8) + version(4) + bucketCount(4)
//	bucketCount × {startNs int64, endNs int64, bitCount uint32, hashCount uint8, bits}
//
// bits 长度由 bitCount 推导：ceil(bitCount/8)。
func (s *Store) saveBloomIndex(path string) (int64, error) {
	starts := make([]int64, 0, len(s.bloomBuckets))
	for start := range s.bloomBuckets {
		starts = append(starts, start)
	}
	sort.Slice(starts, func(i, j int) bool { return starts[i] < starts[j] })

	return writeIndexAtomic(path, func(w io.Writer) error {
		var hdr [bloomHeaderSize]byte
		copy(hdr[:8], []byte(bloomMagic))
		binary.LittleEndian.PutUint32(hdr[8:12], formatVersion)
		binary.LittleEndian.PutUint32(hdr[12:16], uint32(len(starts)))
		if _, err := w.Write(hdr[:]); err != nil {
			return err
		}

		var rec [21]byte
		for _, start := range starts {
			bucket := s.bloomBuckets[start]
			bitsLen := int((bucket.bitCount + 7) / 8)
			if bitsLen <= 0 || len(bucket.bits) < bitsLen {
				return fmt.Errorf("Bloom 桶 %d 的位图长度非法", start)
			}
			binary.LittleEndian.PutUint64(rec[0:8], uint64(bucket.startNs))
			binary.LittleEndian.PutUint64(rec[8:16], uint64(bucket.endNs))
			binary.LittleEndian.PutUint32(rec[16:20], bucket.bitCount)
			rec[20] = bucket.hashCount
			if _, err := w.Write(rec[:]); err != nil {
				return err
			}
			if _, err := w.Write(bucket.bits[:bitsLen]); err != nil {
				return err
			}
		}
		return nil
	})
}

// loadBloomIndex 加载 bloom.idx，校验桶时间对齐、位图参数与总大小。
func (s *Store) loadBloomIndex(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return corruptedf("%s: 读取文件状态失败: %v", path, err)
	}
	fileSize := st.Size()
	if fileSize < bloomHeaderSize {
		return corruptedf("%s 文件过短（%d 字节）", path, fileSize)
	}
	if err := readMagic(f, bloomMagic); err != nil {
		return corruptedf("%s: %v", path, err)
	}
	var head [8]byte
	if _, err := io.ReadFull(f, head[:]); err != nil {
		return corruptedf("%s: 读取文件头失败: %v", path, err)
	}
	if version := binary.LittleEndian.Uint32(head[0:4]); version != formatVersion {
		return corruptedf("%s 格式版本不支持: %d", path, version)
	}
	bucketCount := binary.LittleEndian.Uint32(head[4:8])
	if bucketCount > maxBloomBuckets {
		return corruptedf("%s 桶数量异常: %d", path, bucketCount)
	}
	if int64(bucketCount)*21 > fileSize-bloomHeaderSize {
		return corruptedf("%s 桶数据不完整", path)
	}

	reader := bufio.NewReaderSize(f, indexWriteBufferSize)
	buckets := make(map[int64]*bloomBucket, int(bucketCount))
	var totalBitsBytes int64
	var rec [21]byte

	for i := uint32(0); i < bucketCount; i++ {
		if _, err := io.ReadFull(reader, rec[:]); err != nil {
			return corruptedf("%s: 读取第 %d 个桶头失败: %v", path, i, err)
		}
		start := int64(binary.LittleEndian.Uint64(rec[0:8]))
		end := int64(binary.LittleEndian.Uint64(rec[8:16]))
		bitCount := binary.LittleEndian.Uint32(rec[16:20])
		hashCount := rec[20]

		if start%nsPerHour != 0 {
			return corruptedf("%s: 第 %d 个桶起始时间未按小时对齐", path, i)
		}
		if start > math.MaxInt64-nsPerHour || end != start+nsPerHour {
			return corruptedf("%s: 第 %d 个桶时间范围非法", path, i)
		}
		if bitCount == 0 || bitCount > maxBloomBits {
			return corruptedf("%s: 第 %d 个桶 bitCount 非法: %d", path, i, bitCount)
		}
		if hashCount == 0 || hashCount > maxBloomHashCount {
			return corruptedf("%s: 第 %d 个桶 hashCount 非法: %d", path, i, hashCount)
		}
		if _, exists := buckets[start]; exists {
			return corruptedf("%s: 第 %d 个桶起始时间重复", path, i)
		}

		bitsLen := int64((bitCount + 7) / 8)
		totalBitsBytes += bitsLen
		if totalBitsBytes > maxBloomFileBytes {
			return corruptedf("%s: Bloom 位图总大小超过限制", path)
		}
		bits := make([]byte, bitsLen)
		if _, err := io.ReadFull(reader, bits); err != nil {
			return corruptedf("%s: 读取第 %d 个桶位图失败: %v", path, i, err)
		}
		buckets[start] = &bloomBucket{
			startNs:   start,
			endNs:     end,
			bitCount:  bitCount,
			hashCount: hashCount,
			bits:      bits,
		}
	}

	var extra [1]byte
	if n, err := reader.Read(extra[:]); n != 0 {
		return corruptedf("%s 存在多余数据", path)
	} else if err != nil && err != io.EOF {
		return corruptedf("%s: 读取尾部失败: %v", path, err)
	}

	s.bloomBuckets = buckets
	return nil
}
