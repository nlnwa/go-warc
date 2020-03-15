/*
 * Copyright 2020 National Library of Norway.
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

package countingreader

import (
	"io"
	"sync/atomic"
)

// Reader counts the bytes read through it.
type Reader struct {
	r io.Reader
	n int64
	m int64
}

// NewReader makes a new Reader that counts the bytes
// read through it.
func New(r io.Reader) *Reader {
	return &Reader{
		r: r,
		m: -1,
	}
}

// NewLimited makes a new Reader that counts the bytes
// read through it.
//
// When maxBytes bytes are read, the next read will
// return io.EOF even though the underlying reader has more data.
func NewLimited(r io.Reader, maxBytes int64) *Reader {
	return &Reader{
		r: r,
		m: maxBytes,
	}
}

func (r *Reader) Read(p []byte) (n int, err error) {
	if r.m >= 0 {
		remaining := r.m - r.N()
		if int64(len(p)) > remaining {
			p = p[:remaining]
		}
		n, err = r.r.Read(p)
		atomic.AddInt64(&r.n, int64(n))

		if r.N() >= r.m {
			err = io.EOF
		}
	} else {
		n, err = r.r.Read(p)
		atomic.AddInt64(&r.n, int64(n))
	}
	return
}

// N gets the number of bytes that have been read
// so far.
func (r *Reader) N() int64 {
	return atomic.LoadInt64(&r.n)
}
