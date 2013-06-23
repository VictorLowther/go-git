package git

import (
	"errors"
	"fmt"
	"strings"
)

// Type to hold our map of remote names -> remote specifiers.
type RemoteMap map[string]string

// Get our list of remotes by parsing the git config.
func (r *Repo) Remotes() RemoteMap {
	res := make(RemoteMap)
	r.read_config()
	for k, v := range r.cfg {
		parts := strings.Split(k, ".")
		if parts[0] == "remote" && parts[2] == "url" {
			res[parts[1]] = v
		}
	}
	return res
}

// Test to see if this repository has a specific remote by url.
func (r *Repo) HasRemote(remote string) (ok bool) {
	_, ok = r.Get("remote." + remote + ".url")
	return
}

// Add a new remote.
func (r *Repo) AddRemote(name, url string) (err error) {
	remotes := r.Remotes()
	if remotes[name] != "" {
		msg := fmt.Sprintf("%s already has a remote named %s", r.Path(), name)
		return errors.New(msg)
	}
	cmd, _, _ := r.Git("remote", "add", name, url)
	if err = cmd.Run(); err != nil {
		return err
	}
	r.cfg = nil
	return nil
}

// Rename a remote, while preserving any trackin information it may have.
func (r *Repo) RenameRemote(old, nuevo string) (err error) {
	if !r.HasRemote(old) {
		return fmt.Errorf("%s does not exist, cannot rename it!\n", old)
	}
	if r.HasRemote(nuevo) {
		return fmt.Errorf("%s already exists!\n", nuevo)
	}
	cmd, _, _ := r.Git("remote", "rename", old, nuevo)
	if err = cmd.Run(); err != nil {
		return err
	}
	r.cfg = nil
	return nil
}

// Destroy an old remote mapping
func (r *Repo) ZapRemote(name string) (err error) {
	remotes := r.Remotes()
	if remotes[name] == "" {
		msg := fmt.Sprintf("%s does not have a remote named %s", r.Path(), name)
		return errors.New(msg)
	}
	cmd, _, _ := r.Git("remote", "rm", name)
	if err = cmd.Run(); err != nil {
		return err
	}
	r.cfg = nil
	return nil
}

// Set a new URL for a remote.
func (r *Repo) SetRemoteURL(name, url string) (err error) {
	remotes := r.Remotes()
	if remotes[name] == "" {
		return fmt.Errorf("%s does not have a remote named %s\n", r.Path(), name)
	}
	cmd, _, _ := r.Git("remote", "set-url", name, url)
	if err = cmd.Run(); err != nil {
		return err
	}
	r.cfg = nil
	return nil
}

// Probe a URL to see if there is a git repository there.
func ProbeURL(url string) (found bool, err error) {
	cmd, _, _ := Git("ls-remote", url, "refs/heads/master")
	err = cmd.Run()
	if err != nil {
		return false, err
	}
	return true, nil
}

// Prune remotes that do not point at an actual git repository.
func (r *Repo) PruneRemotes() (res map[string]bool) {
	res = make(map[string]bool)
	for remote, url := range r.Remotes() {
		found, _ := r.ProbeURL(url)
		if !found && r.ZapRemote(remote) == nil {
			res[remote] = true
		} else {
			res[remote] = false
		}
	}
	return res
}

// Helper type for holding the status of a fetch from a single remote.
type FetchStatus struct {
	Ok     bool
	Remote string
}

// Fetch updates from a single remote.
func (r *Repo) fetchOne(remote string, ok chan FetchStatus) {
	cmd, _, _ := r.Git("fetch", "-q", "-t", remote)
	err := cmd.Run()
	ok <- FetchStatus{
		Ok:     (err == nil),
		Remote: remote,
	}
	return
}

// Helper to enable empty slice -> all remotes the repo knows about.
func (r *Repo) allRemotes(remotes []string) []string {
	if len(remotes) > 0 {
		return remotes
	}
	for k, _ := range r.Remotes() {
		remotes = append(remotes, k)
	}
	return remotes
}

// Fetch updates from the passed remotes.
// This expects to be called as a goroutine.
func (r *Repo) AsyncFetch(remotes []string, ok chan FetchStatus) {
	remotes = r.allRemotes(remotes)
	for _, v := range remotes {
		go r.fetchOne(v, ok)
	}
}

// Type that holds our map of remote names -> whether we fetched all updates from the remote.
type FetchMap map[string]bool

// Fetch all updates from our remotes in parallel.
func (r *Repo) Fetch(remotes []string) (res bool, items FetchMap) {
	ok := make(chan FetchStatus)
	items = make(FetchMap)
	res = true
	remotes = r.allRemotes(remotes)
	go r.AsyncFetch(remotes, ok)
	for {
		token := <-ok
		items[token.Remote] = token.Ok
		res = res && token.Ok
		if len(items) == len(remotes) {
			break
		}
	}
	close(ok)
	return res, items
}
