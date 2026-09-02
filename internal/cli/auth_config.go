package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"baiduyunStorage/internal/auth"
	"baiduyunStorage/internal/config"
)

func cmdConfig(args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: bdy config set-app --app-key KEY --secret-key SECRET [--app-id ID] [--sign-key SIGN] | clear-app")
	}
	switch args[0] {
	case "clear-app":
		if len(args) != 1 {
			return errors.New("usage: bdy config clear-app")
		}
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		cfg.AppKey = ""
		cfg.SecretKey = ""
		if err := config.Save(cfg); err != nil {
			return err
		}
		path, _ := config.Path()
		fmt.Fprintf(out, "rewrote %s without app credentials\n", path)
		return nil
	case "set-app":
		fs := flag.NewFlagSet("config set-app", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		appID := fs.String("app-id", "", "")
		appKey := fs.String("app-key", "", "")
		secretKey := fs.String("secret-key", "", "")
		signKey := fs.String("sign-key", "", "")
		_ = appID
		_ = signKey
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 0 {
			return errors.New("usage: bdy config set-app --app-key KEY --secret-key SECRET [--app-id ID] [--sign-key SIGN]")
		}
		if *appKey == "" || *secretKey == "" {
			return errors.New("usage: bdy config set-app --app-key KEY --secret-key SECRET [--app-id ID] [--sign-key SIGN]")
		}
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		cfg.AppKey = *appKey
		cfg.SecretKey = *secretKey
		if err := config.Save(cfg); err != nil {
			return err
		}
		fmt.Fprintln(out, "saved app credentials")
		return nil
	default:
		return errors.New("usage: bdy config set-app --app-key KEY --secret-key SECRET [--app-id ID] [--sign-key SIGN] | clear-app")
	}
}

func cmdAuth(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: bdy auth login|import-token|status")
	}
	switch args[0] {
	case "status":
		cfg, err := config.LoadActive()
		if err != nil {
			return err
		}
		if !cfg.HasToken() {
			return errors.New("not logged in or token expired")
		}
		mode := "persistent"
		if cfg.IsTemporaryReadOnly() {
			mode = "temporary read-only"
		}
		fmt.Fprintf(out, "logged in; mode: %s; token expires at %s", mode, cfg.ExpiresAt.Format(time.RFC3339))
		if cfg.IsTemporaryReadOnly() {
			fmt.Fprintf(out, "; temporary expires at %s", cfg.TemporaryExpiresAt.Format(time.RFC3339))
		}
		fmt.Fprintln(out)
		return nil
	case "login":
		fs := flag.NewFlagSet("auth login", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		temporary := fs.String("temporary", "", "")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 0 {
			return errors.New("usage: bdy auth login [--temporary <duration>]")
		}
		var temporaryDuration time.Duration
		var err error
		if *temporary != "" {
			temporaryDuration, err = parseTemporaryDuration(*temporary)
			if err != nil {
				return err
			}
		}
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if !cfg.HasApp() {
			return errors.New("app credentials missing; run bdy config set-app first")
		}
		oa := auth.New()
		dc, err := oa.RequestDeviceCode(ctx, cfg.AppKey)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "Open: %s\nCode: %s\nQR: %s\n", dc.VerificationURL, dc.UserCode, dc.QRCodeURL)
		deadline := time.Now().Add(time.Duration(dc.ExpiresIn) * time.Second)
		for time.Now().Before(deadline) {
			time.Sleep(time.Duration(dc.Interval) * time.Second)
			tok, err := oa.PollToken(ctx, cfg.AppKey, cfg.SecretKey, dc.DeviceCode)
			if err != nil {
				if strings.Contains(err.Error(), "authorization_pending") || strings.Contains(err.Error(), "slow_down") {
					continue
				}
				return err
			}
			cfg.AccessToken = tok.AccessToken
			cfg.RefreshToken = tok.RefreshToken
			cfg.ExpiresAt = time.Now().Add(time.Duration(tok.ExpiresIn) * time.Second)
			if temporaryDuration > 0 {
				cfg.Temporary = true
				cfg.ReadOnly = true
				cfg.TemporaryExpiresAt = time.Now().Add(temporaryDuration)
				if cfg.ExpiresAt.After(cfg.TemporaryExpiresAt) {
					cfg.ExpiresAt = cfg.TemporaryExpiresAt
				}
				if err := config.SaveTemporary(cfg); err != nil {
					return err
				}
				fmt.Fprintf(out, "temporary read-only login complete; expires at %s\n", cfg.TemporaryExpiresAt.Format(time.RFC3339))
				return nil
			}
			if err := config.Save(cfg); err != nil {
				return err
			}
			fmt.Fprintln(out, "login complete")
			return nil
		}
		return errors.New("device code expired")
	case "import-token":
		fs := flag.NewFlagSet("auth import-token", flag.ContinueOnError)
		fs.SetOutput(io.Discard)
		accessToken := fs.String("access-token", "", "")
		refreshToken := fs.String("refresh-token", "", "")
		expiresIn := fs.String("expires-in", "", "")
		expiresAt := fs.String("expires-at", "", "")
		temporary := fs.String("temporary", "", "")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 0 {
			return errors.New("usage: bdy auth import-token [--access-token TOKEN] [--refresh-token TOKEN] [--expires-in SECONDS] [--expires-at RFC3339] [--temporary 1d]")
		}
		cfg, err := importedTokenConfig(*accessToken, *refreshToken, *expiresIn, *expiresAt)
		if err != nil {
			return err
		}
		return saveAuthConfig(cfg, *temporary, out, "token imported", "temporary read-only token imported")
	default:
		return errors.New("usage: bdy auth login|import-token|status")
	}
}

