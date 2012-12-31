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
}