/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package eventlog

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/pkg/errors"
	"google.golang.org/protobuf/proto"

	"github.com/hyperledger-labs/mirbft/pkg/pb/recording"
	"github.com/hyperledger-labs/mirbft/pkg/pb/state"
)

type RecorderOpt interface{}

type timeSourceOpt func() int64

// TimeSource can be used to override the default time source
// for an interceptor.  This can be useful for changing the
// granularity of the timestamps, or picking some externally
// supplied sync point when trying to synchronize logs.
// The default time source will timestamp with the time, in
// milliseconds since the interceptor was created.
func TimeSourceOpt(source func() int64) RecorderOpt {
	return timeSourceOpt(source)
}

type retainRequestDataOpt struct{}

// RetainRequestDataOpt indicates that the full request data should be
// embedded into the logs.  Usually, this option is undesirable since although
// request data is not actually needed to replay a log, the request data
// increases the size of the log substantially and the request data
// may be considered sensitive so is therefore unsuitable for
// debug/service.  However, for debugging application code, sometimes,
// having the complete logs is available, so this option may be set
// to true.
func RetainRequestDataOpt() RecorderOpt {
	return retainRequestDataOpt{}
}

type compressionLevelOpt int

// DefaultCompressionLevel is used for event capture when not overridden.
// In emperical tests, best speed was only a few tenths of a percent
// worse than best compression, but your results may vary.
const DefaultCompressionLevel = gzip.BestSpeed

// CompressionLevelOpt takes any of the compression levels supported
// by the golang standard gzip package.
func CompressionLevelOpt(level int) RecorderOpt {
	return compressionLevelOpt(level)
}

// DefaultBuffer size is the number of unwritten state events which
// may be held in queue before blocking.
const DefaultBufferSize = 5000

type bufferSizeOpt int

// BufferSizeOpt overrides the default buffer size of the
// interceptor buffer.  Once the buffer overflows, the state
// machine will be blocked from receiving new state events
// until the buffer has room.
func BufferSizeOpt(size int) RecorderOpt {
	return bufferSizeOpt(size)
}

// Recorder is intended to be used as an imlementation of the
// mirbft.EventInterceptor interface.  It receives state events,
// serializes them, compresses them, and writes them to a stream.
type Recorder struct {
	nodeID            uint64
	timeSource        func() int64
	compressionLevel  int
	retainRequestData bool
	eventC            chan eventTime
	doneC             chan struct{}
	exitC             chan struct{}

	exitErr      error
	exitErrMutex sync.Mutex
}

func NewRecorder(nodeID uint64, dest io.Writer, opts ...RecorderOpt) *Recorder {
	startTime := time.Now()

	i := &Recorder{
		nodeID: nodeID,
		timeSource: func() int64 {
			return time.Since(startTime).Milliseconds()
		},
		compressionLevel: DefaultCompressionLevel,
		eventC:           make(chan eventTime, DefaultBufferSize),
		doneC:            make(chan struct{}),
		exitC:            make(chan struct{}),
	}

	for _, opt := range opts {
		switch v := opt.(type) {
		case timeSourceOpt:
			i.timeSource = v
		case retainRequestDataOpt:
			i.retainRequestData = true
		case compressionLevelOpt:
			i.compressionLevel = int(v)
		case bufferSizeOpt:
			i.eventC = make(chan eventTime, v)
		}
	}

	go i.run(dest)

	return i
}

type eventTime struct {
	event *state.Event
	time  int64
}

// Intercept takes an event and enqueues it into the event buffer.
// If there is no room in the buffer, it blocks.  If draining the buffer
// to the output stream has completed (successfully or otherwise), Intercept
// returns an error.
func (i *Recorder) Intercept(event *state.Event) error {
	select {
	case i.eventC <- eventTime{
		event: event,
		time:  i.timeSource(),
	}:
		return nil
	case <-i.exitC:
		i.exitErrMutex.Lock()
		defer i.exitErrMutex.Unlock()
		return i.exitErr
	}
}

