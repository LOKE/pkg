package syslog

import (
	"bytes"
	"io"
	"sync"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
)

const (
	prefixDebug = "<7>"
	prefixInfo  = "<6>"
	prefixWarn  = "<4>"
	prefixError = "<3>"
)

// NewSystemdLogger returns a new Logger which prefixes logs with the systemd/syslog priority.
// The body of the log message is the formatted output from the Logger returned
// by logfmt.
func NewSystemdLogger(w io.Writer, logfmt func(io.Writer) log.Logger) log.Logger {
	return &logger{
		w: w,
		bufPool: sync.Pool{New: func() interface{} {
			buf := &bytes.Buffer{}

			return &loggerBuf{
				buf:    buf,
				logger: logfmt(buf),
			}
		}},
	}
}

type logger struct {
	w       io.Writer
	bufPool sync.Pool
}

func (l *logger) Log(keyvals ...interface{}) error {
	lb := l.getLoggerBuf()
	defer l.putLoggerBuf(lb)

	p := levelPrefix(keyvals...)
	if _, err := lb.buf.WriteString(p); err != nil {
		return err
	}

	if err := lb.logger.Log(keyvals...); err != nil {
		return err
	}

	_, err := l.w.Write(lb.buf.Bytes())
	return err
}

type loggerBuf struct {
	buf    *bytes.Buffer
	logger log.Logger
}

func (l *logger) getLoggerBuf() *loggerBuf {
	return l.bufPool.Get().(*loggerBuf)
}

func (l *logger) putLoggerBuf(lb *loggerBuf) {
	lb.buf.Reset()
	l.bufPool.Put(lb)
}

func levelPrefix(keyvals ...interface{}) string {
	l := len(keyvals)
	for i := 0; i < l; i += 2 {
		if keyvals[i] == level.Key() {
			var val interface{}
			if i+1 < l {
				val = keyvals[i+1]
			}
			if v, ok := val.(level.Value); ok {
				switch v {
				case level.DebugValue():
					return prefixDebug
				case level.InfoValue():
					return prefixInfo
				case level.WarnValue():
					return prefixWarn
				case level.ErrorValue():
					return prefixError
				}
			}
		}
	}

	return prefixInfo
}
