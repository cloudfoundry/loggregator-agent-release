package log

// Level type.
type Level int

// These are the different logging levels. You can set the logging level to log
// on your instance of Logger.
const (
	// FatalLevel level. Logs and then panics.
	FatalLevel = iota
	// ErrorLevel level. Used for errors that should definitely be noted.
	ErrorLevel
	// WarnLevel level. Non-critical entries that deserve eyes.
	WarnLevel
	// InfoLevel level. General operational entries about what's going on inside the
	// application. Default level.
	InfoLevel
	// DebugLevel level. Usually only enabled when debugging. Very verbose logging.
	DebugLevel
)
