package git

import (
	"fmt"
	"strings"
	"errors"
)

func (r *Repo) Remotes() map[string]string {
	res := make(map[string]string)
	conf,err := r.Config()
	if err != nil {
		return res
	}
	for k,v := range conf.cfg {
		parts := strings.Split(k,".")
		if parts[0] == "remote" && parts[2] == "url" {
			res[parts[1]]=v
		}
	}
	return res
}

func (r *Repo) AddRemote(name,url string) (err error){
	remotes := r.Remotes()
	if remotes[name] != "" {
		msg := fmt.Sprintf("%s already has a remote named %s",r.Path(),name)
		return errors.New(msg)
	}
	cmd, _, _ := r.Git("remote","add",name,url)
	if err = cmd.Run(); err != nil {
		return err
	}
	return nil
}

func (r *Repo) ZapRemote(name string) (err error){
	remotes := r.Remotes()
	if remotes[name] == "" {
		msg := fmt.Sprintf("%s does not have a remote named %s",r.Path(),name)
		return errors.New(msg)
	}
	cmd, _, _ := r.Git("remote","rm",name)
	if err = cmd.Run(); err != nil {
		return err
	}
	return nil
}