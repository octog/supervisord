package process

import (
	"os"
	"os/user"
	"path"
	"path/filepath"
	"strings"

	"github.com/AlexStocks/supervisord/config"
	log "github.com/sirupsen/logrus"
)

func path_split(path string) []string {
	r := make([]string, 0)
	cur_path := path
	for {
		dir, file := filepath.Split(cur_path)
		if len(file) > 0 {
			r = append(r, file)
		}
		if len(dir) <= 0 {
			break
		}
		cur_path = dir[0 : len(dir)-1]
	}
	for i, j := 0, len(r)-1; i < j; i, j = i+1, j-1 {
		r[i], r[j] = r[j], r[i]
	}
	return r
}
func Path_expand(path string) (string, error) {
	pathList := path_split(path)

	if len(pathList) > 0 && len(pathList[0]) > 0 && pathList[0][0] == '~' {
		var usr *user.User
		var err error

		if pathList[0] == "~" {
			usr, err = user.Current()
		} else {
			usr, err = user.Lookup(pathList[0][1:])
		}

		if err != nil {
			return "", err
		}
		pathList[0] = usr.HomeDir
		return filepath.Join(pathList...), nil
	}
	return path, nil
}

func getStdoutLogfile(config *config.ConfigEntry) string {
	if nil == config {
		return ""
	}

	file_name := config.GetStringExpression("stdout_logfile", "/dev/null")
	if file_name == "NONE" {
		return "/dev/null"
	}
	expand_file, err := Path_expand(file_name)
	if err != nil {
		// return file_name
		expand_file = file_name
	}

	base_path := path.Dir(expand_file)
	if err = os.MkdirAll(base_path, 0766); err != nil &&
		!strings.Contains(err.Error(), "file exists") {
		log.Errorf("fail to create dir %s, err %s", base_path, err)
	}
	return expand_file
}

func getStderrLogfile(config *config.ConfigEntry) string {
	if nil == config {
		return ""
	}

	file_name := config.GetStringExpression("stderr_logfile", "/dev/null")
	if file_name == "NONE" {
		return "/dev/null"
	}
	expand_file, err := Path_expand(file_name)
	if err != nil {
		// return file_name
		expand_file = file_name
	}

	base_path := path.Dir(expand_file)
	if err = os.MkdirAll(base_path, 0766); err != nil &&
		!strings.Contains(err.Error(), "file exists") {
		log.Errorf("fail to create dir %s, err %s", base_path, err)
	}

	return expand_file
}
