package vfs

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"
)

var DefaultLogger = &Logger{
	Logger: zerolog.New(os.Stdout).Level(zerolog.InfoLevel).With().Timestamp().Logger(),
}

type LoggerConfig struct {
	Path  string        `json:"path"`
	Level zerolog.Level `json:"level"`
}

type Logger struct {
	zerolog.Logger
	close func() error
}

func (l *Logger) Close() error {
	if l.close != nil {
		return l.close()
	}
	return nil
}

func NewLogger(cfg LoggerConfig) (*Logger, error) {
	ws := make([]io.Writer, 0, 2)

	closer := func() error {
		return nil
	}

	stdout := zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: time.RFC3339,
	}

	ws = append(ws, stdout)

	if cfg.Path != "" {
		if _, err := os.Stat(filepath.Dir(cfg.Path)); errors.Is(err, os.ErrNotExist) {
			err = os.MkdirAll(filepath.Dir(cfg.Path), DefaultDirMode)
			if err != nil {
				return nil, err
			}
		}

		f, err := os.OpenFile(cfg.Path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, DefaultFileMode)
		if err != nil {
			return nil, err
		}
		closer = f.Close

		fout := zerolog.ConsoleWriter{
			Out:        f,
			TimeFormat: time.RFC3339,
		}

		ws = append(ws, fout)
	}

	logger := zerolog.New(zerolog.MultiLevelWriter(ws...)).Level(cfg.Level).With().Timestamp().Logger()

	return &Logger{
		Logger: logger,
		close:  closer,
	}, nil
}
