package utils

import (
	"errors"
	"fmt"
	"log"
	"os"
	"runtime"
	"strings"
	"time"
)


type LogLevel int

const (

	DEBUG LogLevel = iota
	INFO
	WARNING
	ERROR
	FATAL
)

var logLevelNames = map[LogLevel]string{
	DEBUG:   "DEBUG",
	INFO:    "INFO",
	WARNING: "WARNING",
	ERROR:   "ERROR",
	FATAL:   "FATAL",
}


type Logger struct {
	level LogLevel
	log   *log.Logger
}


func NewLogger(level LogLevel) *Logger {
	return &Logger{
		level: level,
		log:   log.New(os.Stdout, "", 0),
	}
}

func (l *Logger) getCallerInfo() string {
	_, file, line, ok := runtime.Caller(3)
	if !ok {
		return "unknown:0"
	}
	parts := strings.Split(file, "/")
	if len(parts) > 2 {
		file = strings.Join(parts[len(parts)-2:], "/")
	}
	return fmt.Sprintf("%s:%d", file, line)
}


func (l *Logger) formatLog(level LogLevel, msg string, args ...interface{}) string {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	levelName := logLevelNames[level]
	callerInfo := l.getCallerInfo()
	formattedMsg := fmt.Sprintf(msg, args...)
	return fmt.Sprintf("[%s] [%s] [%s] %s", timestamp, levelName, callerInfo, formattedMsg)
}


func (l *Logger) Debug(msg string, args ...interface{}) {
	if l.level <= DEBUG {
		l.log.Println(l.formatLog(DEBUG, msg, args...))
	}
}


func (l *Logger) Info(msg string, args ...interface{}) {
	if l.level <= INFO {
		l.log.Println(l.formatLog(INFO, msg, args...))
	}
}


func (l *Logger) Warning(msg string, args ...interface{}) {
	if l.level <= WARNING {
		l.log.Println(l.formatLog(WARNING, msg, args...))
	}
}


func (l *Logger) Error(msg string, args ...interface{}) {
	if l.level <= ERROR {
		l.log.Println(l.formatLog(ERROR, msg, args...))
	}
}


func (l *Logger) Fatal(msg string, args ...interface{}) {
	if l.level <= FATAL {
		l.log.Fatalln(l.formatLog(FATAL, msg, args...))
	}
}


func (l *Logger) LogWithError(level LogLevel, err error, msg string, args ...interface{}) {
	if l.level <= level {
		var appErr *AppError
		if errors.As(err, &appErr) {
			message := fmt.Sprintf("%s: %v", msg, appErr.Error())
			l.log.Println(l.formatLog(level, message, args...))
		} else {
			message := fmt.Sprintf("%s: %v", msg, err)
			l.log.Println(l.formatLog(level, message, args...))
		}
	}
}
