// +build !windows

package main

import (
	daemon "github.com/AlexStocks/go-daemon"
	log "github.com/sirupsen/logrus"
)

func Deamonize(proc func(), logFile string) {
	if len(logFile) <= 0 {
		logFile = "/tmp/supervisor-daemon.log"
	}
	context := daemon.Context{LogFileName: logFile}

	child, err := context.Reborn()
	if err != nil {
		log.WithFields(log.Fields{"err": err}).Fatal("Unable to run")
	}
	if child != nil {
		return
	}
	defer context.Release()

	log.Info("daemon started")

	proc()
}
