package util

type Logger interface {
	Infof(format string, args ...interface{})
}
