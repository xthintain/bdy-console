package baidund

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"baiduyunStorage/internal/baidu"
	"baiduyunStorage/internal/bdynd"
	"baiduyunStorage/internal/config"
)

type Credentials struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	ReadOnly     bool
}

type Client struct {
	creds Credentials
	baidu baidu.Client
}

type Repo struct {
	client *Client
	inner  bdynd.Repo
}

type Commit struct {
	OID     string
	Parent  string
	Message string
	Time    time.Time
}

type Status struct {
	Added    []string
	Modified []string
	Deleted  []string
}

func New(creds Credentials) *Client {
	cfg := config.Config{
		AccessToken:  creds.AccessToken,
		RefreshToken: creds.RefreshToken,
		ExpiresAt:    creds.ExpiresAt,
		ReadOnly:     creds.ReadOnly,
	}
	return &Client{creds: creds, baidu: baidu.NewClient(cfg)}
}

func (c *Client) Init(root, remoteRoot string) (Repo, error) {
	r, err := bdynd.Init(root, bdynd.InitOptions{
		RemoteName: bdynd.DefaultRemote,
		RemoteRoot: remoteRoot,
	})
	if err != nil {
		return Repo{}, err
	}
	return Repo{client: c, inner: r}, nil
}

func (c *Client) Open(start string) (Repo, error) {
	r, err := bdynd.Open(start)
	if err != nil {
		return Repo{}, err
	}
	return Repo{client: c, inner: r}, nil
}

func (c *Client) Clone(ctx context.Context, remoteRoot, dest string) (Repo, error) {
	r, err := bdynd.Clone(ctx, remoteStore{client: c.baidu}, remoteRoot, dest)
	if err != nil {
		return Repo{}, err
	}
	return Repo{client: c, inner: r}, nil
}

func (r Repo) Root() string {
	return r.inner.Root
}

func (r Repo) Dir() string {
	return r.inner.Dir
}

func (r Repo) SetRemote(remoteRoot string) error {
	return bdynd.SetRemote(r.inner, bdynd.DefaultRemote, remoteRoot)
}

func (r Repo) Add(paths ...string) error {
	return bdynd.Add(r.inner, paths)
}

func (r Repo) Commit(message string) (Commit, error) {
	c, err := bdynd.Commit(r.inner, bdynd.CommitOptions{Message: message})
	if err != nil {
		return Commit{}, err
	}
	return Commit{OID: c.OID, Parent: c.Parent, Message: c.Message, Time: c.Time}, nil
}

func (r Repo) Push(ctx context.Context) error {
	return bdynd.Push(ctx, r.inner, remoteStore{client: r.client.baidu}, bdynd.DefaultRemote)
}

func (r Repo) Fetch(ctx context.Context) error {
	return bdynd.Fetch(ctx, r.inner, remoteStore{client: r.client.baidu}, bdynd.DefaultRemote)
}

func (r Repo) Pull(ctx context.Context) error {
	return bdynd.Pull(ctx, r.inner, remoteStore{client: r.client.baidu}, bdynd.DefaultRemote)
}

func (r Repo) Status() (Status, error) {
	st, err := bdynd.Status(r.inner)
	if err != nil {
		return Status{}, err
	}
	return Status{Added: st.Added, Modified: st.Modified, Deleted: st.Deleted}, nil
}

func (r Repo) Log(limit int) ([]Commit, error) {
	items, err := bdynd.Log(r.inner, bdynd.LogOptions{Limit: limit})
	if err != nil {
		return nil, err
	}
	out := make([]Commit, 0, len(items))
	for _, item := range items {
		out = append(out, Commit{OID: item.OID, Parent: item.Parent, Message: item.Message, Time: item.Time})
	}
	return out, nil
}

func DefaultRemoteRoot(repoName string) string {
	repoName = strings.Trim(strings.TrimSpace(repoName), "/")
	if repoName == "" {
		repoName = "default"
	}
	return "/apps/baiduyunStorage/nd/repos/" + repoName
}

type remoteStore struct {
	client baidu.Client
}

func (s remoteStore) UploadFile(ctx context.Context, localPath, remotePath string) error {
	return s.client.UploadFile(ctx, localPath, remotePath)
}

func (s remoteStore) DownloadFile(ctx context.Context, remotePath, localPath string) error {
	parent := filepath.ToSlash(filepath.Dir(remotePath))
	name := filepath.Base(remotePath)
	items, err := s.client.List(ctx, parent)
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.Path != remotePath && item.ServerFilename != name {
			continue
		}
		meta, err := s.client.FileMetas(ctx, []uint64{item.FSID}, true)
		if err != nil {
			return err
		}
		if len(meta) == 0 || meta[0].DLink == "" {
			return fmt.Errorf("missing dlink for %s", remotePath)
		}
		return s.client.Download(ctx, meta[0].DLink, localPath)
	}
	return os.ErrNotExist
}

func (s remoteStore) Exists(ctx context.Context, remotePath string) (bool, error) {
	parent := filepath.ToSlash(filepath.Dir(remotePath))
	name := filepath.Base(remotePath)
	items, err := s.client.List(ctx, parent)
	if err != nil {
		return false, nil
	}
	for _, item := range items {
		if item.Path == remotePath || item.ServerFilename == name {
			return true, nil
		}
	}
	return false, nil
}
