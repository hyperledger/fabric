package wal

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
	"unsafe"

	"github.com/tidwall/gjson"
	"github.com/tidwall/tinylru"
)

var (
	// ErrCorrupt is returns when the log is corrupt.
	ErrCorrupt = errors.New("log corrupt")

	// ErrClosed is returned when an operation cannot be completed because
	// the log is closed.
	ErrClosed = errors.New("log closed")

	// ErrNotFound is returned when an entry is not found.
	ErrNotFound = errors.New("not found")

	// ErrOutOfOrder is returned from Write() when the index is not equal to
	// LastIndex()+1. It's required that log monotonically grows by one and has
	// no gaps. Thus, the series 10,11,12,13,14 is valid, but 10,11,13,14 is
	// not because there's a gap between 11 and 13. Also, 10,12,11,13 is not
	// valid because 12 and 11 are out of order.
	ErrOutOfOrder = errors.New("out of order")

	// ErrOutOfRange is returned from TruncateFront() and TruncateBack() when
	// the index not in the range of the log's first and last index. Or, this
	// may be returned when the caller is attempting to remove *all* entries;
	// The log requires that at least one entry exists following a truncate.
	ErrOutOfRange = errors.New("out of range")
)

// LogFormat is the format of the log files.
type LogFormat byte

const (
	// Binary format writes entries in binary. This is the default and, unless
	// a good reason otherwise, should be used in production.
	Binary LogFormat = 0
	// JSON format writes entries as JSON lines. This causes larger, human
	// readable files.
	JSON LogFormat = 1
)

// Options for Log
type Options struct {
	// NoSync disables fsync after writes. This is less durable and puts the
	// log at risk of data loss when there's a server crash.
	NoSync bool
	// SegmentSize of each segment. This is just a target value, actual size
	// may differ. Default is 20 MB.
	SegmentSize int
	// LogFormat is the format of the log files. Default is Binary.
	LogFormat LogFormat
	// SegmentCacheSize is the maximum number of segments that will be held in
	// memory for caching. Increasing this value may enhance performance for
	// concurrent read operations. Default is 1
	SegmentCacheSize int
	// NoCopy allows for the Read() operation to return the raw underlying data
	// slice. This is an optimization to help minimize allocations. When this
	// option is set, do not modify the returned data because it may affect
	// other Read calls. Default false
	NoCopy bool
}

// DefaultOptions for Open().
var DefaultOptions = &Options{
	NoSync:           false,    // Fsync after every write
	SegmentSize:      20971520, // 20 MB log segment files.
	LogFormat:        Binary,   // Binary format is small and fast.
	SegmentCacheSize: 2,        // Number of cached in-memory segments
	NoCopy:           false,    // Make a new copy of data for every Read call.
}

// Log represents a write ahead log
type Log struct {
	mu         sync.RWMutex
	path       string      // absolute path to log directory
	opts       Options     // log options
	closed     bool        // log is closed
	corrupt    bool        // log may be corrupt
	segments   []*segment  // all known log segments
	firstIndex uint64      // index of the first entry in log
	lastIndex  uint64      // index of the last entry in log
	sfile      *os.File    // tail segment file handle
	wbatch     Batch       // reusable write batch
	scache     tinylru.LRU // segment entries cache
}

// segment represents a single segment file.
type segment struct {
	path  string // path of segment file
	index uint64 // first index of segment
	ebuf  []byte // cached entries buffer
	epos  []bpos // cached entries positions in buffer
}

type bpos struct {
	pos int // byte position
	end int // one byte past pos
}

// Open a new write ahead log
func Open(path string, opts *Options) (*Log, error) {
	if opts == nil {
		opts = DefaultOptions
	}
	if opts.SegmentCacheSize <= 0 {
		opts.SegmentCacheSize = DefaultOptions.SegmentCacheSize
	}
	if opts.SegmentSize <= 0 {
		opts.SegmentSize = DefaultOptions.SegmentSize
	}
	var err error
	path, err = abs(path)
	if err != nil {
		return nil, err
	}
	l := &Log{path: path, opts: *opts}
	l.scache.Resize(l.opts.SegmentCacheSize)
	if err := os.MkdirAll(path, 0777); err != nil {
		return nil, err
	}
	if err := l.load(); err != nil {
		return nil, err
	}
	return l, nil
}

