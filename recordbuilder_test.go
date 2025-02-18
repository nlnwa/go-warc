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
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRecordBuilder(t *testing.T) {
	type args struct {
		opts       []WarcRecordOption
		recordType RecordType
		headers    *WarcFields
		data       string
	}
	type want struct {
		headers    *WarcFields
		blockType  interface{}
		data       string
		validation *Validation
		cached     bool
	}
	tests := []struct {
		name    string
		args    args
		want    want
		wantErr bool
	}{
		{
			"valid warcinfo record",
			args{
				[]WarcRecordOption{WithSpecViolationPolicy(ErrFail), WithSyntaxErrorPolicy(ErrFail), WithUnknownRecordTypePolicy(ErrIgnore)},
				Warcinfo,
				&WarcFields{
					&nameValue{Name: WarcRecordID, Value: "<urn:uuid:e9a0cecc-0221-11e7-adb1-0242ac120008>"},
					&nameValue{Name: WarcDate, Value: "2017-03-06T04:03:53Z"},
					&nameValue{Name: WarcFilename, Value: "temp-20170306040353.warc.gz"},
					&nameValue{Name: ContentType, Value: "application/warc-fields"},
					&nameValue{Name: WarcBlockDigest, Value: "sha1:AF4D582B4FFC017D07A947D841E392A821F754F3"},
					&nameValue{Name: ContentLength, Value: "238"},
				},
				"software: Veidemann v1.0\r\n" +
					"format: WARC File Format 1.1\r\n" +
					"creator: temp-MJFXHZ4S\r\n" +
					"isPartOf: Temporary%20Collection\r\n" +
					"json-metadata: {\"title\": \"Temporary Collection\", \"size\": 2865, \"created_at\": 1488772924, \"type\": \"collection\", \"desc\": \"\"}\r\n",
			},
			want{
				&WarcFields{
					&nameValue{Name: WarcDate, Value: "2017-03-06T04:03:53Z"},
					&nameValue{Name: WarcRecordID, Value: "<urn:uuid:e9a0cecc-0221-11e7-adb1-0242ac120008>"},
					&nameValue{Name: WarcFilename, Value: "temp-20170306040353.warc.gz"},
					&nameValue{Name: WarcType, Value: "warcinfo"},
					&nameValue{Name: ContentType, Value: "application/warc-fields"},
					&nameValue{Name: WarcBlockDigest, Value: "sha1:AF4D582B4FFC017D07A947D841E392A821F754F3"},
					&nameValue{Name: ContentLength, Value: "238"},
				},
				&warcFieldsBlock{},
				"software: Veidemann v1.0\r\n" +
					"format: WARC File Format 1.1\r\n" +
					"creator: temp-MJFXHZ4S\r\n" +
					"isPartOf: Temporary%20Collection\r\n" +
					"json-metadata: {\"title\": \"Temporary Collection\", \"size\": 2865, \"created_at\": 1488772924, \"type\": \"collection\", \"desc\": \"\"}\r\n",
				&Validation{},
				true,
			},
			false,
		},
		{
			"valid response record",
			args{
				[]WarcRecordOption{WithSpecViolationPolicy(ErrFail), WithSyntaxErrorPolicy(ErrFail), WithUnknownRecordTypePolicy(ErrIgnore)},
				Response,
				&WarcFields{
					&nameValue{Name: WarcDate, Value: "2017-03-06T04:03:53Z"},
					&nameValue{Name: WarcRecordID, Value: "<urn:uuid:e9a0cecc-0221-11e7-adb1-0242ac120008>"},
					&nameValue{Name: ContentType, Value: "application/http;msgtype=response"},
					&nameValue{Name: WarcBlockDigest, Value: "sha1:71ABF824132CA55D4D46D32E73B4D33E2354E619"},
					&nameValue{Name: WarcPayloadDigest, Value: "sha1:C37FFB221569C553A2476C22C7DAD429F3492977"},
					&nameValue{Name: ContentLength, Value: "259"},
				},
				"HTTP/1.1 200 OK\nDate: Tue, 19 Sep 2016 17:18:40 GMT\nServer: Apache/2.0.54 (Ubuntu)\n" +
					"Last-Modified: Mon, 16 Jun 2013 22:28:51 GMT\nETag: \"3e45-67e-2ed02ec0\"\nAccept-Ranges: bytes\n" +
					"Content-Length: 19\nConnection: close\nContent-Type: text/plain\r\n\r\nThis is the content",
			},
			want{
				&WarcFields{
					&nameValue{Name: WarcDate, Value: "2017-03-06T04:03:53Z"},
					&nameValue{Name: WarcRecordID, Value: "<urn:uuid:e9a0cecc-0221-11e7-adb1-0242ac120008>"},
					&nameValue{Name: WarcType, Value: "response"},
					&nameValue{Name: WarcBlockDigest, Value: "sha1:71ABF824132CA55D4D46D32E73B4D33E2354E619"},
					&nameValue{Name: WarcPayloadDigest, Value: "sha1:C37FFB221569C553A2476C22C7DAD429F3492977"},
					&nameValue{Name: ContentType, Value: "application/http;msgtype=response"},
					&nameValue{Name: ContentLength, Value: "259"},
				},
				&httpResponseBlock{},
				"HTTP/1.1 200 OK\nDate: Tue, 19 Sep 2016 17:18:40 GMT\nServer: Apache/2.0.54 (Ubuntu)\n" +
					"Last-Modified: Mon, 16 Jun 2013 22:28:51 GMT\nETag: \"3e45-67e-2ed02ec0\"\nAccept-Ranges: bytes\n" +
					"Content-Length: 19\nConnection: close\nContent-Type: text/plain\r\n\r\nThis is the content",
				&Validation{},
				true,
			},
			false,
		},
		{
			"valid request record",
			args{
				[]WarcRecordOption{WithSpecViolationPolicy(ErrFail), WithSyntaxErrorPolicy(ErrFail), WithUnknownRecordTypePolicy(ErrIgnore), WithFixDigest(false), WithAddMissingDigest(false)},
				Request,
				&WarcFields{
					&nameValue{Name: WarcDate, Value: "2017-03-06T04:03:53Z"},
					&nameValue{Name: WarcRecordID, Value: "<urn:uuid:e9a0cecc-0221-11e7-adb1-0242ac120008>"},
					&nameValue{Name: ContentType, Value: "application/http;msgtype=request"},
					&nameValue{Name: WarcBlockDigest, Value: "sha1:A3781FF1FC3FB52318F623E22C85D63D74C12932"},
					&nameValue{Name: ContentLength, Value: "263"},
				},
				"GET / HTTP/1.0\n" +
					"Host: example.com\n" +
					"Accept-Language: en-US,en;q=0.8,ru;q=0.6\n" +
					"Referer: http://example.com/foo.html\n" +
					"Connection: close\n" +
					"User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_11_1) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/56.0.2924.87 Safari/537.36\n\n",
			},
			want{
				&WarcFields{
					&nameValue{Name: WarcDate, Value: "2017-03-06T04:03:53Z"},
					&nameValue{Name: WarcRecordID, Value: "<urn:uuid:e9a0cecc-0221-11e7-adb1-0242ac120008>"},
					&nameValue{Name: WarcType, Value: "request"},
					&nameValue{Name: ContentType, Value: "application/http;msgtype=request"},
					&nameValue{Name: WarcBlockDigest, Value: "sha1:A3781FF1FC3FB52318F623E22C85D63D74C12932"},
					&nameValue{Name: ContentLength, Value: "263"},
				},
				&httpRequestBlock{},
				"GET / HTTP/1.0\n" +
					"Host: example.com\n" +
					"Accept-Language: en-US,en;q=0.8,ru;q=0.6\n" +
					"Referer: http://example.com/foo.html\n" +
					"Connection: close\n" +
					"User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_11_1) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/56.0.2924.87 Safari/537.36\n\n",
				&Validation{},
				true,
			},
			false,
		},
		{
			"invalid request record - missing newline - warn",
			args{
				[]WarcRecordOption{WithSpecViolationPolicy(ErrFail), WithSyntaxErrorPolicy(ErrWarn), WithUnknownRecordTypePolicy(ErrIgnore), WithFixDigest(false), WithAddMissingDigest(false), WithFixSyntaxErrors(false)},
				Request,
				&WarcFields{
					&nameValue{Name: WarcDate, Value: "2017-03-06T04:03:53Z"},
					&nameValue{Name: WarcRecordID, Value: "<urn:uuid:e9a0cecc-0221-11e7-adb1-0242ac120008>"},
					&nameValue{Name: ContentType, Value: "application/http;msgtype=request"},
					&nameValue{Name: WarcBlockDigest, Value: "sha1:F45C9D37F9F7E5F822C86444F51D6CB252B7B33B"},
					&nameValue{Name: ContentLength, Value: "262"},
				},
				"GET / HTTP/1.0\n" +
					"Host: example.com\n" +
					"Accept-Language: en-US,en;q=0.8,ru;q=0.6\n" +
					"Referer: http://example.com/foo.html\n" +
					"Connection: close\n" +
					"User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_11_1) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/56.0.2924.87 Safari/537.36\n",
			},
			want{
				&WarcFields{
					&nameValue{Name: WarcDate, Value: "2017-03-06T04:03:53Z"},
					&nameValue{Name: WarcRecordID, Value: "<urn:uuid:e9a0cecc-0221-11e7-adb1-0242ac120008>"},
					&nameValue{Name: WarcType, Value: "request"},
					&nameValue{Name: ContentType, Value: "application/http;msgtype=request"},
					&nameValue{Name: WarcBlockDigest, Value: "sha1:F45C9D37F9F7E5F822C86444F51D6CB252B7B33B"},
					&nameValue{Name: ContentLength, Value: "262"},
				},
				&httpRequestBlock{},
				"GET / HTTP/1.0\n" +
					"Host: example.com\n" +
					"Accept-Language: en-US,en;q=0.8,ru;q=0.6\n" +
					"Referer: http://example.com/foo.html\n" +
					"Connection: close\n" +
					"User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_11_1) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/56.0.2924.87 Safari/537.36\n",
				&Validation{errMissingEndOfHeaders},
				true,
			},
			false,
		},
		{
			"invalid request record - missing newline - fail",
			args{
				[]WarcRecordOption{WithSpecViolationPolicy(ErrFail), WithSyntaxErrorPolicy(ErrFail), WithUnknownRecordTypePolicy(ErrIgnore), WithFixDigest(false), WithAddMissingDigest(false)},
				Request,
				&WarcFields{
					&nameValue{Name: WarcDate, Value: "2017-03-06T04:03:53Z"},
					&nameValue{Name: WarcRecordID, Value: "<urn:uuid:e9a0cecc-0221-11e7-adb1-0242ac120008>"},
					&nameValue{Name: ContentType, Value: "application/http;msgtype=request"},
					&nameValue{Name: WarcBlockDigest, Value: "sha1:F45C9D37F9F7E5F822C86444F51D6CB252B7B33B"},
					&nameValue{Name: ContentLength, Value: "262"},
				},
				"GET / HTTP/1.0\n" +
					"Host: example.com\n" +
					"Accept-Language: en-US,en;q=0.8,ru;q=0.6\n" +
					"Referer: http://example.com/foo.html\n" +
					"Connection: close\n" +
					"User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_11_1) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/56.0.2924.87 Safari/537.36\n",
			},
			want{
				&WarcFields{
					&nameValue{Name: WarcDate, Value: "2017-03-06T04:03:53Z"},
					&nameValue{Name: WarcRecordID, Value: "<urn:uuid:e9a0cecc-0221-11e7-adb1-0242ac120008>"},
					&nameValue{Name: WarcType, Value: "request"},
					&nameValue{Name: ContentType, Value: "application/http;msgtype=request"},
					&nameValue{Name: WarcBlockDigest, Value: "sha1:F45C9D37F9F7E5F822C86444F51D6CB252B7B33B"},
					&nameValue{Name: ContentLength, Value: "262"},
				},
				&httpRequestBlock{},
				"GET / HTTP/1.0\n" +
					"Host: example.com\n" +
					"Accept-Language: en-US,en;q=0.8,ru;q=0.6\n" +
					"Referer: http://example.com/foo.html\n" +
					"Connection: close\n" +
					"User-Agent: Mozilla/5.0 (Macintosh; Intel Mac OS X 10_11_1) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/56.0.2924.87 Safari/537.36\n",
				&Validation{},
				true,
			},
			true,
		},
		{
			"valid metadata record",
			args{
				[]WarcRecordOption{WithSpecViolationPolicy(ErrFail), WithSyntaxErrorPolicy(ErrFail), WithUnknownRecordTypePolicy(ErrIgnore)},
				Metadata,
				&WarcFields{
					&nameValue{Name: WarcDate, Value: "2017-03-06T04:03:53Z"},
					&nameValue{Name: WarcRecordID, Value: "<urn:uuid:e9a0cecc-0221-11e7-adb1-0242ac120008>"},
					&nameValue{Name: ContentType, Value: "application/warc-fields"},
					&nameValue{Name: WarcConcurrentTo, Value: "<urn:uuid:e7c9eff8-f5bc-4aeb-b3d2-9d3df99afb30>"},
					&nameValue{Name: WarcBlockDigest, Value: "sha1:6D924D4C99268BE486042E655B06A83133EFEB59"},
					&nameValue{Name: ContentLength, Value: "64"},
				},
				"via: http://www.example.com/\r\n" +
					"hopsFromSeed: P\r\n" +
					"fetchTimeMs: 47\r\n",
			},
			want{
				&WarcFields{
					&nameValue{Name: WarcDate, Value: "2017-03-06T04:03:53Z"},
					&nameValue{Name: WarcRecordID, Value: "<urn:uuid:e9a0cecc-0221-11e7-adb1-0242ac120008>"},
					&nameValue{Name: WarcType, Value: "metadata"},
					&nameValue{Name: ContentType, Value: "application/warc-fields"},
					&nameValue{Name: WarcConcurrentTo, Value: "<urn:uuid:e7c9eff8-f5bc-4aeb-b3d2-9d3df99afb30>"},
					&nameValue{Name: WarcBlockDigest, Value: "sha1:6D924D4C99268BE486042E655B06A83133EFEB59"},
					&nameValue{Name: ContentLength, Value: "64"},
				},
				&warcFieldsBlock{},
				"via: http://www.example.com/\r\n" +
					"hopsFromSeed: P\r\n" +
					"fetchTimeMs: 47\r\n",
				&Validation{},
				true,
			},
			false,
		},
		{
			"valid resource record",
			args{
				[]WarcRecordOption{WithSpecViolationPolicy(ErrFail), WithSyntaxErrorPolicy(ErrFail), WithUnknownRecordTypePolicy(ErrIgnore)},
				Resource,
				&WarcFields{
					&nameValue{Name: WarcDate, Value: "2017-03-06T04:03:53Z"},
					&nameValue{Name: WarcRecordID, Value: "<urn:uuid:e9a0cecc-0221-11e7-adb1-0242ac120008>"},
					&nameValue{Name: WarcTargetURI, Value: "file://var/www/htdoc/index.html"},
					&nameValue{Name: ContentType, Value: "text/html"},
					&nameValue{Name: WarcConcurrentTo, Value: "<urn:uuid:e7c9eff8-f5bc-4aeb-b3d2-9d3df99afb30>"},
					&nameValue{Name: WarcPayloadDigest, Value: "sha1:307E7DFCAF9A8EA4C4E86A11BCAA83AC6698017F"},
					&nameValue{Name: WarcBlockDigest, Value: "sha1:307E7DFCAF9A8EA4C4E86A11BCAA83AC6698017F"},
					&nameValue{Name: ContentLength, Value: "42"},
				},
				"<html><head></head>\n" +
					"<body></body>\n" +
					"</html>\n",
			},
			want{
				&WarcFields{
					&nameValue{Name: WarcDate, Value: "2017-03-06T04:03:53Z"},
					&nameValue{Name: WarcRecordID, Value: "<urn:uuid:e9a0cecc-0221-11e7-adb1-0242ac120008>"},
					&nameValue{Name: WarcType, Value: "resource"},
					&nameValue{Name: ContentType, Value: "text/html"},
					&nameValue{Name: WarcTargetURI, Value: "file://var/www/htdoc/index.html"},
					&nameValue{Name: WarcConcurrentTo, Value: "<urn:uuid:e7c9eff8-f5bc-4aeb-b3d2-9d3df99afb30>"},
					&nameValue{Name: WarcPayloadDigest, Value: "sha1:307E7DFCAF9A8EA4C4E86A11BCAA83AC6698017F"},
					&nameValue{Name: WarcBlockDigest, Value: "sha1:307E7DFCAF9A8EA4C4E86A11BCAA83AC6698017F"},
					&nameValue{Name: ContentLength, Value: "42"},
				},
				&genericBlock{},
				"<html><head></head>\n" +
					"<body></body>\n" +
					"</html>\n",
				&Validation{},
				true,
			},
			false,
		},
		{
			"valid revisit record",
			args{
				[]WarcRecordOption{WithSpecViolationPolicy(ErrFail), WithSyntaxErrorPolicy(ErrFail), WithUnknownRecordTypePolicy(ErrIgnore)},
				Revisit,
				&WarcFields{
					&nameValue{Name: WarcDate, Value: "2017-03-06T04:03:53Z"},
					&nameValue{Name: WarcRecordID, Value: "<urn:uuid:e9a0cecc-0221-11e7-adb1-0242ac120008>"},
					&nameValue{Name: WarcTargetURI, Value: "http://www.example.org/images/logo.jpg"},
					&nameValue{Name: WarcProfile, Value: "http://netpreserve.org/warc/1.1/server-not-modified"},
					&nameValue{Name: WarcRefersTo, Value: "<urn:uuid:92283950-ef2f-4d72-b224-f54c6ec90bb0>"},
					&nameValue{Name: WarcRefersToTargetURI, Value: "http://www.example.org/images/logo.jpg"},
					&nameValue{Name: WarcRefersToDate, Value: "2016-09-19T17:20:24Z"},
					&nameValue{Name: ContentType, Value: "message/http"},
					&nameValue{Name: WarcBlockDigest, Value: "sha1:7B71E2CE461E4685EED55612850EE0CBB3876EDF"},
					&nameValue{Name: ContentLength, Value: "195"},
				},
				"HTTP/1.x 304 Not Modified\n" +
					"Date: Tue, 06 Mar 2017 00:43:35 GMT\n" +
					"Server: Apache/2.0.54 (Ubuntu) PHP/5.0.5-2ubuntu1.4 Connection: Keep-Alive\n" +
					"Keep-Alive: timeout=15, max=100\n" +
					"ETag: \"3e45-67e-2ed02ec0\"\n",
			},
			want{
				&WarcFields{
					&nameValue{Name: WarcDate, Value: "2017-03-06T04:03:53Z"},
					&nameValue{Name: WarcRecordID, Value: "<urn:uuid:e9a0cecc-0221-11e7-adb1-0242ac120008>"},
					&nameValue{Name: WarcType, Value: "revisit"},
					&nameValue{Name: WarcTargetURI, Value: "http://www.example.org/images/logo.jpg"},
					&nameValue{Name: WarcProfile, Value: "http://netpreserve.org/warc/1.1/server-not-modified"},
					&nameValue{Name: WarcRefersTo, Value: "<urn:uuid:92283950-ef2f-4d72-b224-f54c6ec90bb0>"},
					&nameValue{Name: WarcRefersToTargetURI, Value: "http://www.example.org/images/logo.jpg"},
					&nameValue{Name: WarcRefersToDate, Value: "2016-09-19T17:20:24Z"},
					&nameValue{Name: ContentType, Value: "message/http"},
					&nameValue{Name: WarcBlockDigest, Value: "sha1:7B71E2CE461E4685EED55612850EE0CBB3876EDF"},
					&nameValue{Name: ContentLength, Value: "195"},
				},
				&revisitBlock{},
				"HTTP/1.x 304 Not Modified\n" +
					"Date: Tue, 06 Mar 2017 00:43:35 GMT\n" +
					"Server: Apache/2.0.54 (Ubuntu) PHP/5.0.5-2ubuntu1.4 Connection: Keep-Alive\n" +
					"Keep-Alive: timeout=15, max=100\n" +
					"ETag: \"3e45-67e-2ed02ec0\"\n",
				&Validation{},
				true,
			},
			false,
		},
		{
			"valid conversion record",
			args{
				[]WarcRecordOption{WithSpecViolationPolicy(ErrFail), WithSyntaxErrorPolicy(ErrFail), WithUnknownRecordTypePolicy(ErrIgnore)},
				Conversion,
				&WarcFields{
					&nameValue{Name: WarcDate, Value: "2017-03-06T04:03:53Z"},
					&nameValue{Name: WarcRecordID, Value: "<urn:uuid:e9a0cecc-0221-11e7-adb1-0242ac120008>"},
					&nameValue{Name: WarcTargetURI, Value: "http://www.example.org/index.html"},
					&nameValue{Name: WarcRefersTo, Value: "<urn:uuid:92283950-ef2f-4d72-b224-f54c6ec90bb0>"},
					&nameValue{Name: ContentType, Value: "text/plain"},
					&nameValue{Name: WarcBlockDigest, Value: "sha1:581F7F1CA3D3EB023438808309678ED6D03E2895"},
					&nameValue{Name: ContentLength, Value: "10"},
				},
				"body text\n",
			},
			want{
				&WarcFields{
					&nameValue{Name: WarcDate, Value: "2017-03-06T04:03:53Z"},
					&nameValue{Name: WarcRecordID, Value: "<urn:uuid:e9a0cecc-0221-11e7-adb1-0242ac120008>"},
					&nameValue{Name: WarcTargetURI, Value: "http://www.example.org/index.html"},
					&nameValue{Name: WarcRefersTo, Value: "<urn:uuid:92283950-ef2f-4d72-b224-f54c6ec90bb0>"},
					&nameValue{Name: WarcType, Value: "conversion"},
					&nameValue{Name: ContentType, Value: "text/plain"},
					&nameValue{Name: WarcBlockDigest, Value: "sha1:581F7F1CA3D3EB023438808309678ED6D03E2895"},
					&nameValue{Name: ContentLength, Value: "10"},
				},
				&genericBlock{},
				"body text\n",
				&Validation{},
				true,
			},
			false,
		},
		{
			"valid continuation record",
			args{
				[]WarcRecordOption{WithSpecViolationPolicy(ErrFail), WithSyntaxErrorPolicy(ErrFail), WithUnknownRecordTypePolicy(ErrIgnore)},
				Continuation,
				&WarcFields{
					&nameValue{Name: WarcDate, Value: "2017-03-06T04:03:53Z"},
					&nameValue{Name: WarcRecordID, Value: "<urn:uuid:e9a0cecc-0221-11e7-adb1-0242ac120008>"},
					&nameValue{Name: WarcTargetURI, Value: "http://www.example.org/index.html"},
					&nameValue{Name: WarcSegmentOriginID, Value: "<urn:uuid:39509228-ae2f-11b2-763a-aa4c6ec90bb0>"},
					&nameValue{Name: WarcSegmentNumber, Value: "2"},
					&nameValue{Name: WarcSegmentTotalLength, Value: "1902"},
					&nameValue{Name: ContentType, Value: "text/plain"},
					&nameValue{Name: WarcBlockDigest, Value: "sha1:62B805E388394FF22747D1B10476EA04309CB5A8"},
					&nameValue{Name: WarcPayloadDigest, Value: "sha1:CCHXETFVJD2MUZY6ND6SS7ZENMWF7KQ2"},
					&nameValue{Name: ContentLength, Value: "22"},
				},
				"... last part of data\n",
			},
			want{
				&WarcFields{
					&nameValue{Name: WarcDate, Value: "2017-03-06T04:03:53Z"},
					&nameValue{Name: WarcRecordID, Value: "<urn:uuid:e9a0cecc-0221-11e7-adb1-0242ac120008>"},
					&nameValue{Name: WarcTargetURI, Value: "http://www.example.org/index.html"},
					&nameValue{Name: WarcType, Value: "continuation"},
					&nameValue{Name: WarcSegmentOriginID, Value: "<urn:uuid:39509228-ae2f-11b2-763a-aa4c6ec90bb0>"},
					&nameValue{Name: WarcSegmentNumber, Value: "2"},
					&nameValue{Name: WarcSegmentTotalLength, Value: "1902"},
					&nameValue{Name: ContentType, Value: "text/plain"},
					&nameValue{Name: WarcBlockDigest, Value: "sha1:62B805E388394FF22747D1B10476EA04309CB5A8"},
					&nameValue{Name: WarcPayloadDigest, Value: "sha1:CCHXETFVJD2MUZY6ND6SS7ZENMWF7KQ2"},
					&nameValue{Name: ContentLength, Value: "22"},
				},
				&genericBlock{},
				"... last part of data\n",
				&Validation{},
				true,
			},
			false,
		},
		{
			"valid unknown record type",
			args{
				[]WarcRecordOption{WithSpecViolationPolicy(ErrFail), WithSyntaxErrorPolicy(ErrFail), WithUnknownRecordTypePolicy(ErrIgnore)},
				0,
				&WarcFields{
					&nameValue{Name: WarcType, Value: "myType"},
					&nameValue{Name: WarcDate, Value: "2017-03-06T04:03:53Z"},
					&nameValue{Name: WarcRecordID, Value: "<urn:uuid:e9a0cecc-0221-11e7-adb1-0242ac120008>"},
					&nameValue{Name: WarcTargetURI, Value: "http://www.example.org/index.html"},
					&nameValue{Name: "My-Field", Value: "MyValue"},
					&nameValue{Name: ContentType, Value: "text/plain"},
					&nameValue{Name: WarcBlockDigest, Value: "sha1:7FE70820E08A1AAC0EF224D9C66AB66831CC4AB1"},
					&nameValue{Name: ContentLength, Value: "8"},
				},
				"content\n",
			},
			want{
				&WarcFields{
					&nameValue{Name: WarcType, Value: "myType"},
					&nameValue{Name: WarcDate, Value: "2017-03-06T04:03:53Z"},
					&nameValue{Name: WarcRecordID, Value: "<urn:uuid:e9a0cecc-0221-11e7-adb1-0242ac120008>"},
					&nameValue{Name: WarcTargetURI, Value: "http://www.example.org/index.html"},
					&nameValue{Name: "My-Field", Value: "MyValue"},
					&nameValue{Name: WarcBlockDigest, Value: "sha1:7FE70820E08A1AAC0EF224D9C66AB66831CC4AB1"},
					&nameValue{Name: ContentType, Value: "text/plain"},
					&nameValue{Name: ContentLength, Value: "8"},
				},
				&genericBlock{},
				"content\n",
				&Validation{},
				true,
			},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert := assert.New(t)

			rb := NewRecordBuilder(tt.args.recordType, tt.args.opts...)
			for _, nv := range *tt.args.headers {
				rb.AddWarcHeader(nv.Name, nv.Value)
			}
			_, err := rb.WriteString(tt.args.data)
			assert.NoError(err)
			wr, validation, err := rb.Build()
			if err == nil {
				defer wr.Close() //nolint
			}

			if tt.wantErr {
				assert.Error(err)
				return
			} else {
				assert.NoError(err)
			}

			assert.ElementsMatch([]*nameValue(*tt.want.headers), []*nameValue(*wr.WarcHeader()))
			assert.IsType(tt.want.blockType, wr.Block())
			assert.Equal(tt.want.validation, validation)
			r, err := wr.Block().RawBytes()
			assert.Nil(err)
			b, err := io.ReadAll(r)
			assert.Nil(err)
			assert.Equal(tt.want.data, string(b))

			assert.Equal(tt.want.cached, wr.Block().IsCached())
		})
	}
}

