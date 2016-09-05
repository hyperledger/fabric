/*
Copyright IBM Corp. 2016 All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

		 http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package fsblkstorage

import (
	"bufio"
	"fmt"
	"io"
	"os"

	"github.com/golang/protobuf/proto"
)

////  WRITER ////
type blockfileWriter struct {
	filePath string
	file     *os.File
}

func newBlockfileWriter(filePath string) (*blockfileWriter, error) {
	writer := &blockfileWriter{filePath: filePath}
	return writer, writer.open()
}

func (w *blockfileWriter) truncateFile(targetSize int) error {
	fileStat, err := w.file.Stat()
	if err != nil {
		return err
	}
	if fileStat.Size() > int64(targetSize) {
		w.file.Truncate(int64(targetSize))
	}
	return nil
}

func (w *blockfileWriter) append(b []byte) error {
	_, err := w.file.Write(b)
	if err != nil {
		return err
	}
	w.file.Sync()
	return nil
}

func (w *blockfileWriter) open() error {
	file, err := os.OpenFile(w.filePath, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0660)
	if err != nil {
		return err
	}
	w.file = file
	return nil
}

func (w *blockfileWriter) close() error {
	return w.file.Close()
}

////  READER ////
type blockfileReader struct {
	file *os.File
}

func newBlockfileReader(filePath string) (*blockfileReader, error) {
	file, err := os.OpenFile(filePath, os.O_RDONLY, 0600)
	if err != nil {
		return nil, err
	}
	reader := &blockfileReader{file}
	return reader, nil
}

func (r *blockfileReader) read(offset int, length int) ([]byte, error) {
	b := make([]byte, length)
	_, err := r.file.ReadAt(b, int64(offset))
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (r *blockfileReader) close() error {
	return r.file.Close()
}

type blockStream struct {
	file   *os.File
	reader *bufio.Reader
}

func newBlockStream(filePath string, offset int64) (*blockStream, error) {
	var file *os.File
	var err error
	if file, err = os.OpenFile(filePath, os.O_RDONLY, 0600); err != nil {
		return nil, err
	}

	var newPosition int64
	if newPosition, err = file.Seek(offset, 0); err != nil {
		return nil, err
	}
	if newPosition != offset {
		panic(fmt.Sprintf("Could not seek file [%s] to given offset [%d]. New position = [%d]", filePath, offset, newPosition))
	}
	s := &blockStream{file, bufio.NewReader(file)}
	return s, nil
}

func (s *blockStream) nextBlockBytes() ([]byte, error) {
	lenBytes, err := s.reader.Peek(8)
	if err == io.EOF {
		logger.Debugf("block stream reached end of file. Returning next block as nil")
		return nil, nil
	}
	len, n := proto.DecodeVarint(lenBytes)
	if _, err = s.reader.Discard(n); err != nil {
		return nil, err
	}
	blockBytes := make([]byte, len)
	if _, err = io.ReadAtLeast(s.reader, blockBytes, int(len)); err != nil {
		return nil, err
	}
	return blockBytes, nil
}

func (s *blockStream) close() error {
	return s.file.Close()
}
