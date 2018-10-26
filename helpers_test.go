package main

import (
	"crypto/md5"
	"fmt"
	"strings"
	"testing"
)

func Testmd5Hash(t *testing.T) {
	s := "Clear is better than clever"
	reader := strings.NewReader(s)
	md5sum := fmt.Sprintf("%x", md5.Sum([]byte(s)))
	r, err := md5Match(reader, md5sum)
	if err != nil {
		t.Error(err)
	}
	if !r {
		t.Error("md5sum mismatch")
	}
}

func TestContains(t *testing.T) {
	files := []string{"file1", "file2", "file3"}
	existing := "file1"
	nonExisting := "file4"

	if !Contains(files, existing) {
		t.Error("Expected presense of the file, but got none")
	}
	if Contains(files, nonExisting) {
		t.Error("Did not expect existing of the file")
	}
}
