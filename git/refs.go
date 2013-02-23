package git

import (
	"bytes"
	"errors"
	"strings"
)

// Refs are the basic way to point at an individual commit in Git.
type Ref struct {
	SHA, Path string
	r         *Repo
}

// Test to see if this ref points a a local ref.
// Only local refs are mutable.
func (r *Ref) IsLocal() bool {
	return strings.HasPrefix(r.Path, "refs/heads/")
}

// Test to see if this ref points at a remote ref.
// Remote refs are not locally mutable except as a result of a fetch or
// push operation.
func (r *Ref) IsRemote() bool {
	return strings.HasPrefix(r.Path, "refs/remotes/")
}

// Test to see if this ref points at a tag.
// Tags are immutable once changed.
func (r *Ref) IsTag() bool {
	return strings.HasPrefix(r.Path, "refs/tags/")
}

// Test to see if this points at the HEAD ref.
// HEAD is a special pointer that either points at another ref
// or points directly at a commit.
func (r *Ref) IsHead() bool {
	return r.Path == "HEAD"
}

// Get the name of the current ref.
func (r *Ref) Name() (res string) {
	k := strings.SplitN(r.Path, "/", 3)
	return k[(len(k) - 1)]
}

// Delete a ref.
func (r *Ref) Delete() (err error) {
	var c string
	if r.IsRemote() {
		return errors.New("Cannot delete a remote ref!")
	} else if r.IsHead() {
		return errors.New("Cannot delete HEAD!")
	} else if r.IsTag() {
		c = "tag"
	} else if r.IsLocal() {
		c = "branch"
	} else {
		panic("Cannot happen!")
	}
	cmd, _, _ := r.r.Git(c, "-d", r.Name())
	err = cmd.Run()
	if err == nil {
		delete(r.r.Refs, r.Name())
	}
	return
}

// Create a branch
func (r *Repo) Branch(name string, base interface{}) (ref *Ref, err error) {
	if name == "HEAD" {
		return nil, errors.New("Cannot create a branch named HEAD.")
	} else if r.Refs[name] != nil {
		return nil, errors.New(name + " already exists.")
	} else {
		switch i := base.(type) {
		case Ref:
			cmd, _, _ := r.Git("branch", name, i.SHA)
			err = cmd.Run()
		case string:
			cmd, _, _ := r.Git("branch", name, i)
			err = cmd.Run()
		default:
			return nil, errors.New("Unknown type for base!")
		}
		if err != nil {
			return nil, err
		}
	}
	r.Refs = r.refs()
	return r.Refs[name], nil
}

func (r *Ref) Checkout() (err error) {
	var ref string
	if r.IsLocal() || r.IsTag() {
		ref = r.Name()
	} else {
		ref = r.SHA
	}
	cmd, _, _ := r.r.Git("checkout", "-q", ref)
	err = cmd.Run()
 	return
}

func (r *Repo) Checkout(ref string) (err error) {
	cmd, _, _ := r.Git("checkout", "-q", ref)
	err = cmd.Run()
	return
}

func (r *Repo) refs() (res map[string]*Ref) {
	res = make(map[string]*Ref)
	cmd, out, err := r.Git("show-ref")
	if cmd.Run() != nil {
		panic(err.String())
	}
	for {
		line, err := out.ReadBytes(10)
		if err != nil {
			break
		}
		parts := strings.SplitN(string(bytes.TrimSpace(line)), " ", 2)
		ref := &Ref{parts[0], parts[1], r}
		res[ref.Name()] = ref
	}
	return
}