func TestRecordBuilder_AddWarcHeader(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Oslo")
	assert.NoError(t, err)
	tm, err := time.ParseInLocation(time.ANSIC, "Mon Mar  6 05:03:53 2017", loc)
	assert.NoError(t, err)
	rb := NewRecordBuilder(Warcinfo)
	rb.AddWarcHeader(WarcRecordID, "<urn:uuid:e9a0cecc-0221-11e7-adb1-0242ac120008>")
	rb.AddWarcHeaderTime(WarcDate, tm)
	rb.AddWarcHeaderInt64(ContentLength, int64(238))
	rb.AddWarcHeaderInt(WarcSegmentNumber, 2)
	record, validation, err := rb.Build()
	assert.NoError(t, err)
	defer record.Close() //nolint

	rb = NewRecordBuilder(Warcinfo)
	rb.AddWarcHeader(WarcRecordID, "<urn:uuid:e9a0cecc-0221-11e7-adb1-0242ac120008>")
	rb.AddWarcHeader(WarcDate, "2017-03-06T04:03:53Z")
	rb.AddWarcHeader(ContentLength, "238")
	rb.AddWarcHeader(WarcSegmentNumber, "2")
	expected, expectedValidation, err := rb.Build()
	assert.NoError(t, err)
	defer record.Close() //nolint

	assert.ElementsMatch(t, []*nameValue(*expected.WarcHeader()), []*nameValue(*record.WarcHeader()))
	assert.Equal(t, expectedValidation, validation)
}