func abs(path string) (string, error) {
	if path == ":memory:" {
		return "", errors.New("in-memory log not supported")
	}
	return filepath.Abs(path)
}

func (l *Log) pushCache(segIdx int) {
	_, _, _, v, evicted :=
		l.scache.SetEvicted(segIdx, l.segments[segIdx])
	if evicted {
		s := v.(*segment)
		s.ebuf = nil
		s.epos = nil
	}
}

// load all the segments. This operation also cleans up any START/END segments.
func (l *Log) load() error {
	fis, err := ioutil.ReadDir(l.path)
	if err != nil {
		return err
	}
	startIdx := -1
	endIdx := -1
	for _, fi := range fis {
		name := fi.Name()
		if fi.IsDir() || len(name) < 20 {
			continue
		}
		index, err := strconv.ParseUint(name[:20], 10, 64)
		if err != nil || index == 0 {
			continue
		}
		isStart := len(name) == 26 && strings.HasSuffix(name, ".START")
		isEnd := len(name) == 24 && strings.HasSuffix(name, ".END")
		if len(name) == 20 || isStart || isEnd {
			if isStart {
				startIdx = len(l.segments)
			} else if isEnd && endIdx == -1 {
				endIdx = len(l.segments)
			}
			l.segments = append(l.segments, &segment{
				index: index,
				path:  filepath.Join(l.path, name),
			})
		}
	}
	if len(l.segments) == 0 {
		// Create a new log
		l.segments = append(l.segments, &segment{
			index: 1,
			path:  filepath.Join(l.path, segmentName(1)),
		})
		l.firstIndex = 1
		l.lastIndex = 0
		l.sfile, err = os.Create(l.segments[0].path)
		return err
	}
	// Open existing log. Clean up log if START of END segments exists.
	if startIdx != -1 {
		if endIdx != -1 {
			// There should not be a START and END at the same time
			return ErrCorrupt
		}
		// Delete all files leading up to START
		for i := 0; i < startIdx; i++ {
			if err := os.Remove(l.segments[i].path); err != nil {
				return err
			}
		}
		l.segments = append([]*segment{}, l.segments[startIdx:]...)
		// Rename the START segment
		orgPath := l.segments[0].path
		finalPath := orgPath[:len(orgPath)-len(".START")]
		err := os.Rename(orgPath, finalPath)
		if err != nil {
			return err
		}
		l.segments[0].path = finalPath
	}
	if endIdx != -1 {
		// Delete all files following END
		for i := len(l.segments) - 1; i > endIdx; i-- {
			if err := os.Remove(l.segments[i].path); err != nil {
				return err
			}
		}
		l.segments = append([]*segment{}, l.segments[:endIdx+1]...)
		if len(l.segments) > 1 && l.segments[len(l.segments)-2].index ==
			l.segments[len(l.segments)-1].index {
			// remove the segment prior to the END segment because it shares
			// the same starting index.
			l.segments[len(l.segments)-2] = l.segments[len(l.segments)-1]
			l.segments = l.segments[:len(l.segments)-1]
		}
		// Rename the END segment
		orgPath := l.segments[len(l.segments)-1].path
		finalPath := orgPath[:len(orgPath)-len(".END")]
		err := os.Rename(orgPath, finalPath)
		if err != nil {
			return err
		}
		l.segments[len(l.segments)-1].path = finalPath
	}
	l.firstIndex = l.segments[0].index
	// Open the last segment for appending
	lseg := l.segments[len(l.segments)-1]
	l.sfile, err = os.OpenFile(lseg.path, os.O_WRONLY, 0666)
	if err != nil {
		return err
	}
	if _, err := l.sfile.Seek(0, 2); err != nil {
		return err
	}
	// Load the last segment entries
	if err := l.loadSegmentEntries(lseg); err != nil {
		return err
	}
	l.lastIndex = lseg.index + uint64(len(lseg.epos)) - 1
	return nil
}

// segmentName returns a 20-byte textual representation of an index
// for lexical ordering. This is used for the file names of log segments.
func segmentName(index uint64) string {
	return fmt.Sprintf("%020d", index)
}

// Close the log.
func (l *Log) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		if l.corrupt {
			return ErrCorrupt
		}
		return ErrClosed
	}
	if err := l.sfile.Sync(); err != nil {
		return err
	}
	if err := l.sfile.Close(); err != nil {
		return err
	}
	l.closed = true
	if l.corrupt {
		return ErrCorrupt
	}
	return nil
}

