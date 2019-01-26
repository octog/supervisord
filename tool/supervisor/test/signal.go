package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	Log "github.com/AlexStocks/log4go"
)

func initSignal() {
	var (
		i       int
		signals = make(chan os.Signal, 1)
		ticker  = time.NewTicker(30e9)
	)
	// It is not possible to block SIGKILL or syscall.SIGSTOP
	signal.Notify(signals, os.Interrupt, os.Kill, syscall.SIGHUP,
		syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGINT, syscall.SIGPIPE)
	for {
		select {
		case sig := <-signals:
			Log.Info("get signal %s", sig.String())
		case <-ticker.C:
			i++
			fmt.Printf("hello:%d\n", i)
			Log.Info("signal loop, now:%s", time.Now())
		}
	}
}

func main() {
	initSignal()
}
