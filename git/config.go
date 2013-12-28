package git

import (
	"strings"
	"log"
)

func (r *Repo) readConfig() {
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

// ReloadConfig will force the config for this git repo to be lazily reloaded.
func (r *Repo) ReloadConfig() {
	r.cfg = nil
}

// Get a specific config value.
func (r *Repo) Get(key string) (val string, found bool) {
	r.readConfig()
	val,found = r.cfg[key]
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

// Unset a config variable.
func (r *Repo) Unset(key string) {
	r.readConfig()
	if _,e := r.Get(key); e == true {
		cmd, _, err := r.Git("config", "--unset-all",key)
		delete(r.cfg,key)
		if cmd.Run() == nil {
			parts := strings.Split(key,".")
			switch len(parts) {
			case 0:  panic("Cannot happen!")
			case 1:  r.maybeKillSection(key)
			case 2:  r.maybeKillSection(parts[0])
			default: r.maybeKillSection(strings.Join(parts[0:len(parts)-1],"."))
			}
		} else {
			panic(err.String())
		}
	}
}

// Set a config variable.
func (r *Repo) Set(key,val string) {
	r.Unset(key)
	cmd, _, _ := r.Git("config","--add", key,val)
	if err := cmd.Run(); err != nil {
		panic("Cannot happen!")
	}
	r.cfg[key]=val
}

// Find all config variables with a specific prefix.
func (r *Repo) Find(prefix string) (res map[string]string) {
	r.readConfig()
	res = make(map[string]string)
	for k,v := range r.cfg {
		if strings.HasPrefix(k,prefix) {
			res[k]=v
		}
	}
	return res
}
