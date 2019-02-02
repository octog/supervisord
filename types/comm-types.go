package types

import (
	"fmt"
	"strings"
)

type ProcessInfo struct {
	Name        string `xml:"name" json:"name"`
	Group       string `xml:"group" json:"group"`
	Description string `xml:"description" json:"description"`
	Start       int    `xml:"start" json:"start"`
	Stop        int    `xml:"stop" json:"stop"`
	Now         int    `xml:"now" json:"now"`
	// State          int    `xml:"state" json:"state"`
	State          int    `xml:"status" json:"state"`
	Statename      string `xml:"statename" json:"statename"`
	Spawnerr       string `xml:"spawnerr" json:"spawnerr"`
	Exitstatus     int    `xml:"exitstatus" json:"exitstatus"`
	Logfile        string `xml:"logfile" json:"logfile"`
	Stdout_logfile string `xml:"stdout_logfile" json:"stdout_logfile"`
	Stderr_logfile string `xml:"stderr_logfile" json:"stderr_logfile"`
	Pid            int    `xml:"pid" json:"pid"`
}

func (pi ProcessInfo) GetFullName() string {
	if len(pi.Group) > 0 {
		return fmt.Sprintf("%s:%s", pi.Group, pi.Name)
	} else {
		return pi.Name
	}
}

type ReloadConfigResult struct {
	AddedGroup   []string
	ChangedGroup []string
	RemovedGroup []string
}

// MarshalXML generate XML output for PrecsontructedInfo
func (r ReloadConfigResult) MarshalXML() string {
	var res string
	res += "<array><data>"
	for i := 0; i < len(r.AddedGroup); i++ {
		value := r.AddedGroup[i]
		value = strings.Replace(value, "&", "&amp;", -1)
		value = strings.Replace(value, "\"", "&quot;", -1)
		value = strings.Replace(value, "<", "&lt;", -1)
		value = strings.Replace(value, ">", "&gt;", -1)
		res += fmt.Sprintf("<string>%s</string>", value)
	}
	res += "</data></array>"

	res += "<array><data>"
	for i := 0; i < len(r.ChangedGroup); i++ {
		value := r.ChangedGroup[i]
		value = strings.Replace(value, "&", "&amp;", -1)
		value = strings.Replace(value, "\"", "&quot;", -1)
		value = strings.Replace(value, "<", "&lt;", -1)
		value = strings.Replace(value, ">", "&gt;", -1)
		res += fmt.Sprintf("<string>%s</string>", value)
	}
	res += "</data></array>"

	res += "<array><data>"
	for i := 0; i < len(r.RemovedGroup); i++ {
		value := r.RemovedGroup[i]
		value = strings.Replace(value, "&", "&amp;", -1)
		value = strings.Replace(value, "\"", "&quot;", -1)
		value = strings.Replace(value, "<", "&lt;", -1)
		value = strings.Replace(value, ">", "&gt;", -1)
		res += fmt.Sprintf("<string>%s</string>", value)
	}
	res += "</data></array>"

	return res
}

type ReloadConfigResults struct {
	Results [][]ReloadConfigResult
}

type UpdateResult struct {
	AddedGroup   []string
	ChangedGroup []string
	RemovedGroup []string
}

type ProcessSignal struct {
	Name   string
	Signal string
}

type BooleanReply struct {
	Success bool
}

type ApiMethod struct {
	MethodName string   `xml:"methodName" json:"methodName"`
	Params     []string `xml:"params" json:"params"`
}

type MulticallArgs struct {
	Methods []ApiMethod
}

type ErrorResult struct {
	FaultCode   int    `xml:"faultCode"`
	FaultString string `xml:"faultString"`
}

type MulticallResults struct {
	Results []interface{}
}
