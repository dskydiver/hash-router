package lib

import (
	"gitlab.com/TitanInd/hashrouter/interfaces"
	"go.uber.org/zap"
)

type LoggerMock struct{}

func (l *LoggerMock) Debug(args ...interface{})                    {}
func (l *LoggerMock) Info(args ...interface{})                     {}
func (l *LoggerMock) Warn(args ...interface{})                     {}
func (l *LoggerMock) Error(args ...interface{})                    {}
func (l *LoggerMock) DPanic(args ...interface{})                   {}
func (l *LoggerMock) Panic(args ...interface{})                    {}
func (l *LoggerMock) Fatal(args ...interface{})                    {}
func (l *LoggerMock) Debugf(template string, args ...interface{})  {}
func (l *LoggerMock) Infof(template string, args ...interface{})   {}
func (l *LoggerMock) Warnf(template string, args ...interface{})   {}
func (l *LoggerMock) Errorf(template string, args ...interface{})  {}
func (l *LoggerMock) DPanicf(template string, args ...interface{}) {}
func (l *LoggerMock) Panicf(template string, args ...interface{})  {}
func (l *LoggerMock) Fatalf(template string, args ...interface{})  {}
func (l *LoggerMock) Sync() error {
	return nil
}
func (l *LoggerMock) Named(name string) *zap.SugaredLogger {
	return zap.NewNop().Sugar()
}

var _ interfaces.ILogger = new(LoggerMock)
