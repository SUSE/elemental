/*
Copyright © 2022 - 2025 SUSE LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package log

import (
	"bytes"
	"io"

	log "github.com/sirupsen/logrus"
)

// Logger is the interface we want for our logger, so we can plug different ones easily
type Logger interface {
	Info(string, ...any)
	Warn(string, ...any)
	Debug(string, ...any)
	Error(string, ...any)
	Fatal(string, ...any)
	Panic(string, ...any)
	Trace(string, ...any)

	SetLevel(level uint32)
	GetLevel() uint32
	SetOutput(writer io.Writer)
}

var _ Logger = (*logrusWrapper)(nil)

func DebugLevel() uint32 {
	l, _ := log.ParseLevel("debug")
	return uint32(l)
}

func IsDebugLevel(l Logger) bool {
	return l.GetLevel() == DebugLevel()
}

type LoggerOptions func(l *log.Logger)

func New(opts ...LoggerOptions) Logger {
	logger := log.New()
	for _, o := range opts {
		o(logger)
	}
	return newLogrusWrapper(logger)
}

// WithDiscardAll will set a logger that discards all logs, used mainly for testing
func WithDiscardAll() LoggerOptions {
	return func(l *log.Logger) {
		l.SetOutput(io.Discard)
	}
}

// WithBuffer will set a logger that stores all logs in a buffer, used mainly for testing
func WithBuffer(b *bytes.Buffer) LoggerOptions {
	return func(l *log.Logger) {
		l.SetOutput(b)
	}
}

type logrusWrapper struct {
	*log.Logger
}

func newLogrusWrapper(l *log.Logger) Logger {
	return &logrusWrapper{Logger: l}
}

func (w logrusWrapper) GetLevel() uint32 {
	return uint32(w.Logger.GetLevel())
}

func (w *logrusWrapper) SetLevel(level uint32) {
	w.Logger.SetLevel(log.Level(level))
}

func (w *logrusWrapper) Debug(msg string, args ...any) {
	w.Logger.Debugf(msg, args...)
}

func (w *logrusWrapper) Info(msg string, args ...any) {
	w.Logger.Infof(msg, args...)
}

func (w *logrusWrapper) Warn(msg string, args ...any) {
	w.Logger.Warnf(msg, args...)
}

func (w *logrusWrapper) Error(msg string, args ...any) {
	w.Logger.Errorf(msg, args...)
}

func (w *logrusWrapper) Fatal(msg string, args ...any) {
	w.Logger.Fatalf(msg, args...)
}

func (w *logrusWrapper) Panic(msg string, args ...any) {
	w.Logger.Panicf(msg, args...)
}

func (w *logrusWrapper) Trace(msg string, args ...any) {
	w.Logger.Tracef(msg, args...)
}
