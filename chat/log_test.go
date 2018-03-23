package chat

import (
	"github.com/ethereum/go-ethereum/log"
	"github.com/sudachen/misc/out"
)

type lgx struct {
	f log.Format
}

func init() {
	log.Root().SetHandler(&lgx{log.TerminalFormat(false)})
	out.Warn.SetCurrent()
}

func level(lvl log.Lvl) out.Level {
	switch lvl {
	case log.LvlCrit:
		return out.Crit
	case log.LvlError:
		return out.Error
	case log.LvlWarn:
		return out.Warn
	case log.LvlInfo:
		return out.Info
	case log.LvlDebug:
		return out.Debug
	case log.LvlTrace:
		return out.Trace
	}
	return out.Trace
}

func (l *lgx) Log(r *log.Record) error {
	lvl := level(r.Lvl)
	if lvl.Visible() {
		lvl.Write(l.f.Format(r))
	}
	return nil
}
