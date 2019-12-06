package main

import (
	"github.com/Demonware/balanced/pkg/pidresolver"
	"github.com/Demonware/balanced/pkg/watcher"
)

type config struct {
	Controller watcher.ConfigController   `json:"controller"`
	Resolver   pidresolver.ConfigResolver `json:"resolver"`
}
