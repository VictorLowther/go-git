package git

import (
	"errors"
	"strings"
)

type Config struct {
	repo *Repo
	cfg map[string]string
}

func (r *Repo) Config() (res *Config, err error) {
	cmd,stdout,stderr := r.Git("config", "-l", "-z")
	if err = cmd.Run(); err != nil {
		return nil,errors.New(stderr.String())
	}
	res = new(Config)
	res.cfg = make(map[string]string)
	res.repo = r
	for {
		line,err := stdout.ReadString(0)
		if err != nil { break }
		parts := strings.SplitN(line,"\n",2)
		res.cfg[parts[0]]=parts[1]
	}
	return res,nil
}

func (c *Config) Get(k string) (v string, f bool) {
	v,f = c.cfg[k]
	return
}

func (c *Config) Unset(k string) {
	if _,e := c.Get(k); e == true {
		delete(c.cfg,k)
		cmd, _, _ := c.repo.Git("config", "--unset-all",k)
		_ = cmd.Run()
	}
}

func (c *Config) Set(k,v string) {
	c.Unset(k)
	cmd, _, _ := c.repo.Git("config","--add", k,v)
	if err := cmd.Run(); err != nil {
		panic("Cannot happen!")
	}
	c.cfg[k]=v
}

func (c *Config) Find(prefix string) (res map[string]string) {
	res = make(map[string]string)
	for k,v := range c.cfg {
		if strings.HasPrefix(k,prefix) {
			res[k]=v
		}
	}
	return res
}
