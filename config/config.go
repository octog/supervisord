package config

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"

	ini "github.com/ochinchina/go-ini"
	log "github.com/sirupsen/logrus"
)

type ConfigEntry struct {
	ConfigDir string
	Group     string
	Name      string
	keyValues map[string]string
}

func (c *ConfigEntry) Clone() ConfigEntry {
	m := make(map[string]string, len(c.keyValues))
	for k := range c.keyValues {
		m[k] = c.keyValues[k]
	}

	return ConfigEntry{
		ConfigDir: c.ConfigDir,
		Group:     c.Group,
		Name:      c.Name,
		keyValues: m,
	}
}

func (c *ConfigEntry) IsSame(entry ConfigEntry) bool {
	if c.ConfigDir != entry.ConfigDir {
		return false
	}

	if c.Group != entry.Group {
		return false
	}

	if c.Name != entry.Name {
		return false
	}

	for k, v := range c.keyValues {
		entryValue, ok := entry.keyValues[k]
		if !ok {
			return false
		}
		if v != entryValue {
			return false
		}
	}

	for k, v := range entry.keyValues {
		value, ok := c.keyValues[k]
		if !ok {
			return false
		}
		if v != value {
			return false
		}
	}

	return true
}

func (c *ConfigEntry) IsProgram() bool {
	return strings.HasPrefix(c.Name, "program:")
}

func (c *ConfigEntry) GetProgramName() string {
	if strings.HasPrefix(c.Name, "program:") {
		return c.Name[len("program:"):]
	}
	return ""
}

func (c *ConfigEntry) IsEventListener() bool {
	return strings.HasPrefix(c.Name, "eventlistener:")
}

func (c *ConfigEntry) GetEventListenerName() string {
	if strings.HasPrefix(c.Name, "eventlistener:") {
		return c.Name[len("eventlistener:"):]
	}
	return ""
}

func (c *ConfigEntry) IsGroup() bool {
	return strings.HasPrefix(c.Name, "group:")
}

// 是否用到了numprocs参数
func (c *ConfigEntry) IsMultiIns() bool {
	if c.GetInt("numprocs", 1) > 1 {
		return true
	}
	return false
}

// get the group name if this entry is group
func (c *ConfigEntry) GetGroupName() string {
	if strings.HasPrefix(c.Name, "group:") {
		return c.Name[len("group:"):]
	}
	return ""
}

// get the programs from the group
func (c *ConfigEntry) GetPrograms() []string {
	if c.IsGroup() {
		r := c.GetStringArray("programs", ",")
		for i, p := range r {
			r[i] = strings.TrimSpace(p)
		}
		return r
	}
	return make([]string, 0)
}

func (c *ConfigEntry) setGroup(group string) {
	c.Group = group
}

// dump the configuration as string
func (c *ConfigEntry) String() string {
	buf := bytes.NewBuffer(make([]byte, 0))
	fmt.Fprintf(buf, "configDir=%s\n", c.ConfigDir)
	fmt.Fprintf(buf, "group=%s\n", c.Group)
	for k, v := range c.keyValues {
		fmt.Fprintf(buf, "%s=%s\n", k, v)
	}

	return buf.String()
}

type Config struct {
	lock       sync.Mutex
	configFile string
	//mapping between the section name and the configure
	entries map[string]*ConfigEntry

	ProgramGroup *ProcessGroup
	//用于update 一个group时，表示非program配置信息都已经加载过了
	initialized bool
}

func NewConfigEntry(configDir string) *ConfigEntry {
	return &ConfigEntry{configDir, "", "", make(map[string]string)}
}

func NewConfig(configFile string) *Config {
	return &Config{
		configFile:   configFile,
		entries:      make(map[string]*ConfigEntry),
		ProgramGroup: NewProcessGroup(),
	}
}

//create a new entry or return the already-exist entry
func (c *Config) createEntry(name string, configDir string) *ConfigEntry {
	entry, ok := c.entries[name]

	if !ok {
		entry = NewConfigEntry(configDir)
		c.entries[name] = entry
	}
	return entry
}

