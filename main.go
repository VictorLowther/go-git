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
	c,err := r.Config()
	for k,v := range c.Find("user.") {
		fmt.Printf("%s: %v\n",k,v)
	}
	c.Set("foo.bar","bar")
	v,ok := c.Get("foo.bar")
	fmt.Printf("%v: %v\n",v,ok)
	if clean,statLines := r.StatusClean(); clean {
		fmt.Println("Repo is clean")
	} else {
		for _,l := range statLines {
			fmt.Printf("%s\n",l.Print())
		}
	}
}