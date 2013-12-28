package main

import (
	"fmt"
	"github.com/VictorLowther/go-git/git"
)

func main() {
	r,err := git.Init(".")
	if err != nil {
		panic(err)
	}
	fmt.Printf("Initialized ourself!\n")
	for k,v := range r.Find("user.") {
		fmt.Printf("%s: %v\n",k,v)
	}
	r.Set("foo.bar","bar")
	v,ok := r.Get("foo.bar")
	fmt.Printf("%v: %v\n",v,ok)
	if clean,statLines := r.IsClean(); clean {
		fmt.Println("Repo is clean")
	} else {
		for _,l := range statLines {
			fmt.Printf("%s\n",l.Print())
		}
	}
	fmt.Printf("Creating throwaway branch\n")
	br,err := r.Branch("throwaway","HEAD")
	if err != nil {
		panic(err)
	}
	tag, err := r.Tag("faketag",br)
	if err != nil {
		panic(err)
	}
	for name,r := range r.Refs() {
		fmt.Printf("%s: %s\n",name,r.SHA)
	}
	tag.Delete()
	br.Delete()
}
