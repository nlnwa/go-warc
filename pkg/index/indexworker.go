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

package index

import (
	"github.com/nlnwa/gowarc/pkg/gowarc"
	log "github.com/sirupsen/logrus"
	"io"
	"strconv"
	"sync"
	"time"
)

type job struct {
	fileName string
	timer    *time.Timer
}

type indexWorker struct {
	db          *Db
	noOfWorkers int
	jobs        chan string
	stop        chan bool
	jobMap      map[string]*time.Timer
	mx          *sync.Mutex
}

func NewIndexWorker(db *Db, noOfWorkers int) *indexWorker {
	iw := &indexWorker{
		db:          db,
		noOfWorkers: noOfWorkers,
		jobs:        make(chan string, 10),
		stop:        make(chan bool),
		jobMap:      map[string]*time.Timer{},
		mx:          &sync.Mutex{},
	}

	for i := 0; i < iw.noOfWorkers; i++ {
		go iw.worker(i)
	}

	log.Infof("indexWorker initialzed with %d instances", noOfWorkers)
	return iw
}

func (iw *indexWorker) Shutdown() {
	for i := 0; i < iw.noOfWorkers; i++ {
		iw.stop <- true
	}
}

func (iw *indexWorker) worker(id int) {
	for {
		select {
		case fileName := <-iw.jobs:
			indexFile(iw.db, fileName)
			iw.mx.Lock()
			delete(iw.jobMap, fileName)
			iw.mx.Unlock()
		case <-iw.stop:
			log.Infof("indexWorker #%v stopped", id)
			return
		}
	}
}

func (iw *indexWorker) Queue(fileName string, batchWindow time.Duration) {
	iw.mx.Lock()
	defer iw.mx.Unlock()
	timer, ok := iw.jobMap[fileName]
	if ok {
		timer.Stop()
		timer.Reset(batchWindow)
	} else {
		iw.jobMap[fileName] = time.AfterFunc(batchWindow, func() {
			iw.jobs <- fileName
		})
	}
}

func indexFile(db *Db, fileName string) {
	log.Infof("indexing %v", fileName)
	start := time.Now()
	opts := &gowarc.WarcReaderOpts{Strict: false}
	wf, err := gowarc.NewWarcFilename(fileName, 0, opts)
	if err != nil {
		log.Warnf("error while indexing %v: %v", fileName, err)
		return
	}
	defer wf.Close()

	count := 0
	for {
		wr, currentOffset, err := wf.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Errorf("Error: %v, rec num: %v, Offset %v\n", err.Error(), strconv.Itoa(count), currentOffset)
			break
		}
		count++

		db.Add(wr.RecordID(), fileName, currentOffset)
	}
	db.Flush()
	log.Infof("Finished indexing %s. Found: %d records in: %v", fileName, count, time.Since(start))
	return
}
