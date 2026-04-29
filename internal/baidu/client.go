package baidu

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"baiduyunStorage/internal/auth"
	"baiduyunStorage/internal/config"
)

const (
	PanBaseURL = "https://pan.baidu.com"
	PCSBaseURL = "https://d.pcs.baidu.com"
	UserAgent  = "pan.baidu.com"
)

type Client struct {
	Config config.Config
	HTTP   *http.Client
	PanURL string
	PCSURL string
}

type ResponseError struct {
	Errno   int    `json:"errno"`
	ErrMsg  string `json:"errmsg"`
	Request string `json:"request_id"`
}

func NewClient(cfg config.Config) Client {
	return Client{Config: cfg, HTTP: http.DefaultClient, PanURL: PanBaseURL, PCSURL: PCSBaseURL}
}

func (c Client) httpClient() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

func (c Client) getJSON(ctx context.Context, rawURL string, q url.Values, out any) error {
	q.Set("access_token", c.Config.AccessToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL+"?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", UserAgent)
	return c.doJSON(req, out)
}

func (c Client) postForm(ctx context.Context, rawURL string, q url.Values, form url.Values, out any) error {
	q.Set("access_token", c.Config.AccessToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL+"?"+q.Encode(), strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return c.doJSON(req, out)
}

func (c Client) postMultipartFile(ctx context.Context, rawURL string, q url.Values, fieldName, filePath string, out any) error {
	q.Set("access_token", c.Config.AccessToken)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()
	part, err := writer.CreateFormFile(fieldName, filepath.Base(filePath))
	if err != nil {
		return err
	}
	if _, err := io.Copy(part, f); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL+"?"+q.Encode(), &body)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return c.doJSON(req, out)
}

func (c Client) doJSON(req *http.Request, out any) error {
	resp, err := c.httpClient().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusUnauthorized && c.Config.RefreshToken != "" {
		refreshed, err := auth.New().Refresh(req.Context(), c.Config)
		if err == nil {
			c.Config = refreshed
		}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("baidu http status %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return err
	}
	var apiErr ResponseError
	if err := json.Unmarshal(data, &apiErr); err == nil && apiErr.Errno != 0 {
		return fmt.Errorf("baidu errno %d: %s", apiErr.Errno, apiErr.ErrMsg)
	}
	return nil
}
