package main

import (
	"fmt"
	"io"
	"os"
	"path"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	TraceLevel = iota
	DebugLevel
	InfoLevel
	WarningLevel
	ErrorLevel
	PanicLevel
)

var logLevels = map[int]string{
	  TraceLevel: "trace",
	  DebugLevel: "debug",
	   InfoLevel: "info",
	WarningLevel: "warning",
	  ErrorLevel: "error",
	  PanicLevel: "panic",
}

type Logger struct {
	Level   int
	Format  []string
	Writers []io.Writer

	mux     sync.Mutex
	skip    int		// runtime.Caller() skip
}

var DefaultLogger = Logger{
	  Level: InfoLevel,
	 Format: []string{"date", "timeMs", "level", "file"},
	Writers: []io.Writer{os.Stderr},
	   skip: 3,
}

func (logger *Logger) log(level int, format string, values ...interface{}) string {
	logger.mux.Lock()
	defer logger.mux.Unlock()

	if _, ok := logLevels[level]; !ok {
		panic(fmt.Errorf("unknown level %d", level))
	}

	now := time.Now()

	pc, file, line, ok := runtime.Caller(logger.skip)
	if !ok {
		panic("unknown caller: runtime.Caller()")
	}

	funcobj := runtime.FuncForPC(pc)
	if funcobj == nil {
		panic("unknown caller: runtime.FuncForPC()")
	}

	t := strings.Split(funcobj.Name(), ".")
	if len(t) == 1 {
		panic(fmt.Errorf("unexpected pkg/func name %q", funcobj.Name()))
	}
	pkgname := strings.Join(t[:len(t)-1], ".")
	funcname := t[len(t)-1]

	formatters := map[string]string{
		  "date": now.Format("2006/01/02"),
		  "time": now.Format("15:04:05"),
		"timeMs": now.Format("15:04:05") + fmt.Sprintf(".%03d", now.Nanosecond() / 1000000),
		"timeUs": now.Format("15:04:05") + fmt.Sprintf(".%06d", now.Nanosecond() / 1000),

		 "level": logLevels[level],

		  "name": funcobj.Name(),
		   "pkg": pkgname,
		  "func": funcname,

		  "path": fmt.Sprintf("%s:%d", file, line),
		  "file": fmt.Sprintf("%s:%d", path.Base(file), line),
	}

	var builder strings.Builder
	for _, f := range logger.Format {
		prefix := ""
		if builder.Len() > 0 {
			prefix = " "
		}

		if formatter, ok := formatters[f]; ok {
			if _, err := builder.WriteString(prefix + formatter); err != nil {
				panic(err)
			}
		} else {
			panic(fmt.Errorf("unsupported formatter %q", f))
		}
	}

	if _, err := fmt.Fprintf(&builder, ": " + format, values...); err != nil {
		panic(err)
	}

	entry := builder.String()
	if !strings.HasSuffix(entry, "\n") {
		entry += "\n"
	}

	for _, w := range logger.Writers {
		if _, err := w.Write([]byte(entry)); err != nil {
			panic(err)
		}
	}

	return entry
}

func (l *Logger) Trace(f string, v ...interface{}) {
	if l.Level <= TraceLevel {
		l.log(TraceLevel, f, v...)
	}
}

func (l *Logger) Debug(f string, v ...interface{}) {
	if l.Level <= DebugLevel {
		l.log(DebugLevel, f, v...)
	}
}

func (l *Logger) Info(f string, v ...interface{}) {
	if l.Level <= InfoLevel {
		l.log(InfoLevel, f, v...)
	}
}

func (l *Logger) Warning(f string, v ...interface{}) {
	if l.Level <= WarningLevel {
		l.log(WarningLevel, f, v...)
	}
}
func (l *Logger) Warn(f string, v ...interface{}) {
	l.Warning(f, v)
}

func (l *Logger) Error(f string, v ...interface{}) {
	if l.Level <= ErrorLevel {
		l.log(ErrorLevel, f, v...)
	}
}

func (l *Logger) Panic(f string, v ...interface{}) {
	panic(l.log(PanicLevel, f, v...))
}

func TraceLog(f string, v ...interface{}) {
	DefaultLogger.Trace(f, v...)
}

func DebugLog(f string, v ...interface{}) {
	DefaultLogger.Debug(f, v...)
}

func InfoLog(f string, v ...interface{}) {
	DefaultLogger.Info(f, v...)
}

func WarningLog(f string, v ...interface{}) {
	DefaultLogger.Warning(f, v...)
}
func WarnLog(f string, v ...interface{}) {
	DefaultLogger.Warning(f, v...)
}

func ErrorLog(f string, v ...interface{}) {
	DefaultLogger.Error(f, v...)
}

func PanicLog(f string, v ...interface{}) {
	DefaultLogger.Panic(f, v...)
}
