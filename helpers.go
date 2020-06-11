package main

import (
	"crypto/md5" // #nosec
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func fileMD5Match(filename, md5sum string) (bool, error) {
	f, err := os.Open(filepath.Clean(filename))
	if err != nil {
		if os.IsNotExist(err) {
			// it is expected error, if file does not exists, we consider it as failure
			return false, err
		}
		fmt.Printf("Could not open file: %s, due to error: %s\n", filename, err)
		return false, err
	}

	match, err := md5Match(f, md5sum)
	if err != nil {
		fmt.Printf("Could not calculate m5d sum for file: %s, due to error: %s\n", filename, err)
		return false, err
	}
	_ = f.Close() // at this point we don't really care if we failed to close the file
	return match, err
}

func md5Match(f io.Reader, md5sum string) (bool, error) {
	// we use md5 only to check if a file has changed. it seems pointless to refactor
	// it to use sha256 only to make gosec happy in this particular case.
	h := md5.New() // #nosec
	if _, err := io.Copy(h, f); err != nil {
		return false, err
	}

	return fmt.Sprintf("%x", h.Sum(nil)) == md5sum, nil
}

// Contains checks that string is in array of strings
func Contains(files []string, file string) bool {
	for i := range files {
		if file == files[i] {
			return true
		}
	}
	return false
}
