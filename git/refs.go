package git

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Ref is the basic way to point at an individual commit in Git.
type Ref struct {
	SHA, Path string
	r         *Repo
}

// RefSlice is a slice of pointers to Ref
type RefSlice []*Ref

// IsLocal tests to see if this is a local ref, a.k.a a branch.
func (r *Ref) IsLocal() bool {
	return strings.HasPrefix(r.Path, "refs/heads/")
}

// IsBranch is an alias for IsLocal.
func (r *Ref) IsBranch() bool {
	return r.IsLocal()
}

// IsRemote tests to see if this is a remote ref.
// Remote refs are not locally mutable except as a result of a fetch or
// push operation.
func (r *Ref) IsRemote() bool {
	return strings.HasPrefix(r.Path, "refs/remotes/")
}

// IsTag tests to see if this ref is a tag.
// Tags are immutable once created.
func (r *Ref) IsTag() bool {
	return strings.HasPrefix(r.Path, "refs/tags/")
}

// IsHead tess to see if this is the HEAD ref.
// HEAD is a special pointer that either points at another ref
// or points directly at a commit.
func (r *Ref) IsHead() bool {
	return r.Path == "HEAD"
}

// IsRaw tests to see if this is a raw ref.
// Raw refs refer directly to a SHA1, and have that is its path.
func (r *Ref) IsRaw() bool {
	return r.SHA == r.Path
}

// Name gets the name of the current ref.
// The name is the Path with refs/<whatever>/ stripped off it.
func (r *Ref) Name() (res string) {
	k := strings.SplitN(r.Path, "/", 3)
	return k[(len(k) - 1)]
}

// Branches gets all the local branches in the repository
func (r *Repo) Branches() (res RefSlice) {
	r.loadRefs()
	res = make(RefSlice, 0, 10)
	for _, ref := range r.refs {
		if ref.IsBranch() {
			res = append(res, ref)
		}
	}
	return res
}

// Branch creates a new branch starting at this ref.
func (r *Ref) Branch(name string) (ref *Ref, err error) {
	ref, err = r.r.makeRef("branch", name, r)
	return
}

// Tag creates a new tag at this ref.
func (r *Ref) Tag(name string) (ref *Ref, err error) {
	ref, err = r.r.makeRef("tag", name, r)
	return
}

// Remote returns the remote this ref tracks, if this is a remote ref.
// Otherwise, return an error.
func (r *Ref) Remote() (remote string, err error) {
	if !r.IsRemote() {
		return "", fmt.Errorf("%s is not a remote ref!", r.Path)
	}
	k := strings.SplitN(r.Path, "/", 4)
	return k[2], nil
}

