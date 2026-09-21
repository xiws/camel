package baidu

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"camel/internal/provider"
)

const (
	wappassLogin = "https://wappass.baidu.com/wp/api/login"
)

func (b *BaiduProvider) Login(ctx context.Context, params provider.LoginParams) (map[string]string, error) {
	if params.BDUSS != "" {
		bduss, stoken := parseCredentials(params.BDUSS)
		if bduss == "" {
			return nil, fmt.Errorf("invalid BDUSS")
		}
		return map[string]string{
			"bduss":  bduss,
			"stoken": stoken,
		}, nil
	}

	if params.Username == "" || params.Password == "" {
		return nil, fmt.Errorf("username and password required (or use --bduss)")
	}

	return b.loginWithPassword(params.Username, params.Password)
}

func (b *BaiduProvider) loginWithPassword(username, password string) (map[string]string, error) {
	encryptedPwd, err := rsaEncryptHex(password, true)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt password: %w", err)
	}

	client := &http.Client{
		Timeout: 25 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	form := url.Values{
		"username":      {username},
		"password":      {encryptedPwd},
		"tpl":           {"netdisk"},
		"appid":         {"200019"},
		"isEncrypted":   {"1"},
		"encryptedType": {"rsa"},
		"encryptedId":   {"2"},
		"alg":           {"rsa"},
		"staticpage":    {"https://pan.baidu.com/"},
		"charset":       {"UTF-8"},
		"tt":            {fmt.Sprintf("%d", time.Now().UnixMilli())},
	}

	req, err := http.NewRequest("POST", wappassLogin, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Referer", "https://wappass.baidu.com/passport/")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("login request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse login response: %w", err)
	}

	errInfo, _ := result["errInfo"].(map[string]interface{})
	if errInfo != nil {
		no := fmt.Sprintf("%v", errInfo["no"])
		msg := fmt.Sprintf("%v", errInfo["msg"])
		if no == "50052" {
			return nil, fmt.Errorf("login blocked by anti-bot verification (50052). Use --bduss instead")
		}
		if no != "0" {
			return nil, fmt.Errorf("login failed: %s (code: %s)", msg, no)
		}
	}

	data, _ := result["data"].(map[string]interface{})
	if data == nil {
		return nil, fmt.Errorf("login response missing data field")
	}

	bduss, _ := data["bduss"].(string)
	if bduss == "" {
		return nil, fmt.Errorf("login response missing bduss")
	}

	stoken, _ := data["stoken"].(string)
	if stoken == "" {
		stoken, _ = data["ptoken"].(string)
	}

	return map[string]string{
		"bduss":  bduss,
		"stoken": stoken,
	}, nil
}