// Write an entry to the log.
func (l *Log) Write(index uint64, data []byte) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.corrupt {
		return ErrCorrupt
	} else if l.closed {
		return ErrClosed
	}
	l.wbatch.Clear()
	l.wbatch.Write(index, data)
	return l.writeBatch(&l.wbatch)
}

func (l *Log) appendEntry(dst []byte, index uint64, data []byte) (out []byte,
	epos bpos) {
	if l.opts.LogFormat == JSON {
		return appendJSONEntry(dst, index, data)
	}
	return appendBinaryEntry(dst, data)
}

// Cycle the old segment for a new segment.
func (l *Log) cycle() error {
	if err := l.sfile.Sync(); err != nil {
		return err
	}
	if err := l.sfile.Close(); err != nil {
		return err
	}
	// cache the previous segment
	l.pushCache(len(l.segments) - 1)
	s := &segment{
		index: l.lastIndex + 1,
		path:  filepath.Join(l.path, segmentName(l.lastIndex+1)),
	}
	var err error
	l.sfile, err = os.Create(s.path)
	if err != nil {
		return err
	}
	l.segments = append(l.segments, s)
	return nil
}

func appendJSONEntry(dst []byte, index uint64, data []byte) (out []byte,
	epos bpos) {
	// {"index":number,"data":string}
	mark := len(dst)
	dst = append(dst, `{"index":"`...)
	dst = strconv.AppendUint(dst, index, 10)
	dst = append(dst, `","data":`...)
	dst = appendJSONData(dst, data)
	dst = append(dst, '}', '\n')
	return dst, bpos{mark, len(dst)}
}

func appendJSONData(dst []byte, s []byte) []byte {
	if utf8.Valid(s) {
		b, _ := json.Marshal(*(*string)(unsafe.Pointer(&s)))
		dst = append(dst, '"', '+')
		return append(dst, b[1:]...)
	}
	dst = append(dst, '"', '$')
	dst = append(dst, base64.URLEncoding.EncodeToString(s)...)
	return append(dst, '"')
}

func appendBinaryEntry(dst []byte, data []byte) (out []byte, epos bpos) {
	// data_size + data
	pos := len(dst)
	dst = appendUvarint(dst, uint64(len(data)))
	dst = append(dst, data...)
	return dst, bpos{pos, len(dst)}
}

func appendUvarint(dst []byte, x uint64) []byte {
	var buf [10]byte
	n := binary.PutUvarint(buf[:], x)
	dst = append(dst, buf[:n]...)
	return dst
}

// Batch of entries. Used to write multiple entries at once using WriteBatch().
type Batch struct {
	entries []batchEntry
	datas   []byte
}

type batchEntry struct {
	index uint64
	size  int
}

// Write an entry to the batch
func (b *Batch) Write(index uint64, data []byte) {
	b.entries = append(b.entries, batchEntry{index, len(data)})
	b.datas = append(b.datas, data...)
}

// Clear the batch for reuse.
func (b *Batch) Clear() {
	b.entries = b.entries[:0]
	b.datas = b.datas[:0]
}

// WriteBatch writes the entries in the batch to the log in the order that they
// were added to the batch. The batch is cleared upon a successful return.
func (l *Log) WriteBatch(b *Batch) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.corrupt {
		return ErrCorrupt
	} else if l.closed {
		return ErrClosed
	}
	if len(b.entries) == 0 {
		return nil
	}
	return l.writeBatch(b)
}

