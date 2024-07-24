package log

import (
	"fmt"
	"log"
	"os"
)

// A Logger represents an active logging object that generates lines of output
// to os.Stderr. Each logging operation is passed through to the internal
// Logger. A Logger can be used simultaneously from multiple goroutines.
type Logger struct {
	*log.Logger

	session string
	level   Level
}

// New creates a new Logger.
func New() *Logger {
	return &Logger{
		Logger:  log.New(os.Stderr, "", log.LstdFlags),
		session: "",
		level:   InfoLevel,
	}
}

func (l *Logger) UseRFC3339() {
	l.Logger = log.New(new(rfc3339Writer), "", 0)
}

func (l *Logger) SetLevel(lvl Level) {
	l.level = lvl
}

// Session returns a new Logger with the given session name.
func (l *Logger) Session(s string) *Logger {
	if l.session != "" {
		s = fmt.Sprintf("%s: %s", l.session, s)
	}
	s = fmt.Sprintf("%s: ", s)
	return &Logger{
		Logger:  l.Logger,
		session: s,
		level:   l.level,
	}
}

// Debugf calls l.Printf with the given format and arguments.
func (l *Logger) Debugf(format string, v ...any) {
	if l.level < DebugLevel {
		return
	}

	l.Printf("DEBUG "+l.session+format, v...)
}

// Infof calls l.Printf with the given format and arguments.
func (l *Logger) Infof(format string, v ...any) {
	if l.level < InfoLevel {
		return
	}

	l.Printf("INFO "+l.session+format, v...)
}

// Warnf calls l.Printf with the given format and arguments.
func (l *Logger) Warnf(format string, v ...any) {
	if l.level < WarnLevel {
		return
	}

	l.Printf("WARN "+l.session+format, v...)
}

// Errorf calls l.Printf with the given format and arguments.
func (l *Logger) Errorf(format string, v ...any) {
	if l.level < ErrorLevel {
		return
	}

	l.Printf("ERROR "+l.session+format, v...)
}

// Fatalf calls l.Panicf with the given format and arguments.
func (l *Logger) Fatalf(format string, v ...any) {
	l.Panicf("FATAL "+l.session+format, v...)
}
