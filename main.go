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

	"github.com/fatih/color"
	"github.com/schollz/progressbar/v3"
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

func CopyFile(srcPath, dstPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer src.Close()

	err = os.MkdirAll(filepath.Dir(dstPath), 0755)
	if err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}
	dst, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	if err != nil {
		return fmt.Errorf("failed to copy file: %w", err)
	}

	return nil
}

func DeleteFile(path string) error {
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("delete file %q: %w", path, err)
	}
	return nil
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

	wg.Go(func() {
		sourceHashes, sourceErr = ScanDir(sourcePath)
	})

	wg.Go(func() {
		destHashes, destErr = ScanDir(destPath)
	})

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

	var copied, updated, deleted int

	total := len(toCopy) + len(toUpdate) + len(toDelete)
	if total == 0 {
		fmt.Println("Директории уже синхронизированы, делать нечего.")
		return
	}

	fmt.Println("Синхронизация...")
	bar := progressbar.Default(int64(total))
	green := color.New(color.FgGreen)
	yellow := color.New(color.FgYellow)
	red := color.New(color.FgRed)
	errColor := color.New(color.FgRed, color.Bold)

	var report, errs []string

	for _, file := range toCopy {
		srcPath := filepath.Join(sourcePath, file)
		dstPath := filepath.Join(destPath, file)
		_ = bar.Add(1)
		if err := CopyFile(srcPath, dstPath); err != nil {
			errs = append(errs, fmt.Sprintf("[ERROR] copy file %q: %v", file, err))
			continue
		}
		report = append(report, green.Sprintf("[КОПИРОВАНИЕ] %s", file))
		copied++
	}

	for _, file := range toUpdate {
		srcPath := filepath.Join(sourcePath, file)
		dstPath := filepath.Join(destPath, file)
		_ = bar.Add(1)
		if err := CopyFile(srcPath, dstPath); err != nil {
			errs = append(errs, fmt.Sprintf("[ERROR] update file %q: %v", file, err))
			continue
		}
		report = append(report, yellow.Sprintf("[ОБНОВЛЕНИЕ] %s", file))
		updated++
	}

	for _, file := range toDelete {
		path := filepath.Join(destPath, file)
		_ = bar.Add(1)
		if err := DeleteFile(path); err != nil {
			errs = append(errs, fmt.Sprintf("[ERROR] %v", err))
			continue
		}
		report = append(report, red.Sprintf("[УДАЛЕНИЕ] %s", file))
		deleted++
	}

	for _, line := range report {
		fmt.Println(line)
	}
	for _, line := range errs {
		errColor.Fprintln(os.Stderr, line)
	}

	fmt.Println()
	fmt.Println("Синхронизация завершена!")
	fmt.Printf("Скопировано: %d\n", copied)
	fmt.Printf("Обновлено: %d\n", updated)
	fmt.Printf("Удалено: %d\n", deleted)
}
