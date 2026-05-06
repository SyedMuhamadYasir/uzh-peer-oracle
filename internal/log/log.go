package log

import (
	"encoding/json"
	"log"
	"os"
	"time"
)

type Logger struct {
	base *log.Logger
}

func New() *Logger {
	return &Logger{base: log.New(os.Stdout, "", 0)}
}

func (l *Logger) Info(msg string, fields map[string]any) {
	l.write("info", msg, fields)
}

func (l *Logger) Warn(msg string, fields map[string]any) {
	l.write("warn", msg, fields)
}

func (l *Logger) Error(msg string, fields map[string]any) {
	l.write("error", msg, fields)
}

func (l *Logger) write(level, msg string, fields map[string]any) {
	if fields == nil {
		fields = map[string]any{}
	}
	fields["ts"] = time.Now().UTC().Format(time.RFC3339Nano)
	fields["level"] = level
	fields["msg"] = msg
	b, err := json.Marshal(fields)
	if err != nil {
		l.base.Printf(`{"level":"error","msg":"log marshal failed","err":%q}`, err.Error())
		return
	}
	l.base.Println(string(b))
}
