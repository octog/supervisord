package process

import (
	"os"
	"testing"

	jerrors "github.com/juju/errors"
)

func TestProcessInfoMapStore(t *testing.T) {
	file := "./list.yml"
	psInfoMap := ProcessInfoMap{
		Version: 7,
		InfoMap: map[string]ProcessInfo{
			"hello": ProcessInfo{
				StartTime: 123,
				PID:       123,
				Program:   "./hello",
			},
			"world": ProcessInfo{
				StartTime: 890,
				PID:       1345,
				Program:   "./world",
			},
		},
	}

	err := psInfoMap.Store(file)
	if err != nil {
		t.Errorf("failed to store process list %#v", psInfoMap)
	}

	m := NewProcessInfoMap()
	err = m.Load(file)
	if err != nil {
		t.Errorf("failed to load process list file %s", file)
	}
	if err = m.VerboseEqual(psInfoMap); err != nil {
		t.Errorf("infoMap %#v VerboseEqual(psInfoMap:%#v) = error:%s",
			m, psInfoMap, jerrors.ErrorStack(err))
	}
	os.Remove(file)
}
