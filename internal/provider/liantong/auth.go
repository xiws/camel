package liantong

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"time"

	"camel/internal/provider"
)

func (l *LiantongProvider) Login(ctx context.Context, params provider.LoginParams) (map[string]string, error) {
	if params.Username == "" || params.Password == "" {
		return nil, fmt.Errorf("username (phone) and password required")
	}

	disp := NewDispatcher()
	uuid := generateUUID()

	loginParams := map[string]interface{}{
		"phone":        params.Username,
		"password":     params.Password,
		"uuid":         uuid,
		"verifyCode":   "",
		"clientSecret": secretKey,
	}

	data, err := disp.Call("api-user", "PcWebLogin", loginParams)
	if err != nil {
		errMsg := err.Error()
		if contains(errMsg, "6006") || contains(errMsg, "8001") || contains(errMsg, "6008") {
			return l.handleCaptchaLogin(disp, params.Username, params.Password, uuid)
		}
		return nil, fmt.Errorf("login failed: %w", err)
	}

	needSms, _ := data["needSmsCode"].(string)
	if needSms == "1" {
		return l.handleSMSLogin(disp, params.Username, params.Password, uuid, "")
	}

	accessToken, _ := data["access_token"].(string)
	if accessToken == "" {
		return nil, fmt.Errorf("login response missing access_token")
	}

	return map[string]string{
		"access_token": accessToken,
		"phone":        params.Username,
	}, nil
}

func (l *LiantongProvider) handleCaptchaLogin(disp *Dispatcher, phone, password, uuid string) (map[string]string, error) {
	captchaURL := dispatcherBase + "/api-user/getverifycode?uuid=" + uuid

	req, err := http.NewRequest("GET", captchaURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch captcha: %w", err)
	}
	defer resp.Body.Close()

	tmpFile, err := os.CreateTemp("", "camel_captcha_*.jpg")
	if err != nil {
		return nil, err
	}
	defer tmpFile.Close()

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		return nil, err
	}

	fmt.Printf("Captcha saved to: %s\n", tmpFile.Name())
	if runtime.GOOS == "darwin" {
		exec.Command("open", tmpFile.Name()).Start()
	}

	var captchaCode string
	fmt.Print("Enter captcha code: ")
	fmt.Scanln(&captchaCode)

	loginParams := map[string]interface{}{
		"phone":        phone,
		"password":     password,
		"uuid":         uuid,
		"verifyCode":   captchaCode,
		"clientSecret": secretKey,
	}

	data, err := disp.Call("api-user", "PcWebLogin", loginParams)
	if err != nil {
		return nil, fmt.Errorf("login with captcha failed: %w", err)
	}

	needSms, _ := data["needSmsCode"].(string)
	if needSms == "1" {
		return l.handleSMSLogin(disp, phone, password, uuid, captchaCode)
	}

	accessToken, _ := data["access_token"].(string)
	if accessToken == "" {
		return nil, fmt.Errorf("login response missing access_token")
	}

	return map[string]string{
		"access_token": accessToken,
		"phone":        phone,
	}, nil
}

func (l *LiantongProvider) handleSMSLogin(disp *Dispatcher, phone, password, uuid, verifyCode string) (map[string]string, error) {
	fmt.Print("SMS code sent to your phone. Enter code: ")
	var smsCode string
	fmt.Scanln(&smsCode)

	verifyParams := map[string]interface{}{
		"phone":        phone,
		"password":     password,
		"uuid":         uuid,
		"verifyCode":   verifyCode,
		"messageCode":  smsCode,
		"clientSecret": secretKey,
	}

	data, err := disp.Call("api-user", "PcLoginVerifyCode", verifyParams)
	if err != nil {
		return nil, fmt.Errorf("SMS verification failed: %w", err)
	}

	accessToken, _ := data["access_token"].(string)
	if accessToken == "" {
		return nil, fmt.Errorf("SMS verification response missing access_token")
	}

	return map[string]string{
		"access_token": accessToken,
		"phone":        phone,
	}, nil
}

func generateUUID() string {
	b := make([]byte, 16)
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	r.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
