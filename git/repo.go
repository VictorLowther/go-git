package git

import (
	"os"
	"os/exec"
	"fmt"
	"path/filepath"
	"strings"
	"bytes"
	"errors"
	"regexp"
)

type Repo struct {
	gitDir, workDir string
}

var gitCmd string
var statusRE *regexp.Regexp
var statMap map[string]string

func init() {
	var err error
	if gitCmd,err = exec.LookPath("git"); err != nil {
		panic("Cannot find git command!")
	}
	statusRE = regexp.MustCompile("^([ MADRCU!?])([ MADRCU?!]) (.*)$")
	statMap = make(map[string]string)
	statMap[" "]="unmodified"
	statMap["M"]="modified"
	statMap["A"]="added"
	statMap["D"]="deleted"
	statMap["R"]="renamed"
	statMap["C"]="copied"
	statMap["U"]="unmerged"
	statMap["?"]="untracked"
	statMap["!"]="ignored"
}

func findRepo(path string) (found bool, gitdir, workdir string) {
	stat,err := os.Stat(path)
	if err != nil { panic("Could not stat "+path) }
	if !stat.IsDir() { panic(path+" is not a directory!") }
	if strings.HasSuffix(path,".git") {
		if stat,err = os.Stat(filepath.Join(path,"config")); err == nil {
			found = true
			gitdir = path
			workdir = ""
			return
		}
	}
	if stat,err = os.Stat(filepath.Join(path,".git","config")); err != nil {
		found = false
		return
	}
	found = true
	gitdir = filepath.Join(path,".git")
	workdir = path
	return
}

func Open(path string) (repo *Repo, err error) {
	if path == "" { path = "." }
	path,err  = filepath.Abs(path)
	basepath := path
	if err != nil { return }
	for {
		found, gitdir, workdir := findRepo(path)
		if found {
			repo = new(Repo)
			repo.gitDir = gitdir
			repo.workDir = workdir
			return
		}
		parent := filepath.Dir(path)
		if parent == path { break }
		path = parent
	}
	return nil,errors.New(fmt.Sprintf("Could not find a Git repository in %s or any of its parents!",basepath))
}

func Git(cmd string, args ...string) (res *exec.Cmd, stdout, stderr *bytes.Buffer) {
	cmd_args := make([]string,1)
	cmd_args[0] = cmd
	cmd_args = append(cmd_args,args...)
	res = exec.Command(gitCmd, cmd_args...)
	stdout, stderr = new(bytes.Buffer),new(bytes.Buffer)
	res.Stdout, res.Stderr = stdout, stderr
	return
}

func (r *Repo) Git(cmd string, args ...string) (res *exec.Cmd, out, err *bytes.Buffer) {
	var path string
	if r.workDir == "" {
		path = r.gitDir
	} else {
		path = r.workDir
	}
	res,out,err = Git(cmd, args...)
	res.Dir = path
	return
}

func Init(path string, args ...string) (res *Repo, err error) {
	cmd,_,stderr := Git("init", append(args,path)...)
	if err = cmd.Run(); err != nil {
		return nil,errors.New(stderr.String())
	}
	res, err = Open(path)
	return
}

func Clone(source, target string, args ...string) (res *Repo, err error) {
	cmd,_,stderr := Git("clone", append(args,source,target)...)
	if err = cmd.Run(); err != nil {
		return nil,errors.New(stderr.String())
	}
	res, err = Open(target)
	return
}

type statLine struct {
	indexStat, workStat, oldPath, newPath string
}

func (s *statLine) Print() string {
	var res string
	if s.indexStat == "R" {
		res = fmt.Sprintf("%s was renamed to %s\n",s.oldPath,s.newPath)
	}
	res = res + fmt.Sprintf("%s is %s in the index and %s in the working tree.",
		s.newPath,
		statMap[s.indexStat],
		statMap[s.workStat])
	return res
}

func (r *Repo) mapStatus() (res []*statLine) {
	var thisStat *statLine
	cmd, out, err := r.Git("status","--porcelain","-z")
	if cmd.Run() != nil {
		panic(err.String())
	}
	for {
		line,err := out.ReadString(0)
		if err != nil { break }
		parts := statusRE.FindStringSubmatch(line)
		if parts != nil {
			if thisStat != nil {
				res = append(res,thisStat)
			}
			thisStat = new(statLine)
			thisStat.indexStat = parts[1]
			thisStat.workStat = parts[2]
			thisStat.oldPath = parts[3]
			thisStat.newPath = parts[3]
		} else if thisStat != nil {
			thisStat.newPath = line
		} else {
			panic("Cannot happen!")
		}
	}
	if thisStat != nil {
		res = append(res,thisStat)
	}
	return
}

func (r *Repo) IsClean() (res bool, statLines []*statLine) {
	statLines = r.mapStatus()
	res = len(statLines) == 0
	return
}
