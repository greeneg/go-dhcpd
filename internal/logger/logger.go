package logger

import (
	"log"
	"log/syslog"
)

// Logger wraps syslog functionality
type Logger struct {
	syslogWriter *syslog.Writer
	useStdout    bool
}

var defaultLogger *Logger

// InitLogger initializes the syslog logger
func InitLogger(tag string, useStdout bool) error {
	var writer *syslog.Writer
	var err error

	if !useStdout {
		writer, err = syslog.New(syslog.LOG_DAEMON, tag)
		if err != nil {
			return err
		}
	}

	defaultLogger = &Logger{
		syslogWriter: writer,
		useStdout:    useStdout,
	}

	return nil
}

// Emergency logs an emergency message
func Emergency(msg string) {
	if defaultLogger == nil {
		log.Println("[EMERG]", msg)
		return
	}
	defaultLogger.Emergency(msg)
}

// Alert logs an alert message
func Alert(msg string) {
	if defaultLogger == nil {
		log.Println("[ALERT]", msg)
		return
	}
	defaultLogger.Alert(msg)
}

// Critical logs a critical message
func Critical(msg string) {
	if defaultLogger == nil {
		log.Println("[CRIT]", msg)
		return
	}
	defaultLogger.Critical(msg)
}

// Error logs an error message
func Error(msg string) {
	if defaultLogger == nil {
		log.Println("[ERROR]", msg)
		return
	}
	defaultLogger.Error(msg)
}

// Warning logs a warning message
func Warning(msg string) {
	if defaultLogger == nil {
		log.Println("[WARN]", msg)
		return
	}
	defaultLogger.Warning(msg)
}

// Notice logs a notice message
func Notice(msg string) {
	if defaultLogger == nil {
		log.Println("[NOTICE]", msg)
		return
	}
	defaultLogger.Notice(msg)
}

// Info logs an info message
func Info(msg string) {
	if defaultLogger == nil {
		log.Println("[INFO]", msg)
		return
	}
	defaultLogger.Info(msg)
}

// Debug logs a debug message
func Debug(msg string) {
	if defaultLogger == nil {
		log.Println("[DEBUG]", msg)
		return
	}
	defaultLogger.Debug(msg)
}

// Instance methods
func (l *Logger) Emergency(msg string) {
	if l.useStdout {
		log.Println("[EMERG]", msg)
	} else {
		l.syslogWriter.Emerg(msg)
	}
}

func (l *Logger) Alert(msg string) {
	if l.useStdout {
		log.Println("[ALERT]", msg)
	} else {
		l.syslogWriter.Alert(msg)
	}
}

func (l *Logger) Critical(msg string) {
	if l.useStdout {
		log.Println("[CRIT]", msg)
	} else {
		l.syslogWriter.Crit(msg)
	}
}

func (l *Logger) Error(msg string) {
	if l.useStdout {
		log.Println("[ERROR]", msg)
	} else {
		l.syslogWriter.Err(msg)
	}
}

func (l *Logger) Warning(msg string) {
	if l.useStdout {
		log.Println("[WARN]", msg)
	} else {
		l.syslogWriter.Warning(msg)
	}
}

func (l *Logger) Notice(msg string) {
	if l.useStdout {
		log.Println("[NOTICE]", msg)
	} else {
		l.syslogWriter.Notice(msg)
	}
}

func (l *Logger) Info(msg string) {
	if l.useStdout {
		log.Println("[INFO]", msg)
	} else {
		l.syslogWriter.Info(msg)
	}
}

func (l *Logger) Debug(msg string) {
	if l.useStdout {
		log.Println("[DEBUG]", msg)
	} else {
		l.syslogWriter.Debug(msg)
	}
}

// Close closes the syslog connection
func Close() {
	if defaultLogger != nil && defaultLogger.syslogWriter != nil {
		defaultLogger.syslogWriter.Close()
	}
}
