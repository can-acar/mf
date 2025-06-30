package main

import (
	"context"
	"fmt"
	"github.com/bmatcuk/doublestar/v4"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

func Rimraf(path string, opts Options) error {
	var finder *LockFinder
	if runtime.GOOS == "windows" {
		finder = NewLockFinder()
	}
	ctx := context.Background()
	absPath, _ := filepath.Abs(path)
	info, err := os.Lstat(absPath)
	if err != nil {
		return fmt.Errorf("yol bulunamadı: %v", err)
	}

	// Exclude check
	for _, pattern := range opts.Excludes {
		match, _ := doublestar.Match(pattern, info.Name())
		if match {
			if opts.Verbose {
				fmt.Printf("Atlandı (exclude): %s\n", absPath)
			}
			LogAction("EXCLUDE", absPath, nil)
			return nil
		}
	}

	// Klasörse parallel processing
	if info.IsDir() {
		entries, err := os.ReadDir(absPath)
		if err != nil {
			return fmt.Errorf("dizin okunamadı: %v", err)
		}
		
		// Goroutine pool ile paralel işleme
		const maxWorkers = 10
		semaphore := make(chan struct{}, maxWorkers)
		var wg sync.WaitGroup
		
		for _, entry := range entries {
			wg.Add(1)
			go func(e os.DirEntry) {
				defer wg.Done()
				semaphore <- struct{}{}
				defer func() { <-semaphore }()
				
				child := filepath.Join(absPath, e.Name())
				if err := Rimraf(child, opts); err != nil {
					// Hataları logla ama devam et
					fmt.Fprintf(os.Stderr, "Silinemedi: %s (%v)\n", child, err)
					LogAction("ERROR", child, err)
				}
			}(entry)
		}
		wg.Wait()
	}

	if opts.DryRun {
		fmt.Printf("[dry-run] Silinecek: %s\n", absPath)
		LogAction("DRY-RUN", absPath, nil)
		return nil
	}

	err = os.RemoveAll(absPath)
	if err != nil {
		// HATA: Windows’ta dosya kilitli olabilir mi kontrol et
		if runtime.GOOS == "windows" && opts.Force && finder != nil {
			processes, _ := finder.FindLockingProcesses(ctx, absPath)
			if len(processes) > 0 {
				for _, proc := range processes {
					if err := finder.KillProcess(proc); err == nil {
						fmt.Printf("Process %d kill edildi (dosya kilidi): %s\n", proc.PID, absPath)
						LogAction("KILL", absPath, nil)
					} else {
						fmt.Fprintf(os.Stderr, "Kill edilemedi: PID %d, %v\n", proc.PID, err)
						LogAction("KILL-FAIL", absPath, err)
					}
				}
				// Tekrar silmeyi dene
				err = os.RemoveAll(absPath)
			}
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "Silinemedi: %s (%v)\n", absPath, err)
			LogAction("ERROR", absPath, err)
			return err
		}
	}

	if opts.Verbose {
		fmt.Printf("Silindi: %s\n", absPath)
	}
	LogAction("DELETE", absPath, nil)
	return nil
}
