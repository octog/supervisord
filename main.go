package main

import (
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"unicode"

	"github.com/AlexStocks/goext/sync"
	"github.com/bcicen/grmon/agent"
	"github.com/jessevdk/go-flags"
	reaper "github.com/ochinchina/go-reaper"
	log "github.com/sirupsen/logrus"
)

const (
	goSupervisordLockFile = "/tmp/supervisor/lock/gospd.lck"

	usageStr = `
Usage: supervisord [OPTIONS] <ctl | init | version>
  Go runtime version %s
  Go supervisord version %s
  Application Options:
    -c, --configuration= the configuration file
    -d, --daemon         run as daemon
        --env-file=      the environment file

    Help Options:
      -h, --help           Show this help message

    Available commands:
      ctl      Control a running daemon
      init     initialize a template
      version  show the version of supervisor

    Control Command Options:
      reload
      restart <worker_name>
      restart all
      shutdown
      status <worker_name>
      start <worker_name>
      start all
      signal <signal_name> <process_name> <process_name> ...
      signal all
      stop <worker_name>
      stop all
      pid <process_name>
      update <worker_name>
      update all
    `
)

type Options struct {
	Configuration string `short:"c" long:"configuration" description:"the configuration file"`
	Daemon        bool   `short:"d" long:"daemon" description:"run as daemon"`
	EnvFile       string `long:"env-file" description:"the environment file"`
}

var (
	spLock sync.Mutex
	sp     *Supervisor
)

func init() {
	log.SetOutput(os.Stdout)
	if runtime.GOOS == "windows" {
		log.SetFormatter(&log.TextFormatter{DisableColors: true, FullTimestamp: true})
	} else {
		log.SetFormatter(&log.TextFormatter{DisableColors: false, FullTimestamp: true})
	}
	log.SetLevel(log.DebugLevel)
}

func initSignals() {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	for {
		select {
		case sig := <-sigs:
			log.WithFields(log.Fields{"signal": sig}).Info("receive a signal to stop all processes & exit")
			switch sig {
			default:
				spLock.Lock()
				defer spLock.Unlock()
				if sp != nil {
					sp.procMgr.StopAllProcesses(false, true)
				}
				os.Exit(-1)
			}
		}
	}

	// go func() {
	// 	sig := <-sigs
	// 	log.WithFields(log.Fields{"signal": sig}).Info("receive a signal to stop all processes & exit")
	// 	s.procMgr.StopAllProcesses(false, true)
	// 	os.Exit(-1)
	// }()
}

var options Options
var parser = flags.NewParser(&options, flags.Default & ^flags.PrintErrors)

func LoadEnvFile() {
	if len(options.EnvFile) <= 0 {
		return
	}
	//try to open the environment file
	f, err := os.Open(options.EnvFile)
	if err != nil {
		log.WithFields(log.Fields{"file": options.EnvFile}).Error("Fail to open environment file")
		return
	}
	defer f.Close()
	reader := bufio.NewReader(f)
	for {
		//for each line
		line, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		//if line starts with '#', it is a comment line, ignore it
		line = strings.TrimSpace(line)
		if len(line) > 0 && line[0] == '#' {
			continue
		}
		//if environment variable is exported with "export"
		if strings.HasPrefix(line, "export") && len(line) > len("export") && unicode.IsSpace(rune(line[len("export")])) {
			line = strings.TrimSpace(line[len("export"):])
		}
		//split the environment variable with "="
		pos := strings.Index(line, "=")
		if pos != -1 {
			k := strings.TrimSpace(line[0:pos])
			v := strings.TrimSpace(line[pos+1:])
			//if key and value are not empty, put it into the environment
			if len(k) > 0 && len(v) > 0 {
				os.Setenv(k, v)
			}
		}
	}
}

// find the supervisord.conf in following order:
//
// 1. $CWD/supervisord.conf
// 2. $CWD/etc/supervisord.conf
// 3. /etc/supervisord.conf
// 4. /etc/supervisor/supervisord.conf (since Supervisor 3.3.0)
// 5. ../etc/supervisord.conf (Relative to the executable)
// 6. ../supervisord.conf (Relative to the executable)
func findSupervisordConf() (string, error) {
	possibleSupervisordConf := []string{options.Configuration,
		"./supervisord.conf",
		"./conf/supervisord.conf",
		"./etc/supervisord.conf",
		"/etc/supervisord.conf",
		"/etc/supervisor/supervisord.conf",
		"../etc/supervisord.conf",
		"../supervisord.conf"}

	for _, file := range possibleSupervisordConf {
		if _, err := os.Stat(file); err == nil {
			abs_file, err := filepath.Abs(file)
			if err == nil {
				return abs_file, nil
			} else {
				return file, nil
			}
		}
	}

	return "", fmt.Errorf("fail to find supervisord.conf")
}

func getConfFile() string {
	var (
		err  error
		conf string
	)

	conf = options.Configuration
	if len(conf) == 0 {
		conf, err = findSupervisordConf()
		if err != nil {
			panic("cat not find configure file!")
		}
	}

	return conf
}

func RunServer() {
	go initSignals()
	// infinite loop for handling Restart ('reload' command)
	LoadEnvFile()
	startup := true
	for {
		s := NewSupervisor(getConfFile())
		if sErr, _, _, _ := s.Reload(startup); sErr != nil {
			panic(sErr)
		}
		startup = false
		func(spv *Supervisor) {
			if s != nil {
				spLock.Lock()
				defer spLock.Unlock()
				sp = s
			}
		}(s)
		s.WaitForExit()
	}
}

func main() {
	grmon.Start()
	go reaper.Reap()

	if _, err := parser.Parse(); err != nil {
		flagsErr, ok := err.(*flags.Error)
		if ok {
			switch flagsErr.Type {
			case flags.ErrHelp:
				fmt.Fprintf(os.Stdout, usageStr+"\n", runtime.Version(), VERSION)
				os.Exit(0)

			case flags.ErrCommandRequired:
				fileLock := gxsync.NewFlock(goSupervisordLockFile)
				locked, err := fileLock.TryLock()
				if err != nil {
					fmt.Printf("lock error:%s\n", err)
					return
				}
				if !locked {
					fmt.Printf("failed to lock file-lock %s\n", goSupervisordLockFile)
					return
				}
				defer fileLock.Unlock()

				if options.Daemon {
					Deamonize(RunServer)
				} else {
					RunServer()
				}

			default:
				fmt.Fprintf(os.Stderr, "error when parsing command: %s\n", err)
				os.Exit(1)
			}
		}
	}
}
