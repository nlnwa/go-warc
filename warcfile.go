/*
 * Copyright 2021 National Library of Norway.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *       http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package gowarc

import (
	"bufio"
	"compress/gzip"
	"fmt"
	"github.com/nlnwa/gowarc/internal"
	"github.com/nlnwa/gowarc/internal/countingreader"
	"github.com/nlnwa/gowarc/internal/timestamp"
	"github.com/prometheus/tsdb/fileutil"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// WarcFileNameGenerator is the interface that wraps the NewWarcfileName function.
type WarcFileNameGenerator interface {
	// NewWarcfileName returns a directory (might be the empty string for current directory) and a file name
	NewWarcfileName() (string, string)
}

// PatternNameGenerator implements the WarcFileNameGenerator.
type PatternNameGenerator struct {
	Directory string // Directory to store warcfiles. Defaults to the empty string
	Prefix    string // Prefix available to be used in pattern. Defaults to the empty string
	Serial    int32  // Serial number available for use in pattern. It is atomically increased with every generated file name.
	Pattern   string // Pattern for generated file name. Defaults to: "%{prefix}s%{ts}s-%04{serial}d-%{ip}s.warc"
}

const defaultPattern = "%{prefix}s%{ts}s-%04{serial}d-%{ip}s.warc"

// Allow overriding of time.Now for tests
var now = time.Now

func (g *PatternNameGenerator) NewWarcfileName() (string, string) {
	if g.Pattern == "" {
		g.Pattern = defaultPattern
	}
	params := map[string]interface{}{
		"prefix": g.Prefix,
		"ts":     timestamp.UTC14(now()),
		"serial": atomic.AddInt32(&g.Serial, 1),
		"ip":     internal.GetOutboundIP()}

	name := internal.Sprintt(g.Pattern, params)
	return g.Directory, name
}

type WarcFileWriter struct {
	writers []*singleWarcFileWriter
	jobs    chan *job
}

// NewWarcFileWriter creates a new WarcFileWriter with the supplied options.
func NewWarcFileWriter(opts ...WarcFileWriterOption) *WarcFileWriter {
	o := defaultwarcFileWriterOptions()
	for _, opt := range opts {
		opt.apply(&o)
	}
	w := &WarcFileWriter{}
	w.jobs = make(chan *job)
	for i := 0; i < o.maxConcurrentWriters; i++ {
		writer := &singleWarcFileWriter{opts: &o}
		w.writers = append(w.writers, writer)
		go worker(writer, w.jobs)
	}
	return w
}

func worker(w *singleWarcFileWriter, jobs <-chan *job) {
	for j := range jobs {
		j.fileOffset, j.fileName, j.bytesWritten, j.err = w.Write(j.record)
		j.wg.Done()
	}
}

type job struct {
	record       WarcRecord
	fileName     string
	fileOffset   int64
	bytesWritten int64
	err          error
	wg           sync.WaitGroup
}

// Write marshals a WarcRecord to file.
// Returns the number of uncompressed bytes written.
func (w *WarcFileWriter) Write(record WarcRecord) (int64, string, int64, error) {
	job := &job{
		record: record,
		wg:     sync.WaitGroup{},
	}
	job.wg.Add(1)
	w.jobs <- job
	job.wg.Wait()
	return job.fileOffset, job.fileName, job.bytesWritten, job.err
}

// Close closes the current files beeing written to.
// It is legal to call Write after close, but then new files will be created.
func (w *WarcFileWriter) Close() error {
	var err multiErr
	for _, writer := range w.writers {
		if e := writer.Close(); e != nil {
			err = append(err, e)
		}
	}
	if err != nil {
		return fmt.Errorf("closing error: %w", err)
	}
	return nil
}

// Shutdown closes the current file being written to and then releases all resources used by the WarcFileWriter.
// Calling Write after Shutdown will panic.
func (w *WarcFileWriter) Shutdown() error {
	close(w.jobs)
	return w.Close()
}

type singleWarcFileWriter struct {
	opts              *warcFileWriterOptions
	currentFileName   string
	currentFile       *os.File
	currentFileSize   int64
	currentWarcInfoId string
	writeLock         sync.Mutex
}

func (w *singleWarcFileWriter) Write(record WarcRecord) (int64, string, int64, error) {
	w.writeLock.Lock()
	defer w.writeLock.Unlock()

	// Calculate max record size when segmentation is enabled
	var maxRecordSize int64
	if w.opts.useSegmentation {
		if w.opts.compress {
			maxRecordSize = int64(float64(w.opts.maxFileSize) / w.opts.expectedCompressionRatio)
		} else {
			maxRecordSize = w.opts.maxFileSize
		}
	}

	// Check if the current file has space for the new record
	if w.currentFile != nil && w.opts.maxFileSize > 0 {
		s := record.WarcHeader().Get(ContentLength)
		if s != "" {
			size, err := strconv.ParseInt(s, 10, 64)
			if w.opts.compress {
				// Take compression in account when evaluating if record will fit file
				size = int64(float64(size) * w.opts.expectedCompressionRatio)
			}
			if err != nil {
				return 0, "", 0, err
			}
			if w.currentFileSize > 0 && (w.currentFileSize+size) > w.opts.maxFileSize {
				// Not enough space in file, close it so a new will be created
				err = w.close()
				if err != nil {
					return 0, "", 0, err
				}
			}
		}
	}

	// Create new file if necessary
	if w.currentFile == nil {
		if err := w.createFile(); err != nil {
			return 0, "", 0, err
		}
	}

	offset := w.currentFileSize
	size, err := w.writeRecord(w.currentFile, record, maxRecordSize)
	if err != nil {
		return 0, "", 0, err
	}
	// sync file to reduce possibility of half written records in case of crash
	if err := w.currentFile.Sync(); err != nil {
		return 0, "", 0, err
	}
	fi, err := w.currentFile.Stat()
	if err != nil {
		return 0, "", 0, err
	}
	w.currentFileSize = fi.Size()
	return offset, w.currentFileName, size, err
}

func (w *singleWarcFileWriter) createFile() error {
	var suffix string
	if w.opts.compress {
		suffix = w.opts.compressSuffix
	}
	dir, fileName := w.opts.nameGenerator.NewWarcfileName()
	fileName += suffix
	path := dir
	if path != "" && !strings.HasSuffix(path, "/") {
		path += "/"
	}
	path += fileName + suffix + w.opts.openFileSuffix

	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0666)
	if err != nil {
		return err
	}
	w.currentFileName = fileName
	w.currentFile = file

	if w.opts.warcInfoFunc != nil {
		if _, err := w.createWarcInfoRecord(fileName); err != nil {
			return err
		}
	}
	return nil
}

func (w *singleWarcFileWriter) writeRecord(writer io.Writer, record WarcRecord, maxRecordSize int64) (int64, error) {
	if w.opts.compress {
		gz := gzip.NewWriter(writer)
		defer func() { _ = gz.Close() }()
		writer = gz
	}
	if w.currentWarcInfoId != "" {
		record.WarcHeader().Set(WarcWarcinfoID, w.currentWarcInfoId)
	}
	nextRec, size, err := w.opts.marshaler.Marshal(writer, record, maxRecordSize)
	if err != nil {
		return size, err
	}
	if nextRec != nil {
		_, _, s, e := w.Write(nextRec)
		s += size
		return s, e
	}
	return size, nil
}

func (w *singleWarcFileWriter) createWarcInfoRecord(fileName string) (int64, error) {
	r := NewRecordBuilder(Warcinfo)
	r.AddWarcHeader(WarcDate, timestamp.UTCW3cIso8601(now()))
	r.AddWarcHeader(WarcFilename, fileName)
	r.AddWarcHeader(ContentType, ApplicationWarcFields)

	if err := w.opts.warcInfoFunc(r); err != nil {
		return 0, err
	}

	warcinfo, _, err := r.Build()
	if err != nil {
		return 0, err
	}
	w.currentWarcInfoId = ""
	n, err := w.writeRecord(w.currentFile, warcinfo, 0)
	if err != nil {
		return 0, err
	}
	w.currentWarcInfoId = warcinfo.WarcHeader().Get(WarcRecordID)
	// sync file to reduce possibility of half written records in case of crash
	if err := w.currentFile.Sync(); err != nil {
		return 0, err
	}
	fi, err := w.currentFile.Stat()
	if err != nil {
		return 0, err
	}
	w.currentFileSize = fi.Size()
	return n, err
}

// Close closes the current file being written to.
// It is legal to call Write after close, but then a new file will be opened.
func (w *singleWarcFileWriter) Close() error {
	w.writeLock.Lock()
	defer w.writeLock.Unlock()
	return w.close()
}

// Close closes the current file being written to.
// It is legal to call Write after close, but then a new file will be opened.
func (w *singleWarcFileWriter) close() error {
	if w.currentFile != nil {
		f := w.currentFile
		w.currentFile = nil
		w.currentFileName = ""
		if err := f.Close(); err != nil {
			return err
		}
		if err := fileutil.Rename(f.Name(), strings.TrimSuffix(f.Name(), w.opts.openFileSuffix)); err != nil {
			return err
		}
	}
	return nil
}

type WarcFileReader struct {
	file           *os.File
	initialOffset  int64
	offset         int64
	warcReader     Unmarshaler
	countingReader *countingreader.Reader
	bufferedReader *bufio.Reader
	currentRecord  WarcRecord
}

func NewWarcFileReader(filename string, offset int64, opts ...WarcRecordOption) (*WarcFileReader, error) {
	file, err := os.Open(filename) // For read access.
	if err != nil {
		return nil, err
	}

	wf := &WarcFileReader{
		file:       file,
		offset:     offset,
		warcReader: NewUnmarshaler(opts...),
	}
	_, err = file.Seek(offset, 0)
	if err != nil {
		return nil, err
	}

	wf.countingReader = countingreader.New(file)
	wf.initialOffset = offset
	wf.bufferedReader = bufio.NewReaderSize(wf.countingReader, 4*1024)
	return wf, nil
}

func (wf *WarcFileReader) Next() (WarcRecord, int64, *Validation, error) {
	var validation *Validation
	if wf.currentRecord != nil {
		if err := wf.currentRecord.Close(); err != nil {
			return nil, wf.offset, validation, err
		}
	}
	wf.offset = wf.initialOffset + wf.countingReader.N() - int64(wf.bufferedReader.Buffered())
	fs, _ := wf.file.Stat()
	if fs.Size() <= wf.offset {
		wf.offset = 0
	}

	var err error
	var recordOffset int64
	wf.currentRecord, recordOffset, validation, err = wf.warcReader.Unmarshal(wf.bufferedReader)
	return wf.currentRecord, wf.offset + recordOffset, validation, err
}

func (wf *WarcFileReader) Close() error {
	return wf.file.Close()
}

// Options for Warc file writer
type warcFileWriterOptions struct {
	maxFileSize              int64
	compress                 bool
	expectedCompressionRatio float64
	useSegmentation          bool
	compressSuffix           string
	openFileSuffix           string
	nameGenerator            WarcFileNameGenerator
	marshaler                Marshaler
	maxConcurrentWriters     int
	warcInfoFunc             func(recordBuilder WarcRecordBuilder) error
}

// WarcFileWriterOption configures how to write WARC files.
type WarcFileWriterOption interface {
	apply(*warcFileWriterOptions)
}

// funcWarcFileWriterOption wraps a function that modifies warcFileWriterOptions into an
// implementation of the WarcFileWriterOption interface.
type funcWarcFileWriterOption struct {
	f func(*warcFileWriterOptions)
}

func (fo *funcWarcFileWriterOption) apply(po *warcFileWriterOptions) {
	fo.f(po)
}

func newFuncWarcFileOption(f func(*warcFileWriterOptions)) *funcWarcFileWriterOption {
	return &funcWarcFileWriterOption{
		f: f,
	}
}

func defaultwarcFileWriterOptions() warcFileWriterOptions {
	return warcFileWriterOptions{
		maxFileSize:              1024 ^ 3, // 1 GiB
		compress:                 true,
		expectedCompressionRatio: .5,
		useSegmentation:          false,
		compressSuffix:           ".gz",
		openFileSuffix:           ".open",
		nameGenerator:            &PatternNameGenerator{},
		marshaler:                &defaultMarshaler{},
		maxConcurrentWriters:     1,
	}
}

// WithMaxFileSize sets the max size of the Warc file before creating a new one.
// defaults to 1 GiB
func WithMaxFileSize(size int64) WarcFileWriterOption {
	return newFuncWarcFileOption(func(o *warcFileWriterOptions) {
		o.maxFileSize = size
	})
}

// WithCompression sets if writer should write compressed WARC files.
// defaults to true
func WithCompression(compress bool) WarcFileWriterOption {
	return newFuncWarcFileOption(func(o *warcFileWriterOptions) {
		o.compress = compress
	})
}

// WithSegmentation sets if writer should use segmentation for large WARC records.
// defaults to false
func WithSegmentation() WarcFileWriterOption {
	return newFuncWarcFileOption(func(o *warcFileWriterOptions) {
		o.useSegmentation = true
	})
}

// WithCompressedFileSuffix sets a suffix to be added after the name generated by the WarcFileNameGenerator id compression is on.
// defaults to ".gz"
func WithCompressedFileSuffix(suffix string) WarcFileWriterOption {
	return newFuncWarcFileOption(func(o *warcFileWriterOptions) {
		o.compressSuffix = suffix
	})
}

// WithOpenFileSuffix sets a suffix to be added to the file name while the file is open for writing.
// The suffix is automatically removed when the file is closed.
// defaults to ".open"
func WithOpenFileSuffix(suffix string) WarcFileWriterOption {
	return newFuncWarcFileOption(func(o *warcFileWriterOptions) {
		o.openFileSuffix = suffix
	})
}

// WithFileNameGenerator sets the WarcFileNameGenerator to use for generating new Warc file names.
// defaults to defaultGenerator
func WithFileNameGenerator(generator WarcFileNameGenerator) WarcFileWriterOption {
	return newFuncWarcFileOption(func(o *warcFileWriterOptions) {
		o.nameGenerator = generator
	})
}

// WithMarshaler sets the Warc record marshaler to use.
// defaults to defaultMarshaler
func WithMarshaler(marshaler Marshaler) WarcFileWriterOption {
	return newFuncWarcFileOption(func(o *warcFileWriterOptions) {
		o.marshaler = marshaler
	})
}

// WithMaxConcurrentWriters sets the maximum number of Warc files that can be written to simultaneously.
// defaults to one
func WithMaxConcurrentWriters(count int) WarcFileWriterOption {
	return newFuncWarcFileOption(func(o *warcFileWriterOptions) {
		o.maxConcurrentWriters = count
	})
}

// WithExpectedCompressionRatio sets the expectd reduction in size when using compression.
// This value is used to decide if a record will fit into a Warcfile's MaxFileSize when using compression
// since it's not possible to know this before the record is written. If the value is far from the actual size reduction,
// a under- or overfilled file might be the result.
// defaults to .5 (half the uncompressed size)
func WithExpectedCompressionRatio(ratio float64) WarcFileWriterOption {
	return newFuncWarcFileOption(func(o *warcFileWriterOptions) {
		o.expectedCompressionRatio = ratio
	})
}

// WithWarcInfoFunc sets a warcinfo-record generator function to be called for every new WARC-file created.
// The function receives a WarcRecordBuilder which is prepopulated with WARC-Record-ID, WARC-Type, WARC-Date and Content-Type.
// After the submitted function returns, Content-Length and WARC-Block-Digest fields are calculated.
//
// When this option is set, records written to the warcfile will have the WARC-Warcinfo-ID automatically set to point
// to the generated warcinfo record.
//
// defaults nil (no generation of warcinfo record)
func WithWarcInfoFunc(f func(recordBuilder WarcRecordBuilder) error) WarcFileWriterOption {
	return newFuncWarcFileOption(func(o *warcFileWriterOptions) {
		o.warcInfoFunc = f
	})
}
