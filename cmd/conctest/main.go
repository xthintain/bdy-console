// Command conctest: 实测百度网盘开放平台并发上传/下载/列目录行为。
// 用法:
//
//	go run ./cmd/conctest upload-single    # 单文件分片并发上传(64MB, 并发度1/2/4/8/16)
//	go run ./cmd/conctest upload-multi     # 多文件(8x4MB)并发上传
//	go run ./cmd/conctest download         # 并发下载已上传文件
//	go run ./cmd/conctest list             # 高频/并发列目录, 探测频控
//	go run ./cmd/conctest all              # 依次执行以上全部
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"baiduyunStorage/internal/auth"
	"baiduyunStorage/internal/baidu"
)

const (
	remoteBase = "/apps/baiduyunStorage/concurrency-test"
	file64MB   = 64 * 1024 * 1024
	file4MB    = 4 * 1024 * 1024
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: conctest <upload-single|upload-multi|download|list|all>")
		os.Exit(1)
	}
	ctx := context.Background()
	cfg, err := auth.EnsureToken(ctx)
	if err != nil {
		fatal("ensure token: %v", err)
	}
	client := baidu.NewClient(cfg)

	dir, err := os.MkdirTemp("", "bdy-conc-")
	if err != nil {
		fatal("tempdir: %v", err)
	}
	defer os.RemoveAll(dir)

	// 预生成测试文件
	big := filepath.Join(dir, "f64.bin")
	small := make([]string, 9) // index 1..8
	if err := makeRandFile(big, file64MB); err != nil {
		fatal("gen big: %v", err)
	}
	for i := 1; i <= 8; i++ {
		p := filepath.Join(dir, fmt.Sprintf("s%d.bin", i))
		if err := makeRandFile(p, file4MB); err != nil {
			fatal("gen small: %v", err)
		}
		small[i] = p
	}

	switch os.Args[1] {
	case "upload-single":
		uploadSingle(ctx, client, big)
	case "upload-multi":
		uploadMulti(ctx, client, small)
	case "download":
		downloadConcurrent(ctx, client, dir)
	case "list":
		listProbe(ctx, client)
	case "list-burst":
		listBurst(ctx, client)
	case "all":
		uploadSingle(ctx, client, big)
		uploadMulti(ctx, client, small)
		downloadConcurrent(ctx, client, dir)
		listProbe(ctx, client)
	default:
		fatal("unknown scenario %q", os.Args[1])
	}
}

// ---- 场景 A：单文件分片并发上传 ----

func uploadSingle(ctx context.Context, client baidu.Client, big string) {
	fmt.Println("\n===== 场景A: 单文件(64MB=16x4MB分片) 分片并发上传 superfile2 =====")
	for _, n := range []int{1, 2, 4, 8, 16} {
		remote := fmt.Sprintf("%s/upload-c%d-f64.bin", remoteBase, n)
		_ = os.Setenv("BDY_UPLOAD_CONCURRENCY", fmt.Sprint(n))
		start := time.Now()
		err := client.UploadFile(ctx, big, remote)
		el := time.Since(start)
		if err != nil {
			fmt.Printf("concurrency=%2d  FAIL  (%v)\n", n, err)
			continue
		}
		ok := verifyRemote(ctx, client, remote, file64MB)
		mbps := float64(file64MB) / el.Seconds() / 1024 / 1024
		fmt.Printf("concurrency=%2d  OK  elapsed=%s  %.1f MB/s  verified=%v\n", n, el.Round(time.Millisecond), mbps, ok)
	}
}

// ---- 场景 B：多文件并发上传 ----

func uploadMulti(ctx context.Context, client baidu.Client, small []string) {
	fmt.Println("\n===== 场景B: 8x4MB 独立文件并发上传 (各自完整 precreate->superfile2->create) =====")
	// 先确保远端目录存在
	for _, n := range []int{1, 4, 8} {
		start := time.Now()
		var wg sync.WaitGroup
		errs := make([]error, 9)
		for i := 1; i <= 8; i++ {
			remote := fmt.Sprintf("%s/multi-c%d-f%d.bin", remoteBase, n, i)
			wg.Add(1)
			go func(i int, remote string) {
				defer wg.Done()
				errs[i] = client.UploadFile(ctx, small[i], remote)
			}(i, remote)
		}
		wg.Wait()
		el := time.Since(start)
		fail := 0
		for i := 1; i <= 8; i++ {
			if errs[i] != nil {
				fail++
				fmt.Printf("  file%d error: %v\n", i, errs[i])
			}
		}
		total := 8 * file4MB
		mbps := float64(total) / el.Seconds() / 1024 / 1024
		fmt.Printf("parallel=%d  files=8  ok=%d fail=%d  elapsed=%s  %.1f MB/s\n",
			n, 8-fail, fail, el.Round(time.Millisecond), mbps)
	}
}

// ---- 场景 C：并发下载 ----

