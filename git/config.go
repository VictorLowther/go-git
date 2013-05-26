package git

import (
	"strings"
	"log"
)

func (r *Repo) read_config() {
	if r.cfg != nil {
		return
	}
	cmd,stdout,stderr := r.Git("config", "-l", "-z")
	if err := cmd.Run(); err != nil {
		log.Panic(stderr.String())
		return
	}
	r.cfg = make(ConfigMap)
	for _,line := range strings.Split(stdout.String(),"\x00") {
		parts := strings.SplitN(line,"\n",2)
		if len(parts) != 2 {
			continue
		}
		k := strings.TrimSpace(parts[0])
		v := strings.TrimSpace(parts[1])
		if k == "" {
			continue
		}
		r.cfg[k]=v
	}
	return
}

// Lazily reload our config.
func (r *Repo) ReloadConfig() {
	r.cfg = nil
}

func (r *Repo) Get(k string) (v string, f bool) {
	r.read_config()
	v,f = r.cfg[k]
	return
}

func (r *Repo) maybeKillSection(prefix string) {
	if len(r.Find(prefix)) == 0 {
		cmd, _, err := r.Git("config","--remove-section", prefix)
		if cmd.Run() != nil {
			log.Panic(err.String())
		}
	}
}

func (r *Repo) Unset(k string) {
	r.read_config()
	if _,e := r.Get(k); e == true {
		cmd, _, err := r.Git("config", "--unset-all",k)
		delete(r.cfg,k)
		if cmd.Run() == nil {
			parts := strings.Split(k,".")
			switch len(parts) {
			case 0:  panic("Cannot happen!")
			case 1:  r.maybeKillSection(k)
			case 2:  r.maybeKillSection(parts[0])
			default: r.maybeKillSection(strings.Join(parts[0:len(parts)-1],"."))
			}
		} else {
			panic(err.String())
		}
	}
}

func (r *Repo) Set(k,v string) {
	r.Unset(k)
	cmd, _, _ := r.Git("config","--add", k,v)
	if err := cmd.Run(); err != nil {
		panic("Cannot happen!")
	}
	r.cfg[k]=v
}

func (r *Repo) Find(prefix string) (res map[string]string) {
	r.read_config()
	res = make(map[string]string)
	for k,v := range r.cfg {
		if strings.HasPrefix(k,prefix) {
			res[k]=v
		}
	}
	return res
}
