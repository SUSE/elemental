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
	"fmt"
	"io"
	"regexp"
	"strings"

	log "github.com/sirupsen/logrus"
)

// Logger is the interface we want for our logger, so we can plug different ones easily
type Logger interface {
	Info(...any)
	Warn(...any)
	Debug(...any)
	Error(...any)
	Fatal(...any)
	Warning(...any)
	Panic(...any)
	Trace(...any)
	Infof(string, ...any)
	Warnf(string, ...any)
	Debugf(string, ...any)
	Errorf(string, ...any)
	Fatalf(string, ...any)
	Panicf(string, ...any)
	Tracef(string, ...any)

	SetLevel(level uint32)
	GetLevel() uint32
	SetOutput(writer io.Writer)

	SetContext(string)
	SpinnerStop()
	Spinner()
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

var emojiStrip = regexp.MustCompile(`[:][\w]+[:]`)

func (w *logrusWrapper) Debug(args ...any) {
	converted := convert(args)
	w.Logger.Debug(converted)
}

func (w *logrusWrapper) Info(args ...any) {
	converted := convert(args)
	w.Logger.Info(converted)
}

func (w *logrusWrapper) Warn(args ...any) {
	converted := convert(args)
	w.Logger.Warn(converted)
}

func (w *logrusWrapper) Error(args ...any) {
	converted := convert(args)
	w.Logger.Error(converted)
}

func (w *logrusWrapper) Fatal(args ...any) {
	converted := convert(args)
	w.Logger.Fatal(converted)
}

// convert changes a list of interfaces into a proper joined string ready to log
func convert(args []any) string {
	var together []string
	// Matches a :WORD: and any extra space after that and the next word to remove emojis
	// which are like ":house: realMessageStartsHere"
	emojiStrip = regexp.MustCompile(`[:][\w]+[:]\s`)
	for _, a := range args {
		toClean := fmt.Sprintf("%v", a)                     // coerce into string
		cleaned := emojiStrip.ReplaceAllString(toClean, "") // remove any emoji
		trimmed := strings.Trim(cleaned, " ")               // trim any spaces in prefix/suffix
		together = append(together, trimmed)
	}
	return strings.Join(together, " ") // return them nicely joined with spaces like a normal phrase
}

func (w *logrusWrapper) SetContext(string) {}
func (w *logrusWrapper) Spinner()          {}
func (w *logrusWrapper) SpinnerStop()      {}