//
// return the loaded programs
func (c *Config) Load() ([]string, error) {
	c.lock.Lock()
	defer c.lock.Unlock()

	ini := ini.NewIni()
	// 创建新的 c.ProgramGroup && c.entries
	c.ProgramGroup = NewProcessGroup()
	if c.entries == nil || len(c.entries) != 0 {
		c.entries = make(map[string]*ConfigEntry)
	}
	ini.LoadFile(c.configFile)

	includeFiles := c.getIncludeFiles(ini)
	for _, f := range includeFiles {
		ini.LoadFile(f)
	}
	// 读取新的配置并更新 c.entries，但是并不删除过时的 ConfigEntry
	// programs 中仅仅存储了 [program:xxx]，例如 "supervisorctl"/"supervisord" 等项目并不在其中
	programs := c.parse(ini)
	c.initialized = true
	return programs, nil
}

// 重新update一个group时加载配置文件
func (c *Config) LoadGroup(gName string) []string {
	c.lock.Lock()
	defer c.lock.Unlock()

	ini := ini.NewIni()
	if !c.initialized {
		// 创建新的 c.ProgramGroup && c.entries
		c.ProgramGroup = NewProcessGroup()
	}
	if c.entries == nil {
		c.entries = make(map[string]*ConfigEntry)
	}
	ini.LoadFile(c.configFile)

	includeFiles := c.getIncludeFiles(ini)
	for _, f := range includeFiles {
		ini.LoadFile(f)
	}
	// 读取新的配置并更新 c.entries，但是并不删除过时的 ConfigEntry
	// programs 中仅仅存储了 [program:xxx]，例如 "supervisorctl"/"supervisord" 等项目并不在其中
	//该group下所有program名
	programMap := make(map[string]bool)
	for _, section := range ini.Sections() {
		if strings.HasPrefix(section.Name, "group:") && strings.TrimSpace(section.Name[len("group:"):]) == gName {
			entry := c.createEntry(section.Name, c.GetConfigFileDir())
			entry.parse(section)
			groupName := entry.GetGroupName()
			programs := entry.GetPrograms()
			for _, program := range programs {
				c.ProgramGroup.Add(groupName, program)
				programMap[program] = true
			}
			break
		}
	}
	// 正常情况下program的组为自己
	if len(programMap) == 0 {
		programMap[gName] = true
	}
	loadedPrograms := make([]string, 0)
	for _, section := range ini.Sections() {
		isProgram := strings.HasPrefix(section.Name, "program:")
		if !c.initialized && !strings.HasPrefix(section.Name, "group:") && !isProgram && !strings.HasPrefix(section.Name, "eventlistener:") {
			entry := c.createEntry(section.Name, c.GetConfigFileDir())
			c.entries[section.Name] = entry
			entry.parse(section)
		}
		if isProgram {
			if _, ok := programMap[strings.TrimSpace(section.Name[len("program:"):])]; ok {
				loadedPrograms = append(loadedPrograms, c.parseProgram(section)...)
			}
		}
	}
	c.initialized = true
	return loadedPrograms
}

func (c *Config) getIncludeFiles(cfg *ini.Ini) []string {
	result := make([]string, 0)
	if includeSection, err := cfg.GetSection("include"); err == nil {
		key, err := includeSection.GetValue("files")
		// log.Info("include section key:", key)
		if err == nil {
			env := NewStringExpression("here", c.GetConfigFileDir())
			files := strings.Fields(key)
			// log.Info("influce files:", fmt.Sprintf("%#v", files))
			for _, f_raw := range files {
				dir := c.GetConfigFileDir()
				f, err := env.Eval(f_raw)
				if err != nil {
					continue
				}
				if filepath.IsAbs(f) {
					dir = filepath.Dir(f)
				}
				fileInfos, err := ioutil.ReadDir(dir)
				if err == nil {
					goPattern := toRegexp(filepath.Base(f))
					for _, fileInfo := range fileInfos {
						if matched, err := regexp.MatchString(goPattern, fileInfo.Name()); matched && err == nil {
							result = append(result, filepath.Join(dir, fileInfo.Name()))
						}
					}
				}

			}
		}
	}

	return result
}

