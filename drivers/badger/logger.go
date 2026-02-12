package badger

import (
	"fmt"
	"log/slog"
)

type BadgerSlogAdapter struct {
	core *slog.Logger
}

func (a *BadgerSlogAdapter) Errorf(f string, v ...any) {
	a.core.Error(fmt.Sprintf(f, v...))
}
func (a *BadgerSlogAdapter) Warningf(f string, v ...any) {
	a.core.Warn(fmt.Sprintf(f, v...))
}
func (a *BadgerSlogAdapter) Infof(f string, v ...any) {
	a.core.Info(fmt.Sprintf(f, v...))
}
func (a *BadgerSlogAdapter) Debugf(f string, v ...any) {
	a.core.Debug(fmt.Sprintf(f, v...))
}

func NewBadgerSlogAdapter(l *slog.Logger) *BadgerSlogAdapter {
	return &BadgerSlogAdapter{core: l}
}