func downloadConcurrent(ctx context.Context, client baidu.Client, dir string) {
	fmt.Println("\n===== 场景C: 并发下载已上传文件 =====")
	// 复用场景B的远端文件 (multi-c8-f1..f8)
	fsids := make([]uint64, 0, 8)
	for i := 1; i <= 8; i++ {
		remote := fmt.Sprintf("%s/multi-c8-f%d.bin", remoteBase, i)
		e, err := findRemote(ctx, client, remote)
		if err != nil {
			fmt.Printf("  skip %s: %v\n", remote, err)
			continue
		}
		fsids = append(fsids, e.FSID)
	}
	meta, err := client.FileMetas(ctx, fsids, true)
	if err != nil || len(meta) == 0 {
		fatal("filemetas: %v (need to run upload-multi first)", err)
	}
	dlinks := make([]string, len(meta))
	for i, m := range meta {
		dlinks[i] = m.DLink
	}
	for _, n := range []int{1, 4, 8} {
		start := time.Now()
		var wg sync.WaitGroup
		errs := make([]error, len(dlinks))
		for i, dl := range dlinks {
			dest := filepath.Join(dir, fmt.Sprintf("dl-c%d-f%d.bin", n, i+1))
			wg.Add(1)
			go func(i int, dl, dest string) {
				defer wg.Done()
				errs[i] = client.Download(ctx, dl, dest)
			}(i, dl, dest)
		}
		wg.Wait()
		el := time.Since(start)
		fail := 0
		for i := range errs {
			if errs[i] != nil {
				fail++
			}
		}
		total := int64(len(dlinks)) * file4MB
		mbps := float64(total) / el.Seconds() / 1024 / 1024
		fmt.Printf("parallel=%d  files=%d  ok=%d fail=%d  elapsed=%s  %.1f MB/s\n",
			n, len(dlinks), len(dlinks)-fail, fail, el.Round(time.Millisecond), mbps)
	}
}

// ---- 场景 D：列目录频控探测 ----

func listProbe(ctx context.Context, client baidu.Client) {
	fmt.Println("\n===== 场景D: 高频/并发 list / listall 频控探测 =====")
	// D1: 串行高频 list (同目录 20 次)
	start := time.Now()
	var firstErr error
	firstErrAt := -1
	for i := 0; i < 20; i++ {
		if _, err := client.List(ctx, remoteBase); err != nil {
			if firstErr == nil {
				firstErr = err
				firstErrAt = i
			}
		}
	}
	fmt.Printf("D1 serial list x20: elapsed=%s  firstErr@%d err=%v\n",
		time.Since(start).Round(time.Millisecond), firstErrAt, firstErr)

	// D2: 并发 list (10 goroutine x 5 次 = 50 调用)
	start = time.Now()
	var wg sync.WaitGroup
	var mu sync.Mutex
	fails := 0
	for g := 0; g < 10; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 5; i++ {
				if _, err := client.List(ctx, remoteBase); err != nil {
					mu.Lock()
					fails++
					mu.Unlock()
				}
			}
		}()
	}
	wg.Wait()
	fmt.Printf("D2 concurrent list x50: elapsed=%s  fails=%d\n",
		time.Since(start).Round(time.Millisecond), fails)

	// D3: listall (递归全量) x10, 观察 31034
	start = time.Now()
	fails = 0
	var listAllErr error
	for i := 0; i < 10; i++ {
		if _, err := client.ListAll(ctx, remoteBase); err != nil {
			fails++
			if listAllErr == nil {
				listAllErr = err
			}
		}
	}
	fmt.Printf("D3 listall x10: elapsed=%s  fails=%d  firstErr=%v\n",
		time.Since(start).Round(time.Millisecond), fails, listAllErr)
}

// ---- helpers ----

func makeRandFile(path string, size int64) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	src, err := os.Open("/dev/urandom")
	if err != nil {
		return err
	}
	defer src.Close()
	_, err = copyN(f, src, size)
	return err
}

func copyN(dst *os.File, src *os.File, n int64) (int64, error) {
	buf := make([]byte, 1024*1024)
	var written int64
	for written < n {
		need := n - written
		if need > int64(len(buf)) {
			need = int64(len(buf))
		}
		r, err := src.Read(buf[:need])
		if r > 0 {
			w, werr := dst.Write(buf[:r])
			written += int64(w)
			if werr != nil {
				return written, werr
			}
		}
		if err != nil {
			if err.Error() == "EOF" && written >= n {
				return written, nil
			}
			return written, err
		}
	}
	return written, nil
}

func verifyRemote(ctx context.Context, client baidu.Client, remote string, size int64) bool {
	e, err := findRemote(ctx, client, remote)
	if err != nil {
		return false
	}
	return e.Size == size
}

func findRemote(ctx context.Context, client baidu.Client, remote string) (baidu.FileEntry, error) {
	parent := filepath.ToSlash(filepath.Dir(remote))
	items, err := client.List(ctx, parent)
	if err != nil {
		return baidu.FileEntry{}, err
	}
	for _, item := range items {
		if item.Path == remote || item.ServerFilename == filepath.Base(remote) {
			return item, nil
		}
	}
	return baidu.FileEntry{}, fmt.Errorf("not found: %s", remote)
}

func listBurst(ctx context.Context, client baidu.Client) {
	fmt.Println("\n===== 场景D2: 突发并发 list 300 次 (30 goroutine x 10) =====")
	start := time.Now()
	var wg sync.WaitGroup
	var mu sync.Mutex
	fails := 0
	for g := 0; g < 30; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 10; i++ {
				if _, err := client.List(ctx, remoteBase); err != nil {
					mu.Lock()
					fails++
					mu.Unlock()
				}
			}
		}()
	}
	wg.Wait()
	fmt.Printf("burst concurrent list 300: elapsed=%s  fails=%d\n", time.Since(start).Round(time.Millisecond), fails)
}

func fatal(format string, args ...any) {
	fmt.Printf("FATAL: "+format+"\n", args...)
	os.Exit(1)
}