// Stop must be invoked to release the resources associated with this
// Interceptor, and should only be invoked after the mir node has completely
// exited.  The returned error
func (i *Recorder) Stop() error {
	close(i.doneC)
	<-i.exitC
	i.exitErrMutex.Lock()
	defer i.exitErrMutex.Unlock()
	if i.exitErr == errStopped {
		return nil
	}
	return i.exitErr
}

var errStopped = fmt.Errorf("interceptor stopped at caller request")

func (i *Recorder) run(dest io.Writer) (exitErr error) {
	defer func() {
		i.exitErrMutex.Lock()
		i.exitErr = exitErr
		i.exitErrMutex.Unlock()
		close(i.exitC)
	}()

	gzWriter, err := gzip.NewWriterLevel(dest, i.compressionLevel)
	if err != nil {
		return err
	}
	defer gzWriter.Close()

	write := func(eventTime eventTime) error {
		return WriteRecordedEvent(gzWriter, &recording.Event{
			NodeId:     i.nodeID,
			Time:       eventTime.time,
			StateEvent: eventTime.event,
		})
	}

	for {
		select {
		case <-i.doneC:
			for {
				select {
				case event := <-i.eventC:

					if err := write(event); err != nil {
						return errors.WithMessage(err, "error serializing to stream")
					}
				default:
					return errStopped
				}
			}
		case event := <-i.eventC:
			if err := write(event); err != nil {
				return errors.WithMessage(err, "error serializing to stream")
			}
		}
	}
}

func WriteRecordedEvent(writer io.Writer, event *recording.Event) error {
	return writeSizePrefixedProto(writer, event)
}

func writeSizePrefixedProto(dest io.Writer, msg proto.Message) error {
	msgBytes, err := proto.Marshal(msg)
	if err != nil {
		return errors.WithMessage(err, "could not marshal")
	}

	lenBuf := make([]byte, binary.MaxVarintLen64)
	n := binary.PutVarint(lenBuf, int64(len(msgBytes)))
	if _, err = dest.Write(lenBuf[:n]); err != nil {
		return errors.WithMessage(err, "could not write length prefix")
	}

	if _, err = dest.Write(msgBytes); err != nil {
		return errors.WithMessage(err, "could not write message")
	}

	return nil
}

type Reader struct {
	buffer   *bytes.Buffer
	gzReader *gzip.Reader
	source   *bufio.Reader
}

func NewReader(source io.Reader) (*Reader, error) {
	gzReader, err := gzip.NewReader(source)
	if err != nil {
		return nil, errors.WithMessage(err, "could not read source as a gzip stream")
	}

	return &Reader{
		buffer:   &bytes.Buffer{},
		gzReader: gzReader,
		source:   bufio.NewReader(gzReader),
	}, nil
}

func (r *Reader) ReadEvent() (*recording.Event, error) {
	re := &recording.Event{}
	err := readSizePrefixedProto(r.source, re, r.buffer)
	if err == io.EOF {
		r.gzReader.Close()
		return re, err
	}
	if err != nil {
		return nil, errors.WithMessage(err, "error reading event")
	}
	r.buffer.Reset()

	return re, nil
}

func readSizePrefixedProto(reader *bufio.Reader, msg proto.Message, buffer *bytes.Buffer) error {
	l, err := binary.ReadVarint(reader)
	if err != nil {
		if err == io.EOF {
			return err
		}
		return errors.WithMessage(err, "could not read size prefix")
	}

	buffer.Grow(int(l))

	if _, err := io.CopyN(buffer, reader, l); err != nil {
		return errors.WithMessage(err, "could not read message")
	}

	if err := proto.Unmarshal(buffer.Bytes(), msg); err != nil {
		return errors.WithMessage(err, "could not unmarshal message")
	}

	return nil
}