func (c *Config) parse(cfg *ini.Ini) []string {
	c.parseGroup(cfg)
	loaded_programs := make([]string, 0)
	for _, section := range cfg.Sections() {
		loaded_programs = append(loaded_programs, c.parseProgram(section)...)
	}

	//parse non-group,non-program and non-eventlistener sections
	for _, section := range cfg.Sections() {
		if !strings.HasPrefix(section.Name, "group:") && !strings.HasPrefix(section.Name, "program:") && !strings.HasPrefix(section.Name, "eventlistener:") {
			entry := c.createEntry(section.Name, c.GetConfigFileDir())
			c.entries[section.Name] = entry
			entry.parse(section)
		}
	}
	return loaded_programs
}

func (c *Config) GetConfigFileDir() string {
	return filepath.Dir(c.configFile)
}

func (c *Config) GetConfigFile() string {
	return c.configFile
}

//convert supervisor file pattern to the go regrexp
func toRegexp(pattern string) string {
	tmp := strings.Split(pattern, ".")
	for i, t := range tmp {
		s := strings.Replace(t, "*", ".*", -1)
		tmp[i] = strings.Replace(s, "?", ".", -1)
	}
	return strings.Join(tmp, "\\.") + "$"
}

//get the unix_http_server section
func (c *Config) GetUnixHttpServer() (*ConfigEntry, bool) {
	c.lock.Lock()
	defer c.lock.Unlock()

	entry, ok := c.entries["unix_http_server"]

	return entry, ok
}

//get the supervisord section
func (c *Config) GetSupervisord() (*ConfigEntry, bool) {
	c.lock.Lock()
	defer c.lock.Unlock()

	entry, ok := c.entries["supervisord"]

	return entry, ok
}

// Get the inet_http_server configuration section
func (c *Config) GetInetHttpServer() (*ConfigEntry, bool) {
	c.lock.Lock()
	defer c.lock.Unlock()

	entry, ok := c.entries["inet_http_server"]

	return entry, ok
}

func (c *Config) GetSupervisorctl() (*ConfigEntry, bool) {
	c.lock.Lock()
	defer c.lock.Unlock()

	entry, ok := c.entries["supervisorctl"]

	return entry, ok
}

func (c *Config) GetEntries(filterFunc func(entry *ConfigEntry) bool) []*ConfigEntry {
	c.lock.Lock()
	defer c.lock.Unlock()

	result := make([]*ConfigEntry, 0)
	for _, entry := range c.entries {
		if filterFunc(entry) {
			result = append(result, entry)
		}
	}

	return result
}

func (c *Config) UpdateConfigEntry(name string) error {
	cfg := NewConfig(c.configFile)
	_, err := cfg.Load()
	if err != nil {
		return err
	}
	newEntries := cfg.GetEntries(func(entry *ConfigEntry) bool {
		if entry.Group == name {
			return true
		}
		return false
	})
	if len(newEntries) == 0 {
		return fmt.Errorf(fmt.Sprintf("failed to find program %s config entry", name))
	}

	c.lock.Lock()
	defer c.lock.Unlock()
	for _, v := range newEntries {
		c.entries[v.Name] = v
		var procName string
		if pos := strings.Index(v.Name, ":"); pos != -1 {
			procName = v.Name[pos+1:]
		} else {
			procName = name
		}
		c.ProgramGroup.Add(name, procName)
	}
	return nil
}

func (c *Config) GetGroups() []*ConfigEntry {
	return c.GetEntries(func(entry *ConfigEntry) bool {
		return entry.IsGroup()
	})
}

func (c *Config) GetPrograms() []*ConfigEntry {
	programs := c.GetEntries(func(entry *ConfigEntry) bool {
		return entry.IsProgram()
	})

	c.lock.Lock()
	defer c.lock.Unlock()

	return sortProgram(programs)
}

func (c *Config) GetEventListeners() []*ConfigEntry {
	eventListeners := c.GetEntries(func(entry *ConfigEntry) bool {
		return entry.IsEventListener()
	})

	return eventListeners
}

