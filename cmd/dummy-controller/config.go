package main

import "github.com/Demonware/balanced/pkg/watcher"

type config struct {
	Controller watcher.ConfigController `json:"controller"`
}
