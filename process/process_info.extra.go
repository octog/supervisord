package process

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strings"
	"syscall"
	"time"

	"github.com/AlexStocks/goext/os/process"
	"github.com/AlexStocks/supervisord/config"
	"github.com/AlexStocks/supervisord/signals"
	"github.com/AlexStocks/supervisord/types"
	jerrors "github.com/juju/errors"
	log "github.com/sirupsen/logrus"
	yaml "gopkg.in/yaml.v2"
)

func (p *ProcessInfo) TypeProcessInfo() types.ProcessInfo {
	state := ProcessState(RUNNING)
	if !p.CheckAlive() {
		state = ProcessState(STOPPED)
	}
	info := types.ProcessInfo{
		Name: p.Program,
		// Group:          p.GetGroup(),
		// Description:    p.GetDescription(),
		Start:     int(p.StartTime) / 1e9,
		Stop:      int(p.endTime) / 1e9,
		Now:       int(time.Now().Unix()),
		State:     int(state),
		Statename: state.String(),
		Spawnerr:  "",
		// Exitstatus:     0,
		Logfile:        getStdoutLogfile(p.config),
		Stdout_logfile: getStdoutLogfile(p.config),
		Stderr_logfile: getStderrLogfile(p.config),
		Pid:            int(p.PID),
	}

	startTime := time.Unix(int64(p.StartTime/1e9), int64(p.StartTime%1e9))
	endTime := time.Now()
	if p.endTime != 0 {
		endTime = time.Unix(int64(p.endTime/1e9), int64(p.endTime%1e9))
	}
	seconds := int(endTime.Sub(startTime).Seconds())
	minutes := seconds / 60
	hours := minutes / 60
	days := hours / 24
	if days > 0 {
		info.Description = fmt.Sprintf("pid %d, uptime %d days, %d:%02d:%02d", info.Pid, days, hours%24, minutes%60, seconds%60)
	} else {
		info.Description = fmt.Sprintf("pid %d, uptime %d:%02d:%02d", info.Pid, hours%24, minutes%60, seconds%60)
	}

	return info
}

func (p *ProcessInfo) ConfigEntry() *config.ConfigEntry {
	return p.config
}

func (p *ProcessInfo) IsFrozen() bool {
	if p.PID == int64(FROZEN_PID) {
		return true
	}

	return false
}

func (p *ProcessInfo) CheckAlive() bool {
	if p.PID == int64(FROZEN_PID) {
		return false
	}

	if _, err := gxprocess.FindProcess(int(p.PID)); err != nil {
		return false
	}

	return true
}

//send signal to process to stop it
func (p *ProcessInfo) Stop(wait bool) {
	log.WithFields(log.Fields{"program": p.Program}).Info("stop the program")
	var (
		sigs        []string
		stopasgroup bool
		killasgroup bool
		waitsecs    = time.Duration(10e9)
	)

	if p.IsFrozen() {
		log.WithFields(log.Fields{"processInfo": p.Program}).Info("can not stop the frozen program")
		return
	}

	if nil != p.config {
		sigs = strings.Fields(p.config.GetString("stopsignal", ""))
		waitsecs = time.Duration(p.config.GetInt("stopwaitsecs", 10)) * time.Second
		stopasgroup = p.config.GetBool("stopasgroup", false)
		killasgroup = p.config.GetBool("killasgroup", stopasgroup)
	}
	if stopasgroup && !killasgroup {
		log.WithFields(log.Fields{"program": p.Program}).Error("Cannot set stopasgroup=true and killasgroup=false")
	}

	go func() {
		stopped := false
		for i := 0; i < len(sigs) && !stopped; i++ {
			// send signal to process
			sig, err := signals.ToSignal(sigs[i])
			if err != nil {
				continue
			}
			log.WithFields(log.Fields{"program": p.Program, "signal": sigs[i], "pid": p.PID}).Info("send stop signal to program")
			signals.KillPid(int(p.PID), sig, stopasgroup)
			endTime := time.Now().Add(waitsecs)
			//wait at most "stopwaitsecs" seconds for one signal
			for endTime.After(time.Now()) {
				//if it already exits
				if !p.CheckAlive() {
					stopped = true
					break
				}

				time.Sleep(1 * time.Second)
			}
		}
		if !stopped {
			log.WithFields(log.Fields{"program": p.Program, "signal": "KILL", "pid": p.PID}).Info("force to kill the program")
			signals.KillPid(int(p.PID), syscall.SIGKILL, killasgroup)
		}
		if !wait {
			p.PID = int64(FROZEN_PID)
			p.endTime = uint64(time.Now().UnixNano())
		}
	}()
	if wait {
		for {
			if !p.CheckAlive() {
				break
			}
			time.Sleep(1 * time.Second)
		}
		p.PID = int64(FROZEN_PID)
		p.endTime = uint64(time.Now().UnixNano())
	}
}

