package main

import (
	"fmt"
	"reflect"

	"github.com/letsencrypt/boulder/features"
)

func main() {
	for _, flag := range reflect.VisibleFields(reflect.TypeOf(features.Config{})) {
		fmt.Println(flag.Name)
	}
}
