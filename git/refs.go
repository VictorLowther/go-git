package git

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"
	"os"
	"os/exec"
)

// Refs are the basic way to point at an individual commit in Git.
type Ref struct {
	SHA, Path string
	r         *Repo
}

type RefSlice []*Ref

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

// Test to see if this is a raw ref.  Raw refs refer directly to a
// SHA1, and have that is its path.
func (r *Ref) IsRaw() bool {
	return r.SHA == r.Path
}

// Get the name of the current ref.
func (r *Ref) Name() (res string) {
	k := strings.SplitN(r.Path, "/", 3)
	return k[(len(k) - 1)]
}

func (r *Repo) Branches() (res RefSlice) {
	r.load_refs()
	res = make(RefSlice,0,10)
	for _,ref := range r.refs {
		if ref.IsLocal() {
			res = append(res,ref)
		}
	}
	return
	
}

// If this is a remote ref, return the remote that the ref tracks.
// Otherwise, return an error.
func (r *Ref) Remote() (remote string, err error) {
	if !r.IsRemote() {
		return "", fmt.Errorf("%s is not a remote ref!", r.Path)
	}
	k := strings.SplitN(r.Path, "/", 4)
	return k[2], nil
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
		delete(r.r.refs, r.Name())
	}
	return
}

// Return the remote that this ref is configred to track, if any.
func (r *Ref) Tracks() (remote string, err error) {
	if !r.IsLocal() {
		return "", fmt.Errorf("%s is not a branch, it does not track anything.", r.Path)
	}
	remote, remote_exists := r.r.Get("branch." + r.Name() + ".remote")
	if remote_exists {
		return remote, nil
	}
	return "", fmt.Errorf("%s does not track a remote")
}

// Return the remote ref corresponding to this branch for a
// specific remote, if any.
func (r *Ref) RemoteBranch(remote string) (res *Ref, err error) {
	if !r.IsLocal() {
		return nil, fmt.Errorf("%s is not a branch, cannot find remote tracking branch.\n", r.Path)
	}
	res, found := r.r.refs["refs/remotes/"+remote+"/"+r.Name()]
	if !found {
		return nil, fmt.Errorf("%s has no remote branch at %s\n", r.Path, remote)
	}
	return res, nil
}

// Return the remote ref that this ref tracks, if any.
func (r *Ref) TrackedRef() (res *Ref, err error) {
	remote, err := r.Tracks()
	if err != nil {
		return nil,err
	}
	res, err = r.RemoteBranch(remote)
	return res,err
}

// Reload the SHA for this ref.
func (r *Ref) Reload() (err error) {
	if r.IsHead() || r.IsRaw() {
		return nil
	}
	ref_path := filepath.Join(r.r.GitDir, r.Path)
	sha, err := ioutil.ReadFile(ref_path)
	if err != nil {
		return err
	}
	buf := bytes.NewBuffer(sha)
	r.SHA = strings.TrimSpace(buf.String())
	return nil
}

// Test to see if other is reachable in the commit
// history leading up to this ref.
func (r *Ref) Contains(other *Ref) (bool, error) {
	// A ref ls always reachable from itself.
	if r.SHA == other.SHA {
		return true, nil
	}
	// If other's revision graph has revs that are not in our revision
	// graph, then we do not contain other.
	cmd, out, _ := r.r.Git("rev-list", other.SHA, fmt.Sprintf("^%s", r.SHA))
	if err := cmd.Run(); err != nil {
		return false, err
	}
	// If there is no output, then all of other's revs are members of
	// our revision graph, and we contain other.
	return (out.Len() == 0), nil
}

func merge_rebase_wrapper(op string, head, target *Ref, doer *exec.Cmd, undoer func() (error)) (err error) {
	// if r contains target, no need to do anything.
	ok,err := head.Contains(target)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}
	if !head.IsLocal() {
		return fmt.Errorf("%s is not a branch, cannot %s it!\n",op, head.Path)
	}
	current,err := head.r.CurrentRef()
	if err != nil {
		return err
	}
	if !head.Equals(current) {
		if err = head.Checkout(); err != nil {
			return err
		}
		defer current.Checkout()
	}
	if doer.Run() == nil {
		head.Reload()
		return nil
	}
	return undoer()
}

// Rebase a ref onto target.  If the rebase succeeds, the function will
// return a nil error. If the rebase fails for any reason, it will be
// aborted and the error output of the rebase will be return as an error.
func (r *Ref) RebaseOnto(target *Ref) (err error) {
	cmd, out, err_out := r.r.Git("rebase", "-q", target.SHA, r.Name())
	undoer := func () (err error) {
		// The rebase failed.  Unwind it, by force if needed.
		err = fmt.Errorf("%s\n%s\n", out.String(), err_out.String())
		cmd, _, _ := r.r.Git("rebase", "--abort")
		if cmd.Run() == nil {
			// We unwound successfully.
			return err
		}
		// We could not abort the rebase.
		// Force it.
		cmd, _, _ = r.r.Git("branch", "-f", r.Name(), r.SHA)
		cmd.Run()
		os.Remove(filepath.Join(r.r.GitDir, ".rebase-apply"))
		return err
	}
	return merge_rebase_wrapper("rebase",r,target,cmd,undoer)
}