func (c *Config) GetProgramNames() []string {
	result := make([]string, 0)
	programs := c.GetPrograms()

	programs = sortProgram(programs)
	for _, entry := range programs {
		result = append(result, entry.GetProgramName())
	}

	return result
}

//return the proram configure entry or nil
// 20190320:  update时候，可能指定group，或者是program, 若program为numprocs类型的，则update　该program的group
//func (c *Config) GetProgram(name string) []*ConfigEntry {
//	c.lock.Lock()
//	ret := make([]*ConfigEntry, 0)
//	for _, entry := range c.entries {
//		fmt.Println("GetProgram", "1:", entry.Name, "2:", entry.Group, "3:", entry.GetProgramName())
//		if entry.IsProgram() && entry.Group == name {
//			if entry.IsMultiIns() {
//				c.lock.Unlock()
//				return c.GetGroupPrograms(entry.Group)
//			} else {
//				c.lock.Unlock()
//				return append(ret, entry)
//			}
//		}
//	}
//	c.lock.Unlock()
//	return ret
//}
func (c *Config) GetProgram(name string) *ConfigEntry {
	c.lock.Lock()
	defer c.lock.Unlock()

	for _, entry := range c.entries {
		if entry.IsProgram() && entry.GetProgramName() == name {
			return entry
		}
	}

	return nil
}

//获取Group下的所有programs
func (c *Config) GetGroupPrograms(name string) []*ConfigEntry {
	c.lock.Lock()
	ret := make([]*ConfigEntry, 0)
	defer c.lock.Unlock()
	for _, entry := range c.entries {
		if entry.IsProgram() && entry.Group == name {
			ret = append(ret, entry)
		}
	}
	return ret
}

//获取Group下的所有program的name
func (c *Config) GetGroupProgramNames(name string) []string {
	c.lock.Lock()
	ret := make([]string, 0)
	defer c.lock.Unlock()
	for _, entry := range c.entries {
		//fmt.Println("GetProgramsByGroupName", "1:", entry.Name, "2:", entry.Group, "3:", entry.GetProgramName())
		if entry.IsProgram() && entry.Group == name {
			ret = append(ret, entry.GetProgramName())
		}
	}
	return ret
}

// 20190320 使用通配符匹配, 如groupA:*, 可以得到groupA下面所有的program名称
func (c *Config) MatchProgramName(name string) []string {
	c.lock.Lock()
	ret := make([]string, 0)
	defer c.lock.Unlock()
	if pos := strings.Index(name, ":"); pos != -1 {
		groupName := name[0:pos]
		programName := name[pos+1:]
		for _, entry := range c.entries {
			if entry.IsProgram() && entry.Group == groupName && (programName == "*" || entry.GetProgramName() == programName) {
				ret = append(ret, entry.GetProgramName())
			}
		}
	} else {
		for _, entry := range c.entries {
			if entry.IsProgram() && entry.GetProgramName() == name {
				return append(ret, name)
			}
		}
	}

	return ret

}

// get value of key as bool
func (c *ConfigEntry) GetBool(key string, defValue bool) bool {
	value, ok := c.keyValues[key]
	if ok {
		b, err := strconv.ParseBool(value)
		if err == nil {
			return b
		}
	}

	return defValue
}

// check if has parameter
func (c *ConfigEntry) HasParameter(key string) bool {
	_, ok := c.keyValues[key]
	return ok
}

func toInt(s string, factor int, defValue int) int {
	i, err := strconv.Atoi(s)
	if err == nil {
		return i * factor
	}
	return defValue
}

// get the value of the key as int
func (c *ConfigEntry) GetInt(key string, defValue int) int {
	value, ok := c.keyValues[key]
	if ok {
		return toInt(value, 1, defValue)
	}

	return defValue
}

