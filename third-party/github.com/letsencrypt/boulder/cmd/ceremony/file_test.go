package main

import (
	"testing"
)

func TestWriteFileSuccess(t *testing.T) {
	dir := t.TempDir()
	err := writeFile(dir+"/example", []byte("hi"))
	if err != nil {
		t.Fatal(err)
	}
}

func TestWriteFileFail(t *testing.T) {
	dir := t.TempDir()
	err := writeFile(dir+"/example", []byte("hi"))
	if err != nil {
		t.Fatal(err)
	}
	err = writeFile(dir+"/example", []byte("hi"))
	if err == nil {
		t.Fatal("expected error, got none")
	}
}
