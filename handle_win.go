//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shirou/gopsutil/process"
	"golang.org/x/sys/windows"
)

// ProcessInfo detaylı process bilgilerini tutar
type ProcessInfo struct {
	PID         int32
	Name        string
	ExecutePath string
	OpenFiles   []string
}

// Config performans ayarları
type Config struct {
	MaxGoroutines   int           `default:"0"` // 0 = auto-detect
	ProcessTimeout  time.Duration `default:"3s"`
	GracePeriod     time.Duration `default:"5s"`
	GlobalTimeout   time.Duration `default:"30s"`
	BatchSize       int           `default:"100"`
	ChannelBuffer   int           `default:"1000"`
	AdminCheckCache time.Duration `default:"30s"`
}

// LockFinder optimized ana yapı
type LockFinder struct {
	config          Config
	adminStatus     int32 // atomic: -1=unknown, 0=false, 1=true
	adminCheckTime  int64 // atomic: unix timestamp
	criticalProcMap sync.Map
	processPool     *sync.Pool
	resultPool      *sync.Pool
	pathCache       sync.Map
}

// NewLockFinder yeni bir optimized LockFinder oluşturur
func NewLockFinder() *LockFinder {
	config := Config{
		MaxGoroutines:   0, // Auto-detect
		ProcessTimeout:  3 * time.Second,
		GracePeriod:     5 * time.Second,
		GlobalTimeout:   30 * time.Second,
		BatchSize:       100,
		ChannelBuffer:   1000,
		AdminCheckCache: 30 * time.Second,
	}

	lf := &LockFinder{
		config:      config,
		adminStatus: -1, // Unknown
		processPool: &sync.Pool{
			New: func() interface{} {
				return &ProcessInfo{}
			},
		},
		resultPool: &sync.Pool{
			New: func() interface{} {
				return make([]string, 0, 10)
			},
		},
	}

	// Critical process map'i önceden yükle
	lf.initCriticalProcessMap()
	return lf
}

// initCriticalProcessMap kritik process haritasını başlatır
func (lf *LockFinder) initCriticalProcessMap() {
	criticalProcs := []string{
		"system", "csrss.exe", "winlogon.exe", "services.exe",
		"lsass.exe", "svchost.exe", "explorer.exe", "dwm.exe",
		"smss.exe", "wininit.exe", "spoolsv.exe", "conhost.exe",
	}

	for _, proc := range criticalProcs {
		lf.criticalProcMap.Store(strings.ToLower(proc), true)
	}
}

// calculateOptimalWorkers optimal worker sayısını hesaplar
func (lf *LockFinder) calculateOptimalWorkers(processCount int) int {
	if lf.config.MaxGoroutines > 0 {
		return lf.config.MaxGoroutines
	}

	cpuCount := runtime.NumCPU()

	// CPU core'ları ve process sayısına göre optimal hesaplama
	optimal := cpuCount * 4 // IO-bound işlemler için CPU * 4

	// Process sayısına göre sınırla
	if processCount < optimal {
		optimal = processCount
	}

	// Minimum ve maksimum sınırlar
	if optimal < 4 {
		optimal = 4
	}
	if optimal > 200 {
		optimal = 200
	}

	return optimal
}

// isRunningAsAdmin cached admin kontrolü
func (lf *LockFinder) isRunningAsAdmin() bool {
	now := time.Now().Unix()

	// Cache kontrolü (30 saniye)
	if lastCheck := atomic.LoadInt64(&lf.adminCheckTime); now-lastCheck < int64(lf.config.AdminCheckCache.Seconds()) {
		status := atomic.LoadInt32(&lf.adminStatus)
		if status != -1 {
			return status == 1
		}
	}

	// Admin kontrolü yap
	var sid *windows.SID
	err := windows.AllocateAndInitializeSid(
		&windows.SECURITY_NT_AUTHORITY,
		2,
		windows.SECURITY_BUILTIN_DOMAIN_RID,
		windows.DOMAIN_ALIAS_RID_ADMINS,
		0, 0, 0, 0, 0, 0,
		&sid)
	if err != nil {
		atomic.StoreInt32(&lf.adminStatus, 0)
		atomic.StoreInt64(&lf.adminCheckTime, now)
		return false
	}
	defer windows.FreeSid(sid)

	token := windows.Token(0)
	member, err := token.IsMember(sid)

	// Cache güncelle
	if err != nil {
		atomic.StoreInt32(&lf.adminStatus, 0)
	} else if member {
		atomic.StoreInt32(&lf.adminStatus, 1)
	} else {
		atomic.StoreInt32(&lf.adminStatus, 0)
	}
	atomic.StoreInt64(&lf.adminCheckTime, now)

	return member && err == nil
}

