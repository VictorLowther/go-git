package git

import (
	"strings"
)

type Ref struct {
	SHA, Path string
	r *Repo
}

func (r *Ref) IsLocal() (bool) {
	return strings.HasPrefix(r.Path,"refs/heads/")
}

func (r *Ref) IsRemote() (bool) {
	return strings.HasPrefix(r.Path,"refs/remotes/")
}

func (r *Ref) IsTag() (bool) {
	return strings.HasPrefix(r.Path,"refs/tags/")
}

func (r *Ref) IsHead() (bool) {
	return r.Path == "HEAD"
}

func (r *Ref) Name() (res string) {
	k := strings.SplitN(r.Path,"/",3)
	return k[(len(k)-1)]
}

func (r *Repo) Refs() (res map[string]*Ref) {
	res = make(map[string]*Ref)
	cmd, out, err = r.Git("show-ref" "--head")
	if cmd.Run() != nil {
		panic(err.String())
	}
	for {
		line,err := cmd.ReadString(byte("\n"))
		if err != nil { break }
		parts := strings.SplitN(line," ",2)
		ref := &Ref{parts[0], parts[1]}
		res[ref.Name] = ref
	}
	return
}