func (l *Log) writeBatch(b *Batch) error {
	// check that all indexes in batch are sane
	for i := 0; i < len(b.entries); i++ {
		if b.entries[i].index != l.lastIndex+uint64(i+1) {
			return ErrOutOfOrder
		}
	}
	// load the tail segment
	s := l.segments[len(l.segments)-1]
	if len(s.ebuf) > l.opts.SegmentSize {
		// tail segment has reached capacity. Close it and create a new one.
		if err := l.cycle(); err != nil {
			return err
		}
		s = l.segments[len(l.segments)-1]
	}

	mark := len(s.ebuf)
	datas := b.datas
	for i := 0; i < len(b.entries); i++ {
		data := datas[:b.entries[i].size]
		var epos bpos
		s.ebuf, epos = l.appendEntry(s.ebuf, b.entries[i].index, data)
		s.epos = append(s.epos, epos)
		if len(s.ebuf) >= l.opts.SegmentSize {
			// segment has reached capacity, cycle now
			if _, err := l.sfile.Write(s.ebuf[mark:]); err != nil {
				return err
			}
			l.lastIndex = b.entries[i].index
			if err := l.cycle(); err != nil {
				return err
			}
			s = l.segments[len(l.segments)-1]
			mark = 0
		}
		datas = datas[b.entries[i].size:]
	}
	if len(s.ebuf)-mark > 0 {
		if _, err := l.sfile.Write(s.ebuf[mark:]); err != nil {
			return err
		}
		l.lastIndex = b.entries[len(b.entries)-1].index
	}
	if !l.opts.NoSync {
		if err := l.sfile.Sync(); err != nil {
			return err
		}
	}
	b.Clear()
	return nil
}

// FirstIndex returns the index of the first entry in the log. Returns zero
// when log has no entries.
func (l *Log) FirstIndex() (index uint64, err error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.corrupt {
		return 0, ErrCorrupt
	} else if l.closed {
		return 0, ErrClosed
	}
	// We check the lastIndex for zero because the firstIndex is always one or
	// more, even when there's no entries
	if l.lastIndex == 0 {
		return 0, nil
	}
	return l.firstIndex, nil
}

// LastIndex returns the index of the last entry in the log. Returns zero when
// log has no entries.
func (l *Log) LastIndex() (index uint64, err error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.corrupt {
		return 0, ErrCorrupt
	} else if l.closed {
		return 0, ErrClosed
	}
	if l.lastIndex == 0 {
		return 0, nil
	}
	return l.lastIndex, nil
}

// findSegment performs a bsearch on the segments
func (l *Log) findSegment(index uint64) int {
	i, j := 0, len(l.segments)
	for i < j {
		h := i + (j-i)/2
		if index >= l.segments[h].index {
			i = h + 1
		} else {
			j = h
		}
	}
	return i - 1
}

func (l *Log) loadSegmentEntries(s *segment) error {
	data, err := ioutil.ReadFile(s.path)
	if err != nil {
		return err
	}
	ebuf := data
	var epos []bpos
	var pos int
	for exidx := s.index; len(data) > 0; exidx++ {
		var n int
		if l.opts.LogFormat == JSON {
			n, err = loadNextJSONEntry(data)
		} else {
			n, err = loadNextBinaryEntry(data)
		}
		if err != nil {
			return err
		}
		data = data[n:]
		epos = append(epos, bpos{pos, pos + n})
		pos += n
	}
	s.ebuf = ebuf
	s.epos = epos
	return nil
}

func loadNextJSONEntry(data []byte) (n int, err error) {
	// {"index":number,"data":string}
	idx := bytes.IndexByte(data, '\n')
	if idx == -1 {
		return 0, ErrCorrupt
	}
	line := data[:idx]
	dres := gjson.Get(*(*string)(unsafe.Pointer(&line)), "data")
	if dres.Type != gjson.String {
		return 0, ErrCorrupt
	}
	return idx + 1, nil
}

func loadNextBinaryEntry(data []byte) (n int, err error) {
	// data_size + data
	size, n := binary.Uvarint(data)
	if n <= 0 {
		return 0, ErrCorrupt
	}
	if uint64(len(data)-n) < size {
		return 0, ErrCorrupt
	}
	return n + int(size), nil
}

// loadSegment loads the segment entries into memory, pushes it to the front
// of the lru cache, and returns it.
func (l *Log) loadSegment(index uint64) (*segment, error) {
	// check the last segment first.
	lseg := l.segments[len(l.segments)-1]
	if index >= lseg.index {
		return lseg, nil
	}
	// check the most recent cached segment
	var rseg *segment
	l.scache.Range(func(_, v interface{}) bool {
		s := v.(*segment)
		if index >= s.index && index < s.index+uint64(len(s.epos)) {
			rseg = s
		}
		return false
	})
	if rseg != nil {
		return rseg, nil
	}
	// find in the segment array
	idx := l.findSegment(index)
	s := l.segments[idx]
	if len(s.epos) == 0 {
		// load the entries from cache
		if err := l.loadSegmentEntries(s); err != nil {
			return nil, err
		}
	}
	// push the segment to the front of the cache
	l.pushCache(idx)
	return s, nil
}

