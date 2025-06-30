package main

import (
	"bufio"
	"fmt"
	"os"
	"sync"
	"time"
)

var logFile *os.File
var logWriter *bufio.Writer
var logLock sync.Mutex
var logChan chan logEntry
var logBuffer []logEntry
var lastFlush time.Time

type logEntry struct {
	action    string
	path      string
	err       error
	timestamp time.Time
}

func StartLogger(filename string) {
	f, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Log dosyası açılamadı: %v\n", err)
		return
	}
	logFile = f
	logWriter = bufio.NewWriterSize(f, 8192) // 8KB buffer
	logChan = make(chan logEntry, 1000)
	logBuffer = make([]logEntry, 0, 100)
	lastFlush = time.Now()
	
	// Background log processor
	go logProcessor()
}
func StopLogger() {
	if logChan != nil {
		close(logChan)
	}
	if logWriter != nil {
		logWriter.Flush()
	}
	if logFile != nil {
		logFile.Close()
	}
}

func LogAction(action, path string, err error) {
	if logChan == nil {
		return
	}
	
	entry := logEntry{
		action:    action,
		path:      path,
		err:       err,
		timestamp: time.Now(),
	}
	
	select {
	case logChan <- entry:
	default:
		// Channel full, drop log entry
	}
}

func logProcessor() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case entry, ok := <-logChan:
			if !ok {
				// Channel closed, flush remaining entries
				flushBuffer()
				return
			}
			
			logBuffer = append(logBuffer, entry)
			
			// Flush if buffer is full
			if len(logBuffer) >= 50 {
				flushBuffer()
			}
			
		case <-ticker.C:
			// Periodic flush
			if time.Since(lastFlush) > 5*time.Second && len(logBuffer) > 0 {
				flushBuffer()
			}
		}
	}
}

func flushBuffer() {
	if logWriter == nil || len(logBuffer) == 0 {
		return
	}
	
	logLock.Lock()
	defer logLock.Unlock()
	
	for _, entry := range logBuffer {
		if entry.err != nil {
			fmt.Fprintf(logWriter, "[%s] %s HATA: %v\n", entry.action, entry.path, entry.err)
		} else {
			fmt.Fprintf(logWriter, "[%s] %s\n", entry.action, entry.path)
		}
	}
	
	logWriter.Flush()
	logBuffer = logBuffer[:0] // Reset slice
	lastFlush = time.Now()
}