// GetEnv get the value of key as environment setting. An environment string example:
//  environment = A="env 1",B="this is a test"
func (c *ConfigEntry) GetEnv(key string) []string {
	value, ok := c.keyValues[key]
	env := make([]string, 0)

	if ok {
		start := 0
		n := len(value)
		var i int
		for {
			for i = start; i < n && value[i] != '='; {
				i++
			}
			key := value[start:i]
			start = i + 1
			if value[start] == '"' {
				for i = start + 1; i < n && value[i] != '"'; {
					i++
				}
				if i < n {
					env = append(env, fmt.Sprintf("%s=%s", strings.TrimSpace(key), strings.TrimSpace(value[start+1:i])))
				}
				if i+1 < n && value[i+1] == ',' {
					start = i + 2
				} else {
					break
				}
			} else {
				for i = start; i < n && value[i] != ','; {
					i++
				}
				if i < n {
					val := strings.TrimSpace(value[start:i])
					if val[0] == '"' {
						env = append(env, fmt.Sprintf("%s=%s", strings.TrimSpace(key), val[1:len(val)-1]))
					} else {
						env = append(env, fmt.Sprintf("%s=%s", strings.TrimSpace(key), val))
					}
					start = i + 1
				} else {
					val := strings.TrimSpace(value[start:])
					if val[0] == '"' {
						env = append(env, fmt.Sprintf("%s=%s", strings.TrimSpace(key), val[1:len(val)-1]))
					} else {
						env = append(env, fmt.Sprintf("%s=%s", strings.TrimSpace(key), val))
					}
					break
				}
			}
		}
	}

	result := make([]string, 0)
	for i := 0; i < len(env); i++ {
		tmp, err := NewStringExpression("program_name", c.GetProgramName(),
			"process_num", c.GetString("process_num", "0"),
			"group_name", c.GetGroupName(),
			"here", c.ConfigDir).Eval(env[i])
		if err == nil {
			result = append(result, tmp)
		}
	}
	return result
}

//get the value of key as string
func (c *ConfigEntry) GetString(key string, defValue string) string {
	s, ok := c.keyValues[key]

	if ok {
		env := NewStringExpression("here", c.ConfigDir)
		rep_s, err := env.Eval(s)
		if err == nil {
			return rep_s
		}
		log.WithFields(log.Fields{
			log.ErrorKey: err,
			"program":    c.GetProgramName(),
			"key":        key,
		}).Warn("Unable to parse expression")
	}
	return defValue
}

//get the value of key as string and attempt to parse it with StringExpression
func (c *ConfigEntry) GetStringExpression(key string, defValue string) string {
	s, ok := c.keyValues[key]
	if !ok || s == "" {
		return ""
	}

	host_name, err := os.Hostname()
	if err != nil {
		host_name = "Unknown"
	}
	result, err := NewStringExpression("program_name", c.GetProgramName(),
		"process_num", c.GetString("process_num", "0"),
		"group_name", c.GetGroupName(),
		"here", c.ConfigDir,
		"host_node_name", host_name).Eval(s)

	if err != nil {
		log.WithFields(log.Fields{
			log.ErrorKey: err,
			"program":    c.GetProgramName(),
			"key":        key,
		}).Warn("unable to parse expression")
		return s
	}

	return result
}

func (c *ConfigEntry) GetStringArray(key string, sep string) []string {
	s, ok := c.keyValues[key]

	if ok {
		return strings.Split(s, sep)
	}
	return make([]string, 0)
}

// get the value of key as the bytes setting.
//
//	logSize=1MB
//	logSize=1GB
//	logSize=1KB
//	logSize=1024
//
func (c *ConfigEntry) GetBytes(key string, defValue int) int {
	v, ok := c.keyValues[key]

	if ok {
		if len(v) > 2 {
			lastTwoBytes := v[len(v)-2:]
			if lastTwoBytes == "MB" {
				return toInt(v[:len(v)-2], 1024*1024, defValue)
			} else if lastTwoBytes == "GB" {
				return toInt(v[:len(v)-2], 1024*1024*1024, defValue)
			} else if lastTwoBytes == "KB" {
				return toInt(v[:len(v)-2], 1024, defValue)
			}
		}
		return toInt(v, 1, defValue)
	}
	return defValue
}