// Read an entry from the log. Returns a byte slice containing the data entry.
func (l *Log) Read(index uint64) (data []byte, err error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.corrupt {
		return nil, ErrCorrupt
	} else if l.closed {
		return nil, ErrClosed
	}
	if index == 0 || index < l.firstIndex || index > l.lastIndex {
		return nil, ErrNotFound
	}
	s, err := l.loadSegment(index)
	if err != nil {
		return nil, err
	}
	epos := s.epos[index-s.index]
	edata := s.ebuf[epos.pos:epos.end]
	if l.opts.LogFormat == JSON {
		return readJSON(edata)
	}
	// binary read
	size, n := binary.Uvarint(edata)
	if n <= 0 {
		return nil, ErrCorrupt
	}
	if uint64(len(edata)-n) < size {
		return nil, ErrCorrupt
	}
	if l.opts.NoCopy {
		data = edata[n : uint64(n)+size]
	} else {
		data = make([]byte, size)
		copy(data, edata[n:])
	}
	return data, nil
}

//go:noinline
func readJSON(edata []byte) ([]byte, error) {
	var data []byte
	s := gjson.Get(*(*string)(unsafe.Pointer(&edata)), "data").String()
	if len(s) > 0 && s[0] == '$' {
		var err error
		data, err = base64.URLEncoding.DecodeString(s[1:])
		if err != nil {
			return nil, ErrCorrupt
		}
	} else if len(s) > 0 && s[0] == '+' {
		data = make([]byte, len(s[1:]))
		copy(data, s[1:])
	} else {
		return nil, ErrCorrupt
	}
	return data, nil
}

// ClearCache clears the segment cache
func (l *Log) ClearCache() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.corrupt {
		return ErrCorrupt
	} else if l.closed {
		return ErrClosed
	}
	l.clearCache()
	return nil
}
func (l *Log) clearCache() {
	l.scache.Range(func(_, v interface{}) bool {
		s := v.(*segment)
		s.ebuf = nil
		s.epos = nil
		return true
	})
	l.scache = tinylru.LRU{}
	l.scache.Resize(l.opts.SegmentCacheSize)
}

// TruncateFront truncates the front of the log by removing all entries that
// are before the provided `index`. In other words the entry at
// `index` becomes the first entry in the log.
func (l *Log) TruncateFront(index uint64) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.corrupt {
		return ErrCorrupt
	} else if l.closed {
		return ErrClosed
	}
	return l.truncateFront(index)
}
func (l *Log) truncateFront(index uint64) (err error) {
	if index == 0 || l.lastIndex == 0 ||
		index < l.firstIndex || index > l.lastIndex {
		return ErrOutOfRange
	}
	if index == l.firstIndex {
		// nothing to truncate
		return nil
	}
	segIdx := l.findSegment(index)
	var s *segment
	s, err = l.loadSegment(index)
	if err != nil {
		return err
	}
	epos := s.epos[index-s.index:]
	ebuf := s.ebuf[epos[0].pos:]
	// Create a temp file contains the truncated segment.
	tempName := filepath.Join(l.path, "TEMP")
	err = func() error {
		f, err := os.Create(tempName)
		if err != nil {
			return err
		}
		defer f.Close()
		if _, err := f.Write(ebuf); err != nil {
			return err
		}
		if err := f.Sync(); err != nil {
			return err
		}
		return f.Close()
	}()
	// Rename the TEMP file to it's START file name.
	startName := filepath.Join(l.path, segmentName(index)+".START")
	if err = os.Rename(tempName, startName); err != nil {
		return err
	}
	// The log was truncated but still needs some file cleanup. Any errors
	// following this message will not cause an on-disk data ocorruption, but
	// may cause an inconsistency with the current program, so we'll return
	// ErrCorrupt so the the user can attempt a recover by calling Close()
	// followed by Open().
	defer func() {
		if v := recover(); v != nil {
			err = ErrCorrupt
			l.corrupt = true
		}
	}()
	if segIdx == len(l.segments)-1 {
		// Close the tail segment file
		if err = l.sfile.Close(); err != nil {
			return err
		}
	}
	// Delete truncated segment files
	for i := 0; i <= segIdx; i++ {
		if err = os.Remove(l.segments[i].path); err != nil {
			return err
		}
	}
	// Rename the START file to the final truncated segment name.
	newName := filepath.Join(l.path, segmentName(index))
	if err = os.Rename(startName, newName); err != nil {
		return err
	}
	s.path = newName
	s.index = index
	if segIdx == len(l.segments)-1 {
		// Reopen the tail segment file
		if l.sfile, err = os.OpenFile(newName, os.O_WRONLY, 0666); err != nil {
			return err
		}
		var n int64
		if n, err = l.sfile.Seek(0, 2); err != nil {
			return err
		}
		if n != int64(len(ebuf)) {
			err = errors.New("invalid seek")
			return err
		}
		// Load the last segment entries
		if err = l.loadSegmentEntries(s); err != nil {
			return err
		}
	}
	l.segments = append([]*segment{}, l.segments[segIdx:]...)
	l.firstIndex = index
	l.clearCache()
	return nil
}

