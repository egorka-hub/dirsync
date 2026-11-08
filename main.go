package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

type FileHash struct {
	Path string
	Hash string
}

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

func HashFile(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	result := hasher.Sum(nil)
	hexString := hex.EncodeToString(result)

	return hexString, nil
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

	tasks := make(chan string)
	results := make(chan FileHash)

	go func() {
		for _, file := range files {
			tasks <- file
		}
		close(tasks)
	}()

	var wg sync.WaitGroup

	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range tasks {
				hashFile, err := HashFile(filepath.Join(rootPath, path))
				if err != nil {
					fmt.Fprintln(os.Stderr, err)
					continue
				}
				results <- FileHash{
					Path: path,
					Hash: hashFile,
				}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	for r := range results {
		fmt.Printf("%s: %s\n", r.Path, r.Hash)
	}
}