func (c *ConfigEntry) parse(section *ini.Section) {
	c.Name = section.Name
	for _, key := range section.Keys() {
		c.keyValues[key.Name()] = key.ValueWithDefault("")
	}
}

func (c *Config) parseGroup(cfg *ini.Ini) {
	//parse the group at first
	for _, section := range cfg.Sections() {
		if strings.HasPrefix(section.Name, "group:") {
			entry := c.createEntry(section.Name, c.GetConfigFileDir())
			entry.parse(section)
			groupName := entry.GetGroupName()
			programs := entry.GetPrograms()
			for _, program := range programs {
				c.ProgramGroup.Add(groupName, program)
			}
		}
	}
}

func (c *Config) isProgramOrEventListener(section *ini.Section) (bool, string) {
	//check if it is a program or event listener section
	is_program := strings.HasPrefix(section.Name, "program:")
	is_event_listener := strings.HasPrefix(section.Name, "eventlistener:")
	prefix := ""
	if is_program {
		prefix = "program:"
	} else if is_event_listener {
		prefix = "eventlistener:"
	}
	return is_program || is_event_listener, prefix
}

// parse the sections starts with "program:" prefix.
//
// Return all the parsed program names in the ini
func (c *Config) parseProgram(section *ini.Section) []string {
	loaded_programs := make([]string, 0)
	program_or_event_listener, prefix := c.isProgramOrEventListener(section)

	//if it is program or event listener
	if program_or_event_listener {
		//get the number of processes
		numProcs, err := section.GetInt("numprocs")
		programName := strings.TrimSpace(section.Name[len(prefix):])
		if err != nil {
			numProcs = 1
		}
		procName, err := section.GetValue("process_name")
		if numProcs > 1 {
			// 必须指定字符串表达式中变量的数据类型
			if err != nil || strings.Index(procName, "%(process_num)d") == -1 {
				log.WithFields(log.Fields{
					"numprocs":     numProcs,
					"process_name": procName,
				}).Error("no process_num in process name")
			}
		}
		originalProcName := programName
		if err == nil {
			originalProcName = procName
		}
		numprocsStart := section.GetIntWithDefault("numprocs_start", 0)
		for i := 1; i <= numProcs; i++ {
			envs := NewStringExpression("program_name", programName,
				"process_num", fmt.Sprintf("%d", i-1+numprocsStart),
				"group_name", c.ProgramGroup.GetGroup(programName, programName),
				"here", c.GetConfigFileDir())
			cmd, err := envs.Eval(section.GetValueWithDefault("command", ""))
			if err != nil {
				continue
			}
			section.Add("command", cmd)

			procName, err := envs.Eval(originalProcName)
			if err != nil {
				log.WithFields(log.Fields{
					log.ErrorKey: err,
					"expression": originalProcName,
				}).Warn("envs.Eval error")
				continue
			}

			section.Add("process_name", procName)
			section.Add("numprocs_start", fmt.Sprintf("%d", numprocsStart))
			section.Add("process_num", fmt.Sprintf("%d", i))
			entry := c.createEntry(procName, c.GetConfigFileDir())
			entry.parse(section)
			entry.Name = prefix + procName
			c.ProgramGroup.Add(programName, procName)
			entry.Group = programName
			loaded_programs = append(loaded_programs, procName)
		}
	}
	return loaded_programs

}

func (c *Config) String() string {
	c.lock.Lock()
	defer c.lock.Unlock()

	buf := bytes.NewBuffer(make([]byte, 0))
	fmt.Fprintf(buf, "configFile:%s\n", c.configFile)
	for k, v := range c.entries {
		fmt.Fprintf(buf, "[program:%s]\n", k)
		fmt.Fprintf(buf, "%s\n", v.String())
	}

	return buf.String()
}

func (c *Config) RemoveProgram(programName string) {
	c.lock.Lock()
	defer c.lock.Unlock()

	// delete(c.entries, fmt.Sprintf("program:%s", programName))
	delete(c.entries, programName)
	c.ProgramGroup.Remove(programName)
}

func (c *Config) RemoveGroup(groupName string) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.ProgramGroup.Remove(groupName)
}
