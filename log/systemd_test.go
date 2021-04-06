package syslog_test

import (
	"bytes"
	"testing"

	lokelog "github.com/LOKE/pkg/log"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/google/go-cmp/cmp"
)

func Test_logger_Log(t *testing.T) {
	t.Parallel()

	type args []interface{}

	tests := []struct {
		name    string
		level   func(log.Logger) log.Logger
		args    args
		want    string
		wantErr bool
	}{
		{
			name:  "debug level",
			level: level.Debug,
			args:  args{"msg", "some message"},
			want:  "<7>level=debug msg=\"some message\"\n",
		},
		{
			name:  "info level",
			level: level.Info,
			args:  args{"msg", "some message"},
			want:  "<6>level=info msg=\"some message\"\n",
		},
		{
			name:  "warn level",
			level: level.Warn,
			args:  args{"msg", "some message"},
			want:  "<4>level=warn msg=\"some message\"\n",
		},
		{
			name:  "error level",
			level: level.Error,
			args:  args{"msg", "some message"},
			want:  "<3>level=error msg=\"some message\"\n",
		},
		{
			name:  "no level",
			level: func(l log.Logger) log.Logger { return l },
			args:  args{"msg", "some message"},
			want:  "<6>msg=\"some message\"\n",
		},
	}

	buf := &bytes.Buffer{}
	l := lokelog.NewSystemdLogger(buf, log.NewLogfmtLogger)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf.Reset()

			if err := tt.level(l).Log(tt.args...); (err != nil) != tt.wantErr {
				t.Errorf("logger.Log() error = %v, wantErr %v", err, tt.wantErr)
			}

			if diff := cmp.Diff(tt.want, buf.String()); diff != "" {
				t.Errorf("result mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
