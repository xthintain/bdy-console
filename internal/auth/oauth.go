package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"baiduyunStorage/internal/config"
)

const DefaultBaseURL = "https://openapi.baidu.com"

type OAuth struct {
	BaseURL string
	HTTP    *http.Client
}

type DeviceCode struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURL string `json:"verification_url"`
	QRCodeURL       string `json:"qrcode_url"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

type Token struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Scope        string `json:"scope"`
	Error        string `json:"error"`
	Description  string `json:"error_description"`
}

func New() OAuth {
	return OAuth{BaseURL: DefaultBaseURL, HTTP: http.DefaultClient}
}

func (o OAuth) RequestDeviceCode(ctx context.Context, appKey string) (DeviceCode, error) {
	q := url.Values{}
	q.Set("response_type", "device_code")
	q.Set("client_id", appKey)
	q.Set("scope", "basic,netdisk")
	var out DeviceCode
	err := o.getJSON(ctx, "/oauth/2.0/device/code", q, &out)
	if err != nil {
		return DeviceCode{}, err
	}
	if out.DeviceCode == "" {
		return DeviceCode{}, errors.New("device_code missing from response")
	}
	if out.Interval <= 0 {
		out.Interval = 5
	}
	return out, nil
}

func (o OAuth) PollToken(ctx context.Context, appKey, secretKey, deviceCode string) (Token, error) {
	q := url.Values{}
	q.Set("grant_type", "device_token")
	q.Set("code", deviceCode)
	q.Set("client_id", appKey)
	q.Set("client_secret", secretKey)
	return o.token(ctx, q)
}

func (o OAuth) Refresh(ctx context.Context, cfg config.Config) (config.Config, error) {
	if cfg.RefreshToken == "" {
		return cfg, errors.New("not logged in: missing refresh token")
	}
	q := url.Values{}
	q.Set("grant_type", "refresh_token")
	q.Set("refresh_token", cfg.RefreshToken)
	q.Set("client_id", cfg.AppKey)
	q.Set("client_secret", cfg.SecretKey)
	tok, err := o.token(ctx, q)
	if err != nil {
		return cfg, err
	}
	cfg.AccessToken = tok.AccessToken
	cfg.RefreshToken = tok.RefreshToken
	cfg.ExpiresAt = time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
	return cfg, config.Save(cfg)
}

func EnsureToken(ctx context.Context) (config.Config, error) {
	cfg, err := config.Load()
	if err != nil {
		return cfg, err
	}
	if !cfg.HasApp() {
		return cfg, errors.New("app credentials missing; run bdy config set-app first")
	}
	if cfg.HasToken() {
		return cfg, nil
	}
	return New().Refresh(ctx, cfg)
}

func (o OAuth) token(ctx context.Context, q url.Values) (Token, error) {
	var tok Token
	if err := o.getJSON(ctx, "/oauth/2.0/token", q, &tok); err != nil {
		return Token{}, err
	}
	if tok.Error != "" {
		return Token{}, fmt.Errorf("oauth error %s: %s", tok.Error, tok.Description)
	}
	if tok.AccessToken == "" || tok.RefreshToken == "" {
		return Token{}, errors.New("token response missing access_token or refresh_token")
	}
	if tok.ExpiresIn <= 0 {
		tok.ExpiresIn = 2592000
	}
	return tok, nil
}

func (o OAuth) getJSON(ctx context.Context, path string, q url.Values, out any) error {
	base := o.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	u := base + path + "?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "pan.baidu.com")
	c := o.HTTP
	if c == nil {
		c = http.DefaultClient
	}
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return err
		}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if tok, ok := out.(*Token); ok && tok.Error != "" {
			return nil
		}
		return fmt.Errorf("oauth http status %d", resp.StatusCode)
	}
	return nil
}
