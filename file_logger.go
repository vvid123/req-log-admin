package reqlogmid

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"
)

// FileLogger 文件日志输出实现
type FileLogger struct {
	file     *os.File
	writer   *bufio.Writer
	bufferCh chan *LogEntry
	wg       sync.WaitGroup
	quit     chan struct{}
	closed   bool
	mu       sync.Mutex
}

// NewFileLogger 创建一个新的文件日志输出器
// filename 日志文件路径
// async 是否异步写日志
// bufferSize 异步模式下的缓冲区大小
func NewFileLogger(filename string, async bool, bufferSize int) (*FileLogger, error) {
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}

	logger := &FileLogger{
		file:     file,
		writer:   bufio.NewWriterSize(file, 1), // 最小缓冲区，立即刷盘
		bufferCh: make(chan *LogEntry, bufferSize),
		quit:     make(chan struct{}),
	}

	if async {
		logger.startAsyncWriter()
	}

	return logger, nil
}

// startAsyncWriter 启动异步写入协程
func (l *FileLogger) startAsyncWriter() {
	l.wg.Add(1)
	go func() {
		defer l.wg.Done()
		for {
			select {
			case entry, ok := <-l.bufferCh:
				if !ok {
					// 通道关闭，写入剩余日志
					l.flushBuffer()
					return
				}
				if err := l.writeEntry(entry); err != nil {
					fmt.Fprintf(os.Stderr, "failed to write log: %v\n", err)
				}
			case <-l.quit:
				l.flushBuffer()
				return
			}
		}
	}()
}

// writeEntry 写入单条日志
func (l *FileLogger) writeEntry(entry *LogEntry) error {
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal log entry: %w", err)
	}
	data = append(data, '\n')
	_, err = l.writer.Write(data)
	if err != nil {
		return fmt.Errorf("failed to write log: %w", err)
	}
	return nil
}

// Write 实现 Logger 接口
func (l *FileLogger) Write(entry *LogEntry) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return fmt.Errorf("logger is closed")
	}

	if l.bufferCh != nil {
		// 异步模式：发送到缓冲区
		select {
		case l.bufferCh <- entry:
			return nil
		default:
			// 缓冲区满，丢弃日志或同步写入
			fmt.Fprintf(os.Stderr, "log buffer full, dropping entry: %s %s\n", entry.Method, entry.Path)
			return nil
		}
	}

	// 同步模式
	return l.writeEntry(entry)
}

// Close 实现 Logger 接口
func (l *FileLogger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return nil
	}
	l.closed = true

	// 关闭异步写入
	if l.bufferCh != nil {
		close(l.bufferCh)
		l.wg.Wait()
	}

	// 刷新并关闭文件
	if l.writer != nil {
		l.writer.Flush()
	}
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// Flush 实现 Logger 接口
func (l *FileLogger) Flush() {
	if l.bufferCh == nil {
		return
	}
	l.flushBuffer()
}

// flushBuffer 清空缓冲区中的所有日志
func (l *FileLogger) flushBuffer() {
	for {
		select {
		case entry := <-l.bufferCh:
			if err := l.writeEntry(entry); err != nil {
				fmt.Fprintf(os.Stderr, "failed to flush log: %v\n", err)
			}
		default:
			return
		}
	}
}

// DefaultLogFilename 返回默认日志文件名
func DefaultLogFilename() string {
	return fmt.Sprintf("access-%s.log", time.Now().Format("2006-01-02"))
}