func NewProcessInfoMap() *ProcessInfoMap {
	return &ProcessInfoMap{
		InfoMap: make(map[string]ProcessInfo),
	}
}

func (m *ProcessInfoMap) load(file string) error {
	configFile, err := ioutil.ReadFile(file)
	if err != nil {
		return jerrors.Trace(err)
	}

	err = yaml.Unmarshal(configFile, m)
	if err != nil {
		return jerrors.Trace(err)
	}

	return nil
}

func (m *ProcessInfoMap) findProcessInfo(name string) *ProcessInfo {
	m.lock.Lock()
	defer m.lock.Unlock()

	info, ok := m.InfoMap[name]
	if !ok {
		return nil
	}
	log.Debug("succeed to find process info:", info)

	return &info
}

func (m *ProcessInfoMap) stopProcessInfo(name string, wait bool) *ProcessInfo {
	m.lock.Lock()
	defer m.lock.Unlock()

	info, ok := m.InfoMap[name]
	if ok && !info.IsFrozen() {
		info.Stop(wait)
		m.InfoMap[name] = info
		return &info
	}

	return nil
}

func (m *ProcessInfoMap) store(file string) error {
	if err := m.Validate(); err != nil {
		os.Remove(file)
		return err
	}

	// valid info map
	infoMap := NewProcessInfoMap()
	infoMap.Version = m.Version
	m.lock.Lock()
	for _, info := range m.InfoMap {
		if info.CheckAlive() {
			infoMap.AddProcessInfo(info)
		}
	}
	m.lock.Unlock()

	var fileStream []byte
	fileStream, err := yaml.Marshal(infoMap)
	if err != nil {
		return jerrors.Trace(err)
	}

	basePath := path.Dir(file)
	if err = os.MkdirAll(basePath, 0766); err != nil &&
		!strings.Contains(err.Error(), "file exists") {
		return jerrors.Trace(err)
	}
	os.Remove(file)

	err = ioutil.WriteFile(file, fileStream, 0766)
	if err != nil {
		return jerrors.Trace(err)
	}

	return nil
}

func (m *ProcessInfoMap) AddProcessInfo(info ProcessInfo) {
	m.lock.Lock()
	defer m.lock.Unlock()

	m.InfoMap[info.Program] = info
	m.Version = uint64(time.Now().UnixNano())
}

func (m *ProcessInfoMap) removeProcessInfo(program string) ProcessInfo {
	m.lock.Lock()
	defer m.lock.Unlock()

	info, ok := m.InfoMap[program]
	if ok {
		delete(m.InfoMap, program)
	}
	m.Version = uint64(time.Now().UnixNano())
	return info
}

func (m *ProcessInfoMap) GetProcessInfo(program string) (ProcessInfo, bool) {
	m.lock.Lock()
	defer m.lock.Unlock()

	info, ok := m.InfoMap[program]
	return info, ok
}