func saveAuthConfig(cfg config.Config, temporary string, out io.Writer, persistentMsg, temporaryMsg string) error {
	if temporary != "" {
		temporaryDuration, err := parseTemporaryDuration(temporary)
		if err != nil {
			return err
		}
		cfg.Temporary = true
		cfg.ReadOnly = true
		cfg.TemporaryExpiresAt = time.Now().Add(temporaryDuration)
		if cfg.ExpiresAt.After(cfg.TemporaryExpiresAt) {
			cfg.ExpiresAt = cfg.TemporaryExpiresAt
		}
		if err := config.SaveTemporary(cfg); err != nil {
			return err
		}
		fmt.Fprintf(out, "%s; expires at %s\n", temporaryMsg, cfg.TemporaryExpiresAt.Format(time.RFC3339))
		return nil
	}
	if err := config.Save(cfg); err != nil {
		return err
	}
	fmt.Fprintln(out, persistentMsg)
	return nil
}

func importedTokenConfig(accessToken, refreshToken, expiresIn, expiresAt string) (config.Config, error) {
	if accessToken == "" {
		if env, ok, err := config.LoadEnvToken(); err != nil {
			return config.Config{}, err
		} else if ok {
			return env, nil
		}
	}
	if accessToken == "" {
		return config.Config{}, errors.New("--access-token or BDY_ACCESS_TOKEN is required")
	}
	cfg := config.Config{AccessToken: accessToken, RefreshToken: refreshToken}
	switch {
	case expiresAt != "":
		t, err := time.Parse(time.RFC3339, expiresAt)
		if err != nil {
			return config.Config{}, fmt.Errorf("parse --expires-at: %w", err)
		}
		cfg.ExpiresAt = t
	case expiresIn != "":
		seconds, err := strconv.Atoi(expiresIn)
		if err != nil || seconds <= 0 {
			return config.Config{}, fmt.Errorf("invalid --expires-in %q", expiresIn)
		}
		cfg.ExpiresAt = time.Now().Add(time.Duration(seconds) * time.Second)
	default:
		cfg.ExpiresAt = time.Now().Add(24 * time.Hour)
	}
	return cfg, nil
}

func parseTemporaryDuration(raw string) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, errors.New("--temporary requires a duration")
	}
	if strings.HasSuffix(raw, "d") {
		n, err := strconv.Atoi(strings.TrimSuffix(raw, "d"))
		if err != nil || n <= 0 {
			return 0, fmt.Errorf("invalid temporary duration %q", raw)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("invalid temporary duration %q", raw)
	}
	return d, nil
}