// Delete deletes a ref, if it is deletable.
// Only branches and tags are deletable.
func (r *Ref) Delete() (err error) {
	var c string
	if r.IsRemote() {
		return errors.New("Cannot delete a remote ref!")
	} else if r.IsHead() {
		return errors.New("Cannot delete HEAD!")
	} else if r.IsTag() {
		c = "tag"
	} else if r.IsBranch() {
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

// Tracks returns the remote that this ref is configred to track, if any.
// If this ref does not track anything, then an error is returned.
func (r *Ref) Tracks() (remote string, err error) {
	if !r.IsLocal() {
		return "", fmt.Errorf("%s is not a branch, it does not track anything.", r.Path)
	}
	remote, remoteExists := r.r.Get("branch." + r.Name() + ".remote")
	if remoteExists {
		return remote, nil
	}
	return "", fmt.Errorf("%s does not track a remote")
}

// RemoteBranch returns the remote ref corresponding to this branch for a
// specific remote, if any.  It returns an error if there is no remote tracking branch for
// this ref.
func (r *Ref) RemoteBranch(remote string) (res *Ref, err error) {
	if !r.IsLocal() {
		return nil, fmt.Errorf("%s is not a branch, cannot find remote tracking branch.\n", r.Path)
	}
	remoteName := "refs/remotes/" + remote + "/" + r.Name()
	res, found := r.r.refs[remoteName]
	if !found {
		return nil, fmt.Errorf("%s has no remote branch at %s\n", r.Path, remote)
	}
	return res, nil
}

// TrackedRef returns  the remote ref that this ref tracks, if any.
func (r *Ref) TrackedRef() (res *Ref, err error) {
	remote, err := r.Tracks()
	if err != nil {
		return nil, err
	}
	res, err = r.RemoteBranch(remote)
	return res, err
}

// Reload the SHA for this ref.  This should be called if you suspect that
// the SHA has changed outside the context of this library.
func (r *Ref) Reload() (err error) {
	if r.IsHead() || r.IsRaw() {
		return nil
	}
	refPath := filepath.Join(r.r.GitDir, r.Path)
	sha, err := ioutil.ReadFile(refPath)
	if err != nil {
		return err
	}
	buf := bytes.NewBuffer(sha)
	r.SHA = strings.TrimSpace(buf.String())
	return nil
}

// Contains tests to see if other is reachable in the commit
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

// CurrentRef gets the ref that HEAD is pointing at.
// It handles cases where HEAD is pointing at a symbolic ref
// (a branch, tag, or remote ref), and where HEAD is pointing at a
// raw SHA1.
func (r *Repo) CurrentRef() (current *Ref, err error) {
	r.loadRefs()
	cmd, out, _ := r.Git("symbolic-ref", "HEAD")
	err = cmd.Run()
	if err == nil {
		// If we did not get an error, then out has the symbolic ref
		// of the branch we are on.
		refname := strings.TrimSpace(out.String())
		return r.refs[refname], nil
	}
	// Otherwise, we need to rev-parse HEAD to get what we are currently on.
	cmd, out, _ = r.Git("rev-parse", "HEAD")
	if err = cmd.Run(); err != nil {
		// Something Bad has happened.
		return nil, err
	}
	refname := strings.TrimSpace(out.String())
	// Make a raw ref our of this.
	return &Ref{Path: refname, SHA: refname, r: r}, nil
}

// Equals checks to see if this ref is the same as another ref.
// Refs are equal if they have the same path and the same SHA.
func (r *Ref) Equals(other *Ref) bool {
	return r.Path == other.Path && r.SHA == other.SHA && r.r == other.r
}

func mergeRebaseWrapper(op string, head, target *Ref, doer *exec.Cmd, undoer func() error) (err error) {
	// if r contains target, no need to do anything.
	ok, err := head.Contains(target)
	if err != nil {
		return err
	}
	if ok {
		return nil
	}
	if !head.IsLocal() {
		return fmt.Errorf("%s is not a branch, cannot %s it!\n", op, head.Path)
	}
	current, err := head.r.CurrentRef()
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

// RebaseOnto rebases a ref onto target.
// If the rebase succeeds, the function will return a nil error.
// If the rebase fails for any reason, the rebase will be aborted and the
// error output of the rebase will be return as an error.
func (r *Ref) RebaseOnto(target *Ref) (err error) {
	cmd, out, errOut := r.r.Git("rebase", "-q", target.SHA, r.Name())
	undoer := func() (err error) {
		// The rebase failed.  Unwind it, by force if needed.
		err = fmt.Errorf("%s\n%s\n", out.String(), errOut.String())
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
	return mergeRebaseWrapper("rebase", r, target, cmd, undoer)
}

// MergeWith merges this ref into the target.
// If the merge succeeds, this method will return nil.
// Otherwise the merge will be aborted and the error output of the merge will be returned as an error.
func (r *Ref) MergeWith(target *Ref) (err error) {
	cmd, out, errOut := r.r.Git("merge", "-q", target.SHA, r.Name())
	undoer := func() (err error) {
		// The merge failed.  Unwind it, by force if needed.
		err = fmt.Errorf("%s\n%s\n", out.String(), errOut.String())
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
	return mergeRebaseWrapper("merge", r, target, cmd, undoer)
}

// HasRef tests to see if a ref exists.
// It must be passed a full ref name beginning with "refs/"
func (r *Repo) HasRef(ref string) bool {
	r.loadRefs()
	_, err := r.refs[ref]
	return err
}

// HasRemoteRef checks to see if this branch has a matching branch at a given remote.
func (r *Ref) HasRemoteRef(remote string) (ok bool) {
	if !r.IsLocal() {
		return false
	}
	return r.r.HasRef("refs/remotes/" + remote + "/" + r.Name())
}

// TrackRemote forces a local ref (which should be a branch)
// to track an identically-named branch from that remote.
// An error will be returned if we cannot set the tracking information.
func (r *Ref) TrackRemote(remote string) (err error) {
	if !r.IsLocal() {
		return fmt.Errorf("%s is not a branch, we cannot track it.", r.Path)
	}
	section := "branch." + r.Name()
	branchRemote, branchRemoteExists := r.r.Get(section + ".remote")
	branchMerge, branchMergeExists := r.r.Get(section + ".merge")
	if branchRemoteExists &&
		branchMergeExists &&
		branchRemote == remote &&
		branchMerge == r.Path {
		// We already have the right config.  Nothing to do.
		return nil
	}
	if branchRemoteExists || branchMergeExists {
		r.r.maybeKillSection(section)
	}
	r.r.Set(section+".remote", remote)
	r.r.Set(section+".merge", r.Path)
	return nil
}

// Cat returns a Reader that will contain the contents of the
// file at fullpath in this ref, if it exists.
// Otherwise, it will return an error.
func (r *Ref) Cat(fullpath string) (out io.Reader, err error) {
	cmd, lsout, _ := r.r.Git("ls-tree", "--full-tree", fullpath)
	err = cmd.Run()
	if err != nil {
		return nil, err
	}
	if lsout.Len() == 0 {
		return nil, fmt.Errorf("%s is not present in %s", fullpath, r.r.Path())
	}
	parts := strings.Split(lsout.String(), " ")
	if parts[1] != "blob" {
		return nil, fmt.Errorf("%s is not a file in %s", fullpath, r.r.Path())
	}
	shaname := strings.Split(parts[2], "\t")
	cmd, out, _ = r.r.Git("cat-file", "blob", shaname[0])
	return out, cmd.Run()
}

// Ref returns a ref for the passed name, or an error.
// It can take:
//   raw names prefixed with "refs/",
//   branch names, tags, remote tracking branches,
//   and raw SHA1s.
func (r *Repo) Ref(name string) (res *Ref, err error) {
	r.loadRefs()
	for _, prefix := range []string{"", "refs/heads/", "refs/tags/", "refs/remotes/"} {
		refname := prefix + name
		if res = r.refs[refname]; res != nil {
			return res, nil
		}
	}
	// hmmm... it is not a symbolic ref.  See if it is a raw ref.
	cmd, _, _ := r.Git("rev-parse", "-q", "--verify", name)
	if cmd.Run() == nil {
		return &Ref{Path: name, SHA: name, r: r}, nil
	}
	return nil, fmt.Errorf("No ref for %s", name)
}

func (r *Repo) makeRef(reftype, name string, base interface{}) (ref *Ref, err error) {
	r.loadRefs()
	var path string
	switch reftype {
	case "branch":
		path = "refs/heads/" + name
	case "tag":
		path = "refs/tags/" + name
	default:
		return nil, fmt.Errorf("Cannot create a new %s", reftype)
	}
	if name == "HEAD" {
		return nil, errors.New("Cannot create a branch named HEAD.")
	} else if r.refs[path] != nil {
		return nil, errors.New(name + " already exists.")
	} else {
		switch i := base.(type) {
		case *Ref:
			cmd, _, _ := r.Git(reftype, name, i.Name())
			err = cmd.Run()
		case string:
			cmd, _, _ := r.Git(reftype, name, i)
			err = cmd.Run()
		default:
			return nil, fmt.Errorf("Unknown type %v for base", i)
		}
		if err != nil {
			return nil, err
		}
	}
	r.refs = nil
	r.loadRefs()
	return r.refs[path], nil
}

// Branch creates a branch with the given name based on whatever is passed for base.
// base can be either a Ref type of the name of a ref, in which case it must actually exist.
func (r *Repo) Branch(name string, base interface{}) (ref *Ref, err error) {
	ref, err = r.makeRef("branch", name, base)
	return
}

// Tag creates a tag with the given name based on whatever is passed for base.
// base can be either a Ref type of the name of a ref, in which case it must actually exist.
func (r *Repo) Tag(name string, base interface{}) (ref *Ref, err error) {
	ref, err = r.makeRef("tag", name, base)
	return
}

// Checkout checks this ref out.
func (r *Ref) Checkout() (err error) {
	var ref string
	if r.IsLocal() || r.IsTag() || r.IsRemote() {
		ref = r.Name()
	} else {
		ref = r.SHA
	}
	cmd, _, _ := r.r.Git("checkout", "-q", ref)
	err = cmd.Run()
	return
}

// Cherry will return an array of Refs that correspond to
// unique changes from base to r
func (r *Ref) Cherry(base *Ref) (refs []*Ref, err error) {
	cmd, out, _ := r.r.Git("cherry", base.SHA, r.SHA)
	if err = cmd.Run(); err != nil {
		return nil, err
	}
	refs = make([]*Ref, 0, 10)
	scanner := bufio.NewScanner(out)
	for scanner.Scan() {
		parts := strings.SplitN(strings.TrimSpace(scanner.Text()), " ", 2)
		if parts[0] == "+" {
			sha := strings.TrimSpace(parts[1])
			refs = append(refs, &Ref{Path: sha, SHA: sha, r: r.r})
		}
	}
	return refs, nil
}

// CherryLog will return an array of strings that contain the output from
// git log --cherry-pick --right-only --no-merges --oneline base.SHA...r.SHA
func (r *Ref) CherryLog(base *Ref) (log []string, err error) {
	cmd, out, _ := r.r.Git("log",
		"--cherry-pick",
		"--right-only",
		"--no-merges",
		"--oneline",
		base.SHA+"..."+r.SHA)
	if err = cmd.Run(); err != nil {
		return nil, err
	}
	log = make([]string, 0, 10)
	scanner := bufio.NewScanner(out)
	for scanner.Scan() {
		log = append(log, scanner.Text())
	}
	return log, nil
}

// Checkout checks out a ref by name.
func (r *Repo) Checkout(ref string) (err error) {
	cmd, _, _ := r.Git("checkout", "-q", ref)
	err = cmd.Run()
	return
}

func (r *Repo) loadRefs() {
	if r.refs != nil {
		return
	}
	res := make(RefMap)
	cmd, out, err := r.Git("show-ref")
	if cmd.Run() != nil {
		panic(err.String())
	}
	scanner := bufio.NewScanner(out)
	for scanner.Scan() {
		parts := strings.SplitN(strings.TrimSpace(scanner.Text()), " ", 2)
		ref := &Ref{parts[0], parts[1], r}
		res[ref.Path] = ref
	}
	r.refs = res
}

// Refs returns a slice of all the refs
func (r *Repo) Refs() (res RefSlice) {
	r.ReloadRefs()
	res = make(RefSlice, 0, 10)
	for _, v := range r.refs {
		res = append(res, v)
	}
	return res
}

// ReloadRefs will load all the refs lazily.
func (r *Repo) ReloadRefs() {
	r.refs = nil
}
