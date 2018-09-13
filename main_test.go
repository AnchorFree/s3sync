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