func (m *ProcessInfoMap) getDeadPrestartProcess() (int, []string) {
	m.lock.Lock()
	defer m.lock.Unlock()

	prestartNum := 0
	psArray := make([]string, 0, len(m.InfoMap))
	for name, info := range m.InfoMap {
		if info.StartTime > supervisordStartTime {
			continue
		}
		prestartNum++
		if info.PID == int64(FROZEN_PID) { // 进程是 supervisorctl 杀掉的，不用重启
			continue
		}
		_, err := gxprocess.FindProcess(int(info.PID))
		if err != nil {
			// delete(pm5.psInfoMap.InfoMap, name)
			m.removeProcessInfo(name)
			psArray = append(psArray, name)
		}
	}

	return prestartNum, psArray
}

func (m *ProcessInfoMap) getFrozenPrestartProcess() (int, []ProcessInfo) {
	m.lock.Lock()
	defer m.lock.Unlock()

	num := 0
	psArray := make([]ProcessInfo, 0, len(m.InfoMap))
	for name, info := range m.InfoMap {
		if info.StartTime > supervisordStartTime {
			continue
		}
		num++
		if info.PID == int64(FROZEN_PID) { // 进程是 supervisorctl 杀掉的，不用重启
			m.removeProcessInfo(name)
			psArray = append(psArray, info)
		}
	}

	return num, psArray
}

// 用于 status _infomap
func (m *ProcessInfoMap) getAllInfomapProcess() (int, []ProcessInfo) {
	m.lock.Lock()
	defer m.lock.Unlock()

	prestartNum := 0
	psArray := make([]ProcessInfo, 0, len(m.InfoMap))
	for _, info := range m.InfoMap {
		prestartNum++
		// if _, err := gxprocess.FindProcess(int(info.PID)); err == nil {
		psArray = append(psArray, info)
		// }
	}

	return prestartNum, psArray
}

// the return value list:
// the first is prestart process number
// the second is the active prestart process name array
func (m *ProcessInfoMap) getActivePrestartProcess() (int, []ProcessInfo) {
	m.lock.Lock()
	defer m.lock.Unlock()

	prestartNum := 0
	psArray := make([]ProcessInfo, 0, len(m.InfoMap))
	for _, info := range m.InfoMap {
		if info.StartTime > supervisordStartTime {
			continue
		}
		if info.PID == int64(FROZEN_PID) {
			continue
		}
		prestartNum++
		if _, err := gxprocess.FindProcess(int(info.PID)); err == nil {
			psArray = append(psArray, info)
		}
	}

	return prestartNum, psArray
}

func (m *ProcessInfoMap) getPrestartProcess() (int, []ProcessInfo) {
	m.lock.Lock()
	defer m.lock.Unlock()

	prestartNum := 0
	psArray := make([]ProcessInfo, 0, len(m.InfoMap))
	for _, info := range m.InfoMap {
		if info.StartTime > supervisordStartTime {
			continue
		}
		prestartNum++
		psArray = append(psArray, info)
	}

	return prestartNum, psArray
}

func (m *ProcessInfoMap) validateStartPs(psInfoFile string, startKillAll bool) {
	m.lock.Lock()
	defer m.lock.Unlock()

	m.load(psInfoFile)
	for name, info := range m.InfoMap {
		ps, err := gxprocess.FindProcess(int(info.PID))
		if err != nil {
			m.removeProcessInfo(name)
			continue
		}

		if startKillAll {
			syscall.Kill(ps.Pid(), syscall.SIGKILL)
			m.removeProcessInfo(name)
			continue
		}
	}
}

func (m *ProcessInfoMap) killAllProcess(procFunc func(ProcessInfo)) {
	m.lock.Lock()
	defer m.lock.Unlock()

	for _, info := range m.InfoMap {
		if info.CheckAlive() {
			info.Stop(true)
		}
		if procFunc != nil {
			procFunc(info)
		}
		m.InfoMap[info.Program] = info
	}
}

func (m *ProcessInfoMap) removeAllProcess(procFunc func(ProcessInfo)) {
	m.lock.Lock()
	defer m.lock.Unlock()

	for _, info := range m.InfoMap {
		if !info.CheckAlive() {
			if procFunc != nil {
				procFunc(info)
			}
			m.removeProcessInfo(info.Program)
		}
	}
}
