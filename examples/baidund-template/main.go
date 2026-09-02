package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"baiduyunStorage/pkg/baidund"
)

func main() {
	ctx := context.Background()
	creds := baidund.Credentials{
		AccessToken:  os.Getenv("BDY_ACCESS_TOKEN"),
		RefreshToken: os.Getenv("BDY_REFRESH_TOKEN"),
		ExpiresAt:    time.Now().Add(time.Hour),
	}
	client := baidund.New(creds)

	root := filepath.Join(os.TempDir(), "baidund-template-worktree")
	if err := os.MkdirAll(root, 0o755); err != nil {
		panic(err)
	}
	if err := os.WriteFile(filepath.Join(root, "hello.txt"), []byte("hello baidund\n"), 0o644); err != nil {
		panic(err)
	}

	repoName := "template-demo"
	repo, err := client.Init(root, baidund.DefaultRemoteRoot(repoName))
	if err != nil {
		panic(err)
	}
	if err := repo.Add("hello.txt"); err != nil {
		panic(err)
	}
	commit, err := repo.Commit("store hello.txt")
	if err != nil {
		panic(err)
	}
	fmt.Printf("created commit %s\n", commit.OID)

	if os.Getenv("BDY_PUSH") == "1" {
		if err := repo.Push(ctx); err != nil {
			panic(err)
		}
		fmt.Println("pushed to", baidund.DefaultRemoteRoot(repoName))
	}
}
