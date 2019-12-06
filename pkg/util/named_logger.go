package util

import (
	"github.com/golang/glog"
)

type NamedLogger struct {
	name   string
	logger glog.Verbose
}

func NewNamedLogger(name string, level glog.Level) *NamedLogger {
	return &NamedLogger{
		name:   name,
		logger: glog.V(level),
	}
}

func (nl *NamedLogger) Infof(format string, args ...interface{}) {
	namedFormat := nl.name + ": " + format
	nl.logger.Infof(namedFormat, args...)
}
