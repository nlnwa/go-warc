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
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"mime"
)

var (
	colon           = []byte{':'}
	errEndOfHeaders = errors.New("EOH")
)

type warcfieldsParser struct {
	Options *warcRecordOptions
}

func (p *warcfieldsParser) parseLine(line []byte, nv WarcFields, pos *position) (WarcFields, error) {
	line = bytes.TrimRight(line, sphtcrlf)

	// Support for ‘encoded-word’ mechanism of [RFC2047]
	d := mime.WordDecoder{}
	l, err := d.DecodeHeader(string(line))
	if err != nil {
		return nv, newWrappedSyntaxError("error decoding line", pos, err)
	}
	line = []byte(l)

	fv := bytes.SplitN(line, colon, 2)
	if len(fv) != 2 {
		err = newSyntaxError("could not parse header line. Missing ':' in "+string(fv[0]), pos)
		return nv, err
	}

	name := string(bytes.Trim(fv[0], sphtcrlf))
	value := string(bytes.Trim(fv[1], sphtcrlf))

	nv.Add(name, value)
	return nv, nil
}

// readLine reads the next line from r.
// error is returned for syntax error or if r returns an error. If the error is fatal then line is nil.
// If line is not null it means that readLine was able to get something useful which could be used by a lenient
// parser even though err was not nil.
// nextChar returns the first character a new call to readLine would process.
func (p *warcfieldsParser) readLine(r *bufio.Reader, pos *position) (line []byte, nextChar byte, err error) {
	line, err = r.ReadBytes('\n')
	if err != nil {
		if err == io.EOF {
			err = errEndOfHeaders
		}
		line = bytes.Trim(line, sphtcrlf)
		return
	}
	if p.Options.errSyntax > ErrIgnore && (len(line) < 2 || line[len(line)-2] != '\r') {
		err = newSyntaxError("missing carriage return", pos)
		if p.Options.errSyntax == ErrFail {
			line = bytes.Trim(line, sphtcrlf)
			return
		}
	}
	line = bytes.Trim(line, sphtcrlf)

	n, e := r.Peek(1)
	if e == io.EOF {
		nextChar = 0
		return
	}

	nextChar = n[0]
	return
}

func (p *warcfieldsParser) Parse(r *bufio.Reader, validation *Validation, pos *position) (*WarcFields, error) {
	wf := WarcFields{}
	eoh := false

	for {
		line, nc, err := p.readLine(r, pos.incrLineNumber())
		if err != nil {
			if err == errEndOfHeaders {
				eoh = true
				if len(line) == 0 {
					return &wf, nil
				} else {
					switch p.Options.errSyntax {
					case ErrIgnore:
					case ErrWarn:
						validation.addError(newSyntaxError("missing newline", pos))
					case ErrFail:
						return nil, newSyntaxError("missing newline", pos)
					}
				}
			} else if errors.Is(err, gzip.ErrChecksum) {
				return nil, err
			} else {
				switch p.Options.errSyntax {
				case ErrIgnore:
				case ErrWarn:
					validation.addError(err)
				case ErrFail:
					return nil, err
				}
			}
		}

		// Check for continuation
		for nc == sp || nc == ht {
			var l []byte
			l, nc, err = p.readLine(r, pos.incrLineNumber())
			if err != nil {
				if l == nil {
					return nil, err
				}
				validation.addError(err)
			}
			line = append(line, ' ')
			line = append(line, l...)
		}

		wf, err = p.parseLine(line, wf, pos)
		if err != nil {
			switch p.Options.errSyntax {
			case ErrIgnore:
			case ErrWarn:
				validation.addError(err)
			case ErrFail:
				return nil, err
			}
		}

		if eoh {
			break
		}

		if nc == cr {
			l, err := r.ReadBytes('\n')
			if len(l) != 2 || err != nil {
				return nil, errors.New("missing End of WARC-Fields marker")
			}
			break
		}
		// Handle missing carriage return in line endings
		if nc == lf {
			l, err := r.ReadBytes('\n')
			if len(l) > 2 || err != nil {
				return nil, errors.New("missing End of WARC-Fields marker")
			}
			break
		}
	}
	return &wf, nil
}
