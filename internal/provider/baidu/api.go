package baidu

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	panAPI     = "https://pan.baidu.com/api/"
	pcsFile    = "https://d.pcs.baidu.com/rest/2.0/pcs/file"
	appID      = "250528"
	userAgent  = "netdisk;9.15.1.32;Android;camel"
	referer    = "https://pan.baidu.com/"
	blockSize  = 4 * 1024 * 1024
	rsaByteLen = 256
)

var (
	rsaModulus, _ = new(big.Int).SetString("B3C61EBBA4659C4CE3639287EE871F1F48F7930EA977991C7AFE3CC442FEA49643212E7D570C853F368065CC57A2014666DA8AE7D493FD47D171C0D894EEE3ED7F99F6798B7FFD7B5873227038AD23E3197631A8CB642213B9F27D4901AB0D92BFA27542AE890855396ED92775255C977F5C302F1E7ED4B1E369C12CB6B1822F", 16)
	rsaExponent   = big.NewInt(0x10001)
)

type BaiduProvider struct {
	bduss    string
	stoken   string
	bdstoken string
	client   *http.Client
	dlClient *http.Client
}

func New() *BaiduProvider {
	return &BaiduProvider{
		client:   &http.Client{Timeout: 120 * time.Second},
		dlClient: &http.Client{Timeout: 10 * time.Minute},
	}
}

func (b *BaiduProvider) Name() string {
	return "baidu"
}

func (b *BaiduProvider) Init(ctx context.Context, creds map[string]string) error {
	b.bduss = creds["bduss"]
	b.stoken = creds["stoken"]
	if b.bduss == "" {
		return errors.New("missing bduss in credentials")
	}
	b.bdstoken = md5Hex(b.bduss)
	return nil
}

func md5Hex(s string) string {
	h := md5.Sum([]byte(s))
	return hex.EncodeToString(h[:])
}

func fileMD5(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func chunkMD5(data []byte) string {
	h := md5.Sum(data)
	return hex.EncodeToString(h[:])
}

func (b *BaiduProvider) cookie() string {
	parts := []string{"BDUSS=" + b.bduss}
	if b.stoken != "" {
		parts = append(parts, "STOKEN="+b.stoken)
	}
	return strings.Join(parts, "; ")
}

func (b *BaiduProvider) doRequest(req *http.Request) (*http.Response, error) {
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Referer", referer)
	req.Header.Set("Cookie", b.cookie())
	return b.client.Do(req)
}

func (b *BaiduProvider) get(urlStr string, params url.Values) ([]byte, error) {
	if params != nil {
		if strings.Contains(urlStr, "?") {
			urlStr += "&" + params.Encode()
		} else {
			urlStr += "?" + params.Encode()
		}
	}
	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return nil, err
	}
	resp, err := b.doRequest(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func (b *BaiduProvider) postForm(urlStr string, params url.Values, form url.Values) ([]byte, error) {
	if params != nil {
		if strings.Contains(urlStr, "?") {
			urlStr += "&" + params.Encode()
		} else {
			urlStr += "?" + params.Encode()
		}
	}
	req, err := http.NewRequest("POST", urlStr, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := b.doRequest(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func (b *BaiduProvider) api(endpoint string, params url.Values, form url.Values) (map[string]interface{}, error) {
	if params == nil {
		params = url.Values{}
	}
	params.Set("bdstoken", b.bdstoken)
	params.Set("app_id", appID)

	var raw []byte
	var err error
	if form != nil {
		raw, err = b.postForm(panAPI+endpoint, params, form)
	} else {
		raw, err = b.get(panAPI+endpoint, params)
	}
	if err != nil {
		return nil, err
	}

	var data map[string]interface{}
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w (body: %s)", err, string(raw)[:min(200, len(raw))])
	}

	errno := 0
	if v, ok := data["errno"]; ok {
		switch t := v.(type) {
		case float64:
			errno = int(t)
		case int:
			errno = t
		}
	}
	if errno != 0 {
		msg := ""
		if m, ok := data["errmsg"]; ok {
			msg = fmt.Sprintf("%v", m)
		} else if m, ok := data["error_msg"]; ok {
			msg = fmt.Sprintf("%v", m)
		}
		return nil, fmt.Errorf("API %s error (errno=%d): %s", endpoint, errno, msg)
	}

	return data, nil
}

func rsaEncryptHex(plaintext string, base64Encode bool) (string, error) {
	data := plaintext
	if base64Encode {
		data = base64.StdEncoding.EncodeToString([]byte(plaintext))
	}

	m := new(big.Int).SetBytes([]byte(data))
	c := new(big.Int).Exp(m, rsaExponent, rsaModulus)
	buf := c.Bytes()
	result := make([]byte, rsaByteLen)
	copy(result[rsaByteLen-len(buf):], buf)
	return hex.EncodeToString(result), nil
}

func parseCredentials(raw string) (bduss, stoken string) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "Cookie:")
	raw = strings.TrimSpace(raw)

	if strings.Contains(raw, "BDUSS=") {
		for _, pair := range strings.Split(raw, ";") {
			parts := strings.SplitN(pair, "=", 2)
			if len(parts) != 2 {
				continue
			}
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			if key == "BDUSS" {
				bduss = value
			} else if key == "STOKEN" {
				stoken = value
			}
		}
	} else {
		bduss = raw
	}
	return
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
