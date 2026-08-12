package log

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"
	vfs "github.com/shabatoily/govfs"
)

// Default는 애플리케이션 시작 시 사용되는 기본 로거 인스턴스입니다.
var Default = &Logger{
	Logger: zerolog.New(os.Stdout).Level(zerolog.InfoLevel).With().Timestamp().Logger(),
}

// Config는 로거 생성을 위한 설정 정보를 정의합니다.
type Config struct {
	Path  string        `json:"path"`
	Level zerolog.Level `json:"level"`
}

// Logger는 zerolog 로거와 리소스 정리를 위한 클로저를 포함하는 구조체입니다.
type Logger struct {
	zerolog.Logger
	close func() error
}

// Close는 로거가 사용 중인 파일 핸들 등의 리소스를 안전하게 닫습니다.
func (l *Logger) Close() error {
	if l.close != nil {
		return l.close()
	}
	return nil
}

// NewLogger는 설정을 바탕으로 새로운 로거 인스턴스를 생성하여 반환합니다.
func NewLogger(cfg Config) (*Logger, error) {
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
			err = os.MkdirAll(filepath.Dir(cfg.Path), vfs.DefaultDirMode)
			if err != nil {
				return nil, err
			}
		}

		f, err := os.OpenFile(cfg.Path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, vfs.DefaultFileMode)
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