// TruncateBack truncates the back of the log by removing all entries that
// are after the provided `index`. In other words the entry at `index`
// becomes the last entry in the log.
func (l *Log) TruncateBack(index uint64) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.corrupt {
		return ErrCorrupt
	} else if l.closed {
		return ErrClosed
	}
	return l.truncateBack(index)
}

func (l *Log) truncateBack(index uint64) (err error) {
	if index == 0 || l.lastIndex == 0 ||
		index < l.firstIndex || index > l.lastIndex {
		return ErrOutOfRange
	}
	if index == l.lastIndex {
		// nothing to truncate
		return nil
	}
	segIdx := l.findSegment(index)
	var s *segment
	s, err = l.loadSegment(index)
	if err != nil {
		return err
	}
	epos := s.epos[:index-s.index+1]
	ebuf := s.ebuf[:epos[len(epos)-1].end]
	// Create a temp file contains the truncated segment.
	tempName := filepath.Join(l.path, "TEMP")
	err = func() error {
		f, err := os.Create(tempName)
		if err != nil {
			return err
		}
		defer f.Close()
		if _, err := f.Write(ebuf); err != nil {
			return err
		}
		if err := f.Sync(); err != nil {
			return err
		}
		return f.Close()
	}()
	// Rename the TEMP file to it's END file name.
	endName := filepath.Join(l.path, segmentName(s.index)+".END")
	if err = os.Rename(tempName, endName); err != nil {
		return err
	}
	// The log was truncated but still needs some file cleanup. Any errors
	// following this message will not cause an on-disk data ocorruption, but
	// may cause an inconsistency with the current program, so we'll return
	// ErrCorrupt so the the user can attempt a recover by calling Close()
	// followed by Open().
	defer func() {
		if v := recover(); v != nil {
			err = ErrCorrupt
			l.corrupt = true
		}
	}()

	// Close the tail segment file
	if err = l.sfile.Close(); err != nil {
		return err
	}
	// Delete truncated segment files
	for i := segIdx; i < len(l.segments); i++ {
		if err = os.Remove(l.segments[i].path); err != nil {
			return err
		}
	}
	// Rename the END file to the final truncated segment name.
	newName := filepath.Join(l.path, segmentName(s.index))
	if err = os.Rename(endName, newName); err != nil {
		return err
	}
	// Reopen the tail segment file
	if l.sfile, err = os.OpenFile(newName, os.O_WRONLY, 0666); err != nil {
		return err
	}
	var n int64
	n, err = l.sfile.Seek(0, 2)
	if err != nil {
		return err
	}
	if n != int64(len(ebuf)) {
		err = errors.New("invalid seek")
		return err
	}
	s.path = newName
	l.segments = append([]*segment{}, l.segments[:segIdx+1]...)
	l.lastIndex = index
	l.clearCache()
	if err = l.loadSegmentEntries(s); err != nil {
		return err
	}
	return nil
}

// Sync performs an fsync on the log. This is not necessary when the
// NoSync option is set to false.
func (l *Log) Sync() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.corrupt {
		return ErrCorrupt
	} else if l.closed {
		return ErrClosed
	}
	return l.sfile.Sync()
}

func must(v interface{}, err error) interface{} {
	if err != nil {
		panic(err)
	}
	return v
}
