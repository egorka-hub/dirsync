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
	Err  error
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

func CompareScans(source, dest map[string]string) (toCopy, toUpdate, toDelete []string) {
	for key, srcHash := range source {
		if destHash, exists := dest[key]; !exists {
			toCopy = append(toCopy, key)
		} else if srcHash != destHash {
			toUpdate = append(toUpdate, key)
		}
	}

	for key := range dest {
		if _, exists := source[key]; !exists {
			toDelete = append(toDelete, key)
		}
	}

	return
}

func ScanDir(rootPath string) (map[string]string, error) {
	files, err := ListFiles(rootPath)
	if err != nil {
		return nil, err
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
				results <- FileHash{
					Path: path,
					Hash: hashFile,
					Err:  err,
				}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	hashes := make(map[string]string)
	var scanErr error

	for r := range results {
		if r.Err != nil {
			scanErr = r.Err
			continue
		}
		hashes[r.Path] = r.Hash
	}

	if scanErr != nil {
		return nil, scanErr
	}

	return hashes, nil
}

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: dirsync <source> <dest>")
		os.Exit(1)
	}

	sourcePath := os.Args[1]
	destPath := os.Args[2]

	var sourceHashes, destHashes map[string]string
	var sourceErr, destErr error
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		sourceHashes, sourceErr = ScanDir(sourcePath)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		destHashes, destErr = ScanDir(destPath)
	}()

	wg.Wait()

	if sourceErr != nil {
		fmt.Fprintf(os.Stderr, "scan source: %v\n", sourceErr)
		os.Exit(1)
	}
	if destErr != nil {
		fmt.Fprintf(os.Stderr, "scan dest: %v\n", destErr)
		os.Exit(1)
	}

	toCopy, toUpdate, toDelete := CompareScans(sourceHashes, destHashes)

	fmt.Println("Файлы для КОПИРОВАНИЯ:")
	for _, file := range toCopy {
		fmt.Println("-", file)
	}
	fmt.Println()

	fmt.Println("Файлы для ОБНОВЛЕНИЯ:")
	for _, file := range toUpdate {
		fmt.Println("-", file)
	}
	fmt.Println()

	fmt.Println("Файлы для УДАЛЕНИЯ:")
	for _, file := range toDelete {
		fmt.Println("-", file)
	}
}