func (r *Ref) MergeWith(target *Ref) (err error) {
	cmd, out, err_out := r.r.Git("merge", "-q", target.SHA, r.Name())
	undoer := func () (err error) {
		// The merge failed.  Unwind it, by force if needed.
		err = fmt.Errorf("%s\n%s\n", out.String(), err_out.String())
		cmd, _, _ := r.r.Git("merge", "--abort")
		if cmd.Run() == nil {
			// We unwound successfully.
			return err
		}
		// We could not abort the merge.
		// Force it.
		cmd, _, _ = r.r.Git("branch", "-f", r.Name(), r.SHA)
		cmd.Run()
		return err
	}
	return merge_rebase_wrapper("merge", r, target, cmd, undoer)
}

// Test to see if a ref exists.
func (r *Repo) HasRef(ref string) bool {
	r.load_refs()
	_, err := r.refs[ref]
	return err
}

func (r *Ref) HasRemoteRef(remote string) (ok bool) {
	if !r.IsLocal() {
		return false
	}
	return r.r.HasRef("refs/remotes/" + remote + "/" + r.Name())
}

// Force a local ref (which should be a branch) to track an identically-named branch from that remote.
func (r *Ref) TrackRemote(remote string) (err error) {
	if !r.IsLocal() {
		return fmt.Errorf("%s is not a branch, we cannot track it.", r.Path)
	}
	section := "branch." + r.Name()
	branch_remote, branch_remote_exists := r.r.Get(section + ".remote")
	branch_merge, branch_merge_exists := r.r.Get(section + ".merge")
	if branch_remote_exists &&
		branch_merge_exists &&
		branch_remote == remote &&
		branch_merge == r.Path {
		// We already have the right config.  Nothing to do.
		return nil
	}
	if branch_remote_exists || branch_merge_exists {
		r.r.maybeKillSection(section)
	}
	r.r.Set(section+".remote", remote)
	r.r.Set(section+".merge", r.Path)
	return nil
}

// Given a string that should represent a ref, return that ref or an error.
func (r *Repo) Ref(ref string) (res *Ref, err error) {
	r.load_refs()
	for _, prefix := range []string{"", "refs/heads/", "refs/tags", "refs/remotes"} {
		refname := prefix + ref
		if res = r.refs[refname]; res != nil {
			return res, nil
		}
	}
	// hmmm... it is not a symbolic ref.  See if it is a raw ref.
	cmd, _, _ := r.Git("rev-parse", "-q", "--verify", ref)
	if cmd.Run() != nil {
		return &Ref{Path: ref, SHA: ref, r: r}, nil
	}
	return nil, fmt.Errorf("No ref for %s", ref)
}

func (r *Repo) make_ref(reftype string, name string, base interface{}) (ref *Ref, err error) {
	r.load_refs()
	if name == "HEAD" {
		return nil, errors.New("Cannot create a branch named HEAD.")
	} else if r.refs[name] != nil {
		return nil, errors.New(name + " already exists.")
	} else {
		if !(reftype == "branch" || reftype == "tag") {
			return nil, errors.New("Unknown ref type!")
		}
		switch i := base.(type) {
		case *Ref:
			cmd, _, _ := r.Git(reftype, name, i.Name())
			err = cmd.Run()
		case string:
			cmd, _, _ := r.Git(reftype, name, i)
			err = cmd.Run()
		default:
			return nil, errors.New("Unknown type for base!")
		}
		if err != nil {
			return nil, err
		}
	}
	r.refs = nil
	r.load_refs()
	return r.refs[name], nil
}

// Create a branch
func (r *Repo) Branch(name string, base interface{}) (ref *Ref, err error) {
	ref, err = r.make_ref("branch", name, base)
	return
}

// Create a tag
func (r *Repo) Tag(name string, base interface{}) (ref *Ref, err error) {
	ref, err = r.make_ref("tag", name, base)
	return
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

func (r *Repo) CurrentRef() (current *Ref, err error) {
	cmd,out,_ := r.Git("symbolic-ref","HEAD")
	err = cmd.Run()
	if err == nil {
		// If we did not get an error, then out has the symbolic ref
		// of the branch we are on.
		refname := strings.TrimSpace(out.String())
		return r.refs[refname],nil
	}
	// Otherwise, we need to rev-parse HEAD to get what we are currently on.
	cmd,out,_ = r.Git("rev-parse","HEAD")
	err = cmd.Run()
	if err != nil {
		// Something Bad has happened.
		return nil,err
	}
	refname := strings.TrimSpace(out.String())
	// Make a raw ref our of this.
	return &Ref{Path: refname, SHA: refname, r: r},nil
}

func (r *Ref) Equals(other *Ref) bool {
	return r.Path == other.Path && r.SHA == other.SHA && r.r == other.r
}

func (r *Repo) load_refs() {
	if r.refs != nil {
		return
	}
	res := make(map[string]*Ref)
	cmd, out, err := r.Git("show-ref")
	if cmd.Run() != nil {
		panic(err.String())
	}
	scanner := bufio.NewScanner(out)
	for scanner.Scan() {
		parts := strings.SplitN(strings.TrimSpace(scanner.Text()), " ", 2)
		ref := &Ref{parts[0], parts[1], r}
		res[ref.Name()] = ref
	}
	r.refs = res
}

// Reload all the refs lazily.
func (r *Repo) ReloadRefs() {
	r.refs = nil
}