// isCriticalProcess optimized kritik process kontrolü
func (lf *LockFinder) isCriticalProcess(name string) bool {
	name = strings.ToLower(name)
	_, exists := lf.criticalProcMap.Load(name)
	return exists
}

// processWorker optimized worker fonksiyonu
func (lf *LockFinder) processWorker(ctx context.Context, jobs <-chan *process.Process, results chan<- ProcessInfo, targetPath string, wg *sync.WaitGroup) {
	defer wg.Done()
	defer func() {
		if r := recover(); r != nil {
			// Panic recovery
		}
	}()

	for {
		select {
		case proc, ok := <-jobs:
			if !ok {
				return
			}

			if info := lf.checkProcessOptimized(ctx, proc, targetPath); info != nil {
				select {
				case results <- *info:
				case <-ctx.Done():
					return
				}
			}

		case <-ctx.Done():
			return
		}
	}
}

// checkProcessOptimized optimized process kontrolü
func (lf *LockFinder) checkProcessOptimized(ctx context.Context, proc *process.Process, targetPath string) *ProcessInfo {
	// Timeout control channel
	done := make(chan *ProcessInfo, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- nil
			}
		}()

		// Object pool'dan ProcessInfo al
		info := lf.processPool.Get().(*ProcessInfo)
		defer func() {
			// Reset ve pool'a geri ver
			*info = ProcessInfo{}
			lf.processPool.Put(info)
		}()

		// Process bilgilerini hızlı al
		name, err := proc.Name()
		if err != nil {
			done <- nil
			return
		}

		// Kritik process kontrolü (cached)
		if lf.isCriticalProcess(name) {
			done <- nil
			return
		}

		// OpenFiles kontrolü - en pahalı işlem
		openFiles, err := proc.OpenFiles()
		if err != nil || len(openFiles) == 0 {
			done <- nil
			return
		}

		// Object pool'dan slice al
		matchedFiles := lf.resultPool.Get().([]string)
		matchedFiles = matchedFiles[:0] // Reset slice
		defer lf.resultPool.Put(matchedFiles)

		// Hedef dosyayı ara (optimized)
		for _, f := range openFiles {
			if f.Path == targetPath { // Direct comparison, no Clean
				matchedFiles = append(matchedFiles, f.Path)
			}
		}

		if len(matchedFiles) > 0 {
			// Executable path sadece gerektiğinde al
			execPath, _ := proc.Exe()

			// Copy matched files
			fileCopy := make([]string, len(matchedFiles))
			copy(fileCopy, matchedFiles)

			done <- &ProcessInfo{
				PID:         proc.Pid,
				Name:        name,
				ExecutePath: execPath,
				OpenFiles:   fileCopy,
			}
		} else {
			done <- nil
		}
	}()

	// Timeout veya sonuç bekle
	select {
	case result := <-done:
		return result
	case <-ctx.Done():
		return nil
	case <-time.After(lf.config.ProcessTimeout):
		return nil
	}
}

// FindLockingProcesses optimized process bulucu
func (lf *LockFinder) FindLockingProcesses(ctx context.Context, path string) ([]ProcessInfo, error) {
	// Admin kontrolü (cached)
	if !lf.isRunningAsAdmin() {
		return nil, errors.New("yönetici yetkileri gerekli - programı yönetici olarak çalıştırın")
	}

	// Path cache kontrolü
	if cached, exists := lf.pathCache.Load(path); exists {
		if entry := cached.(pathCacheEntry); time.Since(entry.timestamp) < time.Minute {
			return entry.processes, entry.err
		}
	}

	// Context ile timeout
	ctxWithTimeout, cancel := context.WithTimeout(ctx, lf.config.GlobalTimeout)
	defer cancel()

	// Process listesi al
	procs, err := process.Processes()
	if err != nil {
		return nil, fmt.Errorf("process listesi alınamadı: %w", err)
	}

	// Worker sayısını hesapla
	workerCount := lf.calculateOptimalWorkers(len(procs))

	// Buffered channels
	jobs := make(chan *process.Process, min(lf.config.ChannelBuffer, len(procs)))
	results := make(chan ProcessInfo, workerCount*2) // Worker başına 2 buffer

	// Worker'ları başlat
	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go lf.processWorker(ctxWithTimeout, jobs, results, path, &wg)
	}

	// Job dispatcher
	go func() {
		defer close(jobs)
		for _, proc := range procs {
			select {
			case jobs <- proc:
			case <-ctxWithTimeout.Done():
				return
			}
		}
	}()

	// Result collector
	go func() {
		wg.Wait()
		close(results)
	}()

	// Sonuçları topla
	var lockingProcesses []ProcessInfo
	for result := range results {
		lockingProcesses = append(lockingProcesses, result)
	}

	// Sonucu cache'le
	cacheEntry := pathCacheEntry{
		processes: lockingProcesses,
		timestamp: time.Now(),
		err:       nil,
	}
	if len(lockingProcesses) == 0 {
		cacheEntry.err = errors.New("dosyayı kilitleyebilen process bulunamadı")
	}
	lf.pathCache.Store(path, cacheEntry)

	if len(lockingProcesses) == 0 {
		return nil, errors.New("dosyayı kilitleyebilen process bulunamadı")
	}

	return lockingProcesses, nil
}

