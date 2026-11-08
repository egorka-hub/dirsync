package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

func ListFiles(rootPath string) ([]string, error) {
	var files []string

	err := filepath.WalkDir(rootPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(rootPath, path)
		if err != nil {
			return err
		}

		files = append(files, rel)

		return nil
	})

	return files, err
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: dirsync <directory>")
		os.Exit(1)
	}

	rootPath := os.Args[1]

	files, err := ListFiles(rootPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	for _, file := range files {
		fmt.Println(file)
	}
}
