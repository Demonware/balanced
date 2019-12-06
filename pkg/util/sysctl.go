// Taken from https://github.com/google/seesaw/blob/f11734aa58f0f45d1715844228955a05cdc06f67/ncc/sysctl.go

package util

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strings"

	"github.com/golang/glog"
)

const (
	sysctlPath = "/proc/sys"
)

// sysctl sets the named sysctl to the value specified and returns its
// original value as a string. Note that this cannot be used if a sysctl
// component includes a period it its name - in that case use
// sysctlByComponents instead.
func Sysctl(name, value string) (string, error) {
	glog.Infof("Setting sysctl %s: %s", name, value)
	return sysctlByComponents(strings.Split(name, "."), value)
}

// sysctlByComponents sets the sysctl specified by the individual components
// to the value specified and returns its original value as a string.
func sysctlByComponents(components []string, value string) (string, error) {
	components = append([]string{sysctlPath}, components...)
	f, err := os.OpenFile(path.Join(components...), os.O_RDWR, 0)
	if err != nil {
		return "", err
	}
	defer f.Close()
	b, err := ioutil.ReadAll(f)
	if err != nil {
		return "", err
	}
	if _, err := f.Seek(0, 0); err != nil {
		return "", err
	}
	if _, err := fmt.Fprintf(f, "%s\n", value); err != nil {
		return "", err
	}
	return strings.TrimRight(string(b), "\n"), nil
}