// pathCacheEntry cache entry struct
type pathCacheEntry struct {
	processes []ProcessInfo
	timestamp time.Time
	err       error
}

// KillProcessGracefully optimized graceful kill
func (lf *LockFinder) KillProcessGracefully(ctx context.Context, pid int32, gracePeriod time.Duration) error {
	proc, err := process.NewProcess(pid)
	if err != nil {
		return fmt.Errorf("process bulunamadı (PID %d): %w", pid, err)
	}

	// Process bilgilerini al
	name, _ := proc.Name()

	// Kritik process kontrolü
	if lf.isCriticalProcess(name) {
		return fmt.Errorf("kritik sistem process'i sonlandırılamaz: %s (PID %d)", name, pid)
	}

	// Önce graceful shutdown dene
	if err := lf.terminateProcess(pid); err == nil {
		// Polling ile bekle
		return lf.waitForProcessExit(ctx, pid, gracePeriod)
	}

	// Direct kill
	return proc.Kill()
}

// waitForProcessExit process çıkışını bekler
func (lf *LockFinder) waitForProcessExit(ctx context.Context, pid int32, timeout time.Duration) error {
	ticker := time.NewTicker(50 * time.Millisecond) // Daha hızlı polling
	defer ticker.Stop()

	deadline := time.After(timeout)

	for {
		select {
		case <-ticker.C:
			exists, _ := process.PidExists(pid)
			if !exists {
				return nil
			}
		case <-deadline:
			// Force kill
			if proc, err := process.NewProcess(pid); err == nil {
				return proc.Kill()
			}
			return fmt.Errorf("process sonlandırılamadı: timeout")
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// terminateProcess Windows'a özgü graceful shutdown
func (lf *LockFinder) terminateProcess(pid int32) error {
	handle, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handle)

	return windows.TerminateProcess(handle, 0)
}

// KillAllLockingProcesses batch process killer
func (lf *LockFinder) KillAllLockingProcesses(ctx context.Context, path string) error {
	processes, err := lf.FindLockingProcesses(ctx, path)
	if err != nil {
		return err
	}

	// Batch processing
	var wg sync.WaitGroup
	errChan := make(chan error, len(processes))

	// Process'leri paralel olarak sonlandır
	semaphore := make(chan struct{}, 10) // Max 10 concurrent kills

	for _, proc := range processes {
		wg.Add(1)
		go func(p ProcessInfo) {
			defer wg.Done()
			semaphore <- struct{}{}        // Acquire
			defer func() { <-semaphore }() // Release

			if err := lf.KillProcessGracefully(ctx, p.PID, lf.config.GracePeriod); err != nil {
				errChan <- fmt.Errorf("PID %d sonlandırılamadı: %w", p.PID, err)
			}
		}(proc)
	}

	wg.Wait()
	close(errChan)

	// Hataları topla
	var errors []error
	for err := range errChan {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		return fmt.Errorf("bazı process'ler sonlandırılamadı: %v", errors)
	}

	return nil
}

// KillProcess direct kill optimized
func (lf *LockFinder) KillProcess(pid ProcessInfo) error {
	proc, err := process.NewProcess(pid.PID)
	if err != nil {
		return fmt.Errorf("process bulunamadı (PID %d): %w", pid.PID, err)
	}

	// Kritik process kontrolü
	if lf.isCriticalProcess(pid.Name) {
		return fmt.Errorf("kritik sistem process'i sonlandırılamaz: %s (PID %d)", pid.Name, pid.PID)
	}

	return proc.Kill()
}

// Utility functions
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
