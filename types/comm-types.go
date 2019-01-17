package types

type ProcessInfo struct {
	Name           string `xml:"name" json:"name"`
	Group          string `xml:"group" json:"group"`
	Description    string `xml:"description" json:"description"`
	Start          int    `xml:"start" json:"start"`
	Stop           int    `xml:"stop" json:"stop"`
	Now            int    `xml:"now" json:"now"`
	State          int    `xml:"state" json:"state"`
	Statename      string `xml:"statename" json:"statename"`
	Spawnerr       string `xml:"spawnerr" json:"spawnerr"`
	Exitstatus     int    `xml:"exitstatus" json:"exitstatus"`
	Logfile        string `xml:"logfile" json:"logfile"`
	Stdout_logfile string `xml:"stdout_logfile" json:"stdout_logfile"`
	Stderr_logfile string `xml:"stderr_logfile" json:"stderr_logfile"`
	Pid            int    `xml:"pid" json:"pid"`
}

type ReloadConfigResult struct {
	AddedGroup   []string
	ChangedGroup []string
	RemovedGroup []string
}

type UpdateResult = ReloadConfigResult

type ProcessSignal struct {
	Name   string
	Signal string
}

type BooleanReply struct {
	Success bool
}
