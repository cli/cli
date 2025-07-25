package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

type point struct {
	Sent     time.Time `json:"sent"`
	Finished time.Time `json:"finished"`
	Took     int64     `json:"took"`
	PType    string    `json:"type"`
	Action   string    `json:"action"`
}

type latencyWriter interface {
	Add(action string, sent, finished time.Time, pType string)
	Close()
}

type latencyNoop struct{}

func (ln *latencyNoop) Add(_ string, _, _ time.Time, _ string) {}

func (ln *latencyNoop) Close() {}

type latencyFile struct {
	metrics chan *point
	output  *os.File
	stop    chan struct{}
}

func newLatencyFile(filename string) (latencyWriter, error) {
	if filename == "" {
		return &latencyNoop{}, nil
	}
	fmt.Printf("[+] Opening results file %s\n", filename)
	file, err := os.OpenFile(filename, os.O_RDWR|os.O_APPEND|os.O_CREATE, os.ModePerm)
	if err != nil {
		return nil, err
	}
	f := &latencyFile{
		metrics: make(chan *point, 2048),
		stop:    make(chan struct{}, 1),
		output:  file,
	}
	go f.write()
	return f, nil
}

func (f *latencyFile) write() {
	for {
		select {
		case p := <-f.metrics:
			data, err := json.Marshal(p)
			if err != nil {
				panic(err)
			}
			_, err = f.output.Write(append(data, []byte("\n")...))
			if err != nil {
				panic(err)
			}
		case <-f.stop:
			return
		}
	}
}

// Add writes a point to the file
func (f *latencyFile) Add(action string, sent, finished time.Time, pType string) {
	f.metrics <- &point{
		Sent:     sent,
		Finished: finished,
		Took:     finished.Sub(sent).Nanoseconds(),
		PType:    pType,
		Action:   action,
	}
}

// Close stops f.write() and closes the file, any remaining metrics will be discarded
func (f *latencyFile) Close() {
	f.stop <- struct{}{}
	_ = f.output.Close()
}
