package main

import (
	"os"
	"testing"
)

func TestFileMd5sum(t *testing.T) {
	file := os.Args[0]
	md5, err := FileMd5sum(file)
	if err != nil {
		t.Errorf("Error getting md5sum for file %s: %s", file, err)
	}

	if len(md5) != 32 {
		t.Errorf("Invalid md5sum returned: %s", md5)
	}
}